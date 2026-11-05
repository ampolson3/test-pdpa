-- name: GetSectionByCode :one
SELECT * FROM assess.sections WHERE assessment_id = $1 AND section_code = $2 LIMIT 1;

-- name: InsertSection :one
INSERT INTO assess.sections (id, tenant_id, assessment_id, section_code, status, submitted_at, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, 'submitted', now(),
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ResubmitSection :one
-- A redo of the same section (DPIA-05 answered again): keep the row, refresh submitted_at.
UPDATE assess.sections SET status = 'submitted', submitted_at = now(), updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1
RETURNING *;

-- name: DeleteAnswersForSection :exec
DELETE FROM assess.answers WHERE section_id = $1;

-- name: InsertSectionAnswer :one
INSERT INTO assess.answers (id, tenant_id, assessment_id, section_id, question_code, answer, answered_by, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ListAnswersForSection :many
SELECT * FROM assess.answers WHERE section_id = $1 ORDER BY created_at;
