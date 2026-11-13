-- RRA-07: generic read/transition queries on dpo.tasks, on top of the InsertTask/ListTasksBySource
-- queries DPO-09/PNG-07/DPIA-07 already added in assessments.sql.

-- name: GetTask :one
SELECT id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at
FROM dpo.tasks
WHERE id = $1;

-- name: UpdateTaskStatus :one
UPDATE dpo.tasks
SET status = @status,
    completed_at = CASE WHEN @status::text IN ('done', 'closed') THEN now() ELSE completed_at END,
    updated_by = @actor,
    row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at;

-- DPO-06: the kanban/list view every source feature's own auto-created task already lands in, regardless of
-- source_type — RRA-09's own ListRisks' exact filter/keyset-cursor shape, ported to dpo.tasks.
-- name: ListTasks :many
SELECT id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at
FROM dpo.tasks
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(source_type)::text IS NULL OR source_type = sqlc.narg(source_type))
  AND (sqlc.narg(priority)::text IS NULL OR priority = sqlc.narg(priority))
  AND (sqlc.narg(assignee_user_id)::uuid IS NULL OR assignee_user_id = sqlc.narg(assignee_user_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- DPO-06: set/change who is assigned, who reviews, when it's due and how urgent it is on an already-created
-- task — every existing Open*Task caller only ever sets these once, at creation; this is the one place they
-- can be changed afterward. Never touches source_type/source_id/status — assignment is not a status move.
-- name: AssignTask :one
UPDATE dpo.tasks
SET assignee_user_id = sqlc.narg(assignee_user_id),
    reviewer_user_id = sqlc.narg(reviewer_user_id),
    org_unit_id = sqlc.narg(org_unit_id),
    due_at = sqlc.narg(due_at),
    priority = @priority,
    status = CASE WHEN status = 'created' AND sqlc.narg(assignee_user_id)::uuid IS NOT NULL THEN 'assigned' ELSE status END,
    updated_by = @actor,
    row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, task_no, title, description, source_type, source_id, status, priority, assignee_user_id, reviewer_user_id,
    org_unit_id, due_at, completed_at, row_version, created_at;
