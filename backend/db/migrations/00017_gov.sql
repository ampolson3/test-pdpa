-- +goose Up
-- schema gov: อบรม นโยบาย การตรวจประเมิน retention การติดต่อ สคส. และทะเบียน AI
-- 15 tables · docs: docs/data/gov.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE gov.courses (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    title text NOT NULL,
    description text,
    content_type text NOT NULL CHECK (content_type IN ('video', 'slides', 'scorm', 'xapi')),
    content_file_id uuid,
    quiz_form_id uuid,
    passing_score smallint NOT NULL DEFAULT 80,
    validity_months smallint NOT NULL DEFAULT 12,
    language varchar(5) NOT NULL DEFAULT 'th',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_courses PRIMARY KEY (id)
);

CREATE TABLE gov.training_assignments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    course_id uuid NOT NULL,
    target_type text NOT NULL CHECK (target_type IN ('all', 'org_unit', 'group', 'user')),
    target_id uuid,
    due_at date NOT NULL,
    assigned_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_training_assignments PRIMARY KEY (id)
);

CREATE TABLE gov.training_attempts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    course_id uuid NOT NULL,
    assignment_id uuid,
    user_id uuid NOT NULL,
    started_at timestamptz NOT NULL,
    completed_at timestamptz,
    score smallint,
    passed boolean,
    certificate_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_training_attempts PRIMARY KEY (id)
);

CREATE TABLE gov.policies (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    document_id uuid NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    requires_attestation boolean NOT NULL DEFAULT true,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_policies PRIMARY KEY (id),
    CONSTRAINT uq_policies_document_id_version_no UNIQUE (tenant_id, document_id, version_no)
);

CREATE TABLE gov.policy_attestations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    policy_id uuid NOT NULL,
    policy_version_no int NOT NULL,
    user_id uuid NOT NULL,
    attested_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    CONSTRAINT pk_gov_policy_attestations PRIMARY KEY (id)
);

CREATE TABLE gov.audits (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    audit_type text NOT NULL CHECK (audit_type IN ('maturity', 'pdpa_compliance', 'security', 'related_law')),
    legal_entity_id uuid,
    assessment_id uuid NOT NULL,
    period varchar(20) NOT NULL,
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'in_progress', 'completed')),
    overall_score numeric(5,2),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_audits PRIMARY KEY (id)
);

CREATE TABLE gov.audit_findings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    audit_id uuid NOT NULL,
    title text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    requirement_ref text,
    recommendation text,
    owner_user_id uuid,
    due_at date,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'accepted')),
    task_id uuid,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_audit_findings PRIMARY KEY (id)
);

CREATE TABLE gov.retention_schedules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    retention_rule_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    next_due_at date NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_retention_schedules PRIMARY KEY (id)
);

CREATE TABLE gov.disposal_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    due_at date NOT NULL,
    method text NOT NULL CHECK (method IN ('delete', 'destroy', 'anonymize', 'return')),
    legal_hold_id uuid,
    assignee_user_id uuid,
    approved_by uuid,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'in_progress', 'done', 'on_hold')),
    record_count int,
    completed_at timestamptz,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_disposal_jobs PRIMARY KEY (id)
);

CREATE TABLE gov.regulator_letters (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    direction text NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
    letter_no varchar(60),
    subject text NOT NULL,
    received_at date,
    due_at date,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'responded', 'closed')),
    file_id uuid,
    incident_id uuid,
    responded_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulator_letters PRIMARY KEY (id)
);

CREATE TABLE gov.regulatory_updates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    title text NOT NULL,
    source_url text,
    published_at date NOT NULL,
    summary text NOT NULL,
    impact_areas text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulatory_updates PRIMARY KEY (id)
);

CREATE TABLE gov.regulatory_reviews (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    update_id uuid NOT NULL,
    assessment_id uuid,
    status text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewing', 'done', 'not_applicable')),
    reviewed_by uuid,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulatory_reviews PRIMARY KEY (id)
);

CREATE TABLE gov.ai_systems (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    vendor_party_id uuid,
    purpose text NOT NULL,
    model_type varchar(60),
    data_category_ids uuid[] NOT NULL DEFAULT '{}',
    automated_decision boolean NOT NULL DEFAULT false,
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'unacceptable')),
    assessment_id uuid,
    owner_user_id uuid,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('planned', 'active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_ai_systems PRIMARY KEY (id)
);

CREATE TABLE gov.masking_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    source_file_id uuid NOT NULL,
    output_file_id uuid,
    rules jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    requested_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_masking_jobs PRIMARY KEY (id)
);

