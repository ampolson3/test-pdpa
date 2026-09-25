-- +goose Up
-- schema agreement: ข้อตกลง DPA / DSA (agreement engine)
-- 10 tables · docs: docs/data/agreement.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE agreement.agreements (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_type text NOT NULL CHECK (agreement_type IN ('dpa', 'dsa', 'joint_controller', 'inbound_dpa')),
    agreement_no varchar(40) NOT NULL,
    title text NOT NULL,
    our_role text NOT NULL CHECK (our_role IN ('controller', 'processor', 'joint_controller')),
    counterparty_id uuid NOT NULL,
    vendor_id uuid,
    template_id uuid,
    document_id uuid NOT NULL,
    sharing_direction text CHECK (sharing_direction IN ('one_way', 'two_way')),
    is_government boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'out_for_signature', 'active', 'expired', 'terminated')),
    effective_from date,
    effective_to date,
    auto_renew boolean NOT NULL DEFAULT false,
    renewal_notice_days smallint NOT NULL DEFAULT 60,
    signed_at timestamptz,
    terminated_at timestamptz,
    termination_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_agreements PRIMARY KEY (id),
    CONSTRAINT uq_agreements_agreement_no UNIQUE (tenant_id, agreement_no)
);

CREATE TABLE agreement.parties (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    party_id uuid,
    legal_entity_id uuid,
    party_role text NOT NULL CHECK (party_role IN ('disclosing', 'receiving', 'joint_controller', 'controller', 'processor')),
    signatory_name text,
    signatory_email citext,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_parties PRIMARY KEY (id)
);

CREATE TABLE agreement.agreement_activities (
    agreement_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    recipient_id uuid,
    CONSTRAINT pk_agreement_agreement_activities PRIMARY KEY (agreement_id, activity_id)
);

CREATE TABLE agreement.clauses (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    clause_id uuid,
    clause_version_no int,
    position smallint NOT NULL,
    is_mandatory boolean NOT NULL DEFAULT false,
    customized_body jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_clauses PRIMARY KEY (id)
);

CREATE TABLE agreement.mandatory_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    agreement_type varchar(20) NOT NULL,
    clause_code varchar(80) NOT NULL,
    legal_ref text NOT NULL,
    condition jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_mandatory_rules PRIMARY KEY (id)
);

CREATE TABLE agreement.annexes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    annex_type text NOT NULL CHECK (annex_type IN ('processing_schedule', 'data_list', 'security_measures', 'request_form', 'flow_diagram', 'transfer_clauses')),
    content jsonb,
    file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_annexes PRIMARY KEY (id)
);

CREATE TABLE agreement.signature_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    provider varchar(40) NOT NULL,
    envelope_id varchar(120) NOT NULL,
    status text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'viewed', 'signed', 'declined', 'expired')),
    sent_at timestamptz NOT NULL,
    completed_at timestamptz,
    signed_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_signature_requests PRIMARY KEY (id)
);

CREATE TABLE agreement.obligations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    clause_ref varchar(80),
    obligation_type text NOT NULL CHECK (obligation_type IN ('delete_on_termination', 'usage_report', 'periodic_review', 'breach_notice', 'audit_right', 'other')),
    next_due_at date,
    owner_user_id uuid,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'waived')),
    task_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_obligations PRIMARY KEY (id)
);

CREATE TABLE agreement.return_confirmations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    requested_at timestamptz NOT NULL,
    guest_token_id uuid,
    certificate_file_id uuid,
    confirmed_at timestamptz,
    status text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'received', 'overdue')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_agreement_return_confirmations PRIMARY KEY (id)
);

