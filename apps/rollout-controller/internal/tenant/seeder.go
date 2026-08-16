package tenant

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "tenant-auth:"

func redisKey(apiKeyHash string) string {
	return redisKeyPrefix + apiKeyHash
}

// Seeder publishes tenant API key hashes to Redis as hash -> tenantID, so
// the Edge Evaluator (no direct Postgres access) can authenticate requests
// with a single Redis read. Mirrors modelconfig.Seeder's "Postgres is the
// source of truth, Redis is the fast-read cache" pattern.
type Seeder struct {
	rdb  *redis.Client
	repo *Repository
}

func NewSeeder(rdb *redis.Client, repo *Repository) *Seeder {
	return &Seeder{rdb: rdb, repo: repo}
}

// SeedAll publishes every tenant's auth entry from Postgres to Redis.
func (s *Seeder) SeedAll(ctx context.Context) error {
	entries, err := s.repo.ListAuth(ctx)
	if err != nil {
		return fmt.Errorf("seed tenant auth: %w", err)
	}
	for _, e := range entries {
		if err := s.publishHash(ctx, e.TenantID, e.APIKeyHash); err != nil {
			return err
		}
	}
	return nil
}

// Publish writes a single newly-created tenant's auth entry to Redis,
// given its plaintext key (only available right after Create).
func (s *Seeder) Publish(ctx context.Context, tenantID, plaintextKey string) error {
	return s.publishHash(ctx, tenantID, hashAPIKey(plaintextKey))
}

func (s *Seeder) publishHash(ctx context.Context, tenantID, apiKeyHash string) error {
	if err := s.rdb.Set(ctx, redisKey(apiKeyHash), tenantID, 0).Err(); err != nil {
		return fmt.Errorf("write tenant auth to redis: %w", err)
	}
	return nil
}

// Run re-seeds all tenant auth entries on a heartbeat so Redis recovers
// automatically within one cycle after a restart or flush.
func (s *Seeder) Run(ctx context.Context) {
	heartbeat := time.NewTicker(60 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-heartbeat.C:
			if err := s.SeedAll(ctx); err != nil {
				log.Printf("tenant: heartbeat re-seed failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
