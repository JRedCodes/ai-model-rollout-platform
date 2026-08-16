DROP INDEX IF EXISTS rollouts_single_active_per_tenant_idx;
CREATE UNIQUE INDEX rollouts_single_active_idx
    ON rollouts ((1))
    WHERE status IN ('RUNNING', 'HELD');
ALTER TABLE rollouts DROP COLUMN tenant_id;
