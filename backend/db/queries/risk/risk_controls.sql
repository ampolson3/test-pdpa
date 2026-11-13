-- DPIA-07 (มาตรการลดความเสี่ยงและความเสี่ยงคงเหลือ): link a risk.risks row to one of risk.controls'
-- catalog entries, optionally with an owner/due date; the residual likelihood/impact on risk.risks itself
-- is recomputed (service-side) whenever this link set changes.

-- name: InsertRiskControl :one
INSERT INTO risk.risk_controls (risk_id, control_id, tenant_id, owner_user_id, due_at, task_id, created_by, updated_by)
VALUES ($1, $2, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ListRiskControls :many
SELECT rc.risk_id, rc.control_id, rc.status, rc.owner_user_id, rc.due_at, rc.task_id, rc.row_version, rc.created_at,
    c.code AS control_code, c.name AS control_name, c.category AS control_category
FROM risk.risk_controls rc
JOIN risk.controls c ON c.id = rc.control_id
WHERE rc.risk_id = $1
ORDER BY rc.created_at;

-- name: UpdateRiskControlStatus :one
UPDATE risk.risk_controls SET status = $3, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE risk_id = $1 AND control_id = $2 AND row_version = $4
RETURNING *;

-- name: DeleteRiskControl :execrows
DELETE FROM risk.risk_controls WHERE risk_id = $1 AND control_id = $2;

-- name: CountImplementedControls :one
SELECT count(*)::int FROM risk.risk_controls WHERE risk_id = $1 AND status = 'implemented';
