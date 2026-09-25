-- +goose Up
-- schema dsar: คำขอใช้สิทธิของเจ้าของข้อมูล
-- 12 tables · docs: docs/data/dsar.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE dsar.request_types (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code text NOT NULL CHECK (code IN ('access', 'portability', 'objection', 'erasure', 'restriction', 'rectification', 'withdraw_consent', 'complaint', 'inquiry')),
    name_th text NOT NULL,
    legal_ref varchar(40),
    workflow_definition_id uuid,
    sla_days smallint NOT NULL DEFAULT 30,
    response_template_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_request_types PRIMARY KEY (id),
    CONSTRAINT uq_request_types_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE dsar.requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_no varchar(30) NOT NULL,
    request_type_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    channel text NOT NULL CHECK (channel IN ('web', 'email', 'phone', 'branch', 'letter', 'line', 'api')),
    subject_id uuid,
    requester_name_enc bytea NOT NULL,
    requester_contact_enc bytea NOT NULL,
    requester_blind_index bytea NOT NULL,
    on_behalf boolean NOT NULL DEFAULT false,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    form_submission_id uuid,
    workflow_instance_id uuid,
    status text NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'verifying', 'in_review', 'in_progress', 'awaiting_info', 'completed', 'rejected', 'withdrawn')),
    received_at timestamptz NOT NULL,
    due_at timestamptz NOT NULL,
    verified_at timestamptz,
    closed_at timestamptz,
    outcome text CHECK (outcome IN ('fulfilled', 'partially_fulfilled', 'rejected', 'withdrawn')),
    rejection_reason_code varchar(40),
    assignee_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_requests PRIMARY KEY (id),
    CONSTRAINT uq_requests_request_no UNIQUE (tenant_id, request_no)
);

CREATE TABLE dsar.agents (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    agent_name_enc bytea NOT NULL,
    authority_type text NOT NULL CHECK (authority_type IN ('power_of_attorney', 'parent', 'guardian', 'curator')),
    authority_file_id uuid NOT NULL,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_agents PRIMARY KEY (id)
);

CREATE TABLE dsar.verifications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    method text NOT NULL CHECK (method IN ('otp_sms', 'otp_email', 'id_document', 'in_person', 'idp', 'thaid')),
    subject_verification_id uuid,
    masked_id_file_id uuid,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'passed', 'failed')),
    verified_by uuid,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_verifications PRIMARY KEY (id)
);

CREATE TABLE dsar.subtasks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    asset_id uuid,
    action text NOT NULL CHECK (action IN ('search', 'export', 'delete', 'rectify', 'restrict', 'stop_marketing', 'review')),
    assignee_user_id uuid,
    assignee_group_id uuid,
    assignee_party_id uuid,
    guest_token_id uuid,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'not_applicable')),
    due_at timestamptz,
    completed_at timestamptz,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_subtasks PRIMARY KEY (id)
);

CREATE TABLE dsar.search_results (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    found boolean NOT NULL,
    record_count int,
    summary_enc bytea,
    result_file_id uuid,
    searched_at timestamptz NOT NULL,
    purged_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_search_results PRIMARY KEY (id)
);

CREATE TABLE dsar.legal_holds (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    data_category_id uuid,
    asset_id uuid,
    law_ref text NOT NULL,
    hold_rule jsonb NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_legal_holds PRIMARY KEY (id)
);

CREATE TABLE dsar.exemption_checks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    legal_hold_id uuid,
    result text NOT NULL CHECK (result IN ('can_delete', 'must_retain', 'partial')),
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_exemption_checks PRIMARY KEY (id)
);

CREATE TABLE dsar.packages (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    file_id uuid NOT NULL,
    format text NOT NULL CHECK (format IN ('pdf', 'csv', 'json', 'xml', 'zip')),
    token_hash char(64) NOT NULL,
    password_protected boolean NOT NULL DEFAULT true,
    expires_at timestamptz NOT NULL,
    download_count int NOT NULL DEFAULT 0,
    last_downloaded_at timestamptz,
    purged_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_packages PRIMARY KEY (id),
    CONSTRAINT uq_packages_token_hash UNIQUE (tenant_id, token_hash)
);

CREATE TABLE dsar.redactions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    source_file_id uuid NOT NULL,
    output_file_id uuid,
    regions jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'detecting' CHECK (status IN ('detecting', 'review', 'done')),
    reviewed_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_redactions PRIMARY KEY (id)
);

CREATE TABLE dsar.communications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    direction text NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    channel text NOT NULL CHECK (channel IN ('email', 'sms', 'portal', 'letter')),
    purpose text NOT NULL CHECK (purpose IN ('acknowledge', 'request_info', 'result', 'rejection', 'extension_notice', 'other')),
    document_version_id uuid,
    notification_id uuid,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_communications PRIMARY KEY (id)
);

