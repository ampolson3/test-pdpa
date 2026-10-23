-- ROPA-15 (periodic review reminder): the same "sweep lists tenants, per-tenant job reads its own rows"
-- pattern CON-04's own cookie.scan_schedule_sweep / PNG-15's own notice.review_sweep already established
-- (CLAUDE.md rule 1). platform.tenants has no RLS and is already readable by pdpa_app.

-- name: ListActiveTenants :many
SELECT id FROM platform.tenants WHERE status IN ('trial', 'active') ORDER BY id;

-- name: GetActivityReviewCycle :one
SELECT review_cycle_months FROM ropa.processing_activities WHERE id = $1;

-- name: SetActivityNextReview :exec
UPDATE ropa.processing_activities SET next_review_at = $2 WHERE id = $1;

-- name: ListActivitiesDueForReview :many
SELECT id, name, code, owner_user_id, review_cycle_months, next_review_at
FROM ropa.processing_activities
WHERE status = 'active' AND next_review_at IS NOT NULL AND next_review_at <= $1
ORDER BY next_review_at;
