-- Only one rollout may be RUNNING or HELD at a time. The Management API
-- checks this before inserting; this index is the backstop against races.
CREATE UNIQUE INDEX rollouts_single_active_idx
    ON rollouts ((1))
    WHERE status IN ('RUNNING', 'HELD');
