-- DPIA-10: DPO opinion (assess.dpo_opinions, already fully specified in the baseline migrations) and the
-- assess.assessments status transitions ST-05#2 declares beyond screening (in_progress -> in_review ->
-- approved/rejected/needs_review -> closed).

-- name: InsertDpoOpinion :one
INSERT INTO assess.dpo_opinions (id, tenant_id, assessment_id, dpo_user_id, opinion, recommendation, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2,
        NULLIF(current_setting('app.user_id', true), '')::uuid, $3, $4,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ListOpinionsForAssessment :many
SELECT * FROM assess.dpo_opinions WHERE assessment_id = $1 ORDER BY created_at;

-- name: SetAssessmentStatus :one
UPDATE assess.assessments
SET status = $2, approved_at = sqlc.narg(approved_at), updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND assessment_type = 'dpia' AND row_version = $3
RETURNING *;
