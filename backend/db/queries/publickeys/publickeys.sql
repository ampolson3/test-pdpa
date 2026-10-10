-- PLT-02 / D-10: keys that let /public/v1 find the tenant before any tenant transaction exists.

-- name: ResolvePublicKey :one
-- Readable by every request (RLS policy public_read); only active keys resolve.
SELECT key, tenant_id, entity_type, entity_id, allowed_origins
FROM platform.public_keys WHERE key = $1 AND status = 'active';

-- name: InsertPublicKey :exec
INSERT INTO platform.public_keys (key, tenant_id, entity_type, entity_id, allowed_origins)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4);

-- name: RevokePublicKey :execrows
UPDATE platform.public_keys SET status = 'revoked', revoked_at = now() WHERE key = $1 AND status = 'active';

-- name: SetPublicKeyOrigins :execrows
UPDATE platform.public_keys SET allowed_origins = $2 WHERE key = $1;
