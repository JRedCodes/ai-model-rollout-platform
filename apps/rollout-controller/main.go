package main

import (
	"context"
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
	"github.com/JRedCodes/rollout-controller/internal/tenant"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// pipelinePollInterval is how often the supervisor reconciles which
// tenants' pipelines should be running against Postgres.
const pipelinePollInterval = 5 * time.Second

func main() {
	redisURL := envOr("REDIS_URL", "redis://localhost:6379")
	pgURL := envOr("DATABASE_URL", "postgres://jakeredding@localhost:5432/rollout_platform")
	migrationsPath := envOr("MIGRATIONS_PATH", "./migrations")
	featureFlagKeyPrefix := envOr("FEATURE_FLAG_KEY_PREFIX", "feature-flag:model-routing:")
	adminAPIKey := envOr("ADMIN_API_KEY", "dev-admin-key")

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

	hubs := api.NewSSEHubRegistry()
	pipelines := api.NewPipelineRegistry()
	bl := batchlogger.New(pool, 10*time.Second)

	modelConfigRepo := modelconfig.NewRepository(pool)
	modelConfigSeeder := modelconfig.NewSeeder(rdb, modelConfigRepo)

	// Seed Redis immediately so Model Service has valid configs on first request.
	if err := modelConfigSeeder.SeedAll(ctx); err != nil {
		log.Fatalf("failed to seed model configs: %v", err)
	}
	log.Printf("redis model configs seeded")

	tenantRepo := tenant.NewRepository(pool)
	tenantSeeder := tenant.NewSeeder(rdb, tenantRepo)

	// Seed Redis immediately so the Edge Evaluator can authenticate requests
	// against every existing tenant on first request.
	if err := tenantSeeder.SeedAll(ctx); err != nil {
		log.Fatalf("failed to seed tenant auth: %v", err)
	}
	log.Printf("redis tenant auth seeded")

	srv := api.New(4003, pipelines, repo, hubs, modelConfigRepo, modelConfigSeeder, tenantRepo, tenantSeeder, adminAPIKey, featureFlagKeyPrefix)

	// These four run for the life of the process, independent of which
	// tenants (if any) currently have an active rollout.
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); srv.Run(ctx) }()
	go func() { defer wg.Done(); bl.Run(ctx) }()
	go func() { defer wg.Done(); modelConfigSeeder.Run(ctx) }()
	go func() { defer wg.Done(); tenantSeeder.Run(ctx) }()

	log.Printf("rollout controller started — api :4003")

	// Blocks until ctx is cancelled, running one pipeline per tenant with an
	// active rollout and reconciling as tenants' rollouts start/change/complete.
	runSupervisor(ctx, rdb, bl, repo, hubs, pipelines)

	log.Printf("shutting down...")
	wg.Wait()
	log.Printf("shutdown complete")
}

// runningPipeline tracks one tenant's currently-running pipeline goroutine
// set, so the reconciler can detect when the underlying rollout it was
// built for is no longer the tenant's active one.
type runningPipeline struct {
	cancel    context.CancelFunc
	rolloutID string
}

