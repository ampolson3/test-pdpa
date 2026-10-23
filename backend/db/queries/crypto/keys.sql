-- name: GetActiveKey :one
SELECT id, version, wrapped_key, kek_ref
FROM platform.tenant_keys
WHERE purpose = @purpose AND data_class = @data_class AND status = 'active';

-- name: GetKeyVersion :one
SELECT id, version, wrapped_key, kek_ref
FROM platform.tenant_keys
WHERE purpose = @purpose AND data_class = @data_class AND version = @version;

-- name: InsertKey :execrows
-- Losing a race to create the first key of a class is fine: DO NOTHING, then read the winner's key.
INSERT INTO platform.tenant_keys (tenant_id, purpose, data_class, version, wrapped_key, kek_ref, created_by)
VALUES (current_setting('app.tenant_id')::uuid, @purpose, @data_class, @version, @wrapped_key, @kek_ref,
        NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT DO NOTHING;

-- name: RetireActiveKey :one
-- Locks and retires the active key of a class so a new version can become active; returns its version.
UPDATE platform.tenant_keys SET status = 'retired', row_version = row_version + 1
WHERE purpose = @purpose AND data_class = @data_class AND status = 'active'
RETURNING version;

-- name: ListKeysForRewrap :many
SELECT id, wrapped_key, kek_ref FROM platform.tenant_keys ORDER BY id FOR UPDATE;

-- name: UpdateWrappedKey :exec
UPDATE platform.tenant_keys SET wrapped_key = @wrapped_key, kek_ref = @kek_ref, row_version = row_version + 1
WHERE id = @id;
