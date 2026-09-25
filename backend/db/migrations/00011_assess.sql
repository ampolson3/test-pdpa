-- +goose Up
-- schema assess: แบบประเมิน DPIA / LIA / TIA / security / maturity (assessment engine)
-- 8 tables · docs: docs/data/assess.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE assess.templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    assessment_type text NOT NULL CHECK (assessment_type IN ('dpia', 'pia', 'lia', 'tia', 'ai', 'security', 'maturity', 'dpo_check', 'sme_check', 'vendor', 'inbound_dpa', 'independence')),
    code varchar(60) NOT NULL,
    name text NOT NULL,
    form_id uuid NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    legal_refs text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_templates PRIMARY KEY (id),
    CONSTRAINT uq_templates_assessment_type_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, assessment_type, code, version_no)
);

CREATE TABLE assess.screening_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    criteria jsonb NOT NULL,
    min_factors smallint NOT NULL DEFAULT 2,
    min_score numeric(6,2),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_screening_rules PRIMARY KEY (id)
);

CREATE TABLE assess.assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_type varchar(20) NOT NULL,
    template_id uuid NOT NULL,
    form_version_id uuid NOT NULL,
    title text NOT NULL,
    subject_type text NOT NULL CHECK (subject_type IN ('activity', 'vendor', 'system', 'legal_entity', 'ai_system', 'transfer', 'project')),
    subject_id uuid,
    activity_id uuid,
    round_no smallint NOT NULL DEFAULT 1,
    previous_id uuid,
    status text NOT NULL DEFAULT 'screening' CHECK (status IN ('screening', 'not_required', 'in_progress', 'in_review', 'approved', 'rejected', 'needs_review', 'closed')),
    screening_result text CHECK (screening_result IN ('required', 'recommended', 'not_required')),
    screening_reason text,
    score numeric(8,2),
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'very_high')),
    owner_user_id uuid,
    due_at date,
    approved_at timestamptz,
    next_review_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_assessments PRIMARY KEY (id)
);

CREATE TABLE assess.sections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    section_code varchar(60) NOT NULL,
    assignee_user_id uuid,
    guest_token_id uuid,
    status text NOT NULL DEFAULT 'not_started' CHECK (status IN ('not_started', 'in_progress', 'submitted', 'needs_info')),
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_sections PRIMARY KEY (id)
);

CREATE TABLE assess.answers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    section_id uuid,
    question_code varchar(80) NOT NULL,
    answer jsonb NOT NULL,
    evidence_file_ids uuid[] NOT NULL DEFAULT '{}',
    answered_by uuid,
    ai_suggested boolean NOT NULL DEFAULT false,
    confirmed_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_answers PRIMARY KEY (id)
);

CREATE TABLE assess.assessment_risks (
    assessment_id uuid NOT NULL,
    risk_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    CONSTRAINT pk_assess_assessment_risks PRIMARY KEY (assessment_id, risk_id)
);

CREATE TABLE assess.dpo_opinions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    dpo_user_id uuid NOT NULL,
    opinion text NOT NULL,
    recommendation text NOT NULL CHECK (recommendation IN ('proceed', 'proceed_with_conditions', 'do_not_proceed', 'consult_pdpc')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_dpo_opinions PRIMARY KEY (id)
);

CREATE TABLE assess.consultations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    stakeholder text NOT NULL,
    stakeholder_type text NOT NULL CHECK (stakeholder_type IN ('data_subject', 'processor', 'expert', 'internal', 'regulator')),
    consulted_at date NOT NULL,
    summary text NOT NULL,
    response text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_consultations PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_assess_templates_form_id ON assess.templates (form_id);
CREATE INDEX ix_assess_assessments_assessment_type ON assess.assessments (tenant_id, assessment_type);
CREATE INDEX ix_assess_assessments_template_id ON assess.assessments (tenant_id, template_id);
CREATE INDEX ix_assess_assessments_form_version_id ON assess.assessments (tenant_id, form_version_id);
CREATE INDEX ix_assess_assessments_activity_id ON assess.assessments (tenant_id, activity_id);
CREATE INDEX ix_assess_assessments_previous_id ON assess.assessments (tenant_id, previous_id);
CREATE INDEX ix_assess_assessments_owner_user_id ON assess.assessments (tenant_id, owner_user_id);
CREATE INDEX ix_assess_sections_assessment_id ON assess.sections (tenant_id, assessment_id);
CREATE INDEX ix_assess_sections_assignee_user_id ON assess.sections (tenant_id, assignee_user_id);
CREATE INDEX ix_assess_sections_guest_token_id ON assess.sections (tenant_id, guest_token_id);
CREATE INDEX ix_assess_answers_assessment_id ON assess.answers (tenant_id, assessment_id);
CREATE INDEX ix_assess_answers_section_id ON assess.answers (tenant_id, section_id);
CREATE INDEX ix_assess_answers_confirmed_by ON assess.answers (tenant_id, confirmed_by);
CREATE INDEX ix_assess_assessment_risks_risk_id ON assess.assessment_risks (tenant_id, risk_id);
CREATE INDEX ix_assess_dpo_opinions_assessment_id ON assess.dpo_opinions (tenant_id, assessment_id);
CREATE INDEX ix_assess_dpo_opinions_dpo_user_id ON assess.dpo_opinions (tenant_id, dpo_user_id);
CREATE INDEX ix_assess_consultations_assessment_id ON assess.consultations (tenant_id, assessment_id);

-- row-level security (tenant isolation)
ALTER TABLE assess.templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON assess.templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON assess.templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.screening_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.screening_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.screening_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.assessments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.assessments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.sections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.sections USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.answers ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.answers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.answers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.assessment_risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.assessment_risks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.assessment_risks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.dpo_opinions ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.dpo_opinions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.dpo_opinions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.consultations ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.consultations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.consultations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_templates_updated BEFORE UPDATE ON assess.templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_screening_rules_updated BEFORE UPDATE ON assess.screening_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_assessments_updated BEFORE UPDATE ON assess.assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sections_updated BEFORE UPDATE ON assess.sections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_answers_updated BEFORE UPDATE ON assess.answers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_dpo_opinions_updated BEFORE UPDATE ON assess.dpo_opinions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_consultations_updated BEFORE UPDATE ON assess.consultations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE assess.templates IS 'template แบบประเมิน (DPIA / LIA / TIA / AI / security / maturity ฯลฯ)';
COMMENT ON TABLE assess.screening_rules IS 'เกณฑ์คัดกรอง / บังคับทำ DPIA';
COMMENT ON TABLE assess.assessments IS 'แบบประเมินแต่ละครั้ง';
COMMENT ON TABLE assess.sections IS 'ส่วนของแบบประเมินที่มอบหมายผู้ตอบ';
COMMENT ON TABLE assess.answers IS 'คำตอบรายข้อ + หลักฐาน';
COMMENT ON TABLE assess.assessment_risks IS 'ความเสี่ยงที่ระบุในแบบประเมิน';
COMMENT ON TABLE assess.dpo_opinions IS 'ความเห็นของ DPO';
COMMENT ON TABLE assess.consultations IS 'บันทึกการปรึกษาผู้มีส่วนได้เสีย';

-- +goose Down
DROP TABLE IF EXISTS assess.consultations;
DROP TABLE IF EXISTS assess.dpo_opinions;
DROP TABLE IF EXISTS assess.assessment_risks;
DROP TABLE IF EXISTS assess.answers;
DROP TABLE IF EXISTS assess.sections;
DROP TABLE IF EXISTS assess.assessments;
DROP TABLE IF EXISTS assess.screening_rules;
DROP TABLE IF EXISTS assess.templates;
