-- +goose Up
-- IAM-06 (dual control for sensitive admin actions, BP-12): the first gated action is granting the
-- ORGADMIN role itself — iam.role_assignments.approval_id already anticipated a maker-checker gate on
-- role assignment (a real FK to platform.approvals since the very first baseline migration), unused
-- until now. This table is the thin wrapper every other PLT-08-based feature in this codebase uses
-- (notice.notices, dsar.intake_forms, breach.report_forms, ...): one row per request, carrying what
-- platform.record_versions' own opaque snapshot jsonb can't cheaply answer for the approval inbox and
-- list screen (the target user's and role's own names) without a second decode-and-lookup round trip.
-- The real effect — inserting the iam.role_assignments row — only happens once PLT-08 publishes the
-- request's one version (internal/wiring's OnPublish hook), so the action has no effect until approved,
-- per the acceptance criterion literally: the authz grants loader (iam/service/loader.go's
-- ListEffectiveRoleAssignments) reads iam.role_assignments directly, with no "approval decided" gate of
-- its own, so nothing may insert that row before this request's version is actually published. No new
-- permission grant needed for SEC (the module doc's own approver): versioning.Service.authorize's own
-- "read" case already lets a caller holding one of the record type's own Steps roles through with no
-- separate permission check — SEC decides purely by holding that role, the same as every other PLT-08
-- maker-checker flow in this codebase.
CREATE TABLE iam.role_assignment_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    target_user_id uuid NOT NULL,
    role_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    role_assignment_id uuid,
    applied_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_iam_role_assignment_requests PRIMARY KEY (id)
);

ALTER TABLE iam.role_assignment_requests ADD CONSTRAINT fk_role_assignment_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.role_assignment_requests ADD CONSTRAINT fk_role_assignment_requests_target_user FOREIGN KEY (target_user_id) REFERENCES iam.users (id);
ALTER TABLE iam.role_assignment_requests ADD CONSTRAINT fk_role_assignment_requests_role FOREIGN KEY (role_id) REFERENCES iam.roles (id);
ALTER TABLE iam.role_assignment_requests ADD CONSTRAINT fk_role_assignment_requests_requested_by FOREIGN KEY (requested_by) REFERENCES iam.users (id);
ALTER TABLE iam.role_assignment_requests ADD CONSTRAINT fk_role_assignment_requests_role_assignment FOREIGN KEY (role_assignment_id) REFERENCES iam.role_assignments (id);

CREATE INDEX ix_iam_role_assignment_requests_tenant ON iam.role_assignment_requests (tenant_id, created_at DESC);

ALTER TABLE iam.role_assignment_requests ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.role_assignment_requests
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down
DROP TABLE IF EXISTS iam.role_assignment_requests;