CREATE TABLE gov.kb_chunks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    kb_article_id uuid NOT NULL,
    chunk_no int NOT NULL,
    content text NOT NULL,
    embedding vector(1024) NOT NULL,
    CONSTRAINT pk_gov_kb_chunks PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_gov_courses_content_file_id ON gov.courses (content_file_id);
CREATE INDEX ix_gov_courses_quiz_form_id ON gov.courses (quiz_form_id);
CREATE INDEX ix_gov_training_assignments_course_id ON gov.training_assignments (tenant_id, course_id);
CREATE INDEX ix_gov_training_assignments_assigned_by ON gov.training_assignments (tenant_id, assigned_by);
CREATE INDEX ix_gov_training_attempts_course_id ON gov.training_attempts (tenant_id, course_id);
CREATE INDEX ix_gov_training_attempts_assignment_id ON gov.training_attempts (tenant_id, assignment_id);
CREATE INDEX ix_gov_training_attempts_user_id ON gov.training_attempts (tenant_id, user_id);
CREATE INDEX ix_gov_training_attempts_certificate_file_id ON gov.training_attempts (tenant_id, certificate_file_id);
CREATE INDEX ix_gov_policies_document_id ON gov.policies (tenant_id, document_id);
CREATE INDEX ix_gov_policy_attestations_policy_id ON gov.policy_attestations (tenant_id, policy_id);
CREATE INDEX ix_gov_policy_attestations_user_id ON gov.policy_attestations (tenant_id, user_id);
CREATE INDEX ix_gov_audits_legal_entity_id ON gov.audits (tenant_id, legal_entity_id);
CREATE INDEX ix_gov_audits_assessment_id ON gov.audits (tenant_id, assessment_id);
CREATE INDEX ix_gov_audit_findings_audit_id ON gov.audit_findings (tenant_id, audit_id);
CREATE INDEX ix_gov_audit_findings_owner_user_id ON gov.audit_findings (tenant_id, owner_user_id);
CREATE INDEX ix_gov_audit_findings_task_id ON gov.audit_findings (tenant_id, task_id);
CREATE INDEX ix_gov_audit_findings_evidence_file_id ON gov.audit_findings (tenant_id, evidence_file_id);
CREATE INDEX ix_gov_retention_schedules_retention_rule_id ON gov.retention_schedules (tenant_id, retention_rule_id);
CREATE INDEX ix_gov_retention_schedules_asset_id ON gov.retention_schedules (tenant_id, asset_id);
CREATE INDEX ix_gov_retention_schedules_next_due_at ON gov.retention_schedules (tenant_id, next_due_at);
CREATE INDEX ix_gov_disposal_jobs_schedule_id ON gov.disposal_jobs (tenant_id, schedule_id);
CREATE INDEX ix_gov_disposal_jobs_legal_hold_id ON gov.disposal_jobs (tenant_id, legal_hold_id);
CREATE INDEX ix_gov_disposal_jobs_assignee_user_id ON gov.disposal_jobs (tenant_id, assignee_user_id);
CREATE INDEX ix_gov_disposal_jobs_approved_by ON gov.disposal_jobs (tenant_id, approved_by);
CREATE INDEX ix_gov_disposal_jobs_evidence_file_id ON gov.disposal_jobs (tenant_id, evidence_file_id);
CREATE INDEX ix_gov_regulator_letters_file_id ON gov.regulator_letters (tenant_id, file_id);
CREATE INDEX ix_gov_regulator_letters_incident_id ON gov.regulator_letters (tenant_id, incident_id);
CREATE INDEX ix_gov_regulatory_reviews_update_id ON gov.regulatory_reviews (tenant_id, update_id);
CREATE INDEX ix_gov_regulatory_reviews_assessment_id ON gov.regulatory_reviews (tenant_id, assessment_id);
CREATE INDEX ix_gov_regulatory_reviews_reviewed_by ON gov.regulatory_reviews (tenant_id, reviewed_by);
CREATE INDEX ix_gov_ai_systems_vendor_party_id ON gov.ai_systems (tenant_id, vendor_party_id);
CREATE INDEX ix_gov_ai_systems_assessment_id ON gov.ai_systems (tenant_id, assessment_id);
CREATE INDEX ix_gov_ai_systems_owner_user_id ON gov.ai_systems (tenant_id, owner_user_id);
CREATE INDEX ix_gov_masking_jobs_source_file_id ON gov.masking_jobs (tenant_id, source_file_id);
CREATE INDEX ix_gov_masking_jobs_output_file_id ON gov.masking_jobs (tenant_id, output_file_id);
CREATE INDEX ix_gov_masking_jobs_requested_by ON gov.masking_jobs (tenant_id, requested_by);
CREATE INDEX ix_gov_kb_chunks_kb_article_id ON gov.kb_chunks (kb_article_id);

