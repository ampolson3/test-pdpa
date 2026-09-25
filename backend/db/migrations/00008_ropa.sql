-- +goose Up
-- schema ropa: RoPA ทะเบียนข้อมูล asset การโอน retention และคลังกิจกรรมมาตรฐาน
-- 17 tables · docs: docs/data/ropa.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE ropa.processing_activities (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    code varchar(40) NOT NULL,
    name text NOT NULL,
    description text,
    role text NOT NULL CHECK (role IN ('controller', 'processor')),
    controller_party_id uuid,
    owner_user_id uuid,
    template_id uuid,
    template_version_no int,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'active', 'under_review', 'ended')),
    completeness smallint NOT NULL DEFAULT 0,
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'very_high')),
    rights_and_access text,
    approved_by uuid,
    approved_at timestamptz,
    next_review_at date,
    ended_at timestamptz,
    end_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_processing_activities PRIMARY KEY (id),
    CONSTRAINT uq_processing_activities_code UNIQUE (tenant_id, code)
);

CREATE TABLE ropa.activity_purposes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    purpose_id uuid,
    purpose_text text NOT NULL,
    lawful_basis_code varchar(20) NOT NULL,
    consent_purpose_id uuid,
    lia_assessment_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_purposes PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_data (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    data_category_id uuid NOT NULL,
    subject_type_id uuid NOT NULL,
    source text NOT NULL CHECK (source IN ('direct', 'indirect')),
    source_party_id uuid,
    is_sensitive boolean NOT NULL DEFAULT false,
    volume_band text CHECK (volume_band IN ('lt_1k', '1k_10k', '10k_100k', 'gt_100k')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_data PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_systems (
    activity_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    usage text NOT NULL CHECK (usage IN ('collect', 'store', 'process', 'transfer', 'archive')),
    CONSTRAINT pk_ropa_activity_systems PRIMARY KEY (activity_id, asset_id)
);

CREATE TABLE ropa.activity_recipients (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    party_id uuid NOT NULL,
    recipient_role text NOT NULL CHECK (recipient_role IN ('processor', 'controller', 'joint_controller', 'government')),
    disclosure_basis varchar(40),
    data_category_ids uuid[] NOT NULL DEFAULT '{}',
    agreement_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_recipients PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_transfers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    recipient_id uuid,
    country_code char(2) NOT NULL,
    transfer_basis text NOT NULL CHECK (transfer_basis IN ('adequacy', 'bcr', 'standard_clauses', 'certification', 'exemption', 'consent')),
    safeguards text,
    tia_assessment_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_transfers PRIMARY KEY (id)
);

CREATE TABLE ropa.retention_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    data_category_id uuid,
    retention_months int,
    retention_basis text NOT NULL,
    trigger_event varchar(60) NOT NULL,
    disposal_method text NOT NULL CHECK (disposal_method IN ('delete', 'destroy', 'anonymize', 'return')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_retention_rules PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_controls (
    activity_id uuid NOT NULL,
    control_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    description text,
    assessment_id uuid,
    CONSTRAINT pk_ropa_activity_controls PRIMARY KEY (activity_id, control_id)
);

CREATE TABLE ropa.activity_rejections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    dsar_request_id uuid NOT NULL,
    reason_code varchar(40) NOT NULL,
    rejected_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_rejections PRIMARY KEY (id)
);

CREATE TABLE ropa.assets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    asset_type text NOT NULL CHECK (asset_type IN ('application', 'database', 'file_share', 'saas', 'paper', 'device', 'other')),
    org_unit_id uuid,
    owner_user_id uuid,
    provider_party_id uuid,
    hosting_country_code char(2),
    hosting_type text CHECK (hosting_type IN ('on_prem', 'cloud', 'hybrid')),
    classification text CHECK (classification IN ('public', 'internal', 'confidential', 'restricted')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_assets PRIMARY KEY (id)
);

CREATE TABLE ropa.data_inventory (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    data_category_id uuid NOT NULL,
    org_unit_id uuid,
    owner_user_id uuid,
    source text CHECK (source IN ('direct', 'indirect', 'derived')),
    location_detail text,
    discovered_by_finding_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_data_inventory PRIMARY KEY (id)
);

CREATE TABLE ropa.questionnaires (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    activity_id uuid,
    form_submission_id uuid,
    respondent_user_id uuid NOT NULL,
    due_at timestamptz,
    status text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'answered', 'approved', 'rejected')),
    approved_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_questionnaires PRIMARY KEY (id)
);

