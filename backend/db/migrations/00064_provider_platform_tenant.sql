-- +goose Up
-- IAM-03 (Console ผู้ให้บริการ: tenant และแพ็กเกจ, decisions.md D-24 / ex-Q-19): SUPER accounts live in a
-- special reserved tenant, `platform`, not a real customer tenant — so AuthZ's ordinary per-tenant grant
-- resolution (internal/pkg/authz.StrictMiddleware, keyed by principal.TenantID) works for SUPER exactly
-- the same way it does for any other user, with no special-casing. platform.tenants has no RLS (it's one
-- of the "global tables without RLS" in deploy/db/10-grants.sql, read-only for pdpa_app, written only by
-- the provider console's pdpa_platform role or a migration like this one), so this plain INSERT is safe.
INSERT INTO platform.tenants (id, code, name, status, plan_code, deployment, data_region)
VALUES ('00000000-0000-7000-8000-000000000001', 'platform', 'Provider platform (SUPER)', 'active', 'platform', 'saas_shared', 'TH')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM platform.tenants WHERE id = '00000000-0000-7000-8000-000000000001';
