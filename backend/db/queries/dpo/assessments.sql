-- DPO-09 security-measures assessment.

-- name: InsertSecurityAssessment :one
INSERT INTO dpo.security_assessments (id, tenant_id, legal_entity_id, form_submission_id, score, result, factors,
    assessed_by, assessed_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @legal_entity_id, @form_submission_id, @score, @result, @factors,
    @assessed_by, @assessed_at, @assessed_by, @assessed_by)
RETURNING id, legal_entity_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version;

-- name: GetSecurityAssessment :one
SELECT id, legal_entity_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version
FROM dpo.security_assessments
WHERE id = $1;

-- name: ListSecurityAssessments :many
-- Newest first; keyset cursor on (assessed_at, id).
SELECT id, legal_entity_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version
FROM dpo.security_assessments
WHERE (sqlc.narg(legal_entity_id)::uuid IS NULL OR legal_entity_id = sqlc.narg(legal_entity_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (assessed_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY assessed_at DESC, id DESC
LIMIT @lim;

-- name: LockTaskNumbering :exec
SELECT pg_advisory_xact_lock(hashtext('dpo.task_no:' || current_setting('app.tenant_id') || ':' || @year::text));

-- name: CountTasksInYear :one
SELECT count(*)::int FROM dpo.tasks WHERE task_no LIKE @prefix::text || '%';

-- name: InsertTask :one
-- assignee_user_id/due_at are optional (DPIA-07's own "กำหนดผู้รับผิดชอบและวันเสร็จ → task"; every earlier
-- caller leaves both null, landing status 'created' as before) — status becomes 'assigned' only when an
-- assignee is actually given at creation.
INSERT INTO dpo.tasks (id, tenant_id, task_no, title, description, source_type, source_id, priority,
    assignee_user_id, due_at, status, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @task_no, @title, @description, @source_type, @source_id, @priority,
    sqlc.narg(assignee_user_id), sqlc.narg(due_at),
    CASE WHEN sqlc.narg(assignee_user_id)::uuid IS NOT NULL THEN 'assigned' ELSE 'created' END,
    @actor, @actor)
RETURNING id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at;

-- name: ListTasksBySource :many
SELECT id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at
FROM dpo.tasks
WHERE source_type = @source_type AND source_id = @source_id
ORDER BY created_at;
