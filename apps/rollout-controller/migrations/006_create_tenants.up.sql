CREATE TABLE tenants (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    api_key_hash  TEXT NOT NULL UNIQUE,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Seeded demo tenant so a fresh clone keeps working without bootstrapping
-- one via POST /tenants first. Plaintext key (documented in the README,
-- dev-only): tk_demo_2218a6e29efe8f4b3378390b46a0710d
INSERT INTO tenants (id, name, api_key_hash) VALUES
    ('tenant-demo', 'Demo Tenant', 'e3d53ba21fd273bfa67fa271bf5d48d0084e51bc9aaae1c7bbce96ead82ddc4b');
