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
