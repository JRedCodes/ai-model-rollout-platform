ALTER TABLE model_configurations DROP CONSTRAINT model_configurations_pkey;
ALTER TABLE model_configurations DROP COLUMN tenant_id;
ALTER TABLE model_configurations ADD PRIMARY KEY (model_version_id);
