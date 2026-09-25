-- +goose Up
-- schema risk: risk matrix ทะเบียนความเสี่ยง control และการวิเคราะห์ช่องว่าง
-- 10 tables · docs: docs/data/risk.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE risk.risk_matrices (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    likelihood_levels jsonb NOT NULL,
    impact_levels jsonb NOT NULL,
    thresholds jsonb NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_risk_matrices PRIMARY KEY (id)
);

CREATE TABLE risk.risk_factors (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    applies_to text NOT NULL CHECK (applies_to IN ('activity', 'vendor', 'breach', 'dpia')),
    weight numeric(5,2) NOT NULL DEFAULT 1,
    rule jsonb NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_risk_factors PRIMARY KEY (id),
    CONSTRAINT uq_risk_factors_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE risk.controls (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    category text NOT NULL CHECK (category IN ('organizational', 'technical', 'physical', 'access_control', 'legal')),
    description text,
    framework_refs text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_controls PRIMARY KEY (id),
    CONSTRAINT uq_controls_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE risk.activity_scores (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    matrix_id uuid NOT NULL,
    likelihood smallint NOT NULL,
    impact smallint NOT NULL,
    inherent_score numeric(6,2) NOT NULL,
    residual_score numeric(6,2),
    level text NOT NULL CHECK (level IN ('low', 'medium', 'high', 'very_high')),
    factor_breakdown jsonb NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_risk_activity_scores PRIMARY KEY (id)
);

CREATE TABLE risk.risks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    source_type text NOT NULL CHECK (source_type IN ('dpia', 'activity', 'vendor', 'breach', 'audit', 'manual')),
    source_id uuid,
    title text NOT NULL,
    description text,
    owner_user_id uuid,
    activity_id uuid,
    asset_id uuid,
    vendor_id uuid,
    likelihood smallint NOT NULL,
    impact smallint NOT NULL,
    inherent_score numeric(6,2) NOT NULL,
    residual_likelihood smallint,
    residual_impact smallint,
    residual_score numeric(6,2),
    level text NOT NULL CHECK (level IN ('low', 'medium', 'high', 'very_high')),
    treatment text CHECK (treatment IN ('mitigate', 'accept', 'transfer', 'avoid')),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_treatment', 'accepted', 'closed')),
    review_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_risks PRIMARY KEY (id)
);

CREATE TABLE risk.risk_controls (
    risk_id uuid NOT NULL,
    control_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('existing', 'planned', 'implemented', 'not_effective')),
    owner_user_id uuid,
    due_at date,
    task_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_risk_controls PRIMARY KEY (risk_id, control_id)
);

CREATE TABLE risk.acceptances (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    risk_id uuid NOT NULL,
    reason text NOT NULL,
    approval_id uuid,
    approved_by uuid,
    approved_at timestamptz,
    expires_at date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_acceptances PRIMARY KEY (id)
);

CREATE TABLE risk.gap_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    expression jsonb NOT NULL,
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    legal_ref text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_gap_rules PRIMARY KEY (id),
    CONSTRAINT uq_gap_rules_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE risk.gap_findings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    rule_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'waived')),
    detected_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    task_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_risk_gap_findings PRIMARY KEY (id)
);

CREATE TABLE risk.compliance_scores (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('activity', 'org_unit', 'legal_entity', 'tenant')),
    scope_id uuid,
    score numeric(5,2) NOT NULL,
    breakdown jsonb NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_risk_compliance_scores PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_risk_activity_scores_activity_id ON risk.activity_scores (tenant_id, activity_id);
