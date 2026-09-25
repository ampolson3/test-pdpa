-- +goose Up
-- schema breach: เหตุละเมิดข้อมูลและการแจ้ง สคส. / เจ้าของข้อมูล
-- 12 tables · docs: docs/data/breach.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

CREATE TABLE breach.incidents (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_no varchar(30) NOT NULL,
    legal_entity_id uuid NOT NULL,
    reported_via text NOT NULL CHECK (reported_via IN ('employee_form', 'public_form', 'processor', 'system', 'email', 'phone')),
    reporter_user_id uuid,
    reporter_contact_enc bytea,
    processor_party_id uuid,
    title text NOT NULL,
    description text NOT NULL,
    breach_types text[] NOT NULL,
    incident_type varchar(40),
    occurred_at timestamptz,
    aware_at timestamptz NOT NULL,
    contained_at timestamptz,
    affected_subjects int,
    affected_categories uuid[] NOT NULL DEFAULT '{}',
    risk_level text CHECK (risk_level IN ('none', 'low', 'high')),
    decision text CHECK (decision IN ('no_notification', 'notify_pdpc', 'notify_pdpc_and_subjects')),
    decision_reason text,
    decided_by uuid,
    pdpc_due_at timestamptz NOT NULL,
    late_reason text,
    is_drill boolean NOT NULL DEFAULT false,
    workflow_instance_id uuid,
    status text NOT NULL DEFAULT 'reported' CHECK (status IN ('reported', 'triage', 'assessing', 'notifying', 'remediating', 'closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_incidents PRIMARY KEY (id),
    CONSTRAINT uq_incidents_incident_no UNIQUE (tenant_id, incident_no)
);

CREATE TABLE breach.incident_assets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    asset_id uuid,
    activity_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_incident_assets PRIMARY KEY (id)
);

CREATE TABLE breach.assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    score numeric(6,2) NOT NULL,
    risk_level text NOT NULL CHECK (risk_level IN ('none', 'low', 'high')),
    factors jsonb NOT NULL,
    assessed_by uuid NOT NULL,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_assessments PRIMARY KEY (id)
);

CREATE TABLE breach.pdpc_notifications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    sequence_no smallint NOT NULL,
    notification_type text NOT NULL CHECK (notification_type IN ('initial', 'supplementary', 'final')),
    document_version_id uuid NOT NULL,
    approved_by uuid,
    submitted_at timestamptz,
    submission_ref varchar(60),
    is_late boolean NOT NULL DEFAULT false,
    late_reason text,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_pdpc_notifications PRIMARY KEY (id),
    CONSTRAINT uq_pdpc_notifications_incident_id_sequence_no UNIQUE (incident_id, sequence_no)
);

CREATE TABLE breach.subject_notifications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    channel text NOT NULL CHECK (channel IN ('email', 'sms', 'line', 'letter', 'website')),
    template_id uuid,
    total_recipients int NOT NULL DEFAULT 0,
    sent_count int NOT NULL DEFAULT 0,
    failed_count int NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'sending', 'done', 'failed')),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_subject_notifications PRIMARY KEY (id)
);

CREATE TABLE breach.notification_recipients (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    subject_notification_id uuid NOT NULL,
    subject_id uuid,
    address_enc bytea NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed')),
    sent_at timestamptz,
    error text,
    CONSTRAINT pk_breach_notification_recipients PRIMARY KEY (id)
);

CREATE TABLE breach.playbooks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    incident_type varchar(40) NOT NULL,
    name text NOT NULL,
    steps jsonb NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_playbooks PRIMARY KEY (id)
);

CREATE TABLE breach.response_tasks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    playbook_id uuid,
    step_code varchar(40),
    title text NOT NULL,
    assignee_user_id uuid,
    assignee_group_id uuid,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    due_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_response_tasks PRIMARY KEY (id)
);

CREATE TABLE breach.evidence (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    file_id uuid NOT NULL,
    description text,
    collected_by uuid,
    collected_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_evidence PRIMARY KEY (id)
);

CREATE TABLE breach.timeline_events (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('decision', 'action', 'communication', 'system', 'note')),
    description text NOT NULL,
    actor_id uuid,
    is_auto boolean NOT NULL DEFAULT false,
    CONSTRAINT pk_breach_timeline_events PRIMARY KEY (id)
);

