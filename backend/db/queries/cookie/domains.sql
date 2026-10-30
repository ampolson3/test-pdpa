-- CON-01: cookie.domains — the website a banner is served on. verification_token is generated on insert;
-- VerifyDomain is the manual admin confirmation this pass uses (no DNS/meta-tag automation — see the
-- module doc's own implementation note).

-- name: InsertDomain :one
INSERT INTO cookie.domains (id, tenant_id, domain, legal_entity_id, verification_token, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain, legal_entity_id, verification_token, verified_at, status, row_version, updated_at;

-- name: GetDomain :one
SELECT id, domain, legal_entity_id, verification_token, verified_at, status, row_version, updated_at
FROM cookie.domains WHERE id = $1;

-- name: LockDomain :one
SELECT id, domain, legal_entity_id, verification_token, verified_at, status, row_version, updated_at
FROM cookie.domains WHERE id = $1 FOR UPDATE;

-- name: ListDomains :many
SELECT id, domain, legal_entity_id, verification_token, verified_at, status, row_version, updated_at
FROM cookie.domains
WHERE (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (updated_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @lim;

-- name: VerifyDomain :execrows
UPDATE cookie.domains SET status = 'active', verified_at = now(), row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2;