CREATE INDEX ix_risk_activity_scores_matrix_id ON risk.activity_scores (tenant_id, matrix_id);
CREATE INDEX ix_risk_risks_owner_user_id ON risk.risks (tenant_id, owner_user_id);
CREATE INDEX ix_risk_risks_activity_id ON risk.risks (tenant_id, activity_id);
CREATE INDEX ix_risk_risks_asset_id ON risk.risks (tenant_id, asset_id);
CREATE INDEX ix_risk_risks_vendor_id ON risk.risks (tenant_id, vendor_id);
CREATE INDEX ix_risk_risk_controls_control_id ON risk.risk_controls (tenant_id, control_id);
CREATE INDEX ix_risk_risk_controls_owner_user_id ON risk.risk_controls (tenant_id, owner_user_id);
CREATE INDEX ix_risk_risk_controls_task_id ON risk.risk_controls (tenant_id, task_id);
CREATE INDEX ix_risk_acceptances_risk_id ON risk.acceptances (tenant_id, risk_id);
CREATE INDEX ix_risk_acceptances_approval_id ON risk.acceptances (tenant_id, approval_id);
CREATE INDEX ix_risk_acceptances_approved_by ON risk.acceptances (tenant_id, approved_by);
CREATE INDEX ix_risk_gap_findings_rule_id ON risk.gap_findings (tenant_id, rule_id);
CREATE INDEX ix_risk_gap_findings_activity_id ON risk.gap_findings (tenant_id, activity_id);
CREATE INDEX ix_risk_gap_findings_task_id ON risk.gap_findings (tenant_id, task_id);

-- row-level security (tenant isolation)
ALTER TABLE risk.risk_matrices ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.risk_matrices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.risk_matrices USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.risk_factors ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.risk_factors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON risk.risk_factors FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON risk.risk_factors FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.controls FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON risk.controls FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON risk.controls FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.activity_scores ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.activity_scores FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.activity_scores USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.risks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.risks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.risk_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.risk_controls FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.risk_controls USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.acceptances ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.acceptances FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.acceptances USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.gap_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.gap_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON risk.gap_rules FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON risk.gap_rules FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.gap_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.gap_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.gap_findings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE risk.compliance_scores ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk.compliance_scores FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON risk.compliance_scores USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_risk_matrices_updated BEFORE UPDATE ON risk.risk_matrices FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risk_factors_updated BEFORE UPDATE ON risk.risk_factors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_controls_updated BEFORE UPDATE ON risk.controls FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risks_updated BEFORE UPDATE ON risk.risks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risk_controls_updated BEFORE UPDATE ON risk.risk_controls FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_acceptances_updated BEFORE UPDATE ON risk.acceptances FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_gap_rules_updated BEFORE UPDATE ON risk.gap_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_gap_findings_updated BEFORE UPDATE ON risk.gap_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE risk.risk_matrices IS 'risk matrix ต่อ tenant';
COMMENT ON TABLE risk.risk_factors IS 'ปัจจัยความเสี่ยงและน้ำหนัก (ใช้คำนวณจาก RoPA / คู่ค้า / เหตุ)';
COMMENT ON TABLE risk.controls IS 'คลังมาตรการ / control (ประกาศมาตรการความปลอดภัย, ISO)';
COMMENT ON TABLE risk.activity_scores IS 'คะแนนความเสี่ยงรายกิจกรรม';
COMMENT ON TABLE risk.risks IS 'ทะเบียนความเสี่ยง (จาก DPIA / RoPA / คู่ค้า / เหตุละเมิด / audit)';
COMMENT ON TABLE risk.risk_controls IS 'มาตรการที่ผูกกับความเสี่ยง';
COMMENT ON TABLE risk.acceptances IS 'การยอมรับความเสี่ยงคงเหลือ';
COMMENT ON TABLE risk.gap_rules IS 'กฎวิเคราะห์ช่องว่างทางกฎหมายของ RoPA';
COMMENT ON TABLE risk.gap_findings IS 'ช่องว่างที่พบรายกิจกรรม';
COMMENT ON TABLE risk.compliance_scores IS 'คะแนนความพร้อมรายกิจกรรม / หน่วยงาน / บริษัท';

-- +goose Down
DROP TABLE IF EXISTS risk.compliance_scores;
DROP TABLE IF EXISTS risk.gap_findings;
DROP TABLE IF EXISTS risk.gap_rules;
DROP TABLE IF EXISTS risk.acceptances;
DROP TABLE IF EXISTS risk.risk_controls;
DROP TABLE IF EXISTS risk.risks;
DROP TABLE IF EXISTS risk.activity_scores;
DROP TABLE IF EXISTS risk.controls;
DROP TABLE IF EXISTS risk.risk_factors;
DROP TABLE IF EXISTS risk.risk_matrices;
