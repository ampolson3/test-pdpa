-- DPO-02 (docs/decisions.md Q-37): a generic counterpart to dpia.sql's own GetScreeningTemplate/
-- GetLatestAssessmentForActivity for any non-"dpia" assessment_type and non-activity subject — reused by
-- dpo's own requirement check (ScreenSubject in screening.go), and any future module's own single-subject
-- yes_no screening on assess.templates/assess.assessments.

-- name: GetTemplateByCodeAndType :one
SELECT * FROM assess.templates
WHERE assessment_type = @assessment_type AND code = @code AND status = 'published'
ORDER BY tenant_id NULLS LAST
LIMIT 1;

-- name: GetLatestAssessmentForSubject :one
SELECT * FROM assess.assessments
WHERE assessment_type = @assessment_type AND subject_id = @subject_id
ORDER BY round_no DESC
LIMIT 1;
