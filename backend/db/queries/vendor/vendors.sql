-- VEN-01 vendor registry (vendor.vendors — already fully specified in the baseline migrations).

-- name: ListVendors :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at
FROM vendor.vendors
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ListActiveProcessorVendors :many
-- DPA-12: every vendor still in an active business relationship (approved/conditional, ST-06) marked as a
-- processor (ม.40) — the population the missing-DPA sweep checks. No pagination: a tenant's own active
-- vendor roster is bounded, the same "platform-wide/tenant-wide list, no cursor" reasoning DPO-05/RRA-09
-- already used for their own cross-record aggregations.
SELECT id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at
FROM vendor.vendors
WHERE is_processor = true AND status IN ('approved', 'conditional')
ORDER BY created_at;

-- name: GetVendor :one
SELECT id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at
FROM vendor.vendors
WHERE id = $1;

-- name: GetVendorByPartyID :one
SELECT id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at
FROM vendor.vendors
WHERE party_id = $1;

-- name: InsertVendor :one
INSERT INTO vendor.vendors (id, tenant_id, party_id, service_description, relationship_owner_id, is_processor,
    data_access, processing_countries, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;

-- name: UpdateVendor :one
-- VEN-01 never writes status/tier/next_assessment_at/approved_at/offboarded_at — those are ST-06's own
-- transitions, owned by sibling features (VEN-02/07/08/14) not built yet.
UPDATE vendor.vendors
SET party_id = $3, service_description = $4, relationship_owner_id = $5, is_processor = $6,
    data_access = $7, processing_countries = $8,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;