CREATE TABLE ropa.sme_exemption_checks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    result text NOT NULL CHECK (result IN ('exempt', 'not_exempt', 'partial')),
    basis text NOT NULL,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_sme_exemption_checks PRIMARY KEY (id)
);

CREATE TABLE ropa.template_sets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    name text NOT NULL,
    set_type text NOT NULL CHECK (set_type IN ('standard', 'industry', 'government', 'processor', 'custom')),
    industry varchar(40),
    version_no int NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_template_sets PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    template_set_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    job_category varchar(40) NOT NULL,
    role text NOT NULL CHECK (role IN ('controller', 'processor')),
    defaults jsonb NOT NULL,
    rationale jsonb NOT NULL,
    legal_refs text[] NOT NULL DEFAULT '{}',
    version_no int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_templates PRIMARY KEY (id),
    CONSTRAINT uq_activity_templates_template_set_id_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, template_set_id, code, version_no)
);

CREATE TABLE ropa.generation_runs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    template_ids uuid[] NOT NULL,
    created_activity_ids uuid[] NOT NULL DEFAULT '{}',
    task_ids uuid[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_generation_runs PRIMARY KEY (id)
);

CREATE TABLE ropa.wizard_sessions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id uuid NOT NULL,
    answers jsonb NOT NULL,
    mapped_fields jsonb,
    ai_suggestions jsonb,
    activity_id uuid,
    status text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'abandoned')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_wizard_sessions PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_ropa_processing_activities_legal_entity_id ON ropa.processing_activities (tenant_id, legal_entity_id);
