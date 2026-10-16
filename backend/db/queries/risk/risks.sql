-- DPIA-06 (ระบุและให้คะแนนความเสี่ยง): a tenant-wide risk register entry (risk.risks), scored against the
-- tenant's own matrix (RRA-02). source_type/source_id name whichever feature identified it — "dpia" for
-- DPIA-06, with source_id the assess.assessments round.

-- name: InsertRisk :one
INSERT INTO risk.risks (id, tenant_id, source_type, source_id, title, description, owner_user_id, activity_id,
    likelihood, impact, inherent_score, level, status, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'open',
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetRisk :one
SELECT * FROM risk.risks WHERE id = $1;

-- name: ListRisksByIDs :many
SELECT * FROM risk.risks WHERE id = ANY (sqlc.arg(ids)::uuid[]) ORDER BY created_at;

-- name: UpdateRisk :one
UPDATE risk.risks SET
    title = $2, description = $3, owner_user_id = $4, likelihood = $5, impact = $6, inherent_score = $7,
    level = $8, treatment = $9, status = $10, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $11
RETURNING *;

-- DPIA-07: the residual likelihood/impact/score/level — recomputed (service-side, via Classify) every time
-- the risk's linked controls change, so it's never left stale after a mitigation is added or removed.
-- name: SetResidualRisk :one
UPDATE risk.risks SET residual_likelihood = $2, residual_impact = $3, residual_score = $4, residual_level = $5, updated_at = now()
WHERE id = $1
RETURNING *;
