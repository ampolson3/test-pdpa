-- PLT-06 form & assessment engine.

-- name: ListForms :many
SELECT d.id, d.tenant_id, d.code, d.name, d.form_type, d.status, d.current_version_id, d.row_version, d.updated_at,
       (SELECT max(v.version_no) FROM platform.form_versions v WHERE v.form_id = d.id)::int AS latest_version
FROM platform.form_definitions d
WHERE d.form_type = ANY (@types::text[])
ORDER BY d.form_type, d.name;

-- name: GetForm :one
SELECT id, tenant_id, code, name, form_type, status, current_version_id, row_version, updated_at
FROM platform.form_definitions WHERE id = $1;

-- name: LockForm :one
SELECT id, tenant_id, code, name, form_type, status, current_version_id, row_version, updated_at
FROM platform.form_definitions WHERE id = $1 FOR UPDATE;

-- name: InsertForm :one
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, code, name, form_type, status, current_version_id, row_version, updated_at;

-- name: UpdateFormState :exec
UPDATE platform.form_definitions
SET name = $2, status = $3, current_version_id = $4, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1;

-- name: ListFormVersions :many
SELECT id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at
FROM platform.form_versions WHERE form_id = $1 ORDER BY version_no DESC;

-- name: GetFormVersion :one
SELECT id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at
FROM platform.form_versions WHERE id = $1;

-- name: DraftFormVersion :one
SELECT id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at
FROM platform.form_versions WHERE form_id = $1 AND published_at IS NULL FOR UPDATE;

-- name: InsertFormVersion :one
INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, scoring, languages, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2,
        (SELECT COALESCE(max(version_no), 0) + 1 FROM platform.form_versions WHERE form_id = $2), $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at;

-- name: UpdateDraftVersion :one
UPDATE platform.form_versions
SET schema = $3, scoring = $4, languages = $5, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND published_at IS NULL
RETURNING id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at;

-- name: PublishVersion :one
UPDATE platform.form_versions
SET published_at = now(), published_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND published_at IS NULL
RETURNING id, form_id, version_no, schema, scoring, languages, published_at, published_by, row_version, updated_at;

-- name: InsertSubmission :one
INSERT INTO platform.form_submissions (id, tenant_id, form_version_id, entity_type, entity_id, submitted_by_type, submitted_by, answers, status, score, result, submitted_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, form_version_id, entity_type, entity_id, submitted_by_type, submitted_by, answers, status, score, result, submitted_at, row_version, created_at, updated_at;

-- name: GetSubmission :one
SELECT id, form_version_id, entity_type, entity_id, submitted_by_type, submitted_by, answers, status, score, result, submitted_at, row_version, created_at, updated_at
FROM platform.form_submissions WHERE id = $1;

-- name: LockSubmission :one
SELECT id, form_version_id, entity_type, entity_id, submitted_by_type, submitted_by, answers, status, score, result, submitted_at, row_version, created_at, updated_at
FROM platform.form_submissions WHERE id = $1 FOR UPDATE;

-- name: UpdateSubmission :one
UPDATE platform.form_submissions
SET answers = $2, status = $3, score = $4, result = $5, submitted_at = $6, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1
RETURNING id, form_version_id, entity_type, entity_id, submitted_by_type, submitted_by, answers, status, score, result, submitted_at, row_version, created_at, updated_at;

-- name: ListResponses :many
-- Responses to a form's versions, newest first.
SELECT s.id, s.form_version_id, s.entity_type, s.entity_id, s.submitted_by_type, s.submitted_by, s.answers, s.status, s.score, s.result, s.submitted_at, s.row_version, s.created_at, s.updated_at
FROM platform.form_submissions s JOIN platform.form_versions v ON v.id = s.form_version_id
WHERE v.form_id = $1
ORDER BY s.created_at DESC
LIMIT 200;

-- name: ListAssignments :many
SELECT id, submission_id, section_key, assignee_user_id, status, completed_at, row_version
FROM platform.form_section_assignments WHERE submission_id = $1 ORDER BY created_at, section_key;

-- name: UpsertAssignment :one
INSERT INTO platform.form_section_assignments (id, tenant_id, submission_id, section_key, assignee_user_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT (tenant_id, submission_id, section_key) DO UPDATE
SET assignee_user_id = EXCLUDED.assignee_user_id, status = 'open', completed_at = NULL, row_version = platform.form_section_assignments.row_version + 1,
    updated_by = EXCLUDED.updated_by
RETURNING id, submission_id, section_key, assignee_user_id, status, completed_at, row_version;

-- name: DeleteAssignment :execrows
DELETE FROM platform.form_section_assignments WHERE submission_id = $1 AND section_key = $2 AND status = 'open';

-- name: CompleteAssignment :execrows
UPDATE platform.form_section_assignments
SET status = 'done', completed_at = now(), row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE submission_id = $1 AND section_key = $2 AND assignee_user_id = $3 AND status = 'open';

-- name: MyFormAssignments :many
SELECT a.id, a.submission_id, a.section_key, a.status, a.row_version, a.created_at,
       d.id AS form_id, d.name AS form_name, d.form_type, v.schema
FROM platform.form_section_assignments a
JOIN platform.form_submissions s ON s.id = a.submission_id AND s.status = 'draft'
JOIN platform.form_versions v ON v.id = s.form_version_id
JOIN platform.form_definitions d ON d.id = v.form_id
WHERE a.assignee_user_id = @user_id AND a.status = 'open'
ORDER BY a.created_at
LIMIT 200;
