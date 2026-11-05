-- DSAR-20: a tenant's own override response-letter templates (platform.templates, template_type =
-- 'dsar_response'). The global purpose templates (migration 00043, code = result|rejection|request_info)
-- stay read-only defaults; an override here always uses a tenant-chosen code distinct from those, so it is
-- a brand-new row, never a shadow of a global one — no ambiguity between the two to resolve at read time.

-- name: UpsertResponseTemplateOverride :one
INSERT INTO platform.templates (tenant_id, template_type, code, name, language, content, status)
VALUES (current_setting('app.tenant_id')::uuid, 'dsar_response', sqlc.arg(code), sqlc.arg(name), sqlc.arg(language), sqlc.arg(content), 'published')
ON CONFLICT ON CONSTRAINT uq_templates_template_type_code_language_version_no
    DO UPDATE SET name = EXCLUDED.name, content = EXCLUDED.content
RETURNING id, code, name, language, content, row_version;

-- name: ListResponseTemplateOverrides :many
SELECT id, code, name, language, content, row_version FROM platform.templates
WHERE template_type = 'dsar_response' AND tenant_id = current_setting('app.tenant_id')::uuid
ORDER BY code, language;
