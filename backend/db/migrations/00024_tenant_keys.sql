-- +goose Up
-- PLT-13: wrapped data keys for field-level PII encryption (docs/architecture/security.md § ลำดับชั้นกุญแจ).
-- One row per (tenant, purpose, data class, version): purpose 'dek' = AES-256-GCM key for *_enc columns of
-- that data class, 'blind_index' = HMAC-SHA256 key for exact-match lookup. wrapped_key is the key encrypted by
-- the tenant's KEK (OpenBao Transit); kek_ref records which KEK version wrapped it so rotation can rewrap
-- without touching the data. Keys are never deleted by the application: losing one makes its data unreadable
-- (crypto-shredding is a deliberate tenant-offboarding step), so DELETE is revoked in 10-grants.sql.

CREATE TABLE platform.tenant_keys (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('dek', 'blind_index')),
    data_class varchar(60) NOT NULL,
    version int NOT NULL CHECK (version > 0),
    wrapped_key bytea NOT NULL,
    kek_ref varchar(200) NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_tenant_keys PRIMARY KEY (id),
    CONSTRAINT uq_tenant_keys_purpose_class_version UNIQUE (tenant_id, purpose, data_class, version),
    CONSTRAINT fk_tenant_keys_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id)
);
-- exactly one active key per tenant, purpose and data class
CREATE UNIQUE INDEX uq_tenant_keys_active ON platform.tenant_keys (tenant_id, purpose, data_class) WHERE status = 'active';

ALTER TABLE platform.tenant_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.tenant_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.tenant_keys
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER trg_tenant_keys_updated BEFORE UPDATE ON platform.tenant_keys FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
COMMENT ON TABLE platform.tenant_keys IS 'กุญแจข้อมูล (DEK / blind index key) ต่อ tenant ที่ห่อด้วย KEK ใน OpenBao Transit — ห้ามลบ (PLT-13)';

-- +goose Down
DROP TABLE IF EXISTS platform.tenant_keys;