-- row-level security (tenant isolation)
ALTER TABLE gov.courses ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.courses FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON gov.courses FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON gov.courses FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.training_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.training_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.training_assignments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.training_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.training_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.training_attempts USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.policies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.policies USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.policy_attestations ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.policy_attestations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.policy_attestations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.audits ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.audits FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.audits USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.audit_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.audit_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.audit_findings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.retention_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.retention_schedules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.retention_schedules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.disposal_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.disposal_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.disposal_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.regulator_letters ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.regulator_letters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.regulator_letters USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.regulatory_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.regulatory_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.regulatory_reviews USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.ai_systems ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.ai_systems FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.ai_systems USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.masking_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.masking_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.masking_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.kb_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.kb_chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON gov.kb_chunks FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON gov.kb_chunks FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_courses_updated BEFORE UPDATE ON gov.courses FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_training_assignments_updated BEFORE UPDATE ON gov.training_assignments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_training_attempts_updated BEFORE UPDATE ON gov.training_attempts FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_policies_updated BEFORE UPDATE ON gov.policies FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_audits_updated BEFORE UPDATE ON gov.audits FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_audit_findings_updated BEFORE UPDATE ON gov.audit_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_retention_schedules_updated BEFORE UPDATE ON gov.retention_schedules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_disposal_jobs_updated BEFORE UPDATE ON gov.disposal_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulator_letters_updated BEFORE UPDATE ON gov.regulator_letters FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulatory_updates_updated BEFORE UPDATE ON gov.regulatory_updates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulatory_reviews_updated BEFORE UPDATE ON gov.regulatory_reviews FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_ai_systems_updated BEFORE UPDATE ON gov.ai_systems FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_masking_jobs_updated BEFORE UPDATE ON gov.masking_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE gov.courses IS 'คอร์สอบรม PDPA';
COMMENT ON TABLE gov.training_assignments IS 'การมอบหมายอบรม';
COMMENT ON TABLE gov.training_attempts IS 'ผลการเรียน / สอบรายบุคคล';
COMMENT ON TABLE gov.policies IS 'นโยบาย / คู่มือภายในที่ต้องรับทราบ';
COMMENT ON TABLE gov.policy_attestations IS 'การรับทราบนโยบาย';
COMMENT ON TABLE gov.audits IS 'การตรวจประเมินความพร้อม PDPA ระดับองค์กร';
COMMENT ON TABLE gov.audit_findings IS 'ข้อตรวจพบและแผนแก้ไข';
COMMENT ON TABLE gov.retention_schedules IS 'ตาราง retention ต่อระบบ (สร้างจาก RoPA)';
COMMENT ON TABLE gov.disposal_jobs IS 'งานลบ / ทำลาย / ทำให้ไม่ระบุตัวตน';
COMMENT ON TABLE gov.regulator_letters IS 'ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส.';
COMMENT ON TABLE gov.regulatory_updates IS 'ฟีดประกาศ / แนวปฏิบัติใหม่ (ทีมเนื้อหาผู้ให้บริการ)';
COMMENT ON TABLE gov.regulatory_reviews IS 'การประเมินผลกระทบของประกาศใหม่ต่อ tenant';
COMMENT ON TABLE gov.ai_systems IS 'ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล';
COMMENT ON TABLE gov.masking_jobs IS 'งาน mask / tokenize ข้อมูลสำหรับทดสอบ';
COMMENT ON TABLE gov.kb_chunks IS 'ชิ้นเอกสารสำหรับ RAG ของผู้ช่วย AI (pgvector)';

-- +goose Down
DROP TABLE IF EXISTS gov.kb_chunks;
DROP TABLE IF EXISTS gov.masking_jobs;
DROP TABLE IF EXISTS gov.ai_systems;
DROP TABLE IF EXISTS gov.regulatory_reviews;
DROP TABLE IF EXISTS gov.regulatory_updates;
DROP TABLE IF EXISTS gov.regulator_letters;
DROP TABLE IF EXISTS gov.disposal_jobs;
DROP TABLE IF EXISTS gov.retention_schedules;
DROP TABLE IF EXISTS gov.audit_findings;
DROP TABLE IF EXISTS gov.audits;
DROP TABLE IF EXISTS gov.policy_attestations;
DROP TABLE IF EXISTS gov.policies;
DROP TABLE IF EXISTS gov.training_attempts;
DROP TABLE IF EXISTS gov.training_assignments;
DROP TABLE IF EXISTS gov.courses;
