-- name: ListWizardTemplateGroups :many
SELECT DISTINCT subject_type_code FROM notice.wizard_templates ORDER BY subject_type_code;

-- name: GetWizardTemplateContent :many
SELECT wt.language, t.content
FROM notice.wizard_templates wt
JOIN platform.templates t ON t.id = wt.template_id
WHERE wt.subject_type_code = $1
ORDER BY wt.language;
