-- name: GetResponseTemplateContent :many
SELECT language, content FROM platform.templates
WHERE template_type = 'dsar_response' AND code = $1 AND status = 'published'
ORDER BY language;
