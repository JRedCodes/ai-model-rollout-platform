ALTER TABLE rollouts ADD COLUMN tenant_id TEXT REFERENCES tenants(id);
UPDATE rollouts SET tenant_id = 'tenant-demo' WHERE tenant_id IS NULL;
ALTER TABLE rollouts ALTER COLUMN tenant_id SET NOT NULL;

-- Multi-tenant replaces "one active rollout, period" with "one active
-- rollout per tenant."
DROP INDEX IF EXISTS rollouts_single_active_idx;
CREATE UNIQUE INDEX rollouts_single_active_per_tenant_idx
    ON rollouts (tenant_id)
    WHERE status IN ('RUNNING', 'HELD');
