-- VEN-07 vendor assessment cycles (vendor.vendor_assessments — already fully specified in the baseline
-- migrations, including the review/decision columns VEN-08 will write to).

-- name: NextVendorAssessmentCycle :one
SELECT COALESCE(MAX(cycle_no), 0)::int + 1 FROM vendor.vendor_assessments WHERE vendor_id = $1;

-- name: InsertVendorAssessment :one
INSERT INTO vendor.vendor_assessments (id, tenant_id, vendor_id, assessment_id, cycle_no, submitted_at, score,
    residual_level, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @vendor_id, @assessment_id, @cycle_no, now(), @score,
    @residual_level, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, assessment_id, cycle_no, sent_at, due_at, submitted_at, score, residual_level, decision,
    decided_by, decided_at, row_version, created_at;

-- name: GetVendorAssessment :one
SELECT id, vendor_id, assessment_id, cycle_no, sent_at, due_at, submitted_at, score, residual_level, decision,
    decided_by, decided_at, row_version, created_at
FROM vendor.vendor_assessments
WHERE id = $1;

-- name: ListVendorAssessments :many
-- Newest cycle first.
SELECT id, vendor_id, assessment_id, cycle_no, sent_at, due_at, submitted_at, score, residual_level, decision,
    decided_by, decided_at, row_version, created_at
FROM vendor.vendor_assessments
WHERE vendor_id = $1
ORDER BY cycle_no DESC;

-- name: DecideVendorAssessment :one
-- VEN-08: records the first (only) decision on a cycle. "WHERE decision IS NULL" makes re-deciding an
-- already-decided cycle a no-row update (service maps that to a 409), not a silent overwrite.
UPDATE vendor.vendor_assessments
SET decision = @decision, decided_by = NULLIF(current_setting('app.user_id', true), '')::uuid, decided_at = now(),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND decision IS NULL
RETURNING id, vendor_id, assessment_id, cycle_no, sent_at, due_at, submitted_at, score, residual_level, decision,
    decided_by, decided_at, row_version, created_at;

-- name: MarkVendorOnboarding :one
-- ST-06#1's "prospect -> onboarding" edge, triggered by a vendor's first-ever answered assessment cycle
-- (VEN-07). A vendor already past "prospect" (onboarding/approved/conditional/...) is a no-row update —
-- the service treats that as "nothing to do", not an error, so re-assessing an already-onboarded vendor
-- doesn't fail.
UPDATE vendor.vendors
SET status = 'onboarding', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND status = 'prospect'
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;

-- name: DecideVendorStatus :one
-- ST-06#1's "onboarding -> approved|conditional|rejected" edge (VEN-08's own decision = ... trigger).
-- approved_at is stamped only when the decision is a clean approval.
UPDATE vendor.vendors
SET status = @status, approved_at = CASE WHEN @status::text = 'approved' THEN now() ELSE approved_at END,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND status = 'onboarding'
RETURNING id, party_id, service_description, relationship_owner_id, is_processor, tier, data_access,
    processing_countries, status, next_assessment_at, approved_at, offboarded_at, row_version, created_at, updated_at;

-- name: InsertRemediationItem :one
-- VEN-08's own acceptance criterion: a conditional approval opens one or more of these automatically.
INSERT INTO vendor.remediation_items (id, tenant_id, vendor_id, vendor_assessment_id, title, severity, due_at,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @vendor_id, @vendor_assessment_id, @title, @severity, @due_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, vendor_assessment_id, title, severity, due_at, status, closed_at, evidence_file_id,
    row_version, created_at;

-- name: ListRemediationItemsForVendor :many
SELECT id, vendor_id, vendor_assessment_id, title, severity, due_at, status, closed_at, evidence_file_id,
    row_version, created_at
FROM vendor.remediation_items
WHERE vendor_id = $1
ORDER BY created_at DESC;
