-- PLT-05 workflow & SLA engine.

-- name: ListDefinitions :many
-- The newest active version of each code the tenant can use; the tenant's own before a global one.
SELECT DISTINCT ON (code) id, tenant_id, code, name, entity_type, version_no, definition, is_active, row_version, updated_at
FROM platform.workflow_definitions
WHERE is_active
ORDER BY code, (tenant_id IS NULL), version_no DESC;

-- name: GetDefinition :one
SELECT id, tenant_id, code, name, entity_type, version_no, definition, is_active, row_version, updated_at
FROM platform.workflow_definitions WHERE id = $1;

-- name: ActiveDefinitionByCode :one
SELECT id, tenant_id, code, name, entity_type, version_no, definition, is_active, row_version, updated_at
FROM platform.workflow_definitions
WHERE code = $1 AND is_active
ORDER BY (tenant_id IS NULL), version_no DESC
LIMIT 1;

-- name: MaxOwnDefinitionVersion :one
SELECT COALESCE(max(version_no), 0)::int FROM platform.workflow_definitions
WHERE code = $1 AND tenant_id = current_setting('app.tenant_id')::uuid;

-- name: DeactivateOwnDefinitions :exec
UPDATE platform.workflow_definitions SET is_active = false, row_version = row_version + 1
WHERE code = $1 AND tenant_id = current_setting('app.tenant_id')::uuid AND is_active;

-- name: InsertDefinition :one
INSERT INTO platform.workflow_definitions (id, tenant_id, code, name, entity_type, version_no, definition, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, code, name, entity_type, version_no, definition, is_active, row_version, updated_at;

-- name: InsertInstance :one
INSERT INTO platform.workflow_instances (id, tenant_id, definition_id, entity_type, entity_id, current_state, started_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, definition_id, entity_type, entity_id, current_state, started_at, completed_at, sla_status, row_version, updated_at;

-- name: GetInstance :one
SELECT id, definition_id, entity_type, entity_id, current_state, started_at, completed_at, sla_status, row_version, updated_at
FROM platform.workflow_instances WHERE id = $1;

-- name: LockInstance :one
SELECT id, definition_id, entity_type, entity_id, current_state, started_at, completed_at, sla_status, row_version, updated_at
FROM platform.workflow_instances WHERE id = $1 FOR UPDATE;

-- name: ListInstancesForEntity :many
SELECT id, definition_id, entity_type, entity_id, current_state, started_at, completed_at, sla_status, row_version, updated_at
FROM platform.workflow_instances WHERE entity_type = $1 AND entity_id = $2
ORDER BY started_at DESC;

-- name: UpdateInstance :one
UPDATE platform.workflow_instances
SET current_state = $3, completed_at = $4, sla_status = $5, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, definition_id, entity_type, entity_id, current_state, started_at, completed_at, sla_status, row_version, updated_at;

-- name: SetInstanceSLAStatus :exec
UPDATE platform.workflow_instances SET sla_status = $2, row_version = row_version + 1 WHERE id = $1;

-- name: InsertTask :one
INSERT INTO platform.workflow_tasks (id, tenant_id, instance_id, state, title, assignee_user_id, assignee_group_id, due_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, instance_id, state, title, assignee_user_id, assignee_group_id, status, due_at, completed_at, outcome, comment, row_version, created_at, updated_at;

-- name: ListTasks :many
SELECT id, instance_id, state, title, assignee_user_id, assignee_group_id, status, due_at, completed_at, outcome, comment, row_version, created_at, updated_at
FROM platform.workflow_tasks WHERE instance_id = $1
ORDER BY created_at, id;

-- name: GetTask :one
SELECT id, instance_id, state, title, assignee_user_id, assignee_group_id, status, due_at, completed_at, outcome, comment, row_version, created_at, updated_at
FROM platform.workflow_tasks WHERE id = $1;

-- name: UpdateTask :one
UPDATE platform.workflow_tasks
SET assignee_user_id = $3, assignee_group_id = $4, status = $5, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, instance_id, state, title, assignee_user_id, assignee_group_id, status, due_at, completed_at, outcome, comment, row_version, created_at, updated_at;

-- name: CloseOpenTasks :many
UPDATE platform.workflow_tasks
SET status = @status, outcome = @outcome, comment = @comment, completed_at = @completed_at, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE instance_id = @instance_id AND status IN ('open', 'in_progress')
RETURNING id;

-- name: MyTasks :many
-- Open work of a user: assigned to them, or to one of their groups and not yet claimed.
SELECT t.id, t.instance_id, t.state, t.title, t.assignee_user_id, t.assignee_group_id, t.status, t.due_at, t.row_version, t.created_at,
       i.definition_id, i.entity_type, i.entity_id, i.current_state, i.sla_status, d.name AS workflow_name, d.code AS workflow_code,
       (SELECT min(s.due_at) FROM platform.sla_timers s WHERE s.instance_id = i.id AND s.stopped_at IS NULL)::timestamptz AS sla_due_at
FROM platform.workflow_tasks t
JOIN platform.workflow_instances i ON i.id = t.instance_id
JOIN platform.workflow_definitions d ON d.id = i.definition_id
WHERE t.status IN ('open', 'in_progress')
  AND (t.assignee_user_id = @user_id OR (t.assignee_user_id IS NULL AND t.assignee_group_id = ANY (@group_ids::uuid[])))
ORDER BY COALESCE(t.due_at, (SELECT min(s.due_at) FROM platform.sla_timers s WHERE s.instance_id = i.id AND s.stopped_at IS NULL)) NULLS LAST, t.created_at
LIMIT 200;

-- name: InsertTimer :one
INSERT INTO platform.sla_timers (id, tenant_id, instance_id, code, mode, calendar_id, started_at, due_at, reminders, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, instance_id, code, mode, calendar_id, started_at, due_at, reminders, escalated_at, stopped_at, paused_at, status, row_version;

-- name: ListTimers :many
SELECT id, instance_id, code, mode, calendar_id, started_at, due_at, reminders, escalated_at, stopped_at, paused_at, status, row_version
FROM platform.sla_timers WHERE instance_id = $1 ORDER BY started_at, id;

-- name: LockTimer :one
SELECT id, instance_id, code, mode, calendar_id, started_at, due_at, reminders, escalated_at, stopped_at, paused_at, status, row_version
FROM platform.sla_timers WHERE id = $1 FOR UPDATE;

-- name: UpdateTimer :exec
UPDATE platform.sla_timers
SET due_at = $2, reminders = $3, escalated_at = $4, stopped_at = $5, paused_at = $6, status = $7, row_version = row_version + 1
WHERE id = $1;

-- name: InstanceHistory :many
-- The instance's audit trail, oldest first (transitions, task changes, SLA events).
SELECT id, occurred_at, actor_type, actor_id, action, before, after
FROM platform.audit_log
WHERE entity_type = 'workflow_instance' AND entity_id = $1
ORDER BY occurred_at, id
LIMIT 500;
