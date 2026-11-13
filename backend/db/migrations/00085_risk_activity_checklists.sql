-- RRA-05: per-activity checklist (รายการตรวจสอบรายกิจกรรม). The checklist's own questions are an
-- ordinary PLT-06 form the DPO authors and publishes (CLAUDE.md rule 8 — no wording invented here);
-- this migration only widens platform.form_definitions.form_type to accept a dedicated 'checklist'
-- type ('quiz' is still unclaimed but doesn't fit this domain — DPO-09's own "security" type set the
-- precedent of adding a real name instead of reusing an unrelated spare one) and adds a thin record
-- of each run against a RoPA processing activity, mirroring dpo.security_assessments' own shape
-- (form_submission_id + an optional score/result/factors, since a checklist need not be scored).

-- +goose Up
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security', 'checklist'));

CREATE TABLE risk.activity_checklists (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    score numeric(6, 2),
    result text,
    factors jsonb,
    completed_by uuid,
    completed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_activity_checklists PRIMARY KEY (id)
);

ALTER TABLE risk.activity_checklists ADD CONSTRAINT fk_activity_checklists_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.activity_checklists ADD CONSTRAINT fk_activity_checklists_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE risk.activity_checklists ADD CONSTRAINT fk_activity_checklists_completed_by FOREIGN KEY (completed_by) REFERENCES iam.users (id);
-- activity_id is a logical reference to ropa.processing_activities (cross-schema FKs bypass RLS,
-- CLAUDE.md rule 1), checked visible via risk/service's own Ropa.ActivityVisible before writing.

CREATE INDEX ix_risk_activity_checklists_activity ON risk.activity_checklists (tenant_id, activity_id, completed_at DESC);

ALTER TABLE risk.activity_checklists ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.activity_checklists
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down
DROP TABLE IF EXISTS risk.activity_checklists;
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security'));
