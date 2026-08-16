package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/JRedCodes/rollout-controller/internal/api"
	"github.com/JRedCodes/rollout-controller/internal/batchlogger"
	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/controller"
	"github.com/JRedCodes/rollout-controller/internal/db"
	"github.com/JRedCodes/rollout-controller/internal/guard"
	"github.com/JRedCodes/rollout-controller/internal/ingestion"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/modelconfig"
	redisc "github.com/JRedCodes/rollout-controller/internal/redis"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// pipelinePollInterval is how often the supervisor checks whether a
// different rollout has become active in Postgres.
const pipelinePollInterval = 5 * time.Second

func main() {
	redisURL := envOr("REDIS_URL", "redis://localhost:6379")
	pgURL := envOr("DATABASE_URL", "postgres://jakeredding@localhost:5432/rollout_platform")
	migrationsPath := envOr("MIGRATIONS_PATH", "./migrations")

	// golang-migrate's pgx/v5 driver uses the "pgx5" scheme.
	migrateURL := "pgx5://" + strings.TrimPrefix(pgURL, "postgres://")
	if err := db.RunMigrations(migrateURL, migrationsPath); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Printf("migrations applied")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, pgURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	repo := db.NewRolloutRepository(pool)

	rdb, err := redisc.NewClient(redisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	hub := api.NewSSEHub()
	pipeline := api.NewPipelineHolder()
	bl := batchlogger.New(pool, 10*time.Second)

	modelConfigRepo := modelconfig.NewRepository(pool)
	modelConfigSeeder := modelconfig.NewSeeder(rdb, modelConfigRepo)

	// Seed Redis immediately so Model Service has valid configs on first request.
	if err := modelConfigSeeder.SeedAll(ctx); err != nil {
		log.Fatalf("failed to seed model configs: %v", err)
	}
	log.Printf("redis model configs seeded")

	srv := api.New(4003, pipeline, repo, hub, modelConfigRepo, modelConfigSeeder)

	// These three run for the life of the process, independent of which
	// rollout (if any) is currently active.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); srv.Run(ctx) }()
	go func() { defer wg.Done(); bl.Run(ctx) }()
	go func() { defer wg.Done(); modelConfigSeeder.Run(ctx) }()

	log.Printf("rollout controller started — api :4003")

	// Blocks until ctx is cancelled, cycling through rollouts as they
	// become active/complete/idle.
	runSupervisor(ctx, rdb, bl, repo, hub, pipeline)

	log.Printf("shutting down...")
	wg.Wait()
	log.Printf("shutdown complete")
}

// runSupervisor owns the "which rollout is active" lifecycle. It loads the
// current active rollout, runs its pipeline until that rollout changes
// (completes, is rolled back, or a different one is created), then repeats.
// When no rollout is active, it idles and polls rather than exiting.
func runSupervisor(
	ctx context.Context,
	rdb *redis.Client,
	bl *batchlogger.BatchLogger,
	repo *db.RolloutRepository,
	hub *api.SSEHub,
	pipeline *api.PipelineHolder,
) {
	for ctx.Err() == nil {
		rolloutCfg, policy, err := repo.LoadActive(ctx)
		if err != nil {
			if errors.Is(err, db.ErrNoActiveRollout) {
				log.Printf("supervisor: no active rollout — idling")
			} else {
				log.Printf("supervisor: failed to load active rollout: %v — retrying", err)
			}
			sleepOrDone(ctx, pipelinePollInterval)
			continue
		}

		log.Printf("supervisor: activating rollout %s (%d%% candidate traffic)",
			rolloutCfg.RolloutID, rolloutCfg.CandidatePercentage)
		runPipeline(ctx, rdb, bl, repo, hub, pipeline, rolloutCfg, policy)
	}
}

// runPipeline runs one rollout's ingestion/guard/controller/writer
// goroutines until ctx is cancelled or a different rollout becomes active
// in Postgres, then tears them down and returns.
func runPipeline(
	parentCtx context.Context,
	rdb *redis.Client,
	bl *batchlogger.BatchLogger,
	repo *db.RolloutRepository,
	hub *api.SSEHub,
	pipeline *api.PipelineHolder,
	rolloutCfg config.RolloutConfig,
	policy config.RolloutPolicy,
) {
	pipelineCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	store := metrics.NewStore()

	w := writer.New(
		rdb,
		repo,
		hub.Broadcast,
		rolloutCfg.FeatureFlagKey,
		rolloutCfg.RolloutID,
		rolloutCfg.RolloutPhaseID,
		rolloutCfg.StableModelVersionID,
		rolloutCfg.CandidateModelVersionID,
		rolloutCfg.CandidatePercentage,
		rolloutCfg.ConfigurationVersion,
	)

	// Seed Redis immediately so the Edge Evaluator has a valid feature flag
	// for this rollout before any traffic arrives.
	if err := w.SeedRedis(pipelineCtx); err != nil {
		log.Printf("supervisor: failed to seed redis for rollout %s: %v", rolloutCfg.RolloutID, err)
		return
	}
	log.Printf("supervisor: redis feature flag seeded for rollout %s", rolloutCfg.RolloutID)

	consumer := ingestion.New(
		rdb,
		rolloutCfg.StreamKey,
		rolloutCfg.StreamConsumerGroup,
		rolloutCfg.StreamConsumerName,
		store,
		bl,
	)

	g := guard.New(policy, store, w.Commands)
	ctrl := controller.New(policy, store, w)

	pipeline.Store(&api.ActivePipeline{Cfg: rolloutCfg, Writer: w, Store: store})
	defer pipeline.Store(nil)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); w.Run(pipelineCtx) }()
	go func() { defer wg.Done(); consumer.Run(pipelineCtx) }()
	go func() { defer wg.Done(); g.Run(pipelineCtx) }()
	go func() { defer wg.Done(); ctrl.Run(pipelineCtx) }()

	watchForRolloutChange(pipelineCtx, cancel, repo, rolloutCfg.RolloutID)

	wg.Wait()
	log.Printf("supervisor: pipeline for rollout %s stopped", rolloutCfg.RolloutID)
}

// watchForRolloutChange polls Postgres and cancels once a different rollout
// (or none) is active, so runPipeline can tear down and the supervisor loop
// can pick up whatever's active now.
func watchForRolloutChange(ctx context.Context, cancel context.CancelFunc, repo *db.RolloutRepository, currentID string) {
	ticker := time.NewTicker(pipelinePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			id, err := repo.ActiveRolloutID(ctx)
			if err != nil && !errors.Is(err, db.ErrNoActiveRollout) {
				log.Printf("supervisor: failed to poll active rollout: %v", err)
				continue
			}
			if id != currentID {
				log.Printf("supervisor: active rollout changed (%s -> %q) — tearing down pipeline", currentID, id)
				cancel()
				return
			}
		}
	}
}

// sleepOrDone waits for d or ctx cancellation, whichever comes first.
func sleepOrDone(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
