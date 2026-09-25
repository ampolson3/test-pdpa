-- +goose Up
-- schema vendor: คู่ค้าและผู้ประมวลผล
-- 7 tables · docs: docs/data/vendor.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE vendor.vendors (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    party_id uuid NOT NULL,
    service_description text NOT NULL,
    relationship_owner_id uuid,
    is_processor boolean NOT NULL DEFAULT true,
    tier text CHECK (tier IN ('low', 'medium', 'high', 'critical')),
    data_access jsonb NOT NULL DEFAULT '{}'::jsonb,
    processing_countries char(2)[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'prospect' CHECK (status IN ('prospect', 'onboarding', 'approved', 'conditional', 'rejected', 'offboarding', 'terminated')),
    next_assessment_at date,
    approved_at timestamptz,
    offboarded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_vendors PRIMARY KEY (id),
    CONSTRAINT uq_vendors_party_id UNIQUE (tenant_id, party_id)
);

CREATE TABLE vendor.intakes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    inherent_score numeric(6,2) NOT NULL,
    tier_result text NOT NULL CHECK (tier_result IN ('low', 'medium', 'high', 'critical')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_intakes PRIMARY KEY (id)
);

CREATE TABLE vendor.vendor_assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    cycle_no smallint NOT NULL DEFAULT 1,
    guest_token_id uuid,
    sent_at timestamptz,
    due_at date,
    submitted_at timestamptz,
    score numeric(6,2),
    residual_level text CHECK (residual_level IN ('low', 'medium', 'high', 'critical')),
    decision text CHECK (decision IN ('approved', 'conditional', 'rejected')),
    decided_by uuid,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_vendor_assessments PRIMARY KEY (id),
    CONSTRAINT uq_vendor_assessments_vendor_id_cycle_no UNIQUE (vendor_id, cycle_no)
);

CREATE TABLE vendor.sub_processors (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    party_id uuid NOT NULL,
    service text NOT NULL,
    countries char(2)[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    approved_by uuid,
    approved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_sub_processors PRIMARY KEY (id)
);

CREATE TABLE vendor.certificates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    cert_type varchar(40) NOT NULL,
    issuer text,
    file_id uuid NOT NULL,
    valid_from date,
    valid_to date,
    verified_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_certificates PRIMARY KEY (id)
);

CREATE TABLE vendor.remediation_items (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    vendor_assessment_id uuid,
    title text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    guest_token_id uuid,
    due_at date,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'accepted_risk')),
    closed_at timestamptz,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_remediation_items PRIMARY KEY (id)
);

CREATE TABLE vendor.offboardings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vendor_id uuid NOT NULL,
    data_return_status text NOT NULL CHECK (data_return_status IN ('pending', 'returned', 'destroyed', 'not_applicable')),
    destruction_certificate_file_id uuid,
    access_revoked_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_vendor_offboardings PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_vendor_vendors_party_id ON vendor.vendors (tenant_id, party_id);