CREATE TABLE dsar.downstream_notices (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    request_id uuid NOT NULL,
    party_id uuid NOT NULL,
    method text NOT NULL CHECK (method IN ('webhook', 'connector', 'guest_link', 'email')),
    status text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'confirmed', 'failed')),
    sent_at timestamptz,
    confirmed_at timestamptz,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_downstream_notices PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_dsar_request_types_workflow_definition_id ON dsar.request_types (workflow_definition_id);
CREATE INDEX ix_dsar_request_types_response_template_id ON dsar.request_types (response_template_id);
CREATE INDEX ix_dsar_requests_request_type_id ON dsar.requests (tenant_id, request_type_id);
CREATE INDEX ix_dsar_requests_legal_entity_id ON dsar.requests (tenant_id, legal_entity_id);
CREATE INDEX ix_dsar_requests_subject_id ON dsar.requests (tenant_id, subject_id);
CREATE INDEX ix_dsar_requests_requester_blind_index ON dsar.requests (tenant_id, requester_blind_index);
CREATE INDEX ix_dsar_requests_form_submission_id ON dsar.requests (tenant_id, form_submission_id);
CREATE INDEX ix_dsar_requests_workflow_instance_id ON dsar.requests (tenant_id, workflow_instance_id);
CREATE INDEX ix_dsar_requests_due_at ON dsar.requests (tenant_id, due_at);
CREATE INDEX ix_dsar_requests_assignee_user_id ON dsar.requests (tenant_id, assignee_user_id);
CREATE INDEX ix_dsar_agents_request_id ON dsar.agents (tenant_id, request_id);
CREATE INDEX ix_dsar_agents_authority_file_id ON dsar.agents (tenant_id, authority_file_id);
CREATE INDEX ix_dsar_verifications_request_id ON dsar.verifications (tenant_id, request_id);
CREATE INDEX ix_dsar_verifications_subject_verification_id ON dsar.verifications (tenant_id, subject_verification_id);
CREATE INDEX ix_dsar_verifications_masked_id_file_id ON dsar.verifications (tenant_id, masked_id_file_id);
CREATE INDEX ix_dsar_verifications_verified_by ON dsar.verifications (tenant_id, verified_by);
CREATE INDEX ix_dsar_subtasks_request_id ON dsar.subtasks (tenant_id, request_id);
CREATE INDEX ix_dsar_subtasks_asset_id ON dsar.subtasks (tenant_id, asset_id);
CREATE INDEX ix_dsar_subtasks_assignee_user_id ON dsar.subtasks (tenant_id, assignee_user_id);
CREATE INDEX ix_dsar_subtasks_assignee_group_id ON dsar.subtasks (tenant_id, assignee_group_id);
CREATE INDEX ix_dsar_subtasks_assignee_party_id ON dsar.subtasks (tenant_id, assignee_party_id);
CREATE INDEX ix_dsar_subtasks_guest_token_id ON dsar.subtasks (tenant_id, guest_token_id);
CREATE INDEX ix_dsar_subtasks_evidence_file_id ON dsar.subtasks (tenant_id, evidence_file_id);
CREATE INDEX ix_dsar_search_results_request_id ON dsar.search_results (tenant_id, request_id);
CREATE INDEX ix_dsar_search_results_connector_id ON dsar.search_results (tenant_id, connector_id);
CREATE INDEX ix_dsar_search_results_result_file_id ON dsar.search_results (tenant_id, result_file_id);
CREATE INDEX ix_dsar_legal_holds_data_category_id ON dsar.legal_holds (tenant_id, data_category_id);
CREATE INDEX ix_dsar_legal_holds_asset_id ON dsar.legal_holds (tenant_id, asset_id);
CREATE INDEX ix_dsar_exemption_checks_request_id ON dsar.exemption_checks (tenant_id, request_id);
CREATE INDEX ix_dsar_exemption_checks_legal_hold_id ON dsar.exemption_checks (tenant_id, legal_hold_id);
CREATE INDEX ix_dsar_packages_request_id ON dsar.packages (tenant_id, request_id);
CREATE INDEX ix_dsar_packages_file_id ON dsar.packages (tenant_id, file_id);
CREATE INDEX ix_dsar_redactions_request_id ON dsar.redactions (tenant_id, request_id);
CREATE INDEX ix_dsar_redactions_source_file_id ON dsar.redactions (tenant_id, source_file_id);
CREATE INDEX ix_dsar_redactions_output_file_id ON dsar.redactions (tenant_id, output_file_id);
CREATE INDEX ix_dsar_redactions_reviewed_by ON dsar.redactions (tenant_id, reviewed_by);
CREATE INDEX ix_dsar_communications_request_id ON dsar.communications (tenant_id, request_id);
CREATE INDEX ix_dsar_communications_document_version_id ON dsar.communications (tenant_id, document_version_id);
CREATE INDEX ix_dsar_communications_notification_id ON dsar.communications (tenant_id, notification_id);
CREATE INDEX ix_dsar_downstream_notices_request_id ON dsar.downstream_notices (tenant_id, request_id);
CREATE INDEX ix_dsar_downstream_notices_party_id ON dsar.downstream_notices (tenant_id, party_id);
CREATE INDEX ix_dsar_downstream_notices_evidence_file_id ON dsar.downstream_notices (tenant_id, evidence_file_id);

