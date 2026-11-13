-- ROPA-11: ropa.questionnaires was already fully specified in the baseline migrations (00008_ropa.sql) —
-- a department-facing data mapping questionnaire, one row per round (sent -> answered -> approved|rejected).
-- No queries existed for it yet.

-- name: InsertQuestionnaire :one
INSERT INTO ropa.questionnaires (tenant_id, org_unit_id, respondent_user_id, due_at)
VALUES (sqlc.arg(tenant_id)::uuid, sqlc.arg(org_unit_id)::uuid, sqlc.arg(respondent_user_id)::uuid, sqlc.narg(due_at)::timestamptz)
RETURNING *;

-- name: GetQuestionnaire :one
SELECT * FROM ropa.questionnaires WHERE id = $1;

-- name: ListQuestionnaires :many
SELECT * FROM ropa.questionnaires
WHERE (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC;

-- name: MarkQuestionnaireAnswered :one
UPDATE ropa.questionnaires
SET status = 'answered', form_submission_id = sqlc.arg(form_submission_id)::uuid, updated_at = now(), updated_by = sqlc.narg(updated_by)::uuid,
    row_version = row_version + 1
WHERE id = sqlc.arg(id)::uuid AND status IN ('sent', 'rejected') AND row_version = sqlc.arg(row_version)::int
RETURNING *;

-- name: DecideQuestionnaire :one
UPDATE ropa.questionnaires
SET status = sqlc.arg(status)::text, activity_id = sqlc.narg(activity_id)::uuid, approved_by = sqlc.narg(decided_by)::uuid,
    updated_at = now(), updated_by = sqlc.narg(decided_by)::uuid, row_version = row_version + 1
WHERE id = sqlc.arg(id)::uuid AND status = 'answered' AND row_version = sqlc.arg(row_version)::int
RETURNING *;
