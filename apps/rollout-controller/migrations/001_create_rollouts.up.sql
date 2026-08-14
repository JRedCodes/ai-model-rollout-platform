CREATE TABLE rollouts (
    id                          TEXT PRIMARY KEY,
    rollout_phase_id            TEXT NOT NULL,
    stable_model_version_id     TEXT NOT NULL,
    candidate_model_version_id  TEXT,
    candidate_percentage        INTEGER NOT NULL DEFAULT 0,
    configuration_version       INTEGER NOT NULL DEFAULT 1,
    status                      TEXT NOT NULL DEFAULT 'RUNNING',
    feature_flag_key            TEXT NOT NULL,

    -- Policy fields
    min_requests_before_guard   INTEGER NOT NULL DEFAULT 50,
    fresh_window_size           INTEGER NOT NULL DEFAULT 50,
    fresh_window_max_error_rate DECIMAL NOT NULL DEFAULT 0.30,
    absolute_max_error_rate     DECIMAL NOT NULL DEFAULT 0.05,
    controller_interval_secs    INTEGER NOT NULL DEFAULT 120,
    advance_min_requests        INTEGER NOT NULL DEFAULT 100,
    advance_max_error_rate      DECIMAL NOT NULL DEFAULT 0.02,
    advance_max_p95_latency_ms  INTEGER NOT NULL DEFAULT 250,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER rollouts_updated_at
    BEFORE UPDATE ON rollouts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
