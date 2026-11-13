-- name: ListTemplates :many
-- Newest first within each type; global (tenant_id NULL) and the caller's own tenant rows only (RLS).
SELECT * FROM assess.templates
WHERE (sqlc.narg(assessment_type)::text IS NULL OR assessment_type = sqlc.narg(assessment_type))
ORDER BY assessment_type, code, tenant_id NULLS FIRST;

-- name: GetTemplateByID :one
SELECT * FROM assess.templates WHERE id = $1;

-- name: InsertTemplateRow :one
INSERT INTO assess.templates (id, tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: UpdateTemplateStatus :one
UPDATE assess.templates SET status = $2, version_no = $3, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1
RETURNING *;

-- name: RetireTemplateRow :one
-- Optimistic concurrency on the template's own row_version (If-Match) — unlike PublishTemplate, retiring has
-- no underlying form ETag to lean on.
UPDATE assess.templates SET status = 'retired', updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING *;
