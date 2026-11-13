-- DPX-08: gov.regulator_letters was already fully specified in the baseline migrations (00017_gov.sql,
-- FK'd in 00018: file_id -> platform.files, incident_id -> breach.incidents) — no migration needed.

-- name: InsertRegulatorLetter :one
INSERT INTO gov.regulator_letters (id, tenant_id, direction, letter_no, subject, received_at, due_at, file_id, incident_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @direction, sqlc.narg(letter_no)::varchar, @subject, sqlc.narg(received_at)::date,
    sqlc.narg(due_at)::date, sqlc.narg(file_id)::uuid, sqlc.narg(incident_id)::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, direction, letter_no, subject, received_at, due_at, status, file_id, incident_id, responded_at, row_version, created_at, updated_at;

-- name: GetRegulatorLetter :one
SELECT id, direction, letter_no, subject, received_at, due_at, status, file_id, incident_id, responded_at, row_version, created_at, updated_at
FROM gov.regulator_letters WHERE id = @id;

-- name: ListRegulatorLetters :many
SELECT id, direction, letter_no, subject, received_at, due_at, status, file_id, incident_id, responded_at, row_version, created_at, updated_at
FROM gov.regulator_letters
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction))
ORDER BY coalesce(due_at, '9999-12-31'::date), created_at DESC;

-- name: UpdateRegulatorLetter :one
UPDATE gov.regulator_letters
SET direction = @direction, letter_no = sqlc.narg(letter_no)::varchar, subject = @subject, received_at = sqlc.narg(received_at)::date,
    due_at = sqlc.narg(due_at)::date, file_id = sqlc.narg(file_id)::uuid, incident_id = sqlc.narg(incident_id)::uuid,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, direction, letter_no, subject, received_at, due_at, status, file_id, incident_id, responded_at, row_version, created_at, updated_at;

-- name: SetRegulatorLetterStatus :one
UPDATE gov.regulator_letters
SET status = @status, responded_at = CASE WHEN @status::text = 'responded' THEN current_date ELSE responded_at END,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, direction, letter_no, subject, received_at, due_at, status, file_id, incident_id, responded_at, row_version, created_at, updated_at;

-- name: DeleteRegulatorLetter :execrows
DELETE FROM gov.regulator_letters WHERE id = @id;
