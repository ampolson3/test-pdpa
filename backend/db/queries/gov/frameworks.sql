-- DPX-10: gov.regulatory_frameworks is a global (no tenant_id), read-only reference catalog mapping a
-- related law/notice to its own PLT-06 checklist form (migration 00096) — not RLS-scoped, same as
-- gov.regulatory_updates. gov.framework_assessments is the tenant-scoped run record.

-- name: ListRegulatoryFrameworks :many
SELECT id, code, name_th, name_en, legal_refs, description_th, description_en, checklist_form_id, row_version
FROM gov.regulatory_frameworks ORDER BY code;

-- name: GetRegulatoryFrameworkByCode :one
SELECT id, code, name_th, name_en, legal_refs, description_th, description_en, checklist_form_id, row_version
FROM gov.regulatory_frameworks WHERE code = @code;

-- name: InsertFrameworkAssessment :one
INSERT INTO gov.framework_assessments (id, tenant_id, framework_id, form_submission_id, score, result, factors, assessed_by, assessed_at)
VALUES (@id, current_setting('app.tenant_id')::uuid, @framework_id, @form_submission_id, @score, @result, @factors, sqlc.narg(assessed_by)::uuid, @assessed_at)
RETURNING id, framework_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version, created_at;

-- name: GetFrameworkAssessment :one
SELECT id, framework_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version, created_at
FROM gov.framework_assessments WHERE id = @id;

-- name: ListFrameworkAssessments :many
SELECT id, framework_id, form_submission_id, score, result, factors, assessed_by, assessed_at, row_version, created_at
FROM gov.framework_assessments
WHERE (sqlc.narg(framework_id)::uuid IS NULL OR framework_id = sqlc.narg(framework_id))
ORDER BY assessed_at DESC;
