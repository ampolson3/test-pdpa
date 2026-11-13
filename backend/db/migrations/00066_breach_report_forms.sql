-- +goose Up
-- BRE-01 (แบบฟอร์มแจ้งเหตุ): the public, loginless counterpart to the admin app's own employee intake
-- form (already built by BRE-02's /incidents page, reachable by any EMP holding breach.incident.create —
-- "employee_form" in reported_via's own CHECK constraint). What's missing is PUBLIC's own channel: an
-- external person (a vendor, a customer, anyone who spots a breach) with no login — the same role
-- dsar.intake_forms (DSAR-01), consent.collection_points (CON-09) and notice.notices (PNG-06) already play
-- for their own public surfaces. breach.incidents' own reported_via CHECK already names 'public_form' and
-- carries reporter_contact_enc (00013_breach.sql), unused until now — no change to that table needed.
CREATE TABLE breach.report_forms (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    public_key varchar(64),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_report_forms PRIMARY KEY (id),
    CONSTRAINT fk_breach_report_forms_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id),
    CONSTRAINT fk_breach_report_forms_legal_entity FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id)
);
CREATE INDEX ix_breach_report_forms_tenant_id ON breach.report_forms (tenant_id);

ALTER TABLE breach.report_forms ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.report_forms FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.report_forms
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

COMMENT ON TABLE breach.report_forms IS 'BRE-01: เว็บฟอร์มสาธารณะสำหรับบุคคลภายนอกแจ้งเหตุละเมิด — เมื่อ publish แล้วจะมี public_key (platform.public_keys)';

-- breach.form.* — managing the report form itself (DPO/PRIVACY/SEC configure it; the actual reporters, EMP
-- and PUBLIC, never need a permission at all: EMP already holds breach.incident.create for the admin form,
-- PUBLIC never authenticates). Mirrors dsar.form's own shape (00019 baseline), added fresh here since
-- breach had no equivalent placeholder code seeded.
INSERT INTO iam.permissions (code, module, resource, action, description, is_sensitive) VALUES
    ('breach.form.create', 'breach', 'form', 'C', 'แบบฟอร์มแจ้งเหตุสาธารณะ — สร้าง', false),
    ('breach.form.read', 'breach', 'form', 'R', 'แบบฟอร์มแจ้งเหตุสาธารณะ — ดู', false),
    ('breach.form.update', 'breach', 'form', 'U', 'แบบฟอร์มแจ้งเหตุสาธารณะ — แก้ไข', false),
    ('breach.form.delete', 'breach', 'form', 'D', 'แบบฟอร์มแจ้งเหตุสาธารณะ — ลบ', false),
    ('breach.form.publish', 'breach', 'form', 'P', 'แบบฟอร์มแจ้งเหตุสาธารณะ — เผยแพร่ / เปิดใช้งาน', false);

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, p.code
FROM iam.roles r
CROSS JOIN (VALUES ('breach.form.create'), ('breach.form.read'), ('breach.form.update'), ('breach.form.delete'), ('breach.form.publish')) AS p(code)
WHERE r.tenant_id IS NULL AND r.code = 'DPO';

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, p.code
FROM iam.roles r
CROSS JOIN (VALUES ('breach.form.create'), ('breach.form.read'), ('breach.form.update')) AS p(code)
WHERE r.tenant_id IS NULL AND r.code IN ('PRIVACY', 'SEC');

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, 'breach.form.read'
FROM iam.roles r
WHERE r.tenant_id IS NULL AND r.code = 'AUDIT';

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

DELETE FROM iam.role_permissions WHERE permission_code LIKE 'breach.form.%';

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

DELETE FROM iam.permissions WHERE code LIKE 'breach.form.%';

DROP TABLE breach.report_forms;
