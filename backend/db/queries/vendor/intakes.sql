-- VEN-02 vendor tiering (vendor.intakes — already fully specified in the baseline migrations).

-- name: InsertIntake :one
INSERT INTO vendor.intakes (id, tenant_id, vendor_id, form_submission_id, inherent_score, tier_result, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, form_submission_id, inherent_score, tier_result, row_version, created_at, updated_at;

-- name: GetLatestIntake :one
-- The most recent tiering round for a vendor.
SELECT id, vendor_id, form_submission_id, inherent_score, tier_result, row_version, created_at, updated_at
FROM vendor.intakes
WHERE vendor_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListIntakes :many
SELECT id, vendor_id, form_submission_id, inherent_score, tier_result, row_version, created_at, updated_at
FROM vendor.intakes
WHERE vendor_id = $1
ORDER BY created_at DESC;

-- name: UpdateVendorTier :one
UPDATE vendor.vendors
SET tier = $2, next_assessment_at = $3,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;
