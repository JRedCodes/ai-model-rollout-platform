package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/JRedCodes/rollout-controller/internal/token"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidAPIKey means the presented API key doesn't match any tenant.
var ErrInvalidAPIKey = errors.New("invalid api key")

type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// defaultModelConfig is a starting model simulation profile seeded for
// every new tenant, so POST /rollouts works immediately without a
// separate "set up your model catalog" step. Mirrors the values migration
// 004 originally seeded globally, before model configs became tenant-scoped.
type defaultModelConfig struct {
	modelVersionID string
	failureRate    float64
	minLatencyMs   int
	maxLatencyMs   int
}

var defaultModelConfigs = []defaultModelConfig{
	{modelVersionID: "model-v1", failureRate: 0.01, minLatencyMs: 50, maxLatencyMs: 150},
	{modelVersionID: "model-v2", failureRate: 0.02, minLatencyMs: 50, maxLatencyMs: 200},
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new tenant with a freshly generated API key, and seeds
// its default model catalog in the same transaction. The plaintext key is
// returned exactly once here — only its hash is ever persisted.
func (r *Repository) Create(ctx context.Context, name string) (Tenant, string, error) {
	id := uuid.NewString()

	plaintextKey, err := generateAPIKey()
	if err != nil {
		return Tenant{}, "", fmt.Errorf("generate api key: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Tenant{}, "", fmt.Errorf("begin create tenant tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key_hash) VALUES ($1, $2, $3)
	`, id, name, hashAPIKey(plaintextKey)); err != nil {
		return Tenant{}, "", fmt.Errorf("insert tenant: %w", err)
	}

	for _, mc := range defaultModelConfigs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO model_configurations (tenant_id, model_version_id, failure_rate, min_latency_ms, max_latency_ms)
			VALUES ($1, $2, $3, $4, $5)
		`, id, mc.modelVersionID, mc.failureRate, mc.minLatencyMs, mc.maxLatencyMs); err != nil {
			return Tenant{}, "", fmt.Errorf("seed default model config %s: %w", mc.modelVersionID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Tenant{}, "", fmt.Errorf("commit create tenant tx: %w", err)
	}

	return Tenant{ID: id, Name: name}, plaintextKey, nil
}

// AuthEntry is a tenant's ID paired with its API key hash, for the Seeder
// to publish to Redis. Only the hash is ever read back — the plaintext key
// is never stored anywhere after Create returns it.
type AuthEntry struct {
	TenantID   string
	APIKeyHash string
}

// ListAuth returns every tenant's (id, api_key_hash) pair.
func (r *Repository) ListAuth(ctx context.Context) ([]AuthEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, api_key_hash FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("list tenant auth: %w", err)
	}
	defer rows.Close()

	var entries []AuthEntry
	for rows.Next() {
		var e AuthEntry
		if err := rows.Scan(&e.TenantID, &e.APIKeyHash); err != nil {
			return nil, fmt.Errorf("scan tenant auth: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetIDByAPIKey resolves a presented plaintext API key to a tenant ID.
// Returns ErrInvalidAPIKey if it doesn't match any tenant.
func (r *Repository) GetIDByAPIKey(ctx context.Context, plaintextKey string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM tenants WHERE api_key_hash = $1
	`, hashAPIKey(plaintextKey)).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidAPIKey
	}
	if err != nil {
		return "", fmt.Errorf("get tenant by api key: %w", err)
	}
	return id, nil
}

func hashAPIKey(plaintextKey string) string {
	return token.Hash(plaintextKey)
}

func generateAPIKey() (string, error) {
	raw, err := token.Generate()
	if err != nil {
		return "", err
	}
	return "tk_" + raw, nil
}
