-- DSAR-08 workflow & subtasks (dsar.subtasks — already fully specified in the baseline migrations).

-- name: InsertSubtask :one
INSERT INTO dsar.subtasks (id, tenant_id, request_id, action, assignee_user_id, assignee_group_id, due_at,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @request_id, @action, @assignee_user_id, @assignee_group_id,
    @due_at, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_id, action, assignee_user_id, assignee_group_id, status, due_at, completed_at,
    evidence_file_id, created_at, updated_at, row_version;

-- name: GetSubtask :one
SELECT id, request_id, action, assignee_user_id, assignee_group_id, status, due_at, completed_at,
    evidence_file_id, created_at, updated_at, row_version
FROM dsar.subtasks
WHERE id = $1;

-- name: ListSubtasksForRequest :many
SELECT id, request_id, action, assignee_user_id, assignee_group_id, status, due_at, completed_at,
    evidence_file_id, created_at, updated_at, row_version
FROM dsar.subtasks
WHERE request_id = $1
ORDER BY created_at;

-- name: CountOpenSubtasksForRequest :one
-- DSAR-08's acceptance criterion: a request is closable once every subtask is done (or not_applicable) — open
-- or in_progress subtasks block it. Zero subtasks is vacuously "all done".
SELECT count(*) FROM dsar.subtasks WHERE request_id = $1 AND status IN ('open', 'in_progress');

-- name: UpdateSubtaskStatus :one
UPDATE dsar.subtasks
SET status = @status, completed_at = CASE WHEN @status::text IN ('done', 'not_applicable') THEN now() ELSE NULL END,
    evidence_file_id = coalesce(sqlc.narg(evidence_file_id), evidence_file_id),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, request_id, action, assignee_user_id, assignee_group_id, status, due_at, completed_at,
    evidence_file_id, created_at, updated_at, row_version;

-- name: DeleteSubtask :execrows
DELETE FROM dsar.subtasks WHERE id = $1;
