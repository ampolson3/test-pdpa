-- CON-04: cookie.scan_findings — every cookie observed on every page of one scan round.

-- name: InsertScanFinding :one
INSERT INTO cookie.scan_findings (id, tenant_id, scan_id, cookie_name, cookie_domain, page_url, set_before_consent,
    suggested_category, matched_cookie_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, scan_id, cookie_name, cookie_domain, page_url, set_before_consent, suggested_category, matched_cookie_id, row_version;

-- name: ListScanFindingsByScan :many
SELECT id, scan_id, cookie_name, cookie_domain, page_url, set_before_consent, suggested_category, matched_cookie_id, row_version
FROM cookie.scan_findings
WHERE scan_id = $1
ORDER BY cookie_name, page_url
OFFSET @offs LIMIT @lim;

-- name: CountDistinctCookieNamesForScan :one
SELECT count(DISTINCT cookie_name) FROM cookie.scan_findings WHERE scan_id = $1;

-- name: ListDistinctCookieNamesForScan :many
SELECT DISTINCT cookie_name, cookie_domain FROM cookie.scan_findings WHERE scan_id = $1;

-- name: GetScanFinding :one
SELECT id, scan_id, cookie_name, cookie_domain, page_url, set_before_consent, suggested_category, matched_cookie_id, row_version
FROM cookie.scan_findings WHERE id = $1;

-- name: SetScanFindingMatchedCookie :execrows
UPDATE cookie.scan_findings SET matched_cookie_id = $2, row_version = row_version + 1, updated_at = now()
WHERE id = $1;
