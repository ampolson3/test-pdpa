-- +goose Up
-- schema dataflow: แผนผังการไหลของข้อมูลและ data discovery
-- 5 tables · docs: docs/data/dataflow.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE dataflow.layouts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    view_type text NOT NULL CHECK (view_type IN ('org_unit', 'system', 'legal_entity', 'data_category', 'lineage', 'world')),
    scope_id uuid,
    layout jsonb NOT NULL,
    user_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dataflow_layouts PRIMARY KEY (id)
);

CREATE TABLE dataflow.snapshots (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    view_type varchar(20) NOT NULL,
    scope_id uuid,
    version_no int NOT NULL,
    graph jsonb NOT NULL,
    image_file_id uuid,
    published_by uuid,
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dataflow_snapshots PRIMARY KEY (id),
    CONSTRAINT uq_snapshots_view_type_scope_id_version_no UNIQUE (tenant_id, view_type, scope_id, version_no)
);

CREATE TABLE dataflow.classifiers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    classifier_type text NOT NULL CHECK (classifier_type IN ('regex', 'checksum', 'dictionary', 'ml')),
    pattern text,
    data_category_id uuid,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dataflow_classifiers PRIMARY KEY (id),
    CONSTRAINT uq_classifiers_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE dataflow.discovery_scans (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    asset_id uuid,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    started_at timestamptz,
    finished_at timestamptz,
    objects_scanned int,
    findings_count int,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dataflow_discovery_scans PRIMARY KEY (id)
);

CREATE TABLE dataflow.discovery_findings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    object_path text NOT NULL,
    classifier_id uuid NOT NULL,
    match_ratio numeric(5,2) NOT NULL,
    sample_count int NOT NULL,
    suggested_category_id uuid,
    status text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'accepted', 'rejected')),
    reviewed_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dataflow_discovery_findings PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_dataflow_layouts_user_id ON dataflow.layouts (tenant_id, user_id);
CREATE INDEX ix_dataflow_snapshots_image_file_id ON dataflow.snapshots (tenant_id, image_file_id);
CREATE INDEX ix_dataflow_snapshots_published_by ON dataflow.snapshots (tenant_id, published_by);
CREATE INDEX ix_dataflow_classifiers_data_category_id ON dataflow.classifiers (data_category_id);
CREATE INDEX ix_dataflow_discovery_scans_connector_id ON dataflow.discovery_scans (tenant_id, connector_id);
CREATE INDEX ix_dataflow_discovery_scans_asset_id ON dataflow.discovery_scans (tenant_id, asset_id);
CREATE INDEX ix_dataflow_discovery_findings_scan_id ON dataflow.discovery_findings (tenant_id, scan_id);
CREATE INDEX ix_dataflow_discovery_findings_classifier_id ON dataflow.discovery_findings (tenant_id, classifier_id);
CREATE INDEX ix_dataflow_discovery_findings_suggested_category_id ON dataflow.discovery_findings (tenant_id, suggested_category_id);
CREATE INDEX ix_dataflow_discovery_findings_reviewed_by ON dataflow.discovery_findings (tenant_id, reviewed_by);

-- row-level security (tenant isolation)
ALTER TABLE dataflow.layouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE dataflow.layouts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dataflow.layouts USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dataflow.snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE dataflow.snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dataflow.snapshots USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dataflow.classifiers ENABLE ROW LEVEL SECURITY;
ALTER TABLE dataflow.classifiers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON dataflow.classifiers FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON dataflow.classifiers FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dataflow.discovery_scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE dataflow.discovery_scans FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dataflow.discovery_scans USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dataflow.discovery_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE dataflow.discovery_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dataflow.discovery_findings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_layouts_updated BEFORE UPDATE ON dataflow.layouts FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_snapshots_updated BEFORE UPDATE ON dataflow.snapshots FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_classifiers_updated BEFORE UPDATE ON dataflow.classifiers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_discovery_scans_updated BEFORE UPDATE ON dataflow.discovery_scans FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_discovery_findings_updated BEFORE UPDATE ON dataflow.discovery_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE dataflow.layouts IS 'ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง';
COMMENT ON TABLE dataflow.snapshots IS 'แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน)';
COMMENT ON TABLE dataflow.classifiers IS 'classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย)';
COMMENT ON TABLE dataflow.discovery_scans IS 'รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector';
COMMENT ON TABLE dataflow.discovery_findings IS 'ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง)';

-- +goose Down
DROP TABLE IF EXISTS dataflow.discovery_findings;
DROP TABLE IF EXISTS dataflow.discovery_scans;
DROP TABLE IF EXISTS dataflow.classifiers;
DROP TABLE IF EXISTS dataflow.snapshots;
DROP TABLE IF EXISTS dataflow.layouts;
