-- CON-04: cookie.scans — one round of the automated scanner (manual/schedule/publish triggered).

-- name: InsertScan :one
INSERT INTO cookie.scans (id, tenant_id, domain_id, trigger, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain_id, trigger, status, started_at, finished_at, pages_scanned, cookies_found, new_cookies, row_version, created_at, updated_at;

-- name: GetScan :one
SELECT id, domain_id, trigger, status, started_at, finished_at, pages_scanned, cookies_found, new_cookies, row_version, created_at, updated_at
FROM cookie.scans WHERE id = $1;

-- name: MarkScanRunning :execrows
UPDATE cookie.scans SET status = 'running', started_at = now(), row_version = row_version + 1, updated_at = now()
WHERE id = $1 AND status = 'queued';

-- name: MarkScanDone :execrows
UPDATE cookie.scans SET status = 'done', finished_at = now(), pages_scanned = $2, cookies_found = $3, new_cookies = $4,
    row_version = row_version + 1, updated_at = now()
WHERE id = $1;

-- name: MarkScanFailed :execrows
UPDATE cookie.scans SET status = 'failed', finished_at = now(), row_version = row_version + 1, updated_at = now()
WHERE id = $1;

-- name: ListScansByDomain :many
SELECT id, domain_id, trigger, status, started_at, finished_at, pages_scanned, cookies_found, new_cookies, row_version, created_at, updated_at
FROM cookie.scans
WHERE domain_id = $1
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: LastDoneScanByDomain :one
SELECT id, finished_at FROM cookie.scans
WHERE domain_id = $1 AND status = 'done'
ORDER BY finished_at DESC LIMIT 1;

-- name: ListDomainsWithSchedule :many
-- Every domain of the current tenant that has a scan_schedule set — cookie.scan_schedule_check (a
-- tenant-scoped job, one per tenant, enqueued by the platform-wide cookie.scan_schedule_sweep) reads this
-- inside its own tenant tx, the same two-step "sweep lists tenants, per-tenant job reads its own rows"
-- pattern PLT-11's outbox.sweep already established (CLAUDE.md rule 1 — no cross-tenant read here).
SELECT id, scan_schedule FROM cookie.domains WHERE scan_schedule IS NOT NULL AND status = 'active';