CREATE INDEX ix_ropa_processing_activities_org_unit_id ON ropa.processing_activities (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_processing_activities_controller_party_id ON ropa.processing_activities (tenant_id, controller_party_id);
CREATE INDEX ix_ropa_processing_activities_owner_user_id ON ropa.processing_activities (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_processing_activities_template_id ON ropa.processing_activities (tenant_id, template_id);
CREATE INDEX ix_ropa_processing_activities_approved_by ON ropa.processing_activities (tenant_id, approved_by);
CREATE INDEX ix_ropa_activity_purposes_activity_id ON ropa.activity_purposes (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_purposes_purpose_id ON ropa.activity_purposes (tenant_id, purpose_id);
CREATE INDEX ix_ropa_activity_purposes_lawful_basis_code ON ropa.activity_purposes (tenant_id, lawful_basis_code);
CREATE INDEX ix_ropa_activity_purposes_consent_purpose_id ON ropa.activity_purposes (tenant_id, consent_purpose_id);
CREATE INDEX ix_ropa_activity_purposes_lia_assessment_id ON ropa.activity_purposes (tenant_id, lia_assessment_id);
CREATE INDEX ix_ropa_activity_data_activity_id ON ropa.activity_data (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_data_data_category_id ON ropa.activity_data (tenant_id, data_category_id);
CREATE INDEX ix_ropa_activity_data_subject_type_id ON ropa.activity_data (tenant_id, subject_type_id);
CREATE INDEX ix_ropa_activity_data_source_party_id ON ropa.activity_data (tenant_id, source_party_id);
CREATE INDEX ix_ropa_activity_systems_asset_id ON ropa.activity_systems (tenant_id, asset_id);
CREATE INDEX ix_ropa_activity_recipients_activity_id ON ropa.activity_recipients (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_recipients_party_id ON ropa.activity_recipients (tenant_id, party_id);
CREATE INDEX ix_ropa_activity_recipients_agreement_id ON ropa.activity_recipients (tenant_id, agreement_id);
CREATE INDEX ix_ropa_activity_transfers_activity_id ON ropa.activity_transfers (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_transfers_recipient_id ON ropa.activity_transfers (tenant_id, recipient_id);
CREATE INDEX ix_ropa_activity_transfers_country_code ON ropa.activity_transfers (tenant_id, country_code);
CREATE INDEX ix_ropa_activity_transfers_tia_assessment_id ON ropa.activity_transfers (tenant_id, tia_assessment_id);
CREATE INDEX ix_ropa_retention_rules_activity_id ON ropa.retention_rules (tenant_id, activity_id);
CREATE INDEX ix_ropa_retention_rules_data_category_id ON ropa.retention_rules (tenant_id, data_category_id);
CREATE INDEX ix_ropa_activity_controls_control_id ON ropa.activity_controls (tenant_id, control_id);
CREATE INDEX ix_ropa_activity_controls_assessment_id ON ropa.activity_controls (tenant_id, assessment_id);
CREATE INDEX ix_ropa_activity_rejections_activity_id ON ropa.activity_rejections (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_rejections_dsar_request_id ON ropa.activity_rejections (tenant_id, dsar_request_id);
CREATE INDEX ix_ropa_assets_org_unit_id ON ropa.assets (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_assets_owner_user_id ON ropa.assets (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_assets_provider_party_id ON ropa.assets (tenant_id, provider_party_id);
CREATE INDEX ix_ropa_assets_hosting_country_code ON ropa.assets (tenant_id, hosting_country_code);
CREATE INDEX ix_ropa_data_inventory_asset_id ON ropa.data_inventory (tenant_id, asset_id);
CREATE INDEX ix_ropa_data_inventory_data_category_id ON ropa.data_inventory (tenant_id, data_category_id);
CREATE INDEX ix_ropa_data_inventory_org_unit_id ON ropa.data_inventory (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_data_inventory_owner_user_id ON ropa.data_inventory (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_data_inventory_discovered_by_finding_id ON ropa.data_inventory (tenant_id, discovered_by_finding_id);
CREATE INDEX ix_ropa_questionnaires_org_unit_id ON ropa.questionnaires (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_questionnaires_activity_id ON ropa.questionnaires (tenant_id, activity_id);
CREATE INDEX ix_ropa_questionnaires_form_submission_id ON ropa.questionnaires (tenant_id, form_submission_id);
CREATE INDEX ix_ropa_questionnaires_respondent_user_id ON ropa.questionnaires (tenant_id, respondent_user_id);
CREATE INDEX ix_ropa_questionnaires_approved_by ON ropa.questionnaires (tenant_id, approved_by);
CREATE INDEX ix_ropa_sme_exemption_checks_legal_entity_id ON ropa.sme_exemption_checks (tenant_id, legal_entity_id);
CREATE INDEX ix_ropa_sme_exemption_checks_form_submission_id ON ropa.sme_exemption_checks (tenant_id, form_submission_id);
CREATE INDEX ix_ropa_activity_templates_template_set_id ON ropa.activity_templates (template_set_id);
CREATE INDEX ix_ropa_generation_runs_org_unit_id ON ropa.generation_runs (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_wizard_sessions_user_id ON ropa.wizard_sessions (tenant_id, user_id);
CREATE INDEX ix_ropa_wizard_sessions_activity_id ON ropa.wizard_sessions (tenant_id, activity_id);

-- row-level security (tenant isolation)
ALTER TABLE ropa.processing_activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.processing_activities FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.processing_activities USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_purposes ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_purposes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_purposes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_data ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_data FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_data USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_systems ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_systems FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_systems USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_recipients USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_transfers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_transfers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.retention_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.retention_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.retention_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_controls FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_controls USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_rejections ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_rejections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_rejections USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.assets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.assets USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.data_inventory ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.data_inventory FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.data_inventory USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.questionnaires ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.questionnaires FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.questionnaires USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.sme_exemption_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.sme_exemption_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.sme_exemption_checks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.template_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.template_sets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ropa.template_sets FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON ropa.template_sets FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ropa.activity_templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON ropa.activity_templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.generation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.generation_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.generation_runs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.wizard_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.wizard_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.wizard_sessions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_processing_activities_updated BEFORE UPDATE ON ropa.processing_activities FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_purposes_updated BEFORE UPDATE ON ropa.activity_purposes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_data_updated BEFORE UPDATE ON ropa.activity_data FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_recipients_updated BEFORE UPDATE ON ropa.activity_recipients FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_transfers_updated BEFORE UPDATE ON ropa.activity_transfers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_retention_rules_updated BEFORE UPDATE ON ropa.retention_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_rejections_updated BEFORE UPDATE ON ropa.activity_rejections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_assets_updated BEFORE UPDATE ON ropa.assets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_inventory_updated BEFORE UPDATE ON ropa.data_inventory FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_questionnaires_updated BEFORE UPDATE ON ropa.questionnaires FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sme_exemption_checks_updated BEFORE UPDATE ON ropa.sme_exemption_checks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_template_sets_updated BEFORE UPDATE ON ropa.template_sets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_templates_updated BEFORE UPDATE ON ropa.activity_templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_generation_runs_updated BEFORE UPDATE ON ropa.generation_runs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_wizard_sessions_updated BEFORE UPDATE ON ropa.wizard_sessions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE ropa.processing_activities IS 'กิจกรรมการประมวลผล (RoPA ม.39 / ผู้ประมวลผล)';
COMMENT ON TABLE ropa.activity_purposes IS 'วัตถุประสงค์และฐานกฎหมายของกิจกรรม';
COMMENT ON TABLE ropa.activity_data IS 'ข้อมูลที่เก็บในกิจกรรม';
COMMENT ON TABLE ropa.activity_systems IS 'ระบบ / asset ที่ใช้ในกิจกรรม';
COMMENT ON TABLE ropa.activity_recipients IS 'ผู้รับข้อมูลและการเปิดเผย (ม.27)';
COMMENT ON TABLE ropa.activity_transfers IS 'การโอนไปต่างประเทศ (ม.28 / ม.29)';
COMMENT ON TABLE ropa.retention_rules IS 'ระยะเวลาเก็บรักษาและวิธีทำลาย';
COMMENT ON TABLE ropa.activity_controls IS 'มาตรการความปลอดภัยต่อกิจกรรม (ม.37(1))';
COMMENT ON TABLE ropa.activity_rejections IS 'การปฏิเสธคำขอใช้สิทธิที่เกี่ยวข้อง (ม.39(7))';
COMMENT ON TABLE ropa.assets IS 'ทะเบียนระบบ / asset';
COMMENT ON TABLE ropa.data_inventory IS 'ทะเบียนข้อมูลส่วนบุคคลต่อระบบ';
COMMENT ON TABLE ropa.questionnaires IS 'แบบสอบถามเก็บข้อมูลกิจกรรมจากหน่วยงาน';
COMMENT ON TABLE ropa.sme_exemption_checks IS 'ผลตรวจสิทธิ์ยกเว้น RoPA ของกิจการขนาดเล็ก';
COMMENT ON TABLE ropa.template_sets IS 'ชุด template (มาตรฐาน / อุตสาหกรรม / ภาครัฐ / ขององค์กร)';
COMMENT ON TABLE ropa.activity_templates IS 'กิจกรรมมาตรฐานพร้อมค่าตั้งต้นและเหตุผล';
COMMENT ON TABLE ropa.generation_runs IS 'การสร้างร่าง RoPA จาก template ครั้งละหลายกิจกรรม';
COMMENT ON TABLE ropa.wizard_sessions IS 'session ของ wizard ถาม-ตอบภาษาง่าย';

-- +goose Down
DROP TABLE IF EXISTS ropa.wizard_sessions;
DROP TABLE IF EXISTS ropa.generation_runs;
DROP TABLE IF EXISTS ropa.activity_templates;
DROP TABLE IF EXISTS ropa.template_sets;
DROP TABLE IF EXISTS ropa.sme_exemption_checks;
DROP TABLE IF EXISTS ropa.questionnaires;
DROP TABLE IF EXISTS ropa.data_inventory;
DROP TABLE IF EXISTS ropa.assets;
DROP TABLE IF EXISTS ropa.activity_rejections;
DROP TABLE IF EXISTS ropa.activity_controls;
DROP TABLE IF EXISTS ropa.retention_rules;
DROP TABLE IF EXISTS ropa.activity_transfers;
DROP TABLE IF EXISTS ropa.activity_recipients;
DROP TABLE IF EXISTS ropa.activity_systems;
DROP TABLE IF EXISTS ropa.activity_data;
DROP TABLE IF EXISTS ropa.activity_purposes;
DROP TABLE IF EXISTS ropa.processing_activities;
