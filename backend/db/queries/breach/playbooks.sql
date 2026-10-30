-- BRE-11: response playbooks & tasks. breach.playbooks (global tenant_id IS NULL rows readable by every
-- tenant, same ORG-07-style pattern; writes always tenant-scoped) and breach.response_tasks were already
-- fully specified in the baseline migrations — no migration needed.

-- name: InsertPlaybook :one
INSERT INTO breach.playbooks (id, tenant_id, incident_type, name, steps, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_type, @name, @steps,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, incident_type, name, steps, version_no, row_version, created_at, updated_at;

-- name: GetPlaybook :one
SELECT id, incident_type, name, steps, version_no, row_version, created_at, updated_at
FROM breach.playbooks WHERE id = @id;

-- name: ListPlaybooks :many
-- Every playbook visible to this tenant (global defaults + its own), optionally narrowed to one incident_type.
SELECT id, incident_type, name, steps, version_no, row_version, created_at, updated_at
FROM breach.playbooks
WHERE (sqlc.narg(incident_type)::varchar IS NULL OR incident_type = sqlc.narg(incident_type))
ORDER BY name;

-- name: UpdatePlaybook :one
UPDATE breach.playbooks
SET name = @name, steps = @steps, version_no = version_no + 1, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, incident_type, name, steps, version_no, row_version, created_at, updated_at;

-- name: InsertResponseTask :one
INSERT INTO breach.response_tasks (id, tenant_id, incident_id, playbook_id, step_code, title, assignee_user_id,
    assignee_group_id, due_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @playbook_id, @step_code, @title,
    sqlc.narg(assignee_user_id)::uuid, sqlc.narg(assignee_group_id)::uuid, sqlc.narg(due_at)::timestamptz,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, incident_id, playbook_id, step_code, title, assignee_user_id, assignee_group_id, status, due_at,
    completed_at, row_version, created_at, updated_at;

-- name: GetResponseTask :one
SELECT id, incident_id, playbook_id, step_code, title, assignee_user_id, assignee_group_id, status, due_at,
    completed_at, row_version, created_at, updated_at
FROM breach.response_tasks WHERE id = @id;

-- name: ListResponseTasksByIncident :many
SELECT id, incident_id, playbook_id, step_code, title, assignee_user_id, assignee_group_id, status, due_at,
    completed_at, row_version, created_at, updated_at
FROM breach.response_tasks WHERE incident_id = @incident_id ORDER BY created_at;

-- name: CountResponseTasksByIncidentAndPlaybook :one
-- Idempotency check for ApplyPlaybook: tasks from this exact playbook already exist for this incident.
SELECT count(*) FROM breach.response_tasks WHERE incident_id = @incident_id AND playbook_id = @playbook_id;

-- name: SetResponseTaskStatus :one
UPDATE breach.response_tasks
SET status = @status, completed_at = CASE WHEN @status::text IN ('done', 'cancelled') THEN now() ELSE NULL END,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, incident_id, playbook_id, step_code, title, assignee_user_id, assignee_group_id, status, due_at,
    completed_at, row_version, created_at, updated_at;

-- name: AssignResponseTask :one
UPDATE breach.response_tasks
SET assignee_user_id = sqlc.narg(assignee_user_id)::uuid, assignee_group_id = sqlc.narg(assignee_group_id)::uuid,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, incident_id, playbook_id, step_code, title, assignee_user_id, assignee_group_id, status, due_at,
    completed_at, row_version, created_at, updated_at;
