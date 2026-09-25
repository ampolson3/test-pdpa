-- +goose Up
-- schema dpo: งานของ DPO: การแต่งตั้ง งาน คำปรึกษา คลังความรู้
-- 8 tables · docs: docs/data/dpo.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE dpo.appointments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    dpo_type text NOT NULL CHECK (dpo_type IN ('internal', 'external', 'group')),
    user_id uuid,
    external_name text,
    external_company text,
    contact_email citext NOT NULL,
    contact_phone varchar(30),
    appointed_at date NOT NULL,
    appointment_file_id uuid,
    pdpc_notified_at date,
    pdpc_evidence_file_id uuid,
    ended_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_appointments PRIMARY KEY (id)
);

CREATE TABLE dpo.requirement_checks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    result text NOT NULL CHECK (result IN ('required', 'not_required', 'recommended')),
    basis text NOT NULL,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_requirement_checks PRIMARY KEY (id)
);

CREATE TABLE dpo.independence_declarations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    appointment_id uuid NOT NULL,
    year smallint NOT NULL,
    other_duties text,
    has_conflict boolean NOT NULL DEFAULT false,
    signed_at timestamptz,
    file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_independence_declarations PRIMARY KEY (id)
);

CREATE TABLE dpo.tasks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    task_no varchar(30) NOT NULL,
    title text NOT NULL,
    description text,
    source_type text NOT NULL CHECK (source_type IN ('ropa_gap', 'dpia', 'audit', 'breach', 'risk', 'vendor', 'agreement', 'manual')),
    source_id uuid,
    status text NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'assigned', 'in_review', 'done', 'closed')),
    priority text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    assignee_user_id uuid,
    reviewer_user_id uuid,
    org_unit_id uuid,
    due_at date,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_tasks PRIMARY KEY (id),
    CONSTRAINT uq_tasks_task_no UNIQUE (tenant_id, task_no)
);

CREATE TABLE dpo.advisories (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    advisory_no varchar(30) NOT NULL,
    org_unit_id uuid,
    requester_user_id uuid NOT NULL,
    subject text NOT NULL,
    question text NOT NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'answered', 'closed')),
    answer text,
    answered_by uuid,
    answered_at timestamptz,
    kb_article_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_advisories PRIMARY KEY (id),
    CONSTRAINT uq_advisories_advisory_no UNIQUE (tenant_id, advisory_no)
);

