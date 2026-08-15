package modelconfig

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Profile struct {
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

func (r *Repository) List(ctx context.Context) ([]Profile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT model_version_id, failure_rate, min_latency_ms, max_latency_ms
		FROM model_configurations
		ORDER BY model_version_id
	`)
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
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *Repository) Get(ctx context.Context, modelVersionID string) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		SELECT model_version_id, failure_rate, min_latency_ms, max_latency_ms
		FROM model_configurations
		WHERE model_version_id = $1
	`, modelVersionID).Scan(&p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs)

	if err == pgx.ErrNoRows {
		return p, &NotFoundError{ModelVersionID: modelVersionID}
	}
	if err != nil {
		return p, fmt.Errorf("get model configuration: %w", err)
	}
	return p, nil
}

// Update writes new simulation parameters for an existing model version.
// Returns NotFoundError if the model version doesn't exist.
func (r *Repository) Update(ctx context.Context, modelVersionID string, failureRate float64, minLatencyMs, maxLatencyMs int) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		UPDATE model_configurations
		SET failure_rate = $1, min_latency_ms = $2, max_latency_ms = $3
		WHERE model_version_id = $4
		RETURNING model_version_id, failure_rate, min_latency_ms, max_latency_ms
	`, failureRate, minLatencyMs, maxLatencyMs, modelVersionID).
		Scan(&p.ModelVersionID, &p.FailureRate, &p.MinLatencyMs, &p.MaxLatencyMs)

	if err == pgx.ErrNoRows {
		return p, &NotFoundError{ModelVersionID: modelVersionID}
	}
	if err != nil {
		return p, fmt.Errorf("update model configuration: %w", err)
	}
	return p, nil
}
