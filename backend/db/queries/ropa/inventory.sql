-- name: ListDataInventory :many
-- Newest first; keyset cursor on (created_at, id). is_sensitive/category names come from org.data_categories
-- (global defaults or the tenant's own — its own RLS policy already allows both in this transaction).
SELECT i.id, i.asset_id, i.data_category_id, i.org_unit_id, i.owner_user_id, i.source, i.location_detail,
    i.row_version, i.created_at, i.updated_at, c.is_sensitive, c.sensitive_type, c.name_th AS category_name_th,
    c.name_en AS category_name_en
FROM ropa.data_inventory i
JOIN org.data_categories c ON c.id = i.data_category_id
WHERE (sqlc.narg(asset_id)::uuid IS NULL OR i.asset_id = sqlc.narg(asset_id))
  AND (sqlc.narg(org_unit_id)::uuid IS NULL OR i.org_unit_id = sqlc.narg(org_unit_id))
  AND (sqlc.narg(data_category_id)::uuid IS NULL OR i.data_category_id = sqlc.narg(data_category_id))
  AND (NOT @sensitive_only::bool OR c.is_sensitive)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (i.created_at, i.id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY i.created_at DESC, i.id DESC
LIMIT @lim;

-- name: GetDataInventory :one
SELECT i.id, i.asset_id, i.data_category_id, i.org_unit_id, i.owner_user_id, i.source, i.location_detail,
    i.row_version, i.created_at, i.updated_at, c.is_sensitive, c.sensitive_type, c.name_th AS category_name_th,
    c.name_en AS category_name_en
FROM ropa.data_inventory i
JOIN org.data_categories c ON c.id = i.data_category_id
WHERE i.id = $1;

-- name: InsertDataInventory :one
INSERT INTO ropa.data_inventory (id, tenant_id, asset_id, data_category_id, org_unit_id, owner_user_id, source,
    location_detail, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, asset_id, data_category_id, org_unit_id, owner_user_id, source, location_detail, row_version, created_at, updated_at;

-- name: UpdateDataInventory :one
UPDATE ropa.data_inventory
SET asset_id = $3, data_category_id = $4, org_unit_id = $5, owner_user_id = $6, source = $7, location_detail = $8,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, asset_id, data_category_id, org_unit_id, owner_user_id, source, location_detail, row_version, created_at, updated_at;