// runSupervisor owns the "which tenants have an active rollout" lifecycle.
// Unlike the single-rollout version this replaces, multiple tenants'
// pipelines run concurrently -- this is a reconciliation loop, not a
// one-rollout-at-a-time cycle: every tick it diffs Postgres's current set
// of active (tenant, rollout) pairs against what's actually running, tears
// down pipelines whose tenant went idle or moved to a different rollout,
// and starts pipelines for anything newly active.
func runSupervisor(
	ctx context.Context,
	rdb *redis.Client,
	bl *batchlogger.BatchLogger,
	repo *db.RolloutRepository,
	hubs *api.SSEHubRegistry,
	pipelines *api.PipelineRegistry,
) {
	running := make(map[string]runningPipeline) // tenantID -> its running pipeline
	var wg sync.WaitGroup

	reconcile := func() {
		active, err := repo.ListActiveRollouts(ctx)
		if err != nil {
			log.Printf("supervisor: failed to list active rollouts: %v", err)
			return
		}

		activeByTenant := make(map[string]string, len(active))
		for _, a := range active {
			activeByTenant[a.TenantID] = a.RolloutID
		}

		// Tear down pipelines whose tenant went idle, or whose active
		// rollout changed underneath them (completed/rolled back and a new
		// one created).
		for tenantID, rp := range running {
			if activeByTenant[tenantID] != rp.rolloutID {
				rp.cancel()
				delete(running, tenantID)
			}
		}

		// Start pipelines for tenants that are active but not yet running
		// (newly active, or just torn down above because their rollout changed).
		for tenantID, rolloutID := range activeByTenant {
			if _, ok := running[tenantID]; ok {
				continue
			}

			rolloutCfg, policy, err := repo.LoadActiveForTenant(ctx, tenantID)
			if err != nil {
				log.Printf("supervisor: failed to load active rollout for tenant %s: %v", tenantID, err)
				continue
			}

			pipelineCtx, cancel := context.WithCancel(ctx)
			running[tenantID] = runningPipeline{cancel: cancel, rolloutID: rolloutID}

			log.Printf("supervisor: activating rollout %s for tenant %s (%d%% candidate traffic)",
				rolloutCfg.RolloutID, tenantID, rolloutCfg.CandidatePercentage)

			wg.Add(1)
			go func(tenantID string, cfg config.RolloutConfig, pol config.RolloutPolicy) {
				defer wg.Done()
				runPipeline(pipelineCtx, rdb, bl, repo, hubs.Get(tenantID), pipelines, tenantID, cfg, pol)
			}(tenantID, rolloutCfg, policy)
		}
	}

	reconcile()

	ticker := time.NewTicker(pipelinePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reconcile()
		case <-ctx.Done():
			for _, rp := range running {
				rp.cancel()
			}
			wg.Wait()
			return
		}
	}
}

// runPipeline runs one tenant's ingestion/guard/controller/writer
// goroutines until pipelineCtx is cancelled by the supervisor's reconciler,
// then tears them down and returns.
func runPipeline(
	pipelineCtx context.Context,
	rdb *redis.Client,
	bl *batchlogger.BatchLogger,
	repo *db.RolloutRepository,
	hub *api.SSEHub,
	pipelines *api.PipelineRegistry,
	tenantID string,
	rolloutCfg config.RolloutConfig,
	policy config.RolloutPolicy,
) {
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
	// for this tenant's rollout before any traffic arrives.
	if err := w.SeedRedis(pipelineCtx); err != nil {
		log.Printf("supervisor: failed to seed redis for tenant %s rollout %s: %v", tenantID, rolloutCfg.RolloutID, err)
		return
	}
	log.Printf("supervisor: redis feature flag seeded for tenant %s rollout %s", tenantID, rolloutCfg.RolloutID)

	consumer := ingestion.New(
		rdb,
		rolloutCfg.StreamKey,
		rolloutCfg.StreamConsumerGroup,
		rolloutCfg.StreamConsumerName,
		rolloutCfg.RolloutID,
		store,
		bl,
	)

	g := guard.New(policy, store, w.Commands)
	ctrl := controller.New(policy, store, w)

	pipelines.Set(tenantID, &api.ActivePipeline{Cfg: rolloutCfg, Writer: w, Store: store})
	defer pipelines.Set(tenantID, nil)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); w.Run(pipelineCtx) }()
	go func() { defer wg.Done(); consumer.Run(pipelineCtx) }()
	go func() { defer wg.Done(); g.Run(pipelineCtx) }()
	go func() { defer wg.Done(); ctrl.Run(pipelineCtx) }()

	<-pipelineCtx.Done()
	wg.Wait()
	log.Printf("supervisor: pipeline for tenant %s rollout %s stopped", tenantID, rolloutCfg.RolloutID)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
