-- CON-02: cookie.cookies — the individual cookies catalogued under a category ("รายการคุกกี้ต่อหมวด"),
-- manually entered in this pass (source = 'manual'; 'scan'/'kb' are CON-04's own automated scanner path,
-- unused here).

-- name: InsertCookie :one
INSERT INTO cookie.cookies (id, tenant_id, domain_id, name, provider_domain, category_id, storage_type, duration, descriptions, source, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, 'manual',
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain_id, name, provider_domain, category_id, storage_type, duration, descriptions, is_active, row_version, updated_at;

-- name: ListCookiesByDomain :many
SELECT id, domain_id, name, provider_domain, category_id, storage_type, duration, descriptions, is_active, row_version, updated_at
FROM cookie.cookies
WHERE domain_id = $1
ORDER BY name;

-- name: DeleteCookie :execrows
DELETE FROM cookie.cookies WHERE id = $1;