CREATE TABLE dpo.kb_articles (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    category varchar(40) NOT NULL,
    title text NOT NULL,
    body jsonb NOT NULL,
    language varchar(5) NOT NULL DEFAULT 'th',
    tags text[] NOT NULL DEFAULT '{}',
    source text NOT NULL CHECK (source IN ('law', 'pdpc', 'policy', 'faq', 'internal')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version_no int NOT NULL DEFAULT 1,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_kb_articles PRIMARY KEY (id)
);

CREATE TABLE dpo.calendar_events (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    event_type varchar(40) NOT NULL,
    due_at date NOT NULL,
    recurrence varchar(60),
    source_type varchar(40),
    source_id uuid,
    owner_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_calendar_events PRIMARY KEY (id)
);

CREATE TABLE dpo.report_schedules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    report_type varchar(40) NOT NULL,
    recipients uuid[] NOT NULL,
    cron varchar(40) NOT NULL,
    format text NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf', 'xlsx')),
    last_run_at timestamptz,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dpo_report_schedules PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_dpo_appointments_legal_entity_id ON dpo.appointments (tenant_id, legal_entity_id);
CREATE INDEX ix_dpo_appointments_user_id ON dpo.appointments (tenant_id, user_id);
CREATE INDEX ix_dpo_appointments_appointment_file_id ON dpo.appointments (tenant_id, appointment_file_id);
CREATE INDEX ix_dpo_appointments_pdpc_evidence_file_id ON dpo.appointments (tenant_id, pdpc_evidence_file_id);
CREATE INDEX ix_dpo_requirement_checks_legal_entity_id ON dpo.requirement_checks (tenant_id, legal_entity_id);
CREATE INDEX ix_dpo_requirement_checks_assessment_id ON dpo.requirement_checks (tenant_id, assessment_id);
CREATE INDEX ix_dpo_independence_declarations_appointment_id ON dpo.independence_declarations (tenant_id, appointment_id);
CREATE INDEX ix_dpo_independence_declarations_file_id ON dpo.independence_declarations (tenant_id, file_id);
CREATE INDEX ix_dpo_tasks_assignee_user_id ON dpo.tasks (tenant_id, assignee_user_id);
CREATE INDEX ix_dpo_tasks_reviewer_user_id ON dpo.tasks (tenant_id, reviewer_user_id);
CREATE INDEX ix_dpo_tasks_org_unit_id ON dpo.tasks (tenant_id, org_unit_id);
CREATE INDEX ix_dpo_tasks_due_at ON dpo.tasks (tenant_id, due_at);
CREATE INDEX ix_dpo_advisories_org_unit_id ON dpo.advisories (tenant_id, org_unit_id);
CREATE INDEX ix_dpo_advisories_requester_user_id ON dpo.advisories (tenant_id, requester_user_id);
CREATE INDEX ix_dpo_advisories_answered_by ON dpo.advisories (tenant_id, answered_by);
CREATE INDEX ix_dpo_advisories_kb_article_id ON dpo.advisories (tenant_id, kb_article_id);
CREATE INDEX ix_dpo_calendar_events_owner_user_id ON dpo.calendar_events (tenant_id, owner_user_id);

-- row-level security (tenant isolation)
ALTER TABLE dpo.appointments ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.appointments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.appointments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.requirement_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.requirement_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.requirement_checks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.independence_declarations ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.independence_declarations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.independence_declarations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.tasks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.advisories ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.advisories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.advisories USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.kb_articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.kb_articles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON dpo.kb_articles FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON dpo.kb_articles FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.calendar_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.calendar_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.calendar_events USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE dpo.report_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE dpo.report_schedules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dpo.report_schedules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_appointments_updated BEFORE UPDATE ON dpo.appointments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_requirement_checks_updated BEFORE UPDATE ON dpo.requirement_checks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_independence_declarations_updated BEFORE UPDATE ON dpo.independence_declarations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_tasks_updated BEFORE UPDATE ON dpo.tasks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_advisories_updated BEFORE UPDATE ON dpo.advisories FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_kb_articles_updated BEFORE UPDATE ON dpo.kb_articles FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_calendar_events_updated BEFORE UPDATE ON dpo.calendar_events FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_report_schedules_updated BEFORE UPDATE ON dpo.report_schedules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE dpo.appointments IS 'การแต่งตั้ง DPO และการแจ้ง สคส.';
COMMENT ON TABLE dpo.requirement_checks IS 'ผลประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41)';
COMMENT ON TABLE dpo.independence_declarations IS 'คำรับรองความเป็นอิสระ / ผลประโยชน์ทับซ้อน';
COMMENT ON TABLE dpo.tasks IS 'งาน / ticket 5 สถานะ (สร้างอัตโนมัติจาก gap / DPIA / audit)';
COMMENT ON TABLE dpo.advisories IS 'คำปรึกษาจากหน่วยงานถึง DPO';
COMMENT ON TABLE dpo.kb_articles IS 'คลังเอกสารกฎหมายและ FAQ';
COMMENT ON TABLE dpo.calendar_events IS 'ปฏิทินงาน compliance (กิจกรรมที่ไม่ได้มาจากโมดูลอื่น)';
COMMENT ON TABLE dpo.report_schedules IS 'ตั้งเวลาส่งรายงาน DPO / ผู้บริหาร';

-- +goose Down
DROP TABLE IF EXISTS dpo.report_schedules;
DROP TABLE IF EXISTS dpo.calendar_events;
DROP TABLE IF EXISTS dpo.kb_articles;
DROP TABLE IF EXISTS dpo.advisories;
DROP TABLE IF EXISTS dpo.tasks;
DROP TABLE IF EXISTS dpo.independence_declarations;
DROP TABLE IF EXISTS dpo.requirement_checks;
DROP TABLE IF EXISTS dpo.appointments;
