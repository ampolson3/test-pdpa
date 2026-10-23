-- VEN-10 (ม.37(1)): vendor.vendors.next_assessment_at/tier were already set by VEN-02's own RecordIntake
-- (intake.go) — this is the sweep that acts on them once the date arrives, no new column or migration.

-- name: ListVendorsDueForReassessment :many
SELECT id, relationship_owner_id, tier, next_assessment_at
FROM vendor.vendors
WHERE status IN ('approved', 'conditional') AND tier IS NOT NULL
  AND next_assessment_at IS NOT NULL AND next_assessment_at <= $1
ORDER BY next_assessment_at;

-- name: SetVendorNextAssessment :exec
UPDATE vendor.vendors SET next_assessment_at = $2 WHERE id = $1;
