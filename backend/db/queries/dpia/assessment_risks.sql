-- DPIA-06: the link between one DPIA round (assess.assessments) and a risk.risks row the risk engine owns
-- (rule 9 — dpia never writes risk.risks itself, only this link in its own schema).

-- name: LinkAssessmentRisk :exec
INSERT INTO assess.assessment_risks (assessment_id, risk_id, tenant_id)
VALUES ($1, $2, NULLIF(current_setting('app.tenant_id', true), '')::uuid)
ON CONFLICT (assessment_id, risk_id) DO NOTHING;

-- name: ListAssessmentRiskIDs :many
SELECT risk_id FROM assess.assessment_risks WHERE assessment_id = $1;

-- name: UnlinkAssessmentRisk :execrows
DELETE FROM assess.assessment_risks WHERE assessment_id = $1 AND risk_id = $2;
