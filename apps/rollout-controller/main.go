package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/JRedCodes/rollout-controller/internal/api"
	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/controller"
	"github.com/JRedCodes/rollout-controller/internal/guard"
	"github.com/JRedCodes/rollout-controller/internal/ingestion"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	redisc "github.com/JRedCodes/rollout-controller/internal/redis"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

func main() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	rdb, err := redisc.NewClient(redisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	rolloutCfg := config.DefaultConfig()
	policy := config.DefaultPolicy()

	store := metrics.NewStore()

	w := writer.New(
		rdb,
		rolloutCfg.FeatureFlagKey,
		rolloutCfg.RolloutID,
		rolloutCfg.RolloutPhaseID,
		rolloutCfg.StableModelVersionID,
		rolloutCfg.CandidateModelVersionID,
	)

	consumer := ingestion.New(
		rdb,
		rolloutCfg.StreamKey,
		rolloutCfg.StreamConsumerGroup,
		rolloutCfg.StreamConsumerName,
		store,
	)

	g := guard.New(policy, store, w.Commands)
	ctrl := controller.New(policy, store, w)
	srv := api.New(4003, rolloutCfg, store, w)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(5)

	go func() { defer wg.Done(); w.Run(ctx) }()
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