CREATE TABLE agreement.downloads (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    agreement_id uuid NOT NULL,
    document_version_id uuid NOT NULL,
    user_id uuid,
    downloaded_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    CONSTRAINT pk_agreement_downloads PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_agreement_agreements_counterparty_id ON agreement.agreements (tenant_id, counterparty_id);
CREATE INDEX ix_agreement_agreements_vendor_id ON agreement.agreements (tenant_id, vendor_id);
CREATE INDEX ix_agreement_agreements_template_id ON agreement.agreements (tenant_id, template_id);
CREATE INDEX ix_agreement_agreements_document_id ON agreement.agreements (tenant_id, document_id);
CREATE INDEX ix_agreement_agreements_effective_to ON agreement.agreements (tenant_id, effective_to);
CREATE INDEX ix_agreement_parties_agreement_id ON agreement.parties (tenant_id, agreement_id);
CREATE INDEX ix_agreement_parties_party_id ON agreement.parties (tenant_id, party_id);
CREATE INDEX ix_agreement_parties_legal_entity_id ON agreement.parties (tenant_id, legal_entity_id);
CREATE INDEX ix_agreement_agreement_activities_activity_id ON agreement.agreement_activities (tenant_id, activity_id);
CREATE INDEX ix_agreement_agreement_activities_recipient_id ON agreement.agreement_activities (tenant_id, recipient_id);
CREATE INDEX ix_agreement_clauses_agreement_id ON agreement.clauses (tenant_id, agreement_id);
CREATE INDEX ix_agreement_clauses_clause_id ON agreement.clauses (tenant_id, clause_id);
CREATE INDEX ix_agreement_annexes_agreement_id ON agreement.annexes (tenant_id, agreement_id);
CREATE INDEX ix_agreement_annexes_file_id ON agreement.annexes (tenant_id, file_id);
CREATE INDEX ix_agreement_signature_requests_agreement_id ON agreement.signature_requests (tenant_id, agreement_id);
CREATE INDEX ix_agreement_signature_requests_signed_file_id ON agreement.signature_requests (tenant_id, signed_file_id);
CREATE INDEX ix_agreement_obligations_agreement_id ON agreement.obligations (tenant_id, agreement_id);
CREATE INDEX ix_agreement_obligations_next_due_at ON agreement.obligations (tenant_id, next_due_at);
CREATE INDEX ix_agreement_obligations_owner_user_id ON agreement.obligations (tenant_id, owner_user_id);
CREATE INDEX ix_agreement_obligations_task_id ON agreement.obligations (tenant_id, task_id);
CREATE INDEX ix_agreement_return_confirmations_agreement_id ON agreement.return_confirmations (tenant_id, agreement_id);
CREATE INDEX ix_agreement_return_confirmations_guest_token_id ON agreement.return_confirmations (tenant_id, guest_token_id);
CREATE INDEX ix_agreement_return_confirmations_certificate_file_id ON agreement.return_confirmations (tenant_id, certificate_file_id);
CREATE INDEX ix_agreement_downloads_agreement_id ON agreement.downloads (tenant_id, agreement_id);
CREATE INDEX ix_agreement_downloads_document_version_id ON agreement.downloads (tenant_id, document_version_id);
CREATE INDEX ix_agreement_downloads_user_id ON agreement.downloads (tenant_id, user_id);

-- row-level security (tenant isolation)
ALTER TABLE agreement.agreements ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.agreements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.agreements USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.parties FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.parties USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.agreement_activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.agreement_activities FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.agreement_activities USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.clauses ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.clauses FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.clauses USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.annexes ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.annexes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.annexes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.signature_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.signature_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.signature_requests USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.obligations ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.obligations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.obligations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.return_confirmations ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.return_confirmations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.return_confirmations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE agreement.downloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE agreement.downloads FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agreement.downloads USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_agreements_updated BEFORE UPDATE ON agreement.agreements FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_parties_updated BEFORE UPDATE ON agreement.parties FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_clauses_updated BEFORE UPDATE ON agreement.clauses FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_mandatory_rules_updated BEFORE UPDATE ON agreement.mandatory_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_annexes_updated BEFORE UPDATE ON agreement.annexes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_signature_requests_updated BEFORE UPDATE ON agreement.signature_requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_obligations_updated BEFORE UPDATE ON agreement.obligations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_return_confirmations_updated BEFORE UPDATE ON agreement.return_confirmations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE agreement.agreements IS 'ข้อตกลง DPA / DSA / ผู้ควบคุมร่วม / DPA ขาเข้า';
COMMENT ON TABLE agreement.parties IS 'คู่สัญญาในข้อตกลง (หลายฝ่าย)';
COMMENT ON TABLE agreement.agreement_activities IS 'กิจกรรม RoPA และเส้นทางข้อมูลที่ข้อตกลงครอบคลุม';
COMMENT ON TABLE agreement.clauses IS 'clause ในข้อตกลง (อ้างอิงคลังกลาง + ข้อความที่ปรับ)';
COMMENT ON TABLE agreement.mandatory_rules IS 'กฎ clause บังคับ (ม.27, ม.28-29, ม.37(2), ม.40)';
COMMENT ON TABLE agreement.annexes IS 'ภาคผนวก (รายละเอียดการประมวลผล รายการข้อมูล มาตรการ แผนผัง)';
COMMENT ON TABLE agreement.signature_requests IS 'การส่งลงนามอิเล็กทรอนิกส์';
COMMENT ON TABLE agreement.obligations IS 'ภาระผูกพันตามข้อตกลง';
COMMENT ON TABLE agreement.return_confirmations IS 'ใบยืนยันการคืน / ทำลายข้อมูลเมื่อสิ้นสุด';
COMMENT ON TABLE agreement.downloads IS 'ประวัติการดาวน์โหลดเอกสารสัญญา';

-- +goose Down
DROP TABLE IF EXISTS agreement.downloads;
DROP TABLE IF EXISTS agreement.return_confirmations;
DROP TABLE IF EXISTS agreement.obligations;
DROP TABLE IF EXISTS agreement.signature_requests;
DROP TABLE IF EXISTS agreement.annexes;
DROP TABLE IF EXISTS agreement.mandatory_rules;
DROP TABLE IF EXISTS agreement.clauses;
DROP TABLE IF EXISTS agreement.agreement_activities;
DROP TABLE IF EXISTS agreement.parties;
DROP TABLE IF EXISTS agreement.agreements;
