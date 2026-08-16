ALTER TABLE model_configurations DROP CONSTRAINT model_configurations_pkey;
ALTER TABLE model_configurations ADD COLUMN tenant_id TEXT REFERENCES tenants(id);
UPDATE model_configurations SET tenant_id = 'tenant-demo' WHERE tenant_id IS NULL;
ALTER TABLE model_configurations ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE model_configurations ADD PRIMARY KEY (tenant_id, model_version_id);
