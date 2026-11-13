-- +goose Up
-- PLT-20 (Connector framework): platform.connectors.secret_ref (already in the baseline migration, unused
-- until now) is a logical reference string, not the credential itself — this table is what it points at.
-- The connector's own credential (a DSN password, an API key, ...) is sealed with PLT-13's own Keyring
-- (class "connector", context = the secret_ref string itself, so a forged ref can never decrypt a
-- different connector's secret) and stored here, one row per (tenant, secret_ref). Deleting a connector
-- deletes its secret row too — unlike platform.tenant_keys, there's no crypto-shredding concern here, since
-- the credential is just rotated/reissued by the customer's own source system, not evidence to retain.

CREATE TABLE platform.connector_secrets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    secret_ref varchar(200) NOT NULL,
    value_enc bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_connector_secrets PRIMARY KEY (id),
    CONSTRAINT uq_connector_secrets_ref UNIQUE (tenant_id, secret_ref),
    CONSTRAINT fk_connector_secrets_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id)
);

ALTER TABLE platform.connector_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.connector_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.connector_secrets
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER trg_connector_secrets_updated BEFORE UPDATE ON platform.connector_secrets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
COMMENT ON TABLE platform.connector_secrets IS 'credential ของ connector เข้ารหัสด้วย Keyring ต่อ secret_ref (PLT-20)';

-- +goose Down
DROP TABLE IF EXISTS platform.connector_secrets;
