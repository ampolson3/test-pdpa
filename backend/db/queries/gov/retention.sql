-- name: ListRetentionSchedules :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, retention_rule_id, asset_id, next_due_at, status, row_version, created_at, updated_at
FROM gov.retention_schedules
WHERE (sqlc.narg(asset_id)::uuid IS NULL OR asset_id = sqlc.narg(asset_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetRetentionSchedule :one
SELECT id, retention_rule_id, asset_id, next_due_at, status, row_version, created_at, updated_at
FROM gov.retention_schedules
WHERE id = $1;

-- name: InsertRetentionSchedule :one
INSERT INTO gov.retention_schedules (id, tenant_id, retention_rule_id, asset_id, next_due_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, retention_rule_id, asset_id, next_due_at, status, row_version, created_at, updated_at;

-- name: UpdateRetentionScheduleStatus :one
UPDATE gov.retention_schedules
SET status = $3, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, retention_rule_id, asset_id, next_due_at, status, row_version, created_at, updated_at;

-- name: ListDueSchedulesWithoutOpenJob :many
-- DPX-05's acceptance criterion: a schedule past its due date with no disposal job still open
-- (pending/approved/in_progress/on_hold) is due for a fresh one. Runs inside the per-tenant
-- transaction the daily gov.retention_check job already opens (RLS scopes it to this tenant).
SELECT rs.id, rs.retention_rule_id, rs.asset_id, rs.next_due_at, rs.status, rs.row_version, rs.created_at, rs.updated_at
FROM gov.retention_schedules rs
WHERE rs.status = 'active'
  AND rs.next_due_at <= sqlc.arg(today)::date
  AND NOT EXISTS (
      SELECT 1 FROM gov.disposal_jobs dj
      WHERE dj.schedule_id = rs.id AND dj.status IN ('pending', 'approved', 'in_progress', 'on_hold')
  );

-- name: ListDisposalJobs :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at
FROM gov.disposal_jobs
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetDisposalJob :one
SELECT id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at
FROM gov.disposal_jobs
WHERE id = $1;

-- name: InsertDisposalJob :one
INSERT INTO gov.disposal_jobs (id, tenant_id, schedule_id, due_at, method, legal_hold_id, status, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;

-- name: ApproveDisposalJob :one
UPDATE gov.disposal_jobs
SET status = 'approved', approved_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    assignee_user_id = COALESCE(sqlc.narg(assignee_user_id)::uuid, assignee_user_id),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'pending'
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;

-- name: StartDisposalJob :one
UPDATE gov.disposal_jobs
SET status = 'in_progress', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'approved'
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;

-- name: CompleteDisposalJob :one
-- DPX-05's own gate: only a job carrying an evidence_file_id can ever reach 'done' (see sqlc.arg(evidence_file_id)
-- required, never sqlc.narg) — the service layer refuses the call before this if no file was attached.
UPDATE gov.disposal_jobs
SET status = 'done', completed_at = now(), evidence_file_id = sqlc.arg(evidence_file_id)::uuid,
    record_count = sqlc.narg(record_count)::int,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'in_progress'
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;

-- name: HoldDisposalJob :one
UPDATE gov.disposal_jobs
SET status = 'on_hold', legal_hold_id = $3,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status IN ('pending', 'approved', 'in_progress')
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;

-- name: ReleaseDisposalJobHold :one
UPDATE gov.disposal_jobs
SET status = 'pending', legal_hold_id = NULL,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'on_hold'
RETURNING id, schedule_id, due_at, method, legal_hold_id, assignee_user_id, approved_by, status, record_count,
    completed_at, evidence_file_id, row_version, created_at, updated_at;
