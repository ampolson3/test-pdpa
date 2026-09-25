-- +goose Up
-- schema cookie: โดเมน แบนเนอร์ คุกกี้ ผลสแกน และหลักฐานความยินยอมคุกกี้
-- 11 tables · docs: docs/data/cookie.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE cookie.domains (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain varchar(255) NOT NULL,
    legal_entity_id uuid NOT NULL,
    verification_token varchar(64) NOT NULL,
    verified_at timestamptz,
    scan_schedule varchar(40),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_domains PRIMARY KEY (id),
    CONSTRAINT uq_domains_domain UNIQUE (tenant_id, domain)
);

CREATE TABLE cookie.apps (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    platform text NOT NULL CHECK (platform IN ('ios', 'android', 'web', 'liff', 'ctv')),
    app_identifier varchar(200) NOT NULL,
    name text NOT NULL,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_apps PRIMARY KEY (id)
);

CREATE TABLE cookie.banner_configs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain_id uuid NOT NULL,
    version_no int NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    layout varchar(20) NOT NULL,
    theme jsonb NOT NULL,
    texts jsonb NOT NULL,
    geo_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
    consent_mode_v2 boolean NOT NULL DEFAULT true,
    tcf_enabled boolean NOT NULL DEFAULT false,
    cdn_path text,
    published_at timestamptz,
    published_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_banner_configs PRIMARY KEY (id),
    CONSTRAINT uq_banner_configs_domain_id_version_no UNIQUE (domain_id, version_no)
);

CREATE TABLE cookie.categories (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    code varchar(40) NOT NULL,
    names jsonb NOT NULL,
    descriptions jsonb NOT NULL,
    is_required boolean NOT NULL DEFAULT false,
    display_order smallint NOT NULL DEFAULT 0,
    purpose_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_categories PRIMARY KEY (id),
    CONSTRAINT uq_categories_code UNIQUE (tenant_id, code)
);

CREATE TABLE cookie.cookies (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain_id uuid NOT NULL,
    name varchar(255) NOT NULL,
    provider_domain varchar(255) NOT NULL,
    category_id uuid,
    storage_type text NOT NULL CHECK (storage_type IN ('http_cookie', 'js_cookie', 'local_storage', 'session_storage', 'pixel')),
    duration varchar(40),
    descriptions jsonb,
    source text NOT NULL CHECK (source IN ('scan', 'manual', 'kb')),
    kb_id uuid,
    first_seen_scan_id uuid,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_cookies PRIMARY KEY (id)
);

CREATE TABLE cookie.cookie_kb (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name_pattern varchar(255) NOT NULL,
    provider varchar(200),
    category_code varchar(40) NOT NULL,
    description_th text,
    description_en text,
    source varchar(60) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_cookie_kb PRIMARY KEY (id)
);

CREATE TABLE cookie.scans (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain_id uuid NOT NULL,
    trigger text NOT NULL CHECK (trigger IN ('manual', 'schedule', 'publish')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    started_at timestamptz,
    finished_at timestamptz,
    pages_scanned int,
    cookies_found int,
    new_cookies int,
    report_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_scans PRIMARY KEY (id)
);

CREATE TABLE cookie.scan_findings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    cookie_name varchar(255) NOT NULL,
    cookie_domain varchar(255) NOT NULL,
    page_url text NOT NULL,
    set_before_consent boolean NOT NULL DEFAULT false,
    suggested_category varchar(40),
    matched_cookie_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_scan_findings PRIMARY KEY (id)
);

CREATE TABLE cookie.script_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain_id uuid NOT NULL,
    pattern text NOT NULL,
    category_id uuid NOT NULL,
    action text NOT NULL DEFAULT 'block' CHECK (action IN ('block', 'allow')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_script_rules PRIMARY KEY (id)
);

CREATE TABLE cookie.ab_variants (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    domain_id uuid NOT NULL,
    banner_config_id uuid NOT NULL,
    name varchar(60) NOT NULL,
    traffic_pct smallint NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'running', 'stopped')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_cookie_ab_variants PRIMARY KEY (id)
);

CREATE TABLE cookie.consent_records (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    domain_id uuid NOT NULL,
    visitor_id varchar(64) NOT NULL,
    banner_config_id uuid NOT NULL,
    ab_variant_id uuid,
    action text NOT NULL CHECK (action IN ('accept_all', 'reject_all', 'custom', 'withdraw')),
    choices jsonb NOT NULL,
    ip_hash char(64),
    country char(2),
    user_agent text,
    subject_id uuid,
    CONSTRAINT pk_cookie_consent_records PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);
CREATE TABLE cookie.consent_records_default PARTITION OF cookie.consent_records DEFAULT;

