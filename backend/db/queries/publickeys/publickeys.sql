-- PLT-02 / D-10: keys that let /public/v1 find the tenant before any tenant transaction exists.

-- name: ResolvePublicKey :one
-- Readable by every request (RLS policy public_read); only active keys resolve.
SELECT key, tenant_id, entity_type, entity_id, allowed_origins
FROM platform.public_keys WHERE key = $1 AND status = 'active';

-- name: GetPublicKeyByEntity :one
-- For an entity type that has no public_key column of its own (e.g. cookie.domains, CON-01) — the admin UI's
-- own read of an already-issued key, not the /public/v1 resolve path above. tenant_id filtered here too for
-- defense in depth, though the public_read RLS policy on this table already allows the plain SELECT.
SELECT key FROM platform.public_keys
WHERE entity_type = $1 AND entity_id = $2 AND status = 'active'
  AND tenant_id = current_setting('app.tenant_id')::uuid;

-- name: InsertPublicKey :exec
INSERT INTO platform.public_keys (key, tenant_id, entity_type, entity_id, allowed_origins)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4);

-- name: RevokePublicKey :execrows
UPDATE platform.public_keys SET status = 'revoked', revoked_at = now() WHERE key = $1 AND status = 'active';

-- name: SetPublicKeyOrigins :execrows
UPDATE platform.public_keys SET allowed_origins = $2 WHERE key = $1;
