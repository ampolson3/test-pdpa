-- name: ListTemplateSets :many
-- RTG-01: published template sets (global or this tenant's own) — RLS's tenant_read policy already scopes
-- this to tenant_id IS NULL OR tenant_id = the caller's tenant, the same ORG-07 master-data pattern.
SELECT id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version
FROM ropa.template_sets
WHERE status = 'published'
ORDER BY name;

-- name: ListActivityTemplates :many
SELECT id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version
FROM ropa.activity_templates
WHERE template_set_id = $1
  AND (sqlc.narg('job_category')::varchar IS NULL OR job_category = sqlc.narg('job_category'))
ORDER BY job_category, name_th;

-- name: GetActivityTemplate :one
SELECT id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version
FROM ropa.activity_templates
WHERE id = $1;
