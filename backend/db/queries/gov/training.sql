-- DPX-01: gov.courses/gov.training_assignments/gov.training_attempts were already fully specified in the
-- baseline migrations (00017_gov.sql, FK'd in 00018: content_file_id/certificate_file_id -> platform.files,
-- quiz_form_id -> platform.form_definitions, assignment_id -> gov.training_assignments) — no migration
-- needed.

-- name: InsertCourse :one
INSERT INTO gov.courses (id, tenant_id, title, description, content_type, content_file_id, quiz_form_id,
    passing_score, validity_months, language, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @title, sqlc.narg(description)::text, @content_type,
    sqlc.narg(content_file_id)::uuid, sqlc.narg(quiz_form_id)::uuid, @passing_score, @validity_months, @language,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, title, description, content_type, content_file_id, quiz_form_id, passing_score, validity_months,
    language, status, row_version, created_at, updated_at;

-- name: GetCourse :one
SELECT id, title, description, content_type, content_file_id, quiz_form_id, passing_score, validity_months,
    language, status, row_version, created_at, updated_at
FROM gov.courses WHERE id = @id;

-- name: ListCourses :many
SELECT id, title, description, content_type, content_file_id, quiz_form_id, passing_score, validity_months,
    language, status, row_version, created_at, updated_at
FROM gov.courses
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC;

-- name: UpdateCourse :one
UPDATE gov.courses
SET title = @title, description = sqlc.narg(description)::text, content_type = @content_type,
    content_file_id = sqlc.narg(content_file_id)::uuid, quiz_form_id = sqlc.narg(quiz_form_id)::uuid,
    passing_score = @passing_score, validity_months = @validity_months, language = @language,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version AND status = 'draft'
RETURNING id, title, description, content_type, content_file_id, quiz_form_id, passing_score, validity_months,
    language, status, row_version, created_at, updated_at;

-- name: SetCourseStatus :one
UPDATE gov.courses
SET status = @status, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, title, description, content_type, content_file_id, quiz_form_id, passing_score, validity_months,
    language, status, row_version, created_at, updated_at;

-- name: InsertTrainingAssignment :one
INSERT INTO gov.training_assignments (id, tenant_id, course_id, target_type, target_id, due_at, assigned_by, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @course_id, @target_type, sqlc.narg(target_id)::uuid, @due_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at;

-- name: GetTrainingAssignment :one
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments WHERE id = @id;

-- name: ListTrainingAssignments :many
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments
WHERE (sqlc.narg(course_id)::uuid IS NULL OR course_id = sqlc.narg(course_id))
ORDER BY due_at, created_at DESC;

-- name: ListAssignmentsForAllTarget :many
-- Every "target_type='all'" assignment — applies to every employee regardless of org unit/group.
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments WHERE target_type = 'all'
ORDER BY due_at;

-- name: ListAssignmentsForOrgUnit :many
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments WHERE target_type = 'org_unit' AND target_id = @target_id
ORDER BY due_at;

-- name: ListAssignmentsForGroups :many
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments WHERE target_type = 'group' AND target_id = ANY (@group_ids::uuid[])
ORDER BY due_at;

-- name: ListAssignmentsForUser :many
SELECT id, course_id, target_type, target_id, due_at, assigned_by, row_version, created_at, updated_at
FROM gov.training_assignments WHERE target_type = 'user' AND target_id = @target_id
ORDER BY due_at;

-- name: InsertTrainingAttempt :one
INSERT INTO gov.training_attempts (id, tenant_id, course_id, assignment_id, user_id, started_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @course_id, sqlc.narg(assignment_id)::uuid, @user_id, @started_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, course_id, assignment_id, user_id, started_at, completed_at, score, passed, certificate_file_id, row_version, created_at, updated_at;

-- name: GetTrainingAttempt :one
SELECT id, course_id, assignment_id, user_id, started_at, completed_at, score, passed, certificate_file_id, row_version, created_at, updated_at
FROM gov.training_attempts WHERE id = @id;

-- name: CompleteTrainingAttempt :one
UPDATE gov.training_attempts
SET completed_at = @completed_at, score = sqlc.narg(score)::smallint, passed = @passed,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version AND completed_at IS NULL
RETURNING id, course_id, assignment_id, user_id, started_at, completed_at, score, passed, certificate_file_id, row_version, created_at, updated_at;

-- name: ListAttemptsByCourse :many
-- Newest attempt per user shown first — the per-individual report (the acceptance criterion's other half).
SELECT id, course_id, assignment_id, user_id, started_at, completed_at, score, passed, certificate_file_id, row_version, created_at, updated_at
FROM gov.training_attempts
WHERE course_id = @course_id
ORDER BY user_id, started_at DESC;

-- name: ListAttemptsByUser :many
SELECT id, course_id, assignment_id, user_id, started_at, completed_at, score, passed, certificate_file_id, row_version, created_at, updated_at
FROM gov.training_attempts
WHERE user_id = @user_id
ORDER BY started_at DESC;