-- indexes
CREATE INDEX ix_cookie_domains_legal_entity_id ON cookie.domains (tenant_id, legal_entity_id);
CREATE INDEX ix_cookie_banner_configs_domain_id ON cookie.banner_configs (tenant_id, domain_id);
CREATE INDEX ix_cookie_banner_configs_published_by ON cookie.banner_configs (tenant_id, published_by);
CREATE INDEX ix_cookie_categories_purpose_id ON cookie.categories (tenant_id, purpose_id);
CREATE INDEX ix_cookie_cookies_domain_id ON cookie.cookies (tenant_id, domain_id);
CREATE INDEX ix_cookie_cookies_category_id ON cookie.cookies (tenant_id, category_id);
CREATE INDEX ix_cookie_cookies_kb_id ON cookie.cookies (tenant_id, kb_id);
CREATE INDEX ix_cookie_cookies_first_seen_scan_id ON cookie.cookies (tenant_id, first_seen_scan_id);
CREATE INDEX ix_cookie_scans_domain_id ON cookie.scans (tenant_id, domain_id);
CREATE INDEX ix_cookie_scans_report_file_id ON cookie.scans (tenant_id, report_file_id);
CREATE INDEX ix_cookie_scan_findings_scan_id ON cookie.scan_findings (tenant_id, scan_id);
CREATE INDEX ix_cookie_scan_findings_matched_cookie_id ON cookie.scan_findings (tenant_id, matched_cookie_id);
CREATE INDEX ix_cookie_script_rules_domain_id ON cookie.script_rules (tenant_id, domain_id);
CREATE INDEX ix_cookie_script_rules_category_id ON cookie.script_rules (tenant_id, category_id);
CREATE INDEX ix_cookie_ab_variants_domain_id ON cookie.ab_variants (tenant_id, domain_id);
CREATE INDEX ix_cookie_ab_variants_banner_config_id ON cookie.ab_variants (tenant_id, banner_config_id);
CREATE INDEX ix_cookie_consent_records_domain_id ON cookie.consent_records (tenant_id, domain_id);
CREATE INDEX ix_cookie_consent_records_visitor_id ON cookie.consent_records (tenant_id, visitor_id);
CREATE INDEX ix_cookie_consent_records_banner_config_id ON cookie.consent_records (tenant_id, banner_config_id);
CREATE INDEX ix_cookie_consent_records_ab_variant_id ON cookie.consent_records (tenant_id, ab_variant_id);
CREATE INDEX ix_cookie_consent_records_subject_id ON cookie.consent_records (tenant_id, subject_id);

-- row-level security (tenant isolation)
ALTER TABLE cookie.domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.domains FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.domains USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.apps ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.apps FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.apps USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.banner_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.banner_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.banner_configs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.categories USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.cookies ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.cookies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.cookies USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.scans FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.scans USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.scan_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.scan_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.scan_findings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.script_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.script_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.script_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.ab_variants ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.ab_variants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.ab_variants USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE cookie.consent_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.consent_records FORCE ROW LEVEL SECURITY;
ALTER TABLE cookie.consent_records_default ENABLE ROW LEVEL SECURITY;
ALTER TABLE cookie.consent_records_default FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cookie.consent_records_default USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON cookie.consent_records USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_domains_updated BEFORE UPDATE ON cookie.domains FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_apps_updated BEFORE UPDATE ON cookie.apps FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_banner_configs_updated BEFORE UPDATE ON cookie.banner_configs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_categories_updated BEFORE UPDATE ON cookie.categories FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_cookies_updated BEFORE UPDATE ON cookie.cookies FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_cookie_kb_updated BEFORE UPDATE ON cookie.cookie_kb FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_scans_updated BEFORE UPDATE ON cookie.scans FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_scan_findings_updated BEFORE UPDATE ON cookie.scan_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_script_rules_updated BEFORE UPDATE ON cookie.script_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_ab_variants_updated BEFORE UPDATE ON cookie.ab_variants FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE cookie.domains IS 'โดเมน / เว็บไซต์ที่ใช้แบนเนอร์';
COMMENT ON TABLE cookie.apps IS 'แอป / LINE LIFF ที่ใช้ SDK';
COMMENT ON TABLE cookie.banner_configs IS 'config แบนเนอร์ (มีเวอร์ชัน publish ไป CDN)';
COMMENT ON TABLE cookie.categories IS 'หมวดคุกกี้';
COMMENT ON TABLE cookie.cookies IS 'คุกกี้ที่พบ / ประกาศไว้';
COMMENT ON TABLE cookie.cookie_kb IS 'ฐานข้อมูลคุกกี้กลาง (ใช้จัดหมวดอัตโนมัติ)';
COMMENT ON TABLE cookie.scans IS 'รอบสแกนเว็บไซต์';
COMMENT ON TABLE cookie.scan_findings IS 'คุกกี้ / tracker ที่พบในแต่ละรอบ';
COMMENT ON TABLE cookie.script_rules IS 'กฎบล็อกสคริปต์ก่อนได้รับความยินยอม';
COMMENT ON TABLE cookie.ab_variants IS 'variant แบนเนอร์สำหรับ A/B test';
COMMENT ON TABLE cookie.consent_records IS 'หลักฐานความยินยอมคุกกี้ของผู้เข้าชม (partition รายเดือน)';

-- +goose Down
DROP TABLE IF EXISTS cookie.consent_records;
DROP TABLE IF EXISTS cookie.ab_variants;
DROP TABLE IF EXISTS cookie.script_rules;
DROP TABLE IF EXISTS cookie.scan_findings;
DROP TABLE IF EXISTS cookie.scans;
DROP TABLE IF EXISTS cookie.cookie_kb;
DROP TABLE IF EXISTS cookie.cookies;
DROP TABLE IF EXISTS cookie.categories;
DROP TABLE IF EXISTS cookie.banner_configs;
DROP TABLE IF EXISTS cookie.apps;
DROP TABLE IF EXISTS cookie.domains;
