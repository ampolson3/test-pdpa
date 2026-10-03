-- +goose Up
-- PLT-10: admin.job.read — view the tenant's background jobs (River) on the admin job-status page.
-- Granted to the ORGADMIN and SUPER system roles (docs/security/permissions.yaml). Global rows need the
-- table owner to bypass RLS inside this migration, as in 00019 (FORCE is restored before commit).

INSERT INTO iam.permissions (code, module, resource, action, description, is_sensitive) VALUES
    ('admin.job.read', 'admin', 'job', 'R', 'งานเบื้องหลัง (background job) — ดู', false);

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, 'admin.job.read'
FROM iam.roles r
WHERE r.tenant_id IS NULL AND r.code IN ('ORGADMIN', 'SUPER');

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

DELETE FROM iam.role_permissions WHERE permission_code = 'admin.job.read';

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

DELETE FROM iam.permissions WHERE code = 'admin.job.read';
