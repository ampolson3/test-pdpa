-- +goose Up
-- PLT-19: platform.health.read — the provider console's own system-health dashboard (error rate /
-- latency over the last few minutes). SUPER only, same pattern as 00021's admin.job.read.

INSERT INTO iam.permissions (code, module, resource, action, description, is_sensitive) VALUES
    ('platform.health.read', 'platform', 'health', 'R', 'สุขภาพระบบ (error rate / latency) — ดู', false);

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, 'platform.health.read'
FROM iam.roles r
WHERE r.tenant_id IS NULL AND r.code = 'SUPER';

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

DELETE FROM iam.role_permissions WHERE permission_code = 'platform.health.read';

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

DELETE FROM iam.permissions WHERE code = 'platform.health.read';
