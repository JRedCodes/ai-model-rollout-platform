package modelconfig

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Profile struct {
	TenantID       string  `json:"-"`
	ModelVersionID string  `json:"modelVersionId"`
	FailureRate    float64 `json:"failureRate"`
	MinLatencyMs   int     `json:"minLatencyMs"`
	MaxLatencyMs   int     `json:"maxLatencyMs"`
}

type NotFoundError struct {
	ModelVersionID string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("model configuration not found: %s", e.ModelVersionID)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// List returns tenantID's model catalog. Each tenant has its own
// independent set of model configurations (primary key is
// (tenant_id, model_version_id)) — editing one tenant's model never
// affects another's.
func (r *Repository) List(ctx context.Context, tenantID string) ([]Profile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT model_version_id, failure_rate, min_latency_ms, max_latency_ms
		FROM model_configurations
		WHERE tenant_id = $1
		ORDER BY model_version_id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list model configurations: %w", err)
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs); err != nil {
			return nil, fmt.Errorf("scan model configuration: %w", err)
		}
		p.TenantID = tenantID
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// ListAll returns every tenant's model catalog, for the Seeder's heartbeat
// re-seed — unlike List, this isn't scoped to one tenant.
func (r *Repository) ListAll(ctx context.Context) ([]Profile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tenant_id, model_version_id, failure_rate, min_latency_ms, max_latency_ms
		FROM model_configurations
		ORDER BY tenant_id, model_version_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list all model configurations: %w", err)
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.TenantID, &p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs); err != nil {
			return nil, fmt.Errorf("scan model configuration: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *Repository) Get(ctx context.Context, tenantID, modelVersionID string) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		SELECT model_version_id, failure_rate, min_latency_ms, max_latency_ms
		FROM model_configurations
		WHERE tenant_id = $1 AND model_version_id = $2
	`, tenantID, modelVersionID).Scan(&p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs)

	if err == pgx.ErrNoRows {
		return p, &NotFoundError{ModelVersionID: modelVersionID}
	}
	if err != nil {
		return p, fmt.Errorf("get model configuration: %w", err)
	}
	p.TenantID = tenantID
	return p, nil
}

// Update writes new simulation parameters for an existing model version
// belonging to tenantID. Returns NotFoundError if it doesn't exist (or
// belongs to another tenant).
func (r *Repository) Update(ctx context.Context, tenantID, modelVersionID string, failureRate float64, minLatencyMs, maxLatencyMs int) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		UPDATE model_configurations
		SET failure_rate = $1, min_latency_ms = $2, max_latency_ms = $3
		WHERE tenant_id = $4 AND model_version_id = $5
		RETURNING model_version_id, failure_rate, min_latency_ms, max_latency_ms
	`, failureRate, minLatencyMs, maxLatencyMs, tenantID, modelVersionID).
		Scan(&p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs)

	if err == pgx.ErrNoRows {
		return p, &NotFoundError{ModelVersionID: modelVersionID}
	}
	if err != nil {
		return p, fmt.Errorf("update model configuration: %w", err)
	}
	p.TenantID = tenantID
	return p, nil
}
