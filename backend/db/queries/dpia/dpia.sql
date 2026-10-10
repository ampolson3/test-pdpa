-- name: GetScreeningTemplate :one
-- The tenant's own override of a screening template (same code) wins over the global default (ORG-07/PNG-03
-- override pattern) — RLS already limits visible rows to NULL (global) or the caller's own tenant.
SELECT * FROM assess.templates
WHERE assessment_type = 'dpia' AND code = $1 AND status = 'published'
ORDER BY tenant_id NULLS LAST
LIMIT 1;

-- name: GetActiveScreeningRule :one
SELECT * FROM assess.screening_rules
WHERE is_active = true
ORDER BY created_at DESC
LIMIT 1;

-- name: DeactivateScreeningRules :exec
UPDATE assess.screening_rules SET is_active = false, updated_at = now() WHERE is_active = true;

-- name: InsertScreeningRule :one
INSERT INTO assess.screening_rules (id, tenant_id, criteria, min_factors, min_score, is_active, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, true,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetLatestAssessmentForActivity :one
SELECT * FROM assess.assessments
WHERE assessment_type = 'dpia' AND activity_id = $1
ORDER BY round_no DESC
LIMIT 1;

-- name: InsertAssessment :one
INSERT INTO assess.assessments (id, tenant_id, assessment_type, template_id, form_version_id, title, subject_type,
    subject_id, activity_id, round_no, previous_id, status, screening_result, screening_reason, score,
    created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: InsertAnswer :one
INSERT INTO assess.answers (id, tenant_id, assessment_id, question_code, answer, answered_by, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4,
        NULLIF(current_setting('app.user_id', true), '')::uuid,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ListAnswersForAssessment :many
-- section_id IS NULL: DPIA-01's own screening answers, never a later section's (e.g. DPIA-05's necessity
-- checklist) — Assessment.Factors must stay exactly the screening's own factors.
SELECT * FROM assess.answers WHERE assessment_id = $1 AND section_id IS NULL ORDER BY created_at;

-- name: GetAssessment :one
SELECT * FROM assess.assessments WHERE id = $1 AND assessment_type = 'dpia';

-- name: ListAssessments :many
-- Newest first; keyset cursor on (created_at, id).
SELECT * FROM assess.assessments
WHERE assessment_type = 'dpia'
    AND (sqlc.narg(activity_id)::uuid IS NULL OR activity_id = sqlc.narg(activity_id))
    AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ListAllAssessments :many
-- DPIA-12's registry: every one of the tenant's DPIA rounds, every activity, no pagination — the registry
-- is meant to be viewed as one report (the same precedent as ROPA-04's own processor-RoPA export), and
-- Registry() re-filters/joins in Go against ropa/org (rule 9), so there is nothing to page server-side.
-- Latest round per activity first (one activity may have several rounds; the registry shows the current
-- one), then newest overall.
SELECT DISTINCT ON (activity_id) *
FROM assess.assessments
WHERE assessment_type = 'dpia'
ORDER BY activity_id, round_no DESC;
