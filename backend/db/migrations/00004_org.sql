-- +goose Up
-- schema org: นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก และข้อมูลตั้งต้น
-- 12 tables · docs: docs/data/org.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE org.legal_entities (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    parent_id uuid,
    name_th text NOT NULL,
    name_en text,
    registration_no char(13),
    tax_id char(13),
    address jsonb NOT NULL DEFAULT '{}'::jsonb,
    contact_email citext,
    contact_phone varchar(30),
    logo_file_id uuid,
    is_controller boolean NOT NULL DEFAULT true,
    is_processor boolean NOT NULL DEFAULT false,
    representative jsonb,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_legal_entities PRIMARY KEY (id)
);

CREATE TABLE org.org_units (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    parent_id uuid,
    path ltree NOT NULL,
    code varchar(40) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    unit_type text NOT NULL CHECK (unit_type IN ('group', 'company', 'division', 'department', 'branch', 'team')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_org_units PRIMARY KEY (id),
    CONSTRAINT uq_org_units_legal_entity_id_code UNIQUE (tenant_id, legal_entity_id, code)
);

CREATE TABLE org.privacy_champions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    user_id uuid NOT NULL,
    champion_role text NOT NULL DEFAULT 'primary' CHECK (champion_role IN ('primary', 'backup')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_privacy_champions PRIMARY KEY (id)
);

CREATE TABLE org.external_parties (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    party_type text NOT NULL CHECK (party_type IN ('processor', 'recipient', 'controller', 'joint_controller', 'government', 'other')),
    name_th text NOT NULL,
    name_en text,
    registration_no varchar(30),
    country_code char(2) NOT NULL,
    contact jsonb NOT NULL DEFAULT '{}'::jsonb,
    website text,
    dedupe_key varchar(120),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_external_parties PRIMARY KEY (id)
);

CREATE TABLE org.data_categories (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    is_sensitive boolean NOT NULL DEFAULT false,
    sensitive_type varchar(40),
    parent_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_data_categories PRIMARY KEY (id),
    CONSTRAINT uq_data_categories_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE org.data_subject_types (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(40) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    is_vulnerable boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_data_subject_types PRIMARY KEY (id),
    CONSTRAINT uq_data_subject_types_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE org.processing_purposes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    category varchar(40),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_processing_purposes PRIMARY KEY (id),
    CONSTRAINT uq_processing_purposes_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE org.lawful_bases (
    code varchar(20) NOT NULL,
    section_ref varchar(30) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    for_sensitive boolean NOT NULL DEFAULT false,
    requires_consent boolean NOT NULL DEFAULT false,
    requires_lia boolean NOT NULL DEFAULT false,
    CONSTRAINT pk_org_lawful_bases PRIMARY KEY (code)
);

CREATE TABLE org.countries (
    code char(2) NOT NULL,
    name_th text NOT NULL,
    name_en text NOT NULL,
    adequacy_status text NOT NULL DEFAULT 'unknown' CHECK (adequacy_status IN ('adequate', 'not_adequate', 'unknown')),
    region varchar(40),
    CONSTRAINT pk_org_countries PRIMARY KEY (code)
);

CREATE TABLE org.business_calendars (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    timezone varchar(40) NOT NULL DEFAULT 'Asia/Bangkok',
    workdays smallint[] NOT NULL DEFAULT '{1,2,3,4,5}',
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_business_calendars PRIMARY KEY (id)
);

CREATE TABLE org.holidays (
    calendar_id uuid NOT NULL,
    holiday_date date NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    CONSTRAINT pk_org_holidays PRIMARY KEY (calendar_id, holiday_date)
);

CREATE TABLE org.org_settings (
    tenant_id uuid NOT NULL,
    default_language varchar(5) NOT NULL DEFAULT 'th',
    date_era text NOT NULL DEFAULT 'BE' CHECK (date_era IN ('BE', 'CE')),
    branding jsonb NOT NULL DEFAULT '{}'::jsonb,
    default_calendar_id uuid,
    notification_defaults jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_org_org_settings PRIMARY KEY (tenant_id)
);

-- indexes
CREATE INDEX ix_org_legal_entities_parent_id ON org.legal_entities (tenant_id, parent_id);
CREATE INDEX ix_org_legal_entities_logo_file_id ON org.legal_entities (tenant_id, logo_file_id);
CREATE INDEX ix_org_org_units_legal_entity_id ON org.org_units (tenant_id, legal_entity_id);
CREATE INDEX ix_org_org_units_parent_id ON org.org_units (tenant_id, parent_id);
CREATE INDEX ix_org_org_units_path ON org.org_units (tenant_id, path);
CREATE INDEX ix_org_privacy_champions_org_unit_id ON org.privacy_champions (tenant_id, org_unit_id);
CREATE INDEX ix_org_privacy_champions_user_id ON org.privacy_champions (tenant_id, user_id);
CREATE INDEX ix_org_external_parties_country_code ON org.external_parties (tenant_id, country_code);
CREATE INDEX ix_org_external_parties_dedupe_key ON org.external_parties (tenant_id, dedupe_key);
CREATE INDEX ix_org_data_categories_parent_id ON org.data_categories (parent_id);
CREATE INDEX ix_org_org_settings_default_calendar_id ON org.org_settings (default_calendar_id);

-- row-level security (tenant isolation)
ALTER TABLE org.legal_entities ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.legal_entities FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.legal_entities USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.org_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.org_units FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.org_units USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.privacy_champions ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.privacy_champions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.privacy_champions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.external_parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.external_parties FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.external_parties USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.data_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.data_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON org.data_categories FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON org.data_categories FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.data_subject_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.data_subject_types FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON org.data_subject_types FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON org.data_subject_types FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.processing_purposes ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.processing_purposes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON org.processing_purposes FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON org.processing_purposes FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.business_calendars ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.business_calendars FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.business_calendars USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.holidays ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.holidays FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.holidays USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE org.org_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.org_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org.org_settings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_legal_entities_updated BEFORE UPDATE ON org.legal_entities FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_org_units_updated BEFORE UPDATE ON org.org_units FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_privacy_champions_updated BEFORE UPDATE ON org.privacy_champions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_external_parties_updated BEFORE UPDATE ON org.external_parties FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_categories_updated BEFORE UPDATE ON org.data_categories FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_subject_types_updated BEFORE UPDATE ON org.data_subject_types FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_processing_purposes_updated BEFORE UPDATE ON org.processing_purposes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_business_calendars_updated BEFORE UPDATE ON org.business_calendars FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_org_settings_updated BEFORE UPDATE ON org.org_settings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE org.legal_entities IS 'นิติบุคคล (บริษัทในกลุ่ม)';
COMMENT ON TABLE org.org_units IS 'โครงสร้างหน่วยงาน (ltree)';
COMMENT ON TABLE org.privacy_champions IS 'ผู้ประสานงาน PDPA ประจำหน่วยงาน';
COMMENT ON TABLE org.external_parties IS 'ทะเบียนหน่วยงานภายนอก (ผู้ประมวลผล ผู้รับ หน่วยงานรัฐ)';
COMMENT ON TABLE org.data_categories IS 'หมวดข้อมูลส่วนบุคคล (ทั่วไป / อ่อนไหว ม.26)';
COMMENT ON TABLE org.data_subject_types IS 'กลุ่มเจ้าของข้อมูล';
COMMENT ON TABLE org.processing_purposes IS 'วัตถุประสงค์การประมวลผล (master)';
COMMENT ON TABLE org.lawful_bases IS 'ฐานทางกฎหมาย ม.24 / ม.26 / ม.19';
COMMENT ON TABLE org.countries IS 'ประเทศและสถานะมาตรฐานการคุ้มครองที่เพียงพอ';
COMMENT ON TABLE org.business_calendars IS 'ปฏิทินวันทำการ';
COMMENT ON TABLE org.holidays IS 'วันหยุดในปฏิทิน';
COMMENT ON TABLE org.org_settings IS 'ค่าตั้งค่าองค์กร (ภาษา แบรนด์ รูปแบบวันที่)';

-- +goose Down
DROP TABLE IF EXISTS org.org_settings;
DROP TABLE IF EXISTS org.holidays;
DROP TABLE IF EXISTS org.business_calendars;
DROP TABLE IF EXISTS org.countries;
DROP TABLE IF EXISTS org.lawful_bases;
DROP TABLE IF EXISTS org.processing_purposes;
DROP TABLE IF EXISTS org.data_subject_types;
DROP TABLE IF EXISTS org.data_categories;
DROP TABLE IF EXISTS org.external_parties;
DROP TABLE IF EXISTS org.privacy_champions;
DROP TABLE IF EXISTS org.org_units;
DROP TABLE IF EXISTS org.legal_entities;
