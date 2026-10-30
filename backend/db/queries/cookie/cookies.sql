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

-- CON-04: the scanner's own write path — source = 'scan', first_seen_scan_id records which round first
-- found it. GetCookieByDomainAndName is the idempotency check (no unique constraint on the table itself;
-- this feature's one caller, running inside a single scan's own job, is the only writer that could race).

-- name: GetCookieByDomainAndName :one
SELECT id, domain_id, name, provider_domain, category_id, storage_type, duration, descriptions, is_active, source, kb_id, first_seen_scan_id, row_version, updated_at
FROM cookie.cookies WHERE domain_id = $1 AND name = $2 LIMIT 1;

-- name: InsertCookieFromScan :one
-- storage_type is always 'http_cookie' here: the scanner observes cookies only through the browser's own
-- cookie jar (CDP Storage.getCookies), which doesn't distinguish a header-set cookie from a JS-set one —
-- both land in the same jar. Distinguishing localStorage/sessionStorage findings would need a different
-- column this table doesn't have yet (CON.md's own implementation note); add it once a screen needs it.
INSERT INTO cookie.cookies (id, tenant_id, domain_id, name, provider_domain, category_id, storage_type, source, kb_id, first_seen_scan_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, 'http_cookie', 'scan', $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain_id, name, provider_domain, category_id, storage_type, duration, descriptions, is_active, source, kb_id, first_seen_scan_id, row_version, updated_at;

-- name: SetCookieCategory :execrows
UPDATE cookie.cookies SET category_id = $2, row_version = row_version + 1, updated_at = now()
WHERE id = $1;
