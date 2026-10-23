-- VEN-14 vendor offboarding (vendor.offboardings — already fully specified in the baseline migrations).
-- One active offboarding row per vendor: TransitionVendorToOffboarding only succeeds from approved/conditional
-- (never while one is already in progress), so InsertOffboarding is only ever called right after that.

-- name: InsertOffboarding :one
INSERT INTO vendor.offboardings (id, tenant_id, vendor_id, data_return_status, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, 'pending',
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, data_return_status, destruction_certificate_file_id, access_revoked_at, completed_at, row_version, created_at;

-- name: GetOffboardingForVendor :one
-- The vendor's own most recent offboarding row — the one TransitionVendorToOffboarding just created, or
-- (once terminated) the one that finished it.
SELECT id, vendor_id, data_return_status, destruction_certificate_file_id, access_revoked_at, completed_at, row_version, created_at
FROM vendor.offboardings
WHERE vendor_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetOffboarding :one
SELECT id, vendor_id, data_return_status, destruction_certificate_file_id, access_revoked_at, completed_at, row_version, created_at
FROM vendor.offboardings
WHERE id = $1;

-- name: UpdateOffboarding :one
UPDATE vendor.offboardings
SET data_return_status = $3, destruction_certificate_file_id = $4, access_revoked_at = $5,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, vendor_id, data_return_status, destruction_certificate_file_id, access_revoked_at, completed_at, row_version, created_at;

-- name: CompleteOffboarding :one
UPDATE vendor.offboardings
SET completed_at = now(), updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1
RETURNING id, vendor_id, data_return_status, destruction_certificate_file_id, access_revoked_at, completed_at, row_version, created_at;

-- name: TransitionVendorToOffboarding :one
-- ST-06#1's "approved|conditional -> offboarding" edge ("ยุติการใช้บริการ").
UPDATE vendor.vendors
SET status = 'offboarding',
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status IN ('approved', 'conditional')
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;

-- name: TransitionVendorToTerminated :one
-- ST-06#1's "offboarding -> terminated" edge (VEN-14's own acceptance criterion: only once data return and
-- access revocation are both confirmed — checked in Go before this query ever runs).
UPDATE vendor.vendors
SET status = 'terminated', offboarded_at = now(),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'offboarding'
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;
