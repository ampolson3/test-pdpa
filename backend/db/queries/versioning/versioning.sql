-- PLT-08 versioning & approval.

-- name: ListVersions :many
SELECT id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version
FROM platform.record_versions WHERE entity_type = $1 AND entity_id = $2
ORDER BY version_no DESC;

-- name: GetVersion :one
SELECT id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version
FROM platform.record_versions WHERE id = $1;

-- name: LockVersion :one
SELECT id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version
FROM platform.record_versions WHERE id = $1 FOR UPDATE;

-- name: OpenVersion :one
SELECT id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version
FROM platform.record_versions WHERE entity_type = $1 AND entity_id = $2 AND status IN ('draft', 'in_review', 'approved')
FOR UPDATE;

-- name: PublishedVersion :one
SELECT id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version
FROM platform.record_versions WHERE entity_type = $1 AND entity_id = $2 AND status = 'published';

-- name: NextVersionNo :one
SELECT (COALESCE(max(version_no), 0) + 1)::int FROM platform.record_versions WHERE entity_type = $1 AND entity_id = $2;

-- name: InsertVersion :one
INSERT INTO platform.record_versions (id, tenant_id, entity_type, entity_id, version_no, snapshot, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version;

-- name: UpdateVersion :one
UPDATE platform.record_versions
SET snapshot = $2, diff = $3, status = $4, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1
RETURNING id, entity_type, entity_id, version_no, snapshot, diff, status, created_by, created_at, updated_at, row_version;

-- name: InsertApproval :one
INSERT INTO platform.approvals (id, tenant_id, entity_type, entity_id, record_version_id, step_no, requested_by, approver_role, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id;

-- name: ListApprovals :many
SELECT id, record_version_id, step_no, requested_by, approver_user_id, approver_role, decision, reason, decided_at, created_at, row_version
FROM platform.approvals WHERE record_version_id = $1
ORDER BY created_at, step_no;

-- name: LockApproval :one
SELECT id, entity_type, entity_id, record_version_id, step_no, requested_by, approver_user_id, approver_role, decision, reason, decided_at, created_at, row_version
FROM platform.approvals WHERE id = $1 FOR UPDATE;

-- name: DecideApproval :exec
UPDATE platform.approvals
SET decision = $2, reason = $3, approver_user_id = $4, decided_at = $5, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1;

-- name: DropPendingApprovals :exec
-- A version sent back for changes: its undecided later steps no longer apply (a resubmission starts a new round).
DELETE FROM platform.approvals WHERE record_version_id = $1 AND decision = 'pending';

-- name: MyPendingApprovals :many
-- Approvals waiting on one of these roles: the lowest pending step of a version under review, which the
-- caller neither wrote nor submitted, nor already approved at an earlier step (maker-checker).
SELECT a.id, a.entity_type, a.entity_id, a.record_version_id, a.step_no, a.requested_by, a.approver_role, a.created_at, a.row_version,
       v.version_no, v.created_by AS author
FROM platform.approvals a
JOIN platform.record_versions v ON v.id = a.record_version_id AND v.status = 'in_review'
WHERE a.decision = 'pending' AND a.approver_role = ANY (@roles::text[])
  AND a.requested_by <> @user_id::uuid AND v.created_by IS DISTINCT FROM @user_id::uuid
  AND NOT EXISTS (SELECT 1 FROM platform.approvals e WHERE e.record_version_id = a.record_version_id AND e.decision = 'pending' AND e.step_no < a.step_no)
  AND NOT EXISTS (SELECT 1 FROM platform.approvals d WHERE d.record_version_id = a.record_version_id AND d.approver_user_id = @user_id::uuid AND d.decision = 'approved')
ORDER BY a.created_at
LIMIT 200;
