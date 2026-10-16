-- BRE-15: root cause & lessons learned. breach.root_causes was already fully specified in the baseline
-- migrations (cause_category, description, corrective_actions jsonb array, risk_id) — no migration needed.

-- name: InsertRootCause :one
INSERT INTO breach.root_causes (id, tenant_id, incident_id, cause_category, description, corrective_actions,
    risk_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @cause_category, @description, @corrective_actions,
    @risk_id, NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, incident_id, cause_category, description, corrective_actions, risk_id, row_version, created_at, updated_at;

-- name: GetRootCause :one
SELECT id, incident_id, cause_category, description, corrective_actions, risk_id, row_version, created_at, updated_at
FROM breach.root_causes WHERE id = @id;

-- name: ListRootCausesByIncident :many
SELECT id, incident_id, cause_category, description, corrective_actions, risk_id, row_version, created_at, updated_at
FROM breach.root_causes WHERE incident_id = @incident_id ORDER BY created_at;

-- name: UpdateRootCause :one
UPDATE breach.root_causes
SET cause_category = @cause_category, description = @description, corrective_actions = @corrective_actions,
    risk_id = @risk_id, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, incident_id, cause_category, description, corrective_actions, risk_id, row_version, created_at, updated_at;

-- name: DeleteRootCause :execrows
DELETE FROM breach.root_causes WHERE id = @id AND incident_id = @incident_id;
