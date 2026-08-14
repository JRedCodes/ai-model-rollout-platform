CREATE TABLE inference_events (
    id               TEXT PRIMARY KEY,
    request_id       TEXT NOT NULL,
    user_id          TEXT NOT NULL,
    rollout_id       TEXT REFERENCES rollouts(id),
    rollout_phase_id TEXT,
    model_version_id TEXT NOT NULL,
    assignment       TEXT NOT NULL,
    success          BOOLEAN NOT NULL,
    error_type       TEXT,
    latency_ms       INTEGER NOT NULL,
    occurred_at      TIMESTAMPTZ NOT NULL,
    ingested_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX inference_events_rollout_occurred_idx
    ON inference_events (rollout_id, occurred_at);
