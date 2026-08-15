package modelconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "model-config:"

func redisKey(modelVersionID string) string {
	return redisKeyPrefix + modelVersionID
}

type redisPayload struct {
	ModelVersionID string  `json:"modelVersionId"`
	FailureRate    float64 `json:"failureRate"`
	MinLatencyMs   int     `json:"minLatencyMs"`
	MaxLatencyMs   int     `json:"maxLatencyMs"`
	UpdatedAt      string  `json:"updatedAt"`
}

// Seeder mirrors — for model configuration — the same "Postgres is the
// source of truth, Redis is the fast-read cache" pattern the rollout
// writer already uses for the feature flag: seed on startup, re-seed on a
// heartbeat, and publish immediately on every write.
type Seeder struct {
	rdb  *redis.Client
	repo *Repository
}

func NewSeeder(rdb *redis.Client, repo *Repository) *Seeder {
	return &Seeder{rdb: rdb, repo: repo}
}

// SeedAll writes every model configuration in Postgres to Redis.
func (s *Seeder) SeedAll(ctx context.Context) error {
	profiles, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("seed model configs: %w", err)
	}
	for _, p := range profiles {
		if err := s.Publish(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// Publish writes a single model configuration to Redis.
func (s *Seeder) Publish(ctx context.Context, p Profile) error {
	payload := redisPayload{
		ModelVersionID: p.ModelVersionID,
		FailureRate:    p.FailureRate,
		MinLatencyMs:   p.MinLatencyMs,
		MaxLatencyMs:   p.MaxLatencyMs,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal model config: %w", err)
	}

	if err := s.rdb.Set(ctx, redisKey(p.ModelVersionID), data, 0).Err(); err != nil {
		return fmt.Errorf("write model config to redis: %w", err)
	}
	return nil
}

// Run re-seeds all model configurations on a heartbeat so Redis recovers
// automatically within one cycle after a restart or flush.
func (s *Seeder) Run(ctx context.Context) {
	heartbeat := time.NewTicker(60 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-heartbeat.C:
			if err := s.SeedAll(ctx); err != nil {
				log.Printf("modelconfig: heartbeat re-seed failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
