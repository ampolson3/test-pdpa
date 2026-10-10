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
