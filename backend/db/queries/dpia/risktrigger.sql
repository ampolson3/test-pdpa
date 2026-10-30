-- RRA-03 (ส่งต่อทำ DPIA อัตโนมัติ): a high/very_high RRA-01 risk score opens a DPIA round directly at
-- in_progress — screening_result is fixed to "required" and risk_level/score come from the risk engine's
-- own classification, not from re-answering the screening form.

-- name: InsertRiskTriggeredAssessment :one
INSERT INTO assess.assessments (id, tenant_id, assessment_type, template_id, form_version_id, title, subject_type,
    subject_id, activity_id, round_no, previous_id, status, screening_result, screening_reason, score, risk_level,
    owner_user_id, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;
