-- DPIA-16 (ม.24(5)): legitimate interest assessment rounds — the same assess.assessments/assess.answers
-- tables DPIA-01's own screening and DPIA-05's necessity checklist use, keyed by assessment_type = 'lia'
-- and activity_id directly (one activity may hold several rounds, round-chained via previous_id, same as
-- DPIA-01's own GetLatestAssessmentForActivity/GetAssessment but for this distinct assessment_type — kept
-- separate rather than widening those two, which every "dpia"-type caller relies on staying scoped to it).

-- name: GetLatestLiaForActivity :one
SELECT * FROM assess.assessments
WHERE assessment_type = 'lia' AND activity_id = $1
ORDER BY round_no DESC
LIMIT 1;

-- name: GetLiaAssessment :one
SELECT * FROM assess.assessments WHERE id = $1 AND assessment_type = 'lia';

-- name: ListLiaForActivity :many
SELECT * FROM assess.assessments WHERE assessment_type = 'lia' AND activity_id = $1 ORDER BY round_no DESC;

-- name: SetLiaAssessmentStatus :one
UPDATE assess.assessments
SET status = $2, approved_at = sqlc.narg(approved_at), updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND assessment_type = 'lia' AND row_version = $3
RETURNING *;
