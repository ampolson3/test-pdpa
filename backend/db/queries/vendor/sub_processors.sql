-- VEN-03 (sub-processor management — vendor.sub_processors, already fully specified in the baseline
-- migrations). Unapproved sub-processors are warned in the UI; a decision needs vendor.vendor.approve.

-- name: InsertSubProcessor :one
INSERT INTO vendor.sub_processors (id, tenant_id, vendor_id, party_id, service, countries, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, party_id, service, countries, status, approved_by, approved_at, row_version, created_at, updated_at;

-- name: ListSubProcessors :many
SELECT id, vendor_id, party_id, service, countries, status, approved_by, approved_at, row_version, created_at, updated_at
FROM vendor.sub_processors
WHERE vendor_id = $1
ORDER BY created_at DESC;

-- name: GetSubProcessor :one
SELECT id, vendor_id, party_id, service, countries, status, approved_by, approved_at, row_version, created_at, updated_at
FROM vendor.sub_processors
WHERE id = $1 AND vendor_id = $2;

-- name: DecideSubProcessor :one
UPDATE vendor.sub_processors
SET status = @status, approved_by = NULLIF(current_setting('app.user_id', true), '')::uuid, approved_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND vendor_id = @vendor_id AND row_version = @row_version AND status = 'pending'
RETURNING id, vendor_id, party_id, service, countries, status, approved_by, approved_at, row_version, created_at, updated_at;
