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

-- DPO-08 (compliance calendar): a read-only window query, distinct from ListActivitiesDueForReview's own
-- "already due" filter used by the daily sweep — this one is for a forward-looking calendar view.
-- name: ListActivitiesWithReviewWindow :many
SELECT id, name, code, next_review_at
FROM ropa.processing_activities
WHERE status = 'active' AND next_review_at IS NOT NULL AND next_review_at BETWEEN $1 AND $2
ORDER BY next_review_at;
