-- DPIA-06 (ระบุและให้คะแนนความเสี่ยง): a tenant-wide risk register entry (risk.risks), scored against the
-- tenant's own matrix (RRA-02). source_type/source_id name whichever feature identified it — "dpia" for
-- DPIA-06, with source_id the assess.assessments round.

-- name: InsertRisk :one
-- RRA-09: asset_id/vendor_id (ม.37(1): "ผูกกิจกรรม / ระบบ / คู่ค้า") are set only at creation, exactly like
-- activity_id already was — the register's own edit path never moves a risk's link afterward.
INSERT INTO risk.risks (id, tenant_id, source_type, source_id, title, description, owner_user_id, activity_id,
    asset_id, vendor_id, likelihood, impact, inherent_score, level, status, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
        'open', NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetRisk :one
SELECT * FROM risk.risks WHERE id = $1;

-- name: ListRisksByIDs :many
SELECT * FROM risk.risks WHERE id = ANY (sqlc.arg(ids)::uuid[]) ORDER BY created_at;

-- RRA-09: the shared privacy risk register — every risk.risks row regardless of source_type (dpia/
-- activity/vendor/breach/audit/manual), newest first, keyset cursor on (created_at, id).
-- name: ListRisks :many
SELECT * FROM risk.risks
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(source_type)::text IS NULL OR source_type = sqlc.narg(source_type))
  AND (sqlc.narg(owner_user_id)::uuid IS NULL OR owner_user_id = sqlc.narg(owner_user_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: UpdateRisk :one
-- Deliberately never touches activity_id/asset_id/vendor_id (set once at InsertRisk time) — DPIA-06's own
-- UpdateRisk caller passes no such fields at all, so widening this SET clause to include them would silently
-- clear a link every DPIA-sourced risk already has the moment any later DPIA edit ran.
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

-- RRA-08: flips the risk itself into/out of "accepted" — RecordAcceptance (status='accepted') and the
-- expiry reminder's own revert (status='open') both go through this one setter, guarded by row_version so
-- a concurrent edit is never silently clobbered.
-- name: SetRiskStatus :one
UPDATE risk.risks SET status = $2, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $3
RETURNING *;