CREATE INDEX ix_vendor_vendors_relationship_owner_id ON vendor.vendors (tenant_id, relationship_owner_id);
CREATE INDEX ix_vendor_intakes_vendor_id ON vendor.intakes (tenant_id, vendor_id);
CREATE INDEX ix_vendor_intakes_form_submission_id ON vendor.intakes (tenant_id, form_submission_id);
CREATE INDEX ix_vendor_vendor_assessments_vendor_id ON vendor.vendor_assessments (tenant_id, vendor_id);
CREATE INDEX ix_vendor_vendor_assessments_assessment_id ON vendor.vendor_assessments (tenant_id, assessment_id);
CREATE INDEX ix_vendor_vendor_assessments_guest_token_id ON vendor.vendor_assessments (tenant_id, guest_token_id);
CREATE INDEX ix_vendor_vendor_assessments_decided_by ON vendor.vendor_assessments (tenant_id, decided_by);
CREATE INDEX ix_vendor_sub_processors_vendor_id ON vendor.sub_processors (tenant_id, vendor_id);
CREATE INDEX ix_vendor_sub_processors_party_id ON vendor.sub_processors (tenant_id, party_id);
CREATE INDEX ix_vendor_sub_processors_approved_by ON vendor.sub_processors (tenant_id, approved_by);
CREATE INDEX ix_vendor_certificates_vendor_id ON vendor.certificates (tenant_id, vendor_id);
CREATE INDEX ix_vendor_certificates_file_id ON vendor.certificates (tenant_id, file_id);
CREATE INDEX ix_vendor_certificates_valid_to ON vendor.certificates (tenant_id, valid_to);
CREATE INDEX ix_vendor_certificates_verified_by ON vendor.certificates (tenant_id, verified_by);
CREATE INDEX ix_vendor_remediation_items_vendor_id ON vendor.remediation_items (tenant_id, vendor_id);
CREATE INDEX ix_vendor_remediation_items_vendor_assessment_id ON vendor.remediation_items (tenant_id, vendor_assessment_id);
CREATE INDEX ix_vendor_remediation_items_guest_token_id ON vendor.remediation_items (tenant_id, guest_token_id);
CREATE INDEX ix_vendor_remediation_items_evidence_file_id ON vendor.remediation_items (tenant_id, evidence_file_id);
CREATE INDEX ix_vendor_offboardings_vendor_id ON vendor.offboardings (tenant_id, vendor_id);
CREATE INDEX ix_vendor_offboardings_destruction_certificate_file_id ON vendor.offboardings (tenant_id, destruction_certificate_file_id);

-- row-level security (tenant isolation)
ALTER TABLE vendor.vendors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.vendors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.vendors USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.intakes ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.intakes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.intakes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.vendor_assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.vendor_assessments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.vendor_assessments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.sub_processors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.sub_processors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.sub_processors USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.certificates ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.certificates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.certificates USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.remediation_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.remediation_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.remediation_items USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE vendor.offboardings ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor.offboardings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendor.offboardings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_vendors_updated BEFORE UPDATE ON vendor.vendors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_intakes_updated BEFORE UPDATE ON vendor.intakes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_vendor_assessments_updated BEFORE UPDATE ON vendor.vendor_assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sub_processors_updated BEFORE UPDATE ON vendor.sub_processors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_certificates_updated BEFORE UPDATE ON vendor.certificates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_remediation_items_updated BEFORE UPDATE ON vendor.remediation_items FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_offboardings_updated BEFORE UPDATE ON vendor.offboardings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE vendor.vendors IS 'คู่ค้า / ผู้ประมวลผล (ต่อยอดจาก org.external_parties)';
COMMENT ON TABLE vendor.intakes IS 'แบบ intake สำหรับจัดระดับความเสี่ยง (tier)';
COMMENT ON TABLE vendor.vendor_assessments IS 'รอบประเมินคู่ค้า (ใช้ assessment engine)';
COMMENT ON TABLE vendor.sub_processors IS 'ผู้ประมวลผลช่วง';
COMMENT ON TABLE vendor.certificates IS 'ใบรับรองของคู่ค้า (ISO 27001, SOC 2 ฯลฯ)';
COMMENT ON TABLE vendor.remediation_items IS 'ประเด็นที่คู่ค้าต้องแก้ไข';
COMMENT ON TABLE vendor.offboardings IS 'การยุติการใช้บริการ';

-- +goose Down
DROP TABLE IF EXISTS vendor.offboardings;
DROP TABLE IF EXISTS vendor.remediation_items;
DROP TABLE IF EXISTS vendor.certificates;
DROP TABLE IF EXISTS vendor.sub_processors;
DROP TABLE IF EXISTS vendor.vendor_assessments;
DROP TABLE IF EXISTS vendor.intakes;
DROP TABLE IF EXISTS vendor.vendors;
