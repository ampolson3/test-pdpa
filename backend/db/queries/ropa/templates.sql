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

-- name: ListTemplateVersionsForCode :many
-- RTG-10: every version ever issued of one template (global standard/industry/government/processor, or a
-- tenant's own custom one — same visibility RLS already scopes), newest first — "หน้าประวัติ template".
SELECT id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version
FROM ropa.activity_templates
WHERE template_set_id = $1 AND code = $2
ORDER BY version_no DESC;

-- name: GetLatestTemplateVersionForCode :one
-- RTG-10: the current (highest version_no) row for this template's own (template_set_id, code) — compared
-- against the version_no an activity was actually created from, to decide whether it's now outdated.
SELECT id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version
FROM ropa.activity_templates
WHERE template_set_id = $1 AND code = $2
ORDER BY version_no DESC
LIMIT 1;

-- name: ListOwnTemplateSets :many
-- RTG-08: this tenant's own custom template sets, any status (draft/published/retired) — the management
-- list, distinct from ListTemplateSets' own published-only browse list every other RTG feature relies on.
SELECT id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version
FROM ropa.template_sets
WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
ORDER BY name;

-- name: GetOwnTemplateSet :one
-- Scoped to this tenant's own row (not a global standard/industry/government/processor set) — the
-- ownership check every write (add/update/delete an activity, publish/retire the set) must pass first.
SELECT id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version
FROM ropa.template_sets
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: InsertTemplateSet :one
INSERT INTO ropa.template_sets (id, tenant_id, name, set_type, version_no, status, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, 'custom', 1, 'draft',
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version;

-- name: PublishTemplateSetRow :one
UPDATE ropa.template_sets SET status = 'published', published_at = now(), updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid AND row_version = $2
RETURNING id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version;

-- name: RetireTemplateSetRow :one
UPDATE ropa.template_sets SET status = 'retired', updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid AND row_version = $2
RETURNING id, tenant_id, name, set_type, industry, version_no, status, published_at, row_version;

-- name: InsertActivityTemplate :one
-- RTG-08: a custom activity template, always owned by this tenant and always under one of this tenant's
-- own custom template sets — the caller's own service layer proves the set is editable before this runs.
INSERT INTO ropa.activity_templates (id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version;

-- name: UpdateActivityTemplateRow :one
UPDATE ropa.activity_templates SET
    code = $3, name_th = $4, name_en = $5, job_category = $6, role = $7, defaults = $8, rationale = $9, legal_refs = $10,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
RETURNING id, tenant_id, template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no, row_version;

-- name: DeleteActivityTemplateRow :execrows
DELETE FROM ropa.activity_templates
WHERE id = $1 AND row_version = $2 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;
