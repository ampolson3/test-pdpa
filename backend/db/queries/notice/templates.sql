-- name: ListWizardTemplateGroups :many
-- PNG-11 widened this to exclude industry starter templates (industry IS NOT NULL) from the data-subject
-- group picker -- they share the same table and the same lookup-by-code mechanism (GetWizardTemplateContent
-- below), but are a separate axis the wizard offers through ListIndustryTemplates instead.
SELECT DISTINCT subject_type_code FROM notice.wizard_templates WHERE industry IS NULL ORDER BY subject_type_code;

-- name: ListIndustryTemplates :many
-- PNG-11's own picker: every industry code that has a seeded (or tenant-authored) starter template.
SELECT DISTINCT subject_type_code FROM notice.wizard_templates WHERE industry IS NOT NULL ORDER BY subject_type_code;

-- name: GetWizardTemplateContent :many
SELECT wt.language, t.content
FROM notice.wizard_templates wt
JOIN platform.templates t ON t.id = wt.template_id
WHERE wt.subject_type_code = $1
ORDER BY wt.language;
