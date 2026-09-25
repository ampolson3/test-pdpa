-- +goose Up
-- schema notice: ประกาศความเป็นส่วนตัว เวอร์ชัน การรับทราบ และการแจ้งตาม ม.25
-- 8 tables · docs: docs/data/notice.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE notice.notices (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    subject_type_id uuid,
    notice_type text NOT NULL CHECK (notice_type IN ('privacy_notice', 'privacy_policy', 'cookie_policy', 'cctv', 'layered_short', 'employee')),
    title text NOT NULL,
    slug varchar(120) NOT NULL,
    document_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'published', 'retired')),
    current_version_id uuid,
    owner_user_id uuid,
    review_cycle_months smallint NOT NULL DEFAULT 12,
    next_review_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_notices PRIMARY KEY (id),
    CONSTRAINT uq_notices_slug UNIQUE (tenant_id, slug)
);

CREATE TABLE notice.notice_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    notice_id uuid NOT NULL,
    version_no int NOT NULL,
    document_version_id uuid NOT NULL,
    languages text[] NOT NULL,
    effective_from date NOT NULL,
    is_material_change boolean NOT NULL DEFAULT false,
    changes_purpose boolean NOT NULL DEFAULT false,
    checklist_result jsonb NOT NULL,
    public_url text,
    published_at timestamptz,
    published_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_notice_versions PRIMARY KEY (id),
    CONSTRAINT uq_notice_versions_notice_id_version_no UNIQUE (notice_id, version_no)
);

CREATE TABLE notice.notice_activity_links (
    notice_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    CONSTRAINT pk_notice_notice_activity_links PRIMARY KEY (notice_id, activity_id)
);

CREATE TABLE notice.acknowledgements (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    notice_version_id uuid NOT NULL,
    subject_id uuid,
    user_id uuid,
    channel varchar(20) NOT NULL,
    receipt_id uuid,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    CONSTRAINT pk_notice_acknowledgements PRIMARY KEY (id)
);

CREATE TABLE notice.indirect_collections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    source_party_id uuid NOT NULL,
    activity_id uuid,
    notice_id uuid,
    obtained_at date NOT NULL,
    subject_count int,
    notify_due_at date NOT NULL,
    method text CHECK (method IN ('email', 'sms', 'letter', 'website', 'other')),
    notified_at timestamptz,
    evidence_file_id uuid,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'notified', 'exempted', 'overdue')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_indirect_collections PRIMARY KEY (id)
);

CREATE TABLE notice.embeds (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    notice_id uuid NOT NULL,
    embed_type text NOT NULL CHECK (embed_type IN ('script', 'iframe', 'link', 'qr')),
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_embeds PRIMARY KEY (id)
);

