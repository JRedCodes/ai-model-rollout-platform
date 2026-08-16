package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JRedCodes/rollout-controller/internal/config"
)

// ErrNoActiveRollout means no rollout is currently RUNNING or HELD. This is
// an expected, non-fatal state — e.g. right after a rollout COMPLETEs and
// before the next one is created via the Management API.
var ErrNoActiveRollout = errors.New("no active rollout")

type Decision struct {
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	Source    string    `json:"source"`
	DecidedAt time.Time `json:"decidedAt"`
}

type RolloutRepository struct {
	pool *pgxpool.Pool
}

func NewRolloutRepository(pool *pgxpool.Pool) *RolloutRepository {
	return &RolloutRepository{pool: pool}
}

// LoadActive reads the single RUNNING or HELD rollout from the DB and returns
// its config and policy. Returns an error if no active rollout exists.
func (r *RolloutRepository) LoadActive(ctx context.Context) (config.RolloutConfig, config.RolloutPolicy, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			id, rollout_phase_id, stable_model_version_id,
			candidate_model_version_id, candidate_percentage,
			configuration_version, feature_flag_key,
			min_requests_before_guard, fresh_window_size,
			fresh_window_max_error_rate, absolute_max_error_rate,
			controller_interval_secs, advance_min_requests,
			advance_max_error_rate, advance_max_p95_latency_ms
		FROM rollouts
		WHERE status IN ('RUNNING', 'HELD')
		ORDER BY created_at DESC
		LIMIT 1
	`)

	var (
		cfg         config.RolloutConfig
		policy      config.RolloutPolicy
		candidateID *string
	)

	err := row.Scan(
		&cfg.RolloutID,
		&cfg.RolloutPhaseID,
		&cfg.StableModelVersionID,
		&candidateID,
		&cfg.CandidatePercentage,
		&cfg.ConfigurationVersion,
		&cfg.FeatureFlagKey,
		&policy.MinRequestsBeforeGuard,
		&policy.FreshWindowSize,
		&policy.FreshWindowMaxErrorRate,
		&policy.AbsoluteMaxErrorRate,
		&policy.ControllerIntervalSecs,
		&policy.AdvanceMinRequests,
		&policy.AdvanceMaxErrorRate,
		&policy.AdvanceMaxP95LatencyMs,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cfg, policy, ErrNoActiveRollout
		}
		return cfg, policy, fmt.Errorf("load active rollout: %w", err)
	}

	if candidateID != nil {
		cfg.CandidateModelVersionID = *candidateID
	}

	cfg.StreamKey = "telemetry:inference-completed"
	cfg.StreamConsumerGroup = "rollout-controller"
	cfg.StreamConsumerName = "controller-1"

	return cfg, policy, nil
}

// ActiveRolloutID returns just the ID of the current RUNNING/HELD rollout,
// for the supervisor loop's cheap polling check. Returns ErrNoActiveRollout
// if none exists.
func (r *RolloutRepository) ActiveRolloutID(ctx context.Context) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM rollouts
		WHERE status IN ('RUNNING', 'HELD')
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNoActiveRollout
		}
		return "", fmt.Errorf("active rollout id: %w", err)
	}
	return id, nil
}

func (r *RolloutRepository) UpdateStatus(ctx context.Context, rolloutID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE rollouts SET status = $1 WHERE id = $2`,
		status, rolloutID,
	)
	return err
}

func (r *RolloutRepository) UpdatePercentage(ctx context.Context, rolloutID string, percentage int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE rollouts
		SET candidate_percentage = $1, configuration_version = configuration_version + 1
		WHERE id = $2
	`, percentage, rolloutID)
	return err
}

func (r *RolloutRepository) InsertDecision(ctx context.Context, id, rolloutID, action, reason, source string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO rollout_decisions (id, rollout_id, action, reason, source)
		VALUES ($1, $2, $3, $4, $5)
	`, id, rolloutID, action, reason, source)
	return err
}

// ListDecisions returns the most recent decisions for a rollout, newest first.
func (r *RolloutRepository) ListDecisions(ctx context.Context, rolloutID string, limit int) ([]Decision, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT action, reason, source, decided_at
		FROM rollout_decisions
		WHERE rollout_id = $1
		ORDER BY decided_at DESC
		LIMIT $2
	`, rolloutID, limit)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var decisions []Decision
	for rows.Next() {
		var d Decision
		if err := rows.Scan(&d.Action, &d.Reason, &d.Source, &d.DecidedAt); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}