CREATE TABLE breach.root_causes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_id uuid NOT NULL,
    cause_category varchar(40) NOT NULL,
    description text NOT NULL,
    corrective_actions jsonb NOT NULL DEFAULT '[]'::jsonb,
    risk_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_root_causes PRIMARY KEY (id)
);

CREATE TABLE breach.routing_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    incident_type varchar(40),
    min_risk text CHECK (min_risk IN ('none', 'low', 'high')),
    legal_entity_id uuid,
    group_id uuid NOT NULL,
    channels text[] NOT NULL DEFAULT '{email}',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_breach_routing_rules PRIMARY KEY (id)
);

-- indexes
CREATE INDEX ix_breach_incidents_legal_entity_id ON breach.incidents (tenant_id, legal_entity_id);
CREATE INDEX ix_breach_incidents_reporter_user_id ON breach.incidents (tenant_id, reporter_user_id);
CREATE INDEX ix_breach_incidents_processor_party_id ON breach.incidents (tenant_id, processor_party_id);
CREATE INDEX ix_breach_incidents_decided_by ON breach.incidents (tenant_id, decided_by);
CREATE INDEX ix_breach_incidents_pdpc_due_at ON breach.incidents (tenant_id, pdpc_due_at);
CREATE INDEX ix_breach_incidents_workflow_instance_id ON breach.incidents (tenant_id, workflow_instance_id);
CREATE INDEX ix_breach_incident_assets_incident_id ON breach.incident_assets (tenant_id, incident_id);
CREATE INDEX ix_breach_incident_assets_asset_id ON breach.incident_assets (tenant_id, asset_id);
CREATE INDEX ix_breach_incident_assets_activity_id ON breach.incident_assets (tenant_id, activity_id);
CREATE INDEX ix_breach_assessments_incident_id ON breach.assessments (tenant_id, incident_id);
CREATE INDEX ix_breach_assessments_form_submission_id ON breach.assessments (tenant_id, form_submission_id);
CREATE INDEX ix_breach_assessments_assessed_by ON breach.assessments (tenant_id, assessed_by);
CREATE INDEX ix_breach_pdpc_notifications_incident_id ON breach.pdpc_notifications (tenant_id, incident_id);
CREATE INDEX ix_breach_pdpc_notifications_document_version_id ON breach.pdpc_notifications (tenant_id, document_version_id);
CREATE INDEX ix_breach_pdpc_notifications_approved_by ON breach.pdpc_notifications (tenant_id, approved_by);
CREATE INDEX ix_breach_pdpc_notifications_evidence_file_id ON breach.pdpc_notifications (tenant_id, evidence_file_id);
CREATE INDEX ix_breach_subject_notifications_incident_id ON breach.subject_notifications (tenant_id, incident_id);
CREATE INDEX ix_breach_subject_notifications_template_id ON breach.subject_notifications (tenant_id, template_id);
CREATE INDEX ix_breach_notification_recipients_subject_notification_id ON breach.notification_recipients (tenant_id, subject_notification_id);
CREATE INDEX ix_breach_notification_recipients_subject_id ON breach.notification_recipients (tenant_id, subject_id);
CREATE INDEX ix_breach_response_tasks_incident_id ON breach.response_tasks (tenant_id, incident_id);
CREATE INDEX ix_breach_response_tasks_playbook_id ON breach.response_tasks (tenant_id, playbook_id);
CREATE INDEX ix_breach_response_tasks_assignee_user_id ON breach.response_tasks (tenant_id, assignee_user_id);
CREATE INDEX ix_breach_response_tasks_assignee_group_id ON breach.response_tasks (tenant_id, assignee_group_id);
CREATE INDEX ix_breach_evidence_incident_id ON breach.evidence (tenant_id, incident_id);
CREATE INDEX ix_breach_evidence_file_id ON breach.evidence (tenant_id, file_id);
CREATE INDEX ix_breach_evidence_collected_by ON breach.evidence (tenant_id, collected_by);
CREATE INDEX ix_breach_timeline_events_incident_id ON breach.timeline_events (tenant_id, incident_id);
CREATE INDEX ix_breach_root_causes_incident_id ON breach.root_causes (tenant_id, incident_id);
CREATE INDEX ix_breach_root_causes_risk_id ON breach.root_causes (tenant_id, risk_id);
CREATE INDEX ix_breach_routing_rules_legal_entity_id ON breach.routing_rules (tenant_id, legal_entity_id);
CREATE INDEX ix_breach_routing_rules_group_id ON breach.routing_rules (tenant_id, group_id);

