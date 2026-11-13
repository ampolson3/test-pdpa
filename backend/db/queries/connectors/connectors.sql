-- name: ListConnectors :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, connector_type, name, asset_id, config, capabilities, status, last_tested_at, row_version,
    created_at, updated_at
FROM platform.connectors
WHERE (sqlc.narg(connector_type)::text IS NULL OR connector_type = sqlc.narg(connector_type))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetConnector :one
SELECT id, connector_type, name, asset_id, config, secret_ref, capabilities, status, last_tested_at,
    row_version, created_at, updated_at
FROM platform.connectors
WHERE id = $1;

-- name: InsertConnector :one
INSERT INTO platform.connectors (id, tenant_id, connector_type, name, asset_id, config, secret_ref,
    capabilities, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, connector_type, name, asset_id, config, secret_ref, capabilities, status, last_tested_at,
    row_version, created_at, updated_at;

-- name: UpdateConnector :one
-- secret_ref is only overwritten when sqlc.narg(set_secret_ref) is true, so a PATCH that omits `secret`
-- (meaning "leave it unchanged") never clobbers an already-sealed reference with NULL.
UPDATE platform.connectors
SET name = $3, asset_id = $4, config = $5,
    secret_ref = CASE WHEN sqlc.arg(set_secret_ref)::boolean THEN sqlc.narg(secret_ref) ELSE secret_ref END,
    capabilities = $6,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, connector_type, name, asset_id, config, secret_ref, capabilities, status, last_tested_at,
    row_version, created_at, updated_at;

-- name: SetConnectorTestResult :one
UPDATE platform.connectors
SET status = $2, last_tested_at = now(), updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1
RETURNING id, connector_type, name, asset_id, config, secret_ref, capabilities, status, last_tested_at,
    row_version, created_at, updated_at;

-- name: DeleteConnector :execrows
DELETE FROM platform.connectors WHERE id = $1 AND row_version = $2;
