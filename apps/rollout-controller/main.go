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

	"github.com/JRedCodes/rollout-controller/internal/api"
	"github.com/JRedCodes/rollout-controller/internal/batchlogger"
	"github.com/JRedCodes/rollout-controller/internal/controller"
	"github.com/JRedCodes/rollout-controller/internal/db"
	"github.com/JRedCodes/rollout-controller/internal/guard"
	"github.com/JRedCodes/rollout-controller/internal/ingestion"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	redisc "github.com/JRedCodes/rollout-controller/internal/redis"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

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

	rolloutCfg, policy, err := repo.LoadActive(ctx)
	if err != nil {
		log.Fatalf("no active rollout in DB: %v\nSeed one with: INSERT INTO rollouts (id, rollout_phase_id, stable_model_version_id, candidate_model_version_id, candidate_percentage, feature_flag_key) VALUES (...)", err)
	}
	log.Printf("loaded rollout %s (%d%% candidate traffic)", rolloutCfg.RolloutID, rolloutCfg.CandidatePercentage)

	rdb, err := redisc.NewClient(redisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	store := metrics.NewStore()
	bl := batchlogger.New(pool, 10*time.Second)

	w := writer.New(
		rdb,
		repo,
		rolloutCfg.FeatureFlagKey,
		rolloutCfg.RolloutID,
		rolloutCfg.RolloutPhaseID,
		rolloutCfg.StableModelVersionID,
		rolloutCfg.CandidateModelVersionID,
		rolloutCfg.CandidatePercentage,
		rolloutCfg.ConfigurationVersion,
	)

	// Seed Redis immediately so the Edge Evaluator has a valid feature flag.
	if err := w.SeedRedis(ctx); err != nil {
		log.Fatalf("failed to seed redis: %v", err)
	}
	log.Printf("redis feature flag seeded")

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
	srv := api.New(4003, rolloutCfg, store, w)

	var wg sync.WaitGroup
	wg.Add(6)

	go func() { defer wg.Done(); w.Run(ctx) }()
	go func() { defer wg.Done(); bl.Run(ctx) }()
	go func() { defer wg.Done(); consumer.Run(ctx) }()
	go func() { defer wg.Done(); g.Run(ctx) }()
	go func() { defer wg.Done(); ctrl.Run(ctx) }()
	go func() { defer wg.Done(); srv.Run(ctx) }()

	log.Printf("rollout controller started — rollout %s, stream %s, api :4003",
		rolloutCfg.RolloutID, rolloutCfg.StreamKey)

	<-ctx.Done()
	log.Printf("shutting down...")
	wg.Wait()
	log.Printf("shutdown complete")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
