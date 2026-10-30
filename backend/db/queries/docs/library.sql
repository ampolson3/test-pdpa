-- PLT-16 clause library and document templates: rows are versions of a code; at most one draft per code.

-- name: ListLatestClauses :many
SELECT DISTINCT ON (tenant_id, code) id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory,
       version_no, status, updated_at, row_version
FROM platform.clause_library
WHERE (sqlc.narg(category)::text IS NULL OR category = sqlc.narg(category))
  AND (sqlc.narg(applies_to)::text IS NULL OR sqlc.narg(applies_to) = ANY (applies_to))
  AND (NOT @published_only::bool OR status = 'published')
ORDER BY tenant_id, code, version_no DESC;

-- name: GetClause :one
SELECT id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory, version_no, status, updated_at, row_version
FROM platform.clause_library WHERE id = $1;

-- name: ClauseVersions :many
SELECT id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory, version_no, status, updated_at, row_version
FROM platform.clause_library WHERE code = $1 AND tenant_id IS NOT DISTINCT FROM sqlc.narg(tenant_id)::uuid ORDER BY version_no DESC;

-- name: ClausesByRef :many
-- Published or retired versions only: a draft clause is never used by a document. A tenant's own clause wins over
-- a platform clause with the same code.
SELECT DISTINCT ON (code, version_no) id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory,
       version_no, status, updated_at, row_version
FROM platform.clause_library
WHERE status IN ('published', 'retired') AND code = ANY (@codes::text[])
ORDER BY code, version_no, tenant_id NULLS LAST;

-- name: LatestTenantClause :one
SELECT id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory, version_no, status, updated_at, row_version
FROM platform.clause_library
WHERE tenant_id = current_setting('app.tenant_id')::uuid AND code = $1
ORDER BY version_no DESC LIMIT 1 FOR UPDATE;

-- name: InsertClause :one
INSERT INTO platform.clause_library (id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory,
                                     version_no, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @code, @category, @title, @body_th, sqlc.narg(body_en), sqlc.narg(legal_ref),
        @applies_to, @is_mandatory, @version_no,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id;

-- name: UpdateClauseDraft :exec
UPDATE platform.clause_library
SET category = @category, title = @title, body_th = @body_th, body_en = sqlc.narg(body_en), legal_ref = sqlc.narg(legal_ref),
    applies_to = @applies_to, is_mandatory = @is_mandatory, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status = 'draft';

-- name: SetClauseStatus :exec
UPDATE platform.clause_library SET status = @status, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: RetireOtherClauseVersions :exec
UPDATE platform.clause_library SET status = 'retired', updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE tenant_id = current_setting('app.tenant_id')::uuid AND code = @code AND id <> @id AND status = 'published';

-- name: ListLatestTemplates :many
SELECT DISTINCT ON (tenant_id, template_type, code) id, tenant_id, template_type, code, name, language, industry, content,
       version_no, status, updated_at, row_version
FROM platform.templates
WHERE template_type = ANY (@types::text[]) AND language = 'mul'
  AND (NOT @published_only::bool OR status = 'published')
ORDER BY tenant_id, template_type, code, version_no DESC;

-- name: GetTemplate :one
SELECT id, tenant_id, template_type, code, name, language, industry, content, version_no, status, updated_at, row_version
FROM platform.templates WHERE id = $1;

-- name: LatestTenantTemplate :one
SELECT id, tenant_id, template_type, code, name, language, industry, content, version_no, status, updated_at, row_version
FROM platform.templates
WHERE tenant_id = current_setting('app.tenant_id')::uuid AND template_type = @template_type AND code = @code AND language = 'mul'
ORDER BY version_no DESC LIMIT 1 FOR UPDATE;

-- name: InsertTemplate :one
INSERT INTO platform.templates (id, tenant_id, template_type, code, name, language, content, version_no, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @template_type, @code, @name, 'mul', @content, @version_no,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id;

-- name: UpdateTemplateDraft :exec
UPDATE platform.templates
SET name = @name, content = @content, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status = 'draft';

-- name: SetTemplateStatus :exec
UPDATE platform.templates SET status = @status, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: RetireOtherTemplateVersions :exec
UPDATE platform.templates SET status = 'retired', updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE tenant_id = current_setting('app.tenant_id')::uuid AND template_type = @template_type AND code = @code
  AND language = 'mul' AND id <> @id AND status = 'published';