-- row-level security (tenant isolation)
ALTER TABLE breach.incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.incidents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.incidents USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.incident_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.incident_assets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.incident_assets USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.assessments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.assessments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.pdpc_notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.pdpc_notifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.pdpc_notifications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.subject_notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.subject_notifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.subject_notifications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.notification_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.notification_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.notification_recipients USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.playbooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.playbooks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON breach.playbooks FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON breach.playbooks FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.response_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.response_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.response_tasks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.evidence USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.timeline_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.timeline_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.timeline_events USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.root_causes ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.root_causes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.root_causes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE breach.routing_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE breach.routing_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON breach.routing_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- updated_at / row_version triggers
CREATE TRIGGER trg_incidents_updated BEFORE UPDATE ON breach.incidents FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_incident_assets_updated BEFORE UPDATE ON breach.incident_assets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_assessments_updated BEFORE UPDATE ON breach.assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_pdpc_notifications_updated BEFORE UPDATE ON breach.pdpc_notifications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_subject_notifications_updated BEFORE UPDATE ON breach.subject_notifications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_playbooks_updated BEFORE UPDATE ON breach.playbooks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_response_tasks_updated BEFORE UPDATE ON breach.response_tasks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_evidence_updated BEFORE UPDATE ON breach.evidence FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_root_causes_updated BEFORE UPDATE ON breach.root_causes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_routing_rules_updated BEFORE UPDATE ON breach.routing_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- comments
COMMENT ON TABLE breach.incidents IS 'เหตุละเมิดข้อมูลส่วนบุคคล';
COMMENT ON TABLE breach.incident_assets IS 'ระบบ / กิจกรรมที่เกี่ยวข้องกับเหตุ';
COMMENT ON TABLE breach.assessments IS 'การประเมินความเสี่ยงของเหตุ (ประกาศแจ้งเหตุ พ.ศ. 2565)';
COMMENT ON TABLE breach.pdpc_notifications IS 'การแจ้ง สคส. (ฉบับเบื้องต้น / เพิ่มเติม / สุดท้าย)';
COMMENT ON TABLE breach.subject_notifications IS 'การแจ้งเจ้าของข้อมูล (ความเสี่ยงสูง)';
COMMENT ON TABLE breach.notification_recipients IS 'ผู้รับแจ้งรายคน';
COMMENT ON TABLE breach.playbooks IS 'playbook ตามประเภทเหตุ';
COMMENT ON TABLE breach.response_tasks IS 'งานควบคุม / แก้ไข / กู้คืน';
COMMENT ON TABLE breach.evidence IS 'หลักฐาน (hash)';
COMMENT ON TABLE breach.timeline_events IS 'ลำดับเหตุการณ์และการตัดสินใจ';
COMMENT ON TABLE breach.root_causes IS 'สาเหตุและมาตรการป้องกันซ้ำ';
COMMENT ON TABLE breach.routing_rules IS 'กฎผู้รับแจ้งและทีมตอบสนอง';

-- +goose Down
DROP TABLE IF EXISTS breach.routing_rules;
DROP TABLE IF EXISTS breach.root_causes;
DROP TABLE IF EXISTS breach.timeline_events;
DROP TABLE IF EXISTS breach.evidence;
DROP TABLE IF EXISTS breach.response_tasks;
DROP TABLE IF EXISTS breach.playbooks;
DROP TABLE IF EXISTS breach.notification_recipients;
DROP TABLE IF EXISTS breach.subject_notifications;
DROP TABLE IF EXISTS breach.pdpc_notifications;
DROP TABLE IF EXISTS breach.assessments;
DROP TABLE IF EXISTS breach.incident_assets;
DROP TABLE IF EXISTS breach.incidents;
