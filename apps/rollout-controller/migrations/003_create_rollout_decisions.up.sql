CREATE TABLE rollout_decisions (
    id         TEXT PRIMARY KEY,
    rollout_id TEXT NOT NULL REFERENCES rollouts(id),
    action     TEXT NOT NULL,
    reason     TEXT NOT NULL,
    source     TEXT NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX rollout_decisions_rollout_idx
    ON rollout_decisions (rollout_id, decided_at);
