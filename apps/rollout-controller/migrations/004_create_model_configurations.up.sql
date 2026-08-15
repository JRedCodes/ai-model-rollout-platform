CREATE TABLE model_configurations (
    model_version_id  TEXT PRIMARY KEY,
    failure_rate      DECIMAL NOT NULL,
    min_latency_ms    INTEGER NOT NULL,
    max_latency_ms    INTEGER NOT NULL,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER model_configurations_updated_at
    BEFORE UPDATE ON model_configurations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO model_configurations (model_version_id, failure_rate, min_latency_ms, max_latency_ms) VALUES
    ('model-v1', 0.01, 50, 150),
    ('model-v2', 0.02, 50, 200);
