-- name: ListAssets :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, name, asset_type, org_unit_id, owner_user_id, provider_party_id, hosting_country_code, hosting_type,
    classification, status, row_version, created_at, updated_at
FROM ropa.assets
WHERE (sqlc.narg(asset_type)::text IS NULL OR asset_type = sqlc.narg(asset_type))
  AND (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetAsset :one
SELECT id, name, asset_type, org_unit_id, owner_user_id, provider_party_id, hosting_country_code, hosting_type,
    classification, status, row_version, created_at, updated_at
FROM ropa.assets
WHERE id = $1;

-- name: InsertAsset :one
INSERT INTO ropa.assets (id, tenant_id, name, asset_type, org_unit_id, owner_user_id, provider_party_id,
    hosting_country_code, hosting_type, classification, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, name, asset_type, org_unit_id, owner_user_id, provider_party_id, hosting_country_code, hosting_type,
    classification, status, row_version, created_at, updated_at;

-- name: UpdateAsset :one
UPDATE ropa.assets
SET name = $3, asset_type = $4, org_unit_id = $5, owner_user_id = $6, provider_party_id = $7,
    hosting_country_code = $8, hosting_type = $9, classification = $10, status = $11,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, name, asset_type, org_unit_id, owner_user_id, provider_party_id, hosting_country_code, hosting_type,
    classification, status, row_version, created_at, updated_at;