CREATE TABLE notice.linked_documents (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    notice_id uuid NOT NULL,
    label text NOT NULL,
    url text,
    file_id uuid,
    display_mode text NOT NULL DEFAULT 'link' CHECK (display_mode IN ('inline', 'popup', 'link')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_linked_documents PRIMARY KEY (id)
);

CREATE TABLE notice.wizard_templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    subject_type_code varchar(40) NOT NULL,
    industry varchar(40),
    language varchar(5) NOT NULL,
    template_id uuid NOT NULL,
    questions jsonb NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_notice_wizard_templates PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_notice_notices_legal_entity_id ON notice.notices (tenant_id, legal_entity_id);
CREATE INDEX ix_notice_notices_subject_type_id ON notice.notices (tenant_id, subject_type_id);
CREATE INDEX ix_notice_notices_document_id ON notice.notices (tenant_id, document_id);
CREATE INDEX ix_notice_notices_owner_user_id ON notice.notices (tenant_id, owner_user_id);
CREATE INDEX ix_notice_notice_versions_notice_id ON notice.notice_versions (tenant_id, notice_id);
CREATE INDEX ix_notice_notice_versions_document_version_id ON notice.notice_versions (tenant_id, document_version_id);
CREATE INDEX ix_notice_notice_versions_published_by ON notice.notice_versions (tenant_id, published_by);
CREATE INDEX ix_notice_notice_activity_links_activity_id ON notice.notice_activity_links (tenant_id, activity_id);
CREATE INDEX ix_notice_acknowledgements_notice_version_id ON notice.acknowledgements (tenant_id, notice_version_id);
CREATE INDEX ix_notice_acknowledgements_subject_id ON notice.acknowledgements (tenant_id, subject_id);
CREATE INDEX ix_notice_acknowledgements_user_id ON notice.acknowledgements (tenant_id, user_id);
CREATE INDEX ix_notice_acknowledgements_receipt_id ON notice.acknowledgements (tenant_id, receipt_id);
CREATE INDEX ix_notice_indirect_collections_source_party_id ON notice.indirect_collections (tenant_id, source_party_id);
CREATE INDEX ix_notice_indirect_collections_activity_id ON notice.indirect_collections (tenant_id, activity_id);
CREATE INDEX ix_notice_indirect_collections_notice_id ON notice.indirect_collections (tenant_id, notice_id);
CREATE INDEX ix_notice_indirect_collections_evidence_file_id ON notice.indirect_collections (tenant_id, evidence_file_id);
CREATE INDEX ix_notice_embeds_notice_id ON notice.embeds (tenant_id, notice_id);
CREATE INDEX ix_notice_linked_documents_notice_id ON notice.linked_documents (tenant_id, notice_id);
CREATE INDEX ix_notice_linked_documents_file_id ON notice.linked_documents (tenant_id, file_id);
CREATE INDEX ix_notice_wizard_templates_template_id ON notice.wizard_templates (template_id);

-- row-level security (tenant isolation)
ALTER TABLE notice.notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.notices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.notices USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.notice_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.notice_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.notice_versions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.notice_activity_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.notice_activity_links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.notice_activity_links USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.acknowledgements ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.acknowledgements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.acknowledgements USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.indirect_collections ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.indirect_collections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.indirect_collections USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.embeds ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.embeds FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.embeds USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.linked_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.linked_documents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notice.linked_documents USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE notice.wizard_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE notice.wizard_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON notice.wizard_templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON notice.wizard_templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_notices_updated BEFORE UPDATE ON notice.notices FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_notice_versions_updated BEFORE UPDATE ON notice.notice_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_indirect_collections_updated BEFORE UPDATE ON notice.indirect_collections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_embeds_updated BEFORE UPDATE ON notice.embeds FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_linked_documents_updated BEFORE UPDATE ON notice.linked_documents FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_wizard_templates_updated BEFORE UPDATE ON notice.wizard_templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE notice.notices IS 'ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV';
COMMENT ON TABLE notice.notice_versions IS 'เวอร์ชันที่เผยแพร่ + ผล checklist ม.23';
COMMENT ON TABLE notice.notice_activity_links IS 'กิจกรรม RoPA ที่ประกาศครอบคลุม';
COMMENT ON TABLE notice.acknowledgements IS 'การรับทราบประกาศ';
COMMENT ON TABLE notice.indirect_collections IS 'การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25)';
COMMENT ON TABLE notice.embeds IS 'โค้ดฝังและลิงก์ของประกาศ';
COMMENT ON TABLE notice.linked_documents IS 'เอกสารอ้างอิงที่ลิงก์ในประกาศ';
COMMENT ON TABLE notice.wizard_templates IS 'template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม';

-- +goose Down
DROP TABLE IF EXISTS notice.wizard_templates;
DROP TABLE IF EXISTS notice.linked_documents;
DROP TABLE IF EXISTS notice.embeds;
DROP TABLE IF EXISTS notice.indirect_collections;
DROP TABLE IF EXISTS notice.acknowledgements;
DROP TABLE IF EXISTS notice.notice_activity_links;
DROP TABLE IF EXISTS notice.notice_versions;
DROP TABLE IF EXISTS notice.notices;
