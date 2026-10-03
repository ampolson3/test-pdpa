-- DPO-09: security-measures assessment (ประกาศมาตรการความปลอดภัย พ.ศ. 2565). The checklist itself is an
-- ordinary PLT-06 form the DPO authors (no legally-mandated wording invented here, CLAUDE.md rule 8) — this
-- migration only widens platform.form_definitions.form_type to accept it (no owning module claimed 'quiz'
-- or another spare enum value, so a real 'security' type is clearer than reusing an unrelated one) and adds
-- a thin record of each assessment run, mirroring breach.assessments (score/band + a form_submission_id).

-- +goose Up
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security'));

CREATE TABLE dpo.security_assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    score numeric(6, 2) NOT NULL,
    result text NOT NULL,
    factors jsonb NOT NULL,
    assessed_by uuid,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_security_assessments PRIMARY KEY (id)
);

ALTER TABLE dpo.security_assessments ADD CONSTRAINT fk_security_assessments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.security_assessments ADD CONSTRAINT fk_security_assessments_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE dpo.security_assessments ADD CONSTRAINT fk_security_assessments_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE dpo.security_assessments ADD CONSTRAINT fk_security_assessments_assessed_by FOREIGN KEY (assessed_by) REFERENCES iam.users (id);

CREATE INDEX ix_dpo_security_assessments_legal_entity ON dpo.security_assessments (tenant_id, legal_entity_id, assessed_at DESC);

ALTER TABLE dpo.security_assessments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.security_assessments
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down
DROP TABLE IF EXISTS dpo.security_assessments;
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz'));