-- row-level security (tenant isolation)
ALTER TABLE dsar.request_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.request_types FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON dsar.request_types FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON dsar.request_types FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.requests USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.agents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.agents USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.verifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.verifications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.subtasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.subtasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.subtasks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.search_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.search_results FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.search_results USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.legal_holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.legal_holds FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.legal_holds USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.exemption_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.exemption_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.exemption_checks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.packages ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.packages FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.packages USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.redactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.redactions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.redactions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.communications ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.communications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.communications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dsar.downstream_notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.downstream_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.downstream_notices USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_request_types_updated BEFORE UPDATE ON dsar.request_types FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_requests_updated BEFORE UPDATE ON dsar.requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_agents_updated BEFORE UPDATE ON dsar.agents FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_verifications_updated BEFORE UPDATE ON dsar.verifications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_subtasks_updated BEFORE UPDATE ON dsar.subtasks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_search_results_updated BEFORE UPDATE ON dsar.search_results FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_legal_holds_updated BEFORE UPDATE ON dsar.legal_holds FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_exemption_checks_updated BEFORE UPDATE ON dsar.exemption_checks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_packages_updated BEFORE UPDATE ON dsar.packages FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_redactions_updated BEFORE UPDATE ON dsar.redactions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_communications_updated BEFORE UPDATE ON dsar.communications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_downstream_notices_updated BEFORE UPDATE ON dsar.downstream_notices FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE dsar.request_types IS 'ประเภทคำขอ (ม.19, ม.30-36, ร้องเรียน, สอบถาม)';
COMMENT ON TABLE dsar.requests IS 'คำขอใช้สิทธิ';
COMMENT ON TABLE dsar.agents IS 'ผู้ยื่นแทน / ผู้ใช้อำนาจปกครอง (ม.20)';
COMMENT ON TABLE dsar.verifications IS 'การยืนยันตัวตนของผู้ยื่น';
COMMENT ON TABLE dsar.subtasks IS 'งานย่อยต่อระบบ / ทีม / ผู้ประมวลผล';
COMMENT ON TABLE dsar.search_results IS 'ผลค้นหาข้อมูลข้ามระบบ (เข้ารหัส ลบเมื่อปิดคำขอ)';
COMMENT ON TABLE dsar.legal_holds IS 'ข้อมูลที่ต้องเก็บตามกฎหมายอื่น (ข้อยกเว้น ม.33)';
COMMENT ON TABLE dsar.exemption_checks IS 'ผลตรวจข้อยกเว้นก่อนลบ';
COMMENT ON TABLE dsar.packages IS 'แพ็กเกจข้อมูลที่ส่งคืน (ลิงก์หมดอายุ + รหัสผ่าน)';
COMMENT ON TABLE dsar.redactions IS 'งานปกปิดข้อมูลบุคคลอื่นในเอกสาร';
COMMENT ON TABLE dsar.communications IS 'การติดต่อกับผู้ยื่น (หนังสือตอบ / ขอข้อมูลเพิ่ม)';
COMMENT ON TABLE dsar.downstream_notices IS 'การแจ้งผู้รับข้อมูลให้ดำเนินการตามคำขอ';

-- +goose Down
DROP TABLE IF EXISTS dsar.downstream_notices;
DROP TABLE IF EXISTS dsar.communications;
DROP TABLE IF EXISTS dsar.redactions;
DROP TABLE IF EXISTS dsar.packages;
DROP TABLE IF EXISTS dsar.exemption_checks;
DROP TABLE IF EXISTS dsar.legal_holds;
DROP TABLE IF EXISTS dsar.search_results;
DROP TABLE IF EXISTS dsar.subtasks;
DROP TABLE IF EXISTS dsar.verifications;
DROP TABLE IF EXISTS dsar.agents;
DROP TABLE IF EXISTS dsar.requests;
DROP TABLE IF EXISTS dsar.request_types;
