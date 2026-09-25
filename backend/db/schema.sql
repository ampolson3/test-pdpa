-- =====================================================================
-- PDPA Platform — PostgreSQL schema (DDL) v1.1  | generated from the ERD model
-- Target: PostgreSQL 16+  | Backend: Go (pgx + sqlc + goose)  | ห้ามแก้ไขด้วยมือ ให้แก้ที่ model แล้ว generate ใหม่
-- Conventions:
--   * id = UUID (แอปสร้าง UUIDv7; DEFAULT gen_random_uuid() เป็นค่าสำรอง)
--   * ทุกตารางธุรกิจมี tenant_id + Row-Level Security: แอปต้อง SET LOCAL app.tenant_id = '<uuid>' ในทุก transaction
--   * ตารางที่ tenant_id ว่างได้ = ข้อมูลกลางของแพลตฟอร์ม (อ่านได้ทุก tenant, เขียนได้เฉพาะ role ที่ BYPASSRLS)
--   * *_enc = ค่าที่เข้ารหัสระดับฟิลด์ (envelope encryption) ; blind_index = HMAC สำหรับค้นหาแบบตรงตัว
--   * ตาราง partition รายเดือน: สร้าง partition ล่วงหน้าด้วย pg_partman หรือ job ของ worker
--   * LFK = logical reference ไปตาราง partition (ไม่มี FK constraint) ตรวจความถูกต้องใน service layer
-- =====================================================================

-- gen_random_uuid() อยู่ใน core ตั้งแต่ PostgreSQL 13 จึงไม่ต้องใช้ pgcrypto
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
-- pgvector ใช้กับผู้ช่วย AI (P4): ติดตั้ง extension ก่อนสร้างตาราง gov.kb_chunks
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS platform;
COMMENT ON SCHEMA platform IS 'บริการกลางของแพลตฟอร์ม: tenant, workflow, form, เอกสาร, ไฟล์, แจ้งเตือน, event, audit';
CREATE SCHEMA IF NOT EXISTS iam;
COMMENT ON SCHEMA iam IS 'ผู้ใช้ สิทธิ์ ขอบเขตข้อมูล API client และการยืนยันตัวตน';
CREATE SCHEMA IF NOT EXISTS org;
COMMENT ON SCHEMA org IS 'นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก และข้อมูลตั้งต้น';
CREATE SCHEMA IF NOT EXISTS consent;
COMMENT ON SCHEMA consent IS 'ความยินยอมตาม FSD V3.2: Data Element, Purpose, Purpose Preference, Collection Point, Consent Transaction, Reconcile';
CREATE SCHEMA IF NOT EXISTS cookie;
COMMENT ON SCHEMA cookie IS 'โดเมน แบนเนอร์ คุกกี้ ผลสแกน และหลักฐานความยินยอมคุกกี้';
CREATE SCHEMA IF NOT EXISTS notice;
COMMENT ON SCHEMA notice IS 'ประกาศความเป็นส่วนตัว เวอร์ชัน การรับทราบ และการแจ้งตาม ม.25';
CREATE SCHEMA IF NOT EXISTS ropa;
COMMENT ON SCHEMA ropa IS 'RoPA ทะเบียนข้อมูล asset การโอน retention และคลังกิจกรรมมาตรฐาน';
CREATE SCHEMA IF NOT EXISTS dataflow;
COMMENT ON SCHEMA dataflow IS 'แผนผังการไหลของข้อมูลและ data discovery';
CREATE SCHEMA IF NOT EXISTS risk;
COMMENT ON SCHEMA risk IS 'risk matrix ทะเบียนความเสี่ยง control และการวิเคราะห์ช่องว่าง';
CREATE SCHEMA IF NOT EXISTS assess;
COMMENT ON SCHEMA assess IS 'แบบประเมิน DPIA / LIA / TIA / security / maturity (assessment engine)';
CREATE SCHEMA IF NOT EXISTS dsar;
COMMENT ON SCHEMA dsar IS 'คำขอใช้สิทธิของเจ้าของข้อมูล';
CREATE SCHEMA IF NOT EXISTS breach;
COMMENT ON SCHEMA breach IS 'เหตุละเมิดข้อมูลและการแจ้ง สคส. / เจ้าของข้อมูล';
CREATE SCHEMA IF NOT EXISTS vendor;
COMMENT ON SCHEMA vendor IS 'คู่ค้าและผู้ประมวลผล';
CREATE SCHEMA IF NOT EXISTS agreement;
COMMENT ON SCHEMA agreement IS 'ข้อตกลง DPA / DSA (agreement engine)';
CREATE SCHEMA IF NOT EXISTS dpo;
COMMENT ON SCHEMA dpo IS 'งานของ DPO: การแต่งตั้ง งาน คำปรึกษา คลังความรู้';
CREATE SCHEMA IF NOT EXISTS gov;
COMMENT ON SCHEMA gov IS 'อบรม นโยบาย การตรวจประเมิน retention การติดต่อ สคส. และทะเบียน AI';

CREATE OR REPLACE FUNCTION platform.set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at := now(); NEW.row_version := OLD.row_version + 1; RETURN NEW; END; $$;

CREATE TABLE platform.tenants (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    code varchar(40) NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('trial', 'active', 'suspended', 'terminated')),
    plan_code varchar(40) NOT NULL,
    deployment text NOT NULL DEFAULT 'saas_shared' CHECK (deployment IN ('saas_shared', 'saas_dedicated', 'on_prem')),
    data_region varchar(20) NOT NULL DEFAULT 'TH',
    keycloak_org_id varchar(100),
    primary_domain varchar(255),
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    suspended_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_tenants PRIMARY KEY (id),
    CONSTRAINT uq_tenants_code UNIQUE (code),
    CONSTRAINT uq_tenants_keycloak_org_id UNIQUE (keycloak_org_id),
    CONSTRAINT uq_tenants_primary_domain UNIQUE (primary_domain)
);

CREATE TABLE platform.tenant_modules (
    tenant_id uuid NOT NULL,
    module_code varchar(20) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    license_expires_at date,
    quota jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_tenant_modules PRIMARY KEY (tenant_id, module_code)
);

CREATE TABLE platform.public_keys (
    key varchar(64) NOT NULL,
    tenant_id uuid NOT NULL,
    entity_type text NOT NULL CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form')),
    entity_id uuid NOT NULL,
    allowed_origins text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CONSTRAINT pk_platform_public_keys PRIMARY KEY (key)
);

CREATE TABLE platform.workflow_definitions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    entity_type varchar(60) NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    definition jsonb NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_workflow_definitions PRIMARY KEY (id),
    CONSTRAINT uq_workflow_definitions_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, code, version_no)
);

CREATE TABLE platform.workflow_instances (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    definition_id uuid NOT NULL,
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    current_state varchar(60) NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    sla_status text NOT NULL DEFAULT 'on_track' CHECK (sla_status IN ('on_track', 'at_risk', 'overdue', 'paused', 'done')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_workflow_instances PRIMARY KEY (id)
);

CREATE TABLE platform.workflow_tasks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    instance_id uuid NOT NULL,
    state varchar(60) NOT NULL,
    title text NOT NULL,
    assignee_user_id uuid,
    assignee_group_id uuid,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    due_at timestamptz,
    completed_at timestamptz,
    outcome varchar(40),
    comment text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_workflow_tasks PRIMARY KEY (id)
);

CREATE TABLE platform.sla_timers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    instance_id uuid NOT NULL,
    code varchar(40) NOT NULL,
    mode text NOT NULL CHECK (mode IN ('calendar_days', 'business_days', 'hours')),
    calendar_id uuid,
    started_at timestamptz NOT NULL,
    due_at timestamptz NOT NULL,
    reminders jsonb NOT NULL DEFAULT '[]'::jsonb,
    escalated_at timestamptz,
    stopped_at timestamptz,
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'met', 'breached', 'stopped')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_sla_timers PRIMARY KEY (id)
);

CREATE TABLE platform.form_definitions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    form_type text NOT NULL CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    current_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_form_definitions PRIMARY KEY (id),
    CONSTRAINT uq_form_definitions_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE platform.form_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    form_id uuid NOT NULL,
    version_no int NOT NULL,
    schema jsonb NOT NULL,
    scoring jsonb,
    languages text[] NOT NULL DEFAULT '{th,en}',
    published_at timestamptz,
    published_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_form_versions PRIMARY KEY (id)
);

CREATE TABLE platform.form_submissions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    form_version_id uuid NOT NULL,
    entity_type varchar(60),
    entity_id uuid,
    submitted_by_type text NOT NULL CHECK (submitted_by_type IN ('user', 'guest', 'data_subject', 'system')),
    submitted_by uuid,
    answers jsonb NOT NULL,
    score numeric(8,2),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_form_submissions PRIMARY KEY (id)
);

CREATE TABLE platform.record_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    version_no int NOT NULL,
    snapshot jsonb NOT NULL,
    diff jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'published', 'superseded')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_record_versions PRIMARY KEY (id),
    CONSTRAINT uq_record_versions_entity_type_entity_id_version_no UNIQUE (tenant_id, entity_type, entity_id, version_no)
);

CREATE TABLE platform.approvals (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    record_version_id uuid,
    step_no int NOT NULL DEFAULT 1,
    requested_by uuid NOT NULL,
    approver_user_id uuid,
    approver_role varchar(40),
    decision text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending', 'approved', 'rejected', 'returned')),
    reason text,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_approvals PRIMARY KEY (id)
);

CREATE TABLE platform.comments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    parent_id uuid,
    author_type text NOT NULL CHECK (author_type IN ('user', 'guest')),
    author_id uuid NOT NULL,
    body text NOT NULL,
    mentions uuid[] NOT NULL DEFAULT '{}',
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_comments PRIMARY KEY (id)
);

CREATE TABLE platform.files (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    bucket varchar(63) NOT NULL,
    object_key text NOT NULL,
    file_name text NOT NULL,
    mime_type varchar(120) NOT NULL,
    size_bytes bigint NOT NULL,
    sha256 char(64) NOT NULL,
    encryption_key_id varchar(120),
    av_status text NOT NULL DEFAULT 'pending' CHECK (av_status IN ('pending', 'clean', 'infected', 'error')),
    entity_type varchar(60),
    entity_id uuid,
    retention_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_files PRIMARY KEY (id),
    CONSTRAINT uq_files_object_key UNIQUE (tenant_id, object_key)
);

CREATE TABLE platform.documents (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    doc_type text NOT NULL CHECK (doc_type IN ('notice', 'policy', 'dpa', 'dsa', 'dsar_letter', 'pdpc_form', 'breach_letter', 'report', 'other')),
    entity_type varchar(60),
    entity_id uuid,
    title text NOT NULL,
    template_id uuid,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'published', 'archived')),
    current_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_documents PRIMARY KEY (id)
);

CREATE TABLE platform.document_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    document_id uuid NOT NULL,
    version_no int NOT NULL,
    content jsonb NOT NULL,
    languages text[] NOT NULL DEFAULT '{th}',
    pdf_file_id uuid,
    docx_file_id uuid,
    change_summary text,
    effective_from date,
    approved_by uuid,
    approved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_document_versions PRIMARY KEY (id),
    CONSTRAINT uq_document_versions_document_id_version_no UNIQUE (document_id, version_no)
);

CREATE TABLE platform.templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    template_type varchar(40) NOT NULL,
    code varchar(80) NOT NULL,
    name text NOT NULL,
    language varchar(5) NOT NULL,
    industry varchar(40),
    content jsonb NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_templates PRIMARY KEY (id),
    CONSTRAINT uq_templates_template_type_code_language_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, template_type, code, language, version_no)
);

CREATE TABLE platform.clause_library (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(80) NOT NULL,
    category varchar(60) NOT NULL,
    title text NOT NULL,
    body_th jsonb NOT NULL,
    body_en jsonb,
    legal_ref text,
    applies_to text[] NOT NULL DEFAULT '{dpa,dsa}',
    is_mandatory boolean NOT NULL DEFAULT false,
    version_no int NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_clause_library PRIMARY KEY (id),
    CONSTRAINT uq_clause_library_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, code, version_no)
);

CREATE TABLE platform.notification_templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(80) NOT NULL,
    channel text NOT NULL CHECK (channel IN ('email', 'sms', 'line', 'in_app')),
    language varchar(5) NOT NULL,
    subject text,
    body text NOT NULL,
    variables jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_notification_templates PRIMARY KEY (id),
    CONSTRAINT uq_notification_templates_code_channel_language UNIQUE NULLS NOT DISTINCT (tenant_id, code, channel, language)
);

CREATE TABLE platform.notifications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    template_id uuid,
    channel text NOT NULL CHECK (channel IN ('email', 'sms', 'line', 'in_app')),
    recipient_user_id uuid,
    recipient_address_enc bytea,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'delivered', 'failed', 'cancelled')),
    attempts int NOT NULL DEFAULT 0,
    provider_message_id varchar(120),
    sent_at timestamptz,
    error text,
    entity_type varchar(60),
    entity_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_notifications PRIMARY KEY (id)
);

CREATE TABLE platform.outbox_events (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    aggregate_type varchar(60) NOT NULL,
    aggregate_id uuid NOT NULL,
    event_type varchar(80) NOT NULL,
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_outbox_events PRIMARY KEY (id)
);

CREATE TABLE platform.webhook_subscriptions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    api_client_id uuid NOT NULL,
    url text NOT NULL,
    event_types text[] NOT NULL,
    secret_ref varchar(200) NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_webhook_subscriptions PRIMARY KEY (id)
);

CREATE TABLE platform.webhook_deliveries (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    subscription_id uuid NOT NULL,
    event_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead')),
    http_status int,
    attempts int NOT NULL DEFAULT 0,
    next_retry_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_webhook_deliveries PRIMARY KEY (id)
);

CREATE TABLE platform.audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
    tenant_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_type text NOT NULL CHECK (actor_type IN ('user', 'api_client', 'guest', 'data_subject', 'system')),
    actor_id uuid,
    action varchar(80) NOT NULL,
    entity_type varchar(60),
    entity_id uuid,
    before jsonb,
    after jsonb,
    ip inet,
    user_agent text,
    prev_hash char(64),
    hash char(64) NOT NULL,
    CONSTRAINT pk_platform_audit_log PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);
CREATE TABLE platform.audit_log_default PARTITION OF platform.audit_log DEFAULT;

CREATE TABLE platform.import_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    import_type varchar(60) NOT NULL,
    file_id uuid NOT NULL,
    mapping jsonb NOT NULL DEFAULT '{}'::jsonb,
    dry_run boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'validating', 'ready', 'importing', 'done', 'failed')),
    total_rows int,
    success_rows int,
    error_rows int,
    error_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_import_jobs PRIMARY KEY (id)
);

CREATE TABLE platform.export_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    export_type varchar(60) NOT NULL,
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    format text NOT NULL CHECK (format IN ('csv', 'xlsx', 'pdf', 'json')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed', 'expired')),
    file_id uuid,
    requested_by uuid NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_export_jobs PRIMARY KEY (id)
);

CREATE TABLE platform.connectors (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    connector_type varchar(40) NOT NULL,
    name text NOT NULL,
    asset_id uuid,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_ref varchar(200),
    capabilities text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'error', 'disabled')),
    last_tested_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_connectors PRIMARY KEY (id)
);

CREATE TABLE platform.ai_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    feature_code varchar(40) NOT NULL,
    provider varchar(40) NOT NULL,
    model varchar(80) NOT NULL,
    prompt_template_code varchar(80) NOT NULL,
    redacted boolean NOT NULL DEFAULT true,
    tokens_in int,
    tokens_out int,
    cost numeric(12,4),
    status text NOT NULL CHECK (status IN ('ok', 'error', 'blocked')),
    entity_type varchar(60),
    entity_id uuid,
    confirmed_by uuid,
    confirmed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_ai_requests PRIMARY KEY (id)
);

CREATE TABLE iam.users (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    keycloak_user_id uuid,
    email citext NOT NULL,
    display_name text NOT NULL,
    phone varchar(30),
    status text NOT NULL DEFAULT 'invited' CHECK (status IN ('invited', 'active', 'disabled', 'locked')),
    locale varchar(5) NOT NULL DEFAULT 'th',
    primary_org_unit_id uuid,
    mfa_required boolean NOT NULL DEFAULT false,
    notification_prefs jsonb NOT NULL DEFAULT '{}'::jsonb,
    avatar_file_id uuid,
    last_login_at timestamptz,
    disabled_at timestamptz,
    source text NOT NULL DEFAULT 'local' CHECK (source IN ('local', 'sso_jit', 'scim', 'import')),
    external_id varchar(200),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_users PRIMARY KEY (id),
    CONSTRAINT uq_users_keycloak_user_id UNIQUE (tenant_id, keycloak_user_id)
);

CREATE TABLE iam.roles (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    code varchar(40) NOT NULL,
    name_th text NOT NULL,
    name_en text NOT NULL,
    description text,
    is_system boolean NOT NULL DEFAULT false,
    requires_mfa boolean NOT NULL DEFAULT false,
    cloned_from_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_roles PRIMARY KEY (id),
    CONSTRAINT uq_roles_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)
);

CREATE TABLE iam.permissions (
    code varchar(80) NOT NULL,
    module varchar(20) NOT NULL,
    resource varchar(40) NOT NULL,
    action text NOT NULL CHECK (action IN ('C', 'R', 'U', 'D', 'A', 'P', 'E', 'X')),
    description text,
    is_sensitive boolean NOT NULL DEFAULT false,
    CONSTRAINT pk_iam_permissions PRIMARY KEY (code)
);

CREATE TABLE iam.role_permissions (
    role_id uuid NOT NULL,
    permission_code varchar(80) NOT NULL,
    tenant_id uuid,
    CONSTRAINT pk_iam_role_permissions PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE iam.groups (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    source text NOT NULL DEFAULT 'local' CHECK (source IN ('local', 'scim', 'idp')),
    external_id varchar(200),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_groups PRIMARY KEY (id)
);

CREATE TABLE iam.group_members (
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_group_members PRIMARY KEY (group_id, user_id)
);

CREATE TABLE iam.role_assignments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id uuid,
    group_id uuid,
    role_id uuid NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('tenant', 'legal_entity', 'org_unit', 'self')),
    legal_entity_id uuid,
    org_unit_id uuid,
    include_descendants boolean NOT NULL DEFAULT true,
    valid_from timestamptz NOT NULL DEFAULT now(),
    valid_to timestamptz,
    granted_by uuid,
    approval_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_role_assignments PRIMARY KEY (id)
);

CREATE TABLE iam.api_clients (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    keycloak_client_id varchar(120) NOT NULL,
    owner_user_id uuid,
    system_asset_id uuid,
    scopes text[] NOT NULL,
    ip_allowlist cidr[] NOT NULL DEFAULT '{}',
    rate_limit_per_min int NOT NULL DEFAULT 600,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'revoked')),
    secret_rotated_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_api_clients PRIMARY KEY (id),
    CONSTRAINT uq_api_clients_keycloak_client_id UNIQUE (tenant_id, keycloak_client_id)
);

CREATE TABLE iam.guest_tokens (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('vendor_assessment', 'assessment_section', 'breach_report', 'dsar_subtask', 'agreement_review', 'remediation', 'deletion_proof')),
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    email citext NOT NULL,
    token_hash char(64) NOT NULL,
    otp_required boolean NOT NULL DEFAULT true,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_used_at timestamptz,
    issued_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_guest_tokens PRIMARY KEY (id),
    CONSTRAINT uq_guest_tokens_token_hash UNIQUE (tenant_id, token_hash)
);

CREATE TABLE iam.idp_configs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    idp_type text NOT NULL CHECK (idp_type IN ('oidc', 'saml', 'ldap', 'thaid', 'google')),
    alias varchar(60) NOT NULL,
    display_name text NOT NULL,
    keycloak_alias varchar(60) NOT NULL,
    group_mappings jsonb NOT NULL DEFAULT '[]'::jsonb,
    jit_enabled boolean NOT NULL DEFAULT true,
    enforced boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_idp_configs PRIMARY KEY (id)
);

CREATE TABLE iam.security_policies (
    tenant_id uuid NOT NULL,
    password_policy jsonb NOT NULL,
    lockout_policy jsonb NOT NULL,
    mfa_role_codes text[] NOT NULL DEFAULT '{}',
    session_idle_min int NOT NULL DEFAULT 30,
    session_max_hours int NOT NULL DEFAULT 12,
    admin_ip_allowlist cidr[] NOT NULL DEFAULT '{}',
    access_review_months int NOT NULL DEFAULT 6,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_security_policies PRIMARY KEY (tenant_id)
);

CREATE TABLE iam.security_events (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    user_id uuid,
    event_type text NOT NULL CHECK (event_type IN ('login', 'login_failed', 'logout', 'lockout', 'mfa_enrolled', 'mfa_reset', 'new_device', 'breakglass', 'session_revoked')),
    ip inet,
    country char(2),
    user_agent text,
    details jsonb,
    CONSTRAINT pk_iam_security_events PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);
CREATE TABLE iam.security_events_default PARTITION OF iam.security_events DEFAULT;

CREATE TABLE iam.access_reviews (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    scope jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed')),
    due_at timestamptz NOT NULL,
    auto_revoke boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_access_reviews PRIMARY KEY (id)
);

CREATE TABLE iam.access_review_items (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    review_id uuid NOT NULL,
    role_assignment_id uuid NOT NULL,
    reviewer_user_id uuid NOT NULL,
    decision text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending', 'keep', 'revoke')),
    decided_at timestamptz,
    comment text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_access_review_items PRIMARY KEY (id)
);

CREATE TABLE iam.breakglass_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    reason text NOT NULL,
    approved_by uuid,
    starts_at timestamptz,
    ends_at timestamptz,
    status text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'active', 'expired', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_breakglass_requests PRIMARY KEY (id)
);

CREATE TABLE iam.delegations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    from_user_id uuid NOT NULL,
    to_user_id uuid NOT NULL,
    role_id uuid,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_delegations PRIMARY KEY (id)
);

CREATE TABLE iam.field_masking_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    entity_type varchar(60) NOT NULL,
    field_name varchar(60) NOT NULL,
    mask_pattern varchar(60) NOT NULL,
    unmask_permission varchar(80) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_field_masking_rules PRIMARY KEY (id)
);

CREATE TABLE iam.unmask_logs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id uuid NOT NULL,
    entity_type varchar(60) NOT NULL,
    entity_id uuid NOT NULL,
    field_name varchar(60) NOT NULL,
    reason text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_iam_unmask_logs PRIMARY KEY (id)
);

CREATE TABLE iam.subject_verifications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    subject_id uuid,
    identifier_blind_index bytea NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('preference_center', 'consent', 'dsar', 'double_opt_in', 'guardian')),
    method text NOT NULL CHECK (method IN ('otp_sms', 'otp_email', 'magic_link', 'idp', 'thaid')),
    otp_hash char(64),
    attempts int NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'failed', 'expired')),
    assurance_level smallint NOT NULL DEFAULT 1,
    expires_at timestamptz NOT NULL,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_iam_subject_verifications PRIMARY KEY (id)
);

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

CREATE TABLE consent.data_elements (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    data_category_id uuid,
    is_identifier boolean NOT NULL DEFAULT false,
    identifier_type text CHECK (identifier_type IN ('email', 'phone', 'national_id', 'customer_id', 'passport', 'other')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_data_elements PRIMARY KEY (id),
    CONSTRAINT uq_data_elements_code UNIQUE (tenant_id, code)
);

CREATE TABLE consent.purposes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    legal_entity_id uuid NOT NULL,
    lawful_basis_code varchar(20) NOT NULL,
    is_sensitive boolean NOT NULL DEFAULT false,
    requires_explicit boolean NOT NULL DEFAULT false,
    min_age smallint,
    lifespan_days int,
    double_opt_in boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    current_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_purposes PRIMARY KEY (id),
    CONSTRAINT uq_purposes_code UNIQUE (tenant_id, code)
);

CREATE TABLE consent.purpose_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    version_no int NOT NULL,
    text_th text NOT NULL,
    text_en text,
    change_type text NOT NULL DEFAULT 'initial' CHECK (change_type IN ('initial', 'minor', 'material')),
    requires_reconsent boolean NOT NULL DEFAULT false,
    published_at timestamptz,
    approved_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_purpose_versions PRIMARY KEY (id),
    CONSTRAINT uq_purpose_versions_purpose_id_version_no UNIQUE (purpose_id, version_no)
);

CREATE TABLE consent.purpose_preferences (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    pref_type text NOT NULL CHECK (pref_type IN ('channel', 'topic', 'frequency', 'other')),
    options jsonb NOT NULL DEFAULT '[]'::jsonb,
    display_order smallint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_purpose_preferences PRIMARY KEY (id),
    CONSTRAINT uq_purpose_preferences_purpose_id_code UNIQUE (purpose_id, code)
);

CREATE TABLE consent.purpose_data_elements (
    purpose_id uuid NOT NULL,
    data_element_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    CONSTRAINT pk_consent_purpose_data_elements PRIMARY KEY (purpose_id, data_element_id)
);

CREATE TABLE consent.collection_points (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name text NOT NULL,
    channel text NOT NULL CHECK (channel IN ('web', 'app', 'pos', 'call_center', 'kiosk', 'paper', 'line', 'api', 'import', 'cookie')),
    legal_entity_id uuid NOT NULL,
    form_id uuid,
    notice_id uuid,
    verification_method text NOT NULL DEFAULT 'none' CHECK (verification_method IN ('none', 'otp', 'magic_link', 'idp')),
    double_opt_in boolean NOT NULL DEFAULT false,
    age_gate boolean NOT NULL DEFAULT false,
    qr_token varchar(64),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_collection_points PRIMARY KEY (id),
    CONSTRAINT uq_collection_points_qr_token UNIQUE (tenant_id, qr_token),
    CONSTRAINT uq_collection_points_code UNIQUE (tenant_id, code)
);

CREATE TABLE consent.collection_point_purposes (
    collection_point_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    is_required boolean NOT NULL DEFAULT false,
    display_order smallint NOT NULL DEFAULT 0,
    CONSTRAINT pk_consent_collection_point_purposes PRIMARY KEY (collection_point_id, purpose_id)
);

CREATE TABLE consent.data_subjects (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    subject_key varchar(80) NOT NULL,
    display_name_enc bytea,
    birth_date_enc bytea,
    is_minor boolean NOT NULL DEFAULT false,
    guardian_subject_id uuid,
    legal_capacity text NOT NULL DEFAULT 'full' CHECK (legal_capacity IN ('full', 'minor', 'incompetent', 'quasi_incompetent')),
    last_activity_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_data_subjects PRIMARY KEY (id),
    CONSTRAINT uq_data_subjects_subject_key UNIQUE (tenant_id, subject_key)
);

CREATE TABLE consent.subject_identifiers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    identifier_type text NOT NULL CHECK (identifier_type IN ('email', 'phone', 'national_id', 'customer_id', 'passport', 'line_uid', 'other')),
    value_enc bytea NOT NULL,
    blind_index bytea NOT NULL,
    is_primary boolean NOT NULL DEFAULT false,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_subject_identifiers PRIMARY KEY (id),
    CONSTRAINT uq_subject_identifiers_identifier_type_blind_index UNIQUE (tenant_id, identifier_type, blind_index)
);

CREATE TABLE consent.consent_receipts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    receipt_no varchar(40) NOT NULL,
    subject_id uuid NOT NULL,
    collection_point_id uuid NOT NULL,
    channel varchar(20) NOT NULL,
    captured_by_user_id uuid,
    branch_org_unit_id uuid,
    notice_version_id uuid,
    form_submission_id uuid,
    verification_id uuid,
    ip inet,
    user_agent text,
    country char(2),
    language varchar(5),
    evidence_file_id uuid,
    occurred_at timestamptz NOT NULL,
    prev_hash char(64),
    hash char(64) NOT NULL,
    CONSTRAINT pk_consent_consent_receipts PRIMARY KEY (id),
    CONSTRAINT uq_consent_receipts_receipt_no UNIQUE (tenant_id, receipt_no)
);

CREATE TABLE consent.consent_transactions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    receipt_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    purpose_version_id uuid NOT NULL,
    transaction_type text NOT NULL CHECK (transaction_type IN ('CONSENTED', 'NOT_CONSENTED', 'WITHDRAWN', 'EXPIRED', 'EXTENDED', 'PENDING', 'CONFIRMED', 'CANCELLED', 'CHANGED_PREFERENCES')),
    preferences jsonb,
    reason_code varchar(40),
    expires_at timestamptz,
    source text NOT NULL CHECK (source IN ('web', 'app', 'api', 'staff', 'import', 'campaign', 'dsar', 'system')),
    idempotency_key varchar(80),
    CONSTRAINT pk_consent_consent_transactions PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);
CREATE TABLE consent.consent_transactions_default PARTITION OF consent.consent_transactions DEFAULT;

CREATE TABLE consent.consent_status (
    subject_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('ACTIVE', 'NOT_GIVEN', 'WITHDRAWN', 'EXPIRED', 'PENDING')),
    purpose_version_id uuid NOT NULL,
    last_transaction_id uuid NOT NULL,
    preferences jsonb,
    expires_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_consent_consent_status PRIMARY KEY (subject_id, purpose_id)
);

CREATE TABLE consent.double_optin_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    channel text NOT NULL CHECK (channel IN ('email', 'sms')),
    token_hash char(64) NOT NULL,
    sent_at timestamptz,
    expires_at timestamptz NOT NULL,
    confirmed_at timestamptz,
    status text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'confirmed', 'cancelled', 'expired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_double_optin_requests PRIMARY KEY (id),
    CONSTRAINT uq_double_optin_requests_token_hash UNIQUE (tenant_id, token_hash)
);

CREATE TABLE consent.guardian_approvals (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    minor_subject_id uuid NOT NULL,
    guardian_subject_id uuid NOT NULL,
    receipt_id uuid,
    relationship text NOT NULL CHECK (relationship IN ('parent', 'legal_guardian', 'curator', 'custodian')),
    status text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'approved', 'rejected', 'expired')),
    requested_at timestamptz NOT NULL,
    approved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_guardian_approvals PRIMARY KEY (id)
);

CREATE TABLE consent.campaigns (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    purpose_ids uuid[] NOT NULL,
    audience_query jsonb NOT NULL,
    channel text NOT NULL CHECK (channel IN ('email', 'sms', 'line')),
    template_id uuid,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'scheduled', 'sending', 'done', 'cancelled')),
    scheduled_at timestamptz,
    sent_count int NOT NULL DEFAULT 0,
    response_count int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_campaigns PRIMARY KEY (id)
);

CREATE TABLE consent.campaign_recipients (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    campaign_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    token_hash char(64),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'opened', 'responded', 'bounced')),
    sent_at timestamptz,
    responded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_campaign_recipients PRIMARY KEY (id),
    CONSTRAINT uq_campaign_recipients_token_hash UNIQUE (tenant_id, token_hash)
);

CREATE TABLE consent.reconcile_runs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    target_api_client_id uuid,
    connector_id uuid,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed')),
    total int,
    mismatched int,
    report_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_reconcile_runs PRIMARY KEY (id)
);

CREATE TABLE consent.reconcile_items (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    run_id uuid NOT NULL,
    subject_id uuid,
    purpose_id uuid,
    platform_status varchar(20),
    target_status varchar(20),
    mismatch_type text NOT NULL CHECK (mismatch_type IN ('missing_in_target', 'missing_in_platform', 'status_diff')),
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_reconcile_items PRIMARY KEY (id)
);

CREATE TABLE consent.downstream_syncs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    webhook_delivery_id uuid,
    target_api_client_id uuid,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged', 'failed')),
    acknowledged_at timestamptz,
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_consent_downstream_syncs PRIMARY KEY (id)
);

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

CREATE TABLE ropa.processing_activities (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    code varchar(40) NOT NULL,
    name text NOT NULL,
    description text,
    role text NOT NULL CHECK (role IN ('controller', 'processor')),
    controller_party_id uuid,
    owner_user_id uuid,
    template_id uuid,
    template_version_no int,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'active', 'under_review', 'ended')),
    completeness smallint NOT NULL DEFAULT 0,
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'very_high')),
    rights_and_access text,
    approved_by uuid,
    approved_at timestamptz,
    next_review_at date,
    ended_at timestamptz,
    end_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_processing_activities PRIMARY KEY (id),
    CONSTRAINT uq_processing_activities_code UNIQUE (tenant_id, code)
);

CREATE TABLE ropa.activity_purposes (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    purpose_id uuid,
    purpose_text text NOT NULL,
    lawful_basis_code varchar(20) NOT NULL,
    consent_purpose_id uuid,
    lia_assessment_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_purposes PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_data (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    data_category_id uuid NOT NULL,
    subject_type_id uuid NOT NULL,
    source text NOT NULL CHECK (source IN ('direct', 'indirect')),
    source_party_id uuid,
    is_sensitive boolean NOT NULL DEFAULT false,
    volume_band text CHECK (volume_band IN ('lt_1k', '1k_10k', '10k_100k', 'gt_100k')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_data PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_systems (
    activity_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    usage text NOT NULL CHECK (usage IN ('collect', 'store', 'process', 'transfer', 'archive')),
    CONSTRAINT pk_ropa_activity_systems PRIMARY KEY (activity_id, asset_id)
);

CREATE TABLE ropa.activity_recipients (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    party_id uuid NOT NULL,
    recipient_role text NOT NULL CHECK (recipient_role IN ('processor', 'controller', 'joint_controller', 'government')),
    disclosure_basis varchar(40),
    data_category_ids uuid[] NOT NULL DEFAULT '{}',
    agreement_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_recipients PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_transfers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    recipient_id uuid,
    country_code char(2) NOT NULL,
    transfer_basis text NOT NULL CHECK (transfer_basis IN ('adequacy', 'bcr', 'standard_clauses', 'certification', 'exemption', 'consent')),
    safeguards text,
    tia_assessment_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_transfers PRIMARY KEY (id)
);

CREATE TABLE ropa.retention_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    data_category_id uuid,
    retention_months int,
    retention_basis text NOT NULL,
    trigger_event varchar(60) NOT NULL,
    disposal_method text NOT NULL CHECK (disposal_method IN ('delete', 'destroy', 'anonymize', 'return')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_retention_rules PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_controls (
    activity_id uuid NOT NULL,
    control_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    description text,
    assessment_id uuid,
    CONSTRAINT pk_ropa_activity_controls PRIMARY KEY (activity_id, control_id)
);

CREATE TABLE ropa.activity_rejections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    dsar_request_id uuid NOT NULL,
    reason_code varchar(40) NOT NULL,
    rejected_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_rejections PRIMARY KEY (id)
);

CREATE TABLE ropa.assets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    asset_type text NOT NULL CHECK (asset_type IN ('application', 'database', 'file_share', 'saas', 'paper', 'device', 'other')),
    org_unit_id uuid,
    owner_user_id uuid,
    provider_party_id uuid,
    hosting_country_code char(2),
    hosting_type text CHECK (hosting_type IN ('on_prem', 'cloud', 'hybrid')),
    classification text CHECK (classification IN ('public', 'internal', 'confidential', 'restricted')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_assets PRIMARY KEY (id)
);

CREATE TABLE ropa.data_inventory (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    data_category_id uuid NOT NULL,
    org_unit_id uuid,
    owner_user_id uuid,
    source text CHECK (source IN ('direct', 'indirect', 'derived')),
    location_detail text,
    discovered_by_finding_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_data_inventory PRIMARY KEY (id)
);

CREATE TABLE ropa.questionnaires (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    activity_id uuid,
    form_submission_id uuid,
    respondent_user_id uuid NOT NULL,
    due_at timestamptz,
    status text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'answered', 'approved', 'rejected')),
    approved_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_questionnaires PRIMARY KEY (id)
);

CREATE TABLE ropa.sme_exemption_checks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    result text NOT NULL CHECK (result IN ('exempt', 'not_exempt', 'partial')),
    basis text NOT NULL,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_sme_exemption_checks PRIMARY KEY (id)
);

CREATE TABLE ropa.template_sets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    name text NOT NULL,
    set_type text NOT NULL CHECK (set_type IN ('standard', 'industry', 'government', 'processor', 'custom')),
    industry varchar(40),
    version_no int NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_template_sets PRIMARY KEY (id)
);

CREATE TABLE ropa.activity_templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    template_set_id uuid NOT NULL,
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text,
    job_category varchar(40) NOT NULL,
    role text NOT NULL CHECK (role IN ('controller', 'processor')),
    defaults jsonb NOT NULL,
    rationale jsonb NOT NULL,
    legal_refs text[] NOT NULL DEFAULT '{}',
    version_no int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_activity_templates PRIMARY KEY (id),
    CONSTRAINT uq_activity_templates_template_set_id_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, template_set_id, code, version_no)
);

CREATE TABLE ropa.generation_runs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    org_unit_id uuid NOT NULL,
    template_ids uuid[] NOT NULL,
    created_activity_ids uuid[] NOT NULL DEFAULT '{}',
    task_ids uuid[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_generation_runs PRIMARY KEY (id)
);

CREATE TABLE ropa.wizard_sessions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id uuid NOT NULL,
    answers jsonb NOT NULL,
    mapped_fields jsonb,
    ai_suggestions jsonb,
    activity_id uuid,
    status text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'abandoned')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_ropa_wizard_sessions PRIMARY KEY (id)
);

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

CREATE TABLE assess.templates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    assessment_type text NOT NULL CHECK (assessment_type IN ('dpia', 'pia', 'lia', 'tia', 'ai', 'security', 'maturity', 'dpo_check', 'sme_check', 'vendor', 'inbound_dpa', 'independence')),
    code varchar(60) NOT NULL,
    name text NOT NULL,
    form_id uuid NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    legal_refs text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_templates PRIMARY KEY (id),
    CONSTRAINT uq_templates_assessment_type_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, assessment_type, code, version_no)
);

CREATE TABLE assess.screening_rules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    criteria jsonb NOT NULL,
    min_factors smallint NOT NULL DEFAULT 2,
    min_score numeric(6,2),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_screening_rules PRIMARY KEY (id)
);

CREATE TABLE assess.assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_type varchar(20) NOT NULL,
    template_id uuid NOT NULL,
    form_version_id uuid NOT NULL,
    title text NOT NULL,
    subject_type text NOT NULL CHECK (subject_type IN ('activity', 'vendor', 'system', 'legal_entity', 'ai_system', 'transfer', 'project')),
    subject_id uuid,
    activity_id uuid,
    round_no smallint NOT NULL DEFAULT 1,
    previous_id uuid,
    status text NOT NULL DEFAULT 'screening' CHECK (status IN ('screening', 'not_required', 'in_progress', 'in_review', 'approved', 'rejected', 'needs_review', 'closed')),
    screening_result text CHECK (screening_result IN ('required', 'recommended', 'not_required')),
    screening_reason text,
    score numeric(8,2),
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'very_high')),
    owner_user_id uuid,
    due_at date,
    approved_at timestamptz,
    next_review_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_assessments PRIMARY KEY (id)
);

CREATE TABLE assess.sections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    section_code varchar(60) NOT NULL,
    assignee_user_id uuid,
    guest_token_id uuid,
    status text NOT NULL DEFAULT 'not_started' CHECK (status IN ('not_started', 'in_progress', 'submitted', 'needs_info')),
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_sections PRIMARY KEY (id)
);

CREATE TABLE assess.answers (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    section_id uuid,
    question_code varchar(80) NOT NULL,
    answer jsonb NOT NULL,
    evidence_file_ids uuid[] NOT NULL DEFAULT '{}',
    answered_by uuid,
    ai_suggested boolean NOT NULL DEFAULT false,
    confirmed_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_answers PRIMARY KEY (id)
);

CREATE TABLE assess.assessment_risks (
    assessment_id uuid NOT NULL,
    risk_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    CONSTRAINT pk_assess_assessment_risks PRIMARY KEY (assessment_id, risk_id)
);

CREATE TABLE assess.dpo_opinions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    dpo_user_id uuid NOT NULL,
    opinion text NOT NULL,
    recommendation text NOT NULL CHECK (recommendation IN ('proceed', 'proceed_with_conditions', 'do_not_proceed', 'consult_pdpc')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_dpo_opinions PRIMARY KEY (id)
);

CREATE TABLE assess.consultations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    stakeholder text NOT NULL,
    stakeholder_type text NOT NULL CHECK (stakeholder_type IN ('data_subject', 'processor', 'expert', 'internal', 'regulator')),
    consulted_at date NOT NULL,
    summary text NOT NULL,
    response text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_assess_consultations PRIMARY KEY (id)
);

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

CREATE TABLE gov.courses (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    title text NOT NULL,
    description text,
    content_type text NOT NULL CHECK (content_type IN ('video', 'slides', 'scorm', 'xapi')),
    content_file_id uuid,
    quiz_form_id uuid,
    passing_score smallint NOT NULL DEFAULT 80,
    validity_months smallint NOT NULL DEFAULT 12,
    language varchar(5) NOT NULL DEFAULT 'th',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_courses PRIMARY KEY (id)
);

CREATE TABLE gov.training_assignments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    course_id uuid NOT NULL,
    target_type text NOT NULL CHECK (target_type IN ('all', 'org_unit', 'group', 'user')),
    target_id uuid,
    due_at date NOT NULL,
    assigned_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_training_assignments PRIMARY KEY (id)
);

CREATE TABLE gov.training_attempts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    course_id uuid NOT NULL,
    assignment_id uuid,
    user_id uuid NOT NULL,
    started_at timestamptz NOT NULL,
    completed_at timestamptz,
    score smallint,
    passed boolean,
    certificate_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_training_attempts PRIMARY KEY (id)
);

CREATE TABLE gov.policies (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    document_id uuid NOT NULL,
    version_no int NOT NULL DEFAULT 1,
    requires_attestation boolean NOT NULL DEFAULT true,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_policies PRIMARY KEY (id),
    CONSTRAINT uq_policies_document_id_version_no UNIQUE (tenant_id, document_id, version_no)
);

CREATE TABLE gov.policy_attestations (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    policy_id uuid NOT NULL,
    policy_version_no int NOT NULL,
    user_id uuid NOT NULL,
    attested_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    CONSTRAINT pk_gov_policy_attestations PRIMARY KEY (id)
);

CREATE TABLE gov.audits (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    audit_type text NOT NULL CHECK (audit_type IN ('maturity', 'pdpa_compliance', 'security', 'related_law')),
    legal_entity_id uuid,
    assessment_id uuid NOT NULL,
    period varchar(20) NOT NULL,
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'in_progress', 'completed')),
    overall_score numeric(5,2),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_audits PRIMARY KEY (id)
);

CREATE TABLE gov.audit_findings (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    audit_id uuid NOT NULL,
    title text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    requirement_ref text,
    recommendation text,
    owner_user_id uuid,
    due_at date,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'accepted')),
    task_id uuid,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_audit_findings PRIMARY KEY (id)
);

CREATE TABLE gov.retention_schedules (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    retention_rule_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    next_due_at date NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_retention_schedules PRIMARY KEY (id)
);

CREATE TABLE gov.disposal_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    due_at date NOT NULL,
    method text NOT NULL CHECK (method IN ('delete', 'destroy', 'anonymize', 'return')),
    legal_hold_id uuid,
    assignee_user_id uuid,
    approved_by uuid,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'in_progress', 'done', 'on_hold')),
    record_count int,
    completed_at timestamptz,
    evidence_file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_disposal_jobs PRIMARY KEY (id)
);

CREATE TABLE gov.regulator_letters (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    direction text NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
    letter_no varchar(60),
    subject text NOT NULL,
    received_at date,
    due_at date,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'responded', 'closed')),
    file_id uuid,
    incident_id uuid,
    responded_at date,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulator_letters PRIMARY KEY (id)
);

CREATE TABLE gov.regulatory_updates (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    title text NOT NULL,
    source_url text,
    published_at date NOT NULL,
    summary text NOT NULL,
    impact_areas text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulatory_updates PRIMARY KEY (id)
);

CREATE TABLE gov.regulatory_reviews (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    update_id uuid NOT NULL,
    assessment_id uuid,
    status text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewing', 'done', 'not_applicable')),
    reviewed_by uuid,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulatory_reviews PRIMARY KEY (id)
);

CREATE TABLE gov.ai_systems (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    vendor_party_id uuid,
    purpose text NOT NULL,
    model_type varchar(60),
    data_category_ids uuid[] NOT NULL DEFAULT '{}',
    automated_decision boolean NOT NULL DEFAULT false,
    risk_level text CHECK (risk_level IN ('low', 'medium', 'high', 'unacceptable')),
    assessment_id uuid,
    owner_user_id uuid,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('planned', 'active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_ai_systems PRIMARY KEY (id)
);

CREATE TABLE gov.masking_jobs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    source_file_id uuid NOT NULL,
    output_file_id uuid,
    rules jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    requested_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_masking_jobs PRIMARY KEY (id)
);

CREATE TABLE gov.kb_chunks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid,
    kb_article_id uuid NOT NULL,
    chunk_no int NOT NULL,
    content text NOT NULL,
    embedding vector(1024) NOT NULL,
    CONSTRAINT pk_gov_kb_chunks PRIMARY KEY (id)
);

-- ---------------------------------------------------------------- foreign keys
ALTER TABLE platform.tenant_modules ADD CONSTRAINT fk_tenant_modules_tenant_id FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id) ON DELETE CASCADE;
ALTER TABLE platform.public_keys ADD CONSTRAINT fk_public_keys_tenant_id FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.workflow_definitions ADD CONSTRAINT fk_workflow_definitions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.workflow_instances ADD CONSTRAINT fk_workflow_instances_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.workflow_instances ADD CONSTRAINT fk_workflow_instances_definition_id FOREIGN KEY (definition_id) REFERENCES platform.workflow_definitions (id);
ALTER TABLE platform.workflow_tasks ADD CONSTRAINT fk_workflow_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.workflow_tasks ADD CONSTRAINT fk_workflow_tasks_instance_id FOREIGN KEY (instance_id) REFERENCES platform.workflow_instances (id);
ALTER TABLE platform.workflow_tasks ADD CONSTRAINT fk_workflow_tasks_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE platform.workflow_tasks ADD CONSTRAINT fk_workflow_tasks_assignee_group_id FOREIGN KEY (assignee_group_id) REFERENCES iam.groups (id);
ALTER TABLE platform.sla_timers ADD CONSTRAINT fk_sla_timers_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.sla_timers ADD CONSTRAINT fk_sla_timers_instance_id FOREIGN KEY (instance_id) REFERENCES platform.workflow_instances (id);
ALTER TABLE platform.sla_timers ADD CONSTRAINT fk_sla_timers_calendar_id FOREIGN KEY (calendar_id) REFERENCES org.business_calendars (id);
ALTER TABLE platform.form_definitions ADD CONSTRAINT fk_form_definitions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.form_versions ADD CONSTRAINT fk_form_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.form_versions ADD CONSTRAINT fk_form_versions_form_id FOREIGN KEY (form_id) REFERENCES platform.form_definitions (id);
ALTER TABLE platform.form_versions ADD CONSTRAINT fk_form_versions_published_by FOREIGN KEY (published_by) REFERENCES iam.users (id);
ALTER TABLE platform.form_submissions ADD CONSTRAINT fk_form_submissions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.form_submissions ADD CONSTRAINT fk_form_submissions_form_version_id FOREIGN KEY (form_version_id) REFERENCES platform.form_versions (id);
ALTER TABLE platform.record_versions ADD CONSTRAINT fk_record_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.approvals ADD CONSTRAINT fk_approvals_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.approvals ADD CONSTRAINT fk_approvals_record_version_id FOREIGN KEY (record_version_id) REFERENCES platform.record_versions (id);
ALTER TABLE platform.approvals ADD CONSTRAINT fk_approvals_requested_by FOREIGN KEY (requested_by) REFERENCES iam.users (id);
ALTER TABLE platform.approvals ADD CONSTRAINT fk_approvals_approver_user_id FOREIGN KEY (approver_user_id) REFERENCES iam.users (id);
ALTER TABLE platform.comments ADD CONSTRAINT fk_comments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.comments ADD CONSTRAINT fk_comments_parent_id FOREIGN KEY (parent_id) REFERENCES platform.comments (id);
ALTER TABLE platform.files ADD CONSTRAINT fk_files_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.documents ADD CONSTRAINT fk_documents_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.documents ADD CONSTRAINT fk_documents_template_id FOREIGN KEY (template_id) REFERENCES platform.templates (id);
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_document_id FOREIGN KEY (document_id) REFERENCES platform.documents (id);
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_pdf_file_id FOREIGN KEY (pdf_file_id) REFERENCES platform.files (id);
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_docx_file_id FOREIGN KEY (docx_file_id) REFERENCES platform.files (id);
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE platform.templates ADD CONSTRAINT fk_templates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.clause_library ADD CONSTRAINT fk_clause_library_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.notification_templates ADD CONSTRAINT fk_notification_templates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.notifications ADD CONSTRAINT fk_notifications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.notifications ADD CONSTRAINT fk_notifications_template_id FOREIGN KEY (template_id) REFERENCES platform.notification_templates (id);
ALTER TABLE platform.notifications ADD CONSTRAINT fk_notifications_recipient_user_id FOREIGN KEY (recipient_user_id) REFERENCES iam.users (id);
ALTER TABLE platform.outbox_events ADD CONSTRAINT fk_outbox_events_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.webhook_subscriptions ADD CONSTRAINT fk_webhook_subscriptions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.webhook_subscriptions ADD CONSTRAINT fk_webhook_subscriptions_api_client_id FOREIGN KEY (api_client_id) REFERENCES iam.api_clients (id);
ALTER TABLE platform.webhook_deliveries ADD CONSTRAINT fk_webhook_deliveries_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.webhook_deliveries ADD CONSTRAINT fk_webhook_deliveries_subscription_id FOREIGN KEY (subscription_id) REFERENCES platform.webhook_subscriptions (id);
ALTER TABLE platform.webhook_deliveries ADD CONSTRAINT fk_webhook_deliveries_event_id FOREIGN KEY (event_id) REFERENCES platform.outbox_events (id);
ALTER TABLE platform.audit_log ADD CONSTRAINT fk_audit_log_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.import_jobs ADD CONSTRAINT fk_import_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.import_jobs ADD CONSTRAINT fk_import_jobs_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE platform.import_jobs ADD CONSTRAINT fk_import_jobs_error_file_id FOREIGN KEY (error_file_id) REFERENCES platform.files (id);
ALTER TABLE platform.export_jobs ADD CONSTRAINT fk_export_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.export_jobs ADD CONSTRAINT fk_export_jobs_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE platform.export_jobs ADD CONSTRAINT fk_export_jobs_requested_by FOREIGN KEY (requested_by) REFERENCES iam.users (id);
ALTER TABLE platform.connectors ADD CONSTRAINT fk_connectors_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.connectors ADD CONSTRAINT fk_connectors_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE platform.ai_requests ADD CONSTRAINT fk_ai_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.ai_requests ADD CONSTRAINT fk_ai_requests_confirmed_by FOREIGN KEY (confirmed_by) REFERENCES iam.users (id);
ALTER TABLE iam.users ADD CONSTRAINT fk_users_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.users ADD CONSTRAINT fk_users_primary_org_unit_id FOREIGN KEY (primary_org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE iam.users ADD CONSTRAINT fk_users_avatar_file_id FOREIGN KEY (avatar_file_id) REFERENCES platform.files (id);
ALTER TABLE iam.roles ADD CONSTRAINT fk_roles_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.roles ADD CONSTRAINT fk_roles_cloned_from_id FOREIGN KEY (cloned_from_id) REFERENCES iam.roles (id);
ALTER TABLE iam.role_permissions ADD CONSTRAINT fk_role_permissions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.role_permissions ADD CONSTRAINT fk_role_permissions_role_id FOREIGN KEY (role_id) REFERENCES iam.roles (id) ON DELETE CASCADE;
ALTER TABLE iam.role_permissions ADD CONSTRAINT fk_role_permissions_permission_code FOREIGN KEY (permission_code) REFERENCES iam.permissions (code) ON DELETE CASCADE;
ALTER TABLE iam.groups ADD CONSTRAINT fk_groups_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.group_members ADD CONSTRAINT fk_group_members_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.group_members ADD CONSTRAINT fk_group_members_group_id FOREIGN KEY (group_id) REFERENCES iam.groups (id) ON DELETE CASCADE;
ALTER TABLE iam.group_members ADD CONSTRAINT fk_group_members_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id) ON DELETE CASCADE;
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_group_id FOREIGN KEY (group_id) REFERENCES iam.groups (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_role_id FOREIGN KEY (role_id) REFERENCES iam.roles (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_granted_by FOREIGN KEY (granted_by) REFERENCES iam.users (id);
ALTER TABLE iam.role_assignments ADD CONSTRAINT fk_role_assignments_approval_id FOREIGN KEY (approval_id) REFERENCES platform.approvals (id);
ALTER TABLE iam.api_clients ADD CONSTRAINT fk_api_clients_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.api_clients ADD CONSTRAINT fk_api_clients_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE iam.api_clients ADD CONSTRAINT fk_api_clients_system_asset_id FOREIGN KEY (system_asset_id) REFERENCES ropa.assets (id);
ALTER TABLE iam.guest_tokens ADD CONSTRAINT fk_guest_tokens_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.guest_tokens ADD CONSTRAINT fk_guest_tokens_issued_by FOREIGN KEY (issued_by) REFERENCES iam.users (id);
ALTER TABLE iam.idp_configs ADD CONSTRAINT fk_idp_configs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.security_policies ADD CONSTRAINT fk_security_policies_tenant_id FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id) ON DELETE CASCADE;
ALTER TABLE iam.security_events ADD CONSTRAINT fk_security_events_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.security_events ADD CONSTRAINT fk_security_events_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE iam.access_reviews ADD CONSTRAINT fk_access_reviews_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.access_review_items ADD CONSTRAINT fk_access_review_items_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.access_review_items ADD CONSTRAINT fk_access_review_items_review_id FOREIGN KEY (review_id) REFERENCES iam.access_reviews (id);
ALTER TABLE iam.access_review_items ADD CONSTRAINT fk_access_review_items_role_assignment_id FOREIGN KEY (role_assignment_id) REFERENCES iam.role_assignments (id);
ALTER TABLE iam.access_review_items ADD CONSTRAINT fk_access_review_items_reviewer_user_id FOREIGN KEY (reviewer_user_id) REFERENCES iam.users (id);
ALTER TABLE iam.breakglass_requests ADD CONSTRAINT fk_breakglass_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.breakglass_requests ADD CONSTRAINT fk_breakglass_requests_requested_by FOREIGN KEY (requested_by) REFERENCES iam.users (id);
ALTER TABLE iam.breakglass_requests ADD CONSTRAINT fk_breakglass_requests_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE iam.delegations ADD CONSTRAINT fk_delegations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.delegations ADD CONSTRAINT fk_delegations_from_user_id FOREIGN KEY (from_user_id) REFERENCES iam.users (id);
ALTER TABLE iam.delegations ADD CONSTRAINT fk_delegations_to_user_id FOREIGN KEY (to_user_id) REFERENCES iam.users (id);
ALTER TABLE iam.delegations ADD CONSTRAINT fk_delegations_role_id FOREIGN KEY (role_id) REFERENCES iam.roles (id);
ALTER TABLE iam.field_masking_rules ADD CONSTRAINT fk_field_masking_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.field_masking_rules ADD CONSTRAINT fk_field_masking_rules_unmask_permission FOREIGN KEY (unmask_permission) REFERENCES iam.permissions (code);
ALTER TABLE iam.unmask_logs ADD CONSTRAINT fk_unmask_logs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.unmask_logs ADD CONSTRAINT fk_unmask_logs_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE iam.subject_verifications ADD CONSTRAINT fk_subject_verifications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE iam.subject_verifications ADD CONSTRAINT fk_subject_verifications_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE org.legal_entities ADD CONSTRAINT fk_legal_entities_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.legal_entities ADD CONSTRAINT fk_legal_entities_parent_id FOREIGN KEY (parent_id) REFERENCES org.legal_entities (id);
ALTER TABLE org.legal_entities ADD CONSTRAINT fk_legal_entities_logo_file_id FOREIGN KEY (logo_file_id) REFERENCES platform.files (id);
ALTER TABLE org.org_units ADD CONSTRAINT fk_org_units_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.org_units ADD CONSTRAINT fk_org_units_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE org.org_units ADD CONSTRAINT fk_org_units_parent_id FOREIGN KEY (parent_id) REFERENCES org.org_units (id);
ALTER TABLE org.privacy_champions ADD CONSTRAINT fk_privacy_champions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.privacy_champions ADD CONSTRAINT fk_privacy_champions_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE org.privacy_champions ADD CONSTRAINT fk_privacy_champions_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE org.external_parties ADD CONSTRAINT fk_external_parties_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.external_parties ADD CONSTRAINT fk_external_parties_country_code FOREIGN KEY (country_code) REFERENCES org.countries (code);
ALTER TABLE org.data_categories ADD CONSTRAINT fk_data_categories_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.data_categories ADD CONSTRAINT fk_data_categories_parent_id FOREIGN KEY (parent_id) REFERENCES org.data_categories (id);
ALTER TABLE org.data_subject_types ADD CONSTRAINT fk_data_subject_types_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.processing_purposes ADD CONSTRAINT fk_processing_purposes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.business_calendars ADD CONSTRAINT fk_business_calendars_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.holidays ADD CONSTRAINT fk_holidays_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE org.holidays ADD CONSTRAINT fk_holidays_calendar_id FOREIGN KEY (calendar_id) REFERENCES org.business_calendars (id) ON DELETE CASCADE;
ALTER TABLE org.org_settings ADD CONSTRAINT fk_org_settings_tenant_id FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id) ON DELETE CASCADE;
ALTER TABLE org.org_settings ADD CONSTRAINT fk_org_settings_default_calendar_id FOREIGN KEY (default_calendar_id) REFERENCES org.business_calendars (id);
ALTER TABLE consent.data_elements ADD CONSTRAINT fk_data_elements_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.data_elements ADD CONSTRAINT fk_data_elements_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE consent.purposes ADD CONSTRAINT fk_purposes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.purposes ADD CONSTRAINT fk_purposes_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE consent.purposes ADD CONSTRAINT fk_purposes_lawful_basis_code FOREIGN KEY (lawful_basis_code) REFERENCES org.lawful_bases (code);
ALTER TABLE consent.purpose_versions ADD CONSTRAINT fk_purpose_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.purpose_versions ADD CONSTRAINT fk_purpose_versions_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE consent.purpose_versions ADD CONSTRAINT fk_purpose_versions_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE consent.purpose_preferences ADD CONSTRAINT fk_purpose_preferences_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.purpose_preferences ADD CONSTRAINT fk_purpose_preferences_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE consent.purpose_data_elements ADD CONSTRAINT fk_purpose_data_elements_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.purpose_data_elements ADD CONSTRAINT fk_purpose_data_elements_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id) ON DELETE CASCADE;
ALTER TABLE consent.purpose_data_elements ADD CONSTRAINT fk_purpose_data_elements_data_element_id FOREIGN KEY (data_element_id) REFERENCES consent.data_elements (id) ON DELETE CASCADE;
ALTER TABLE consent.collection_points ADD CONSTRAINT fk_collection_points_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.collection_points ADD CONSTRAINT fk_collection_points_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE consent.collection_points ADD CONSTRAINT fk_collection_points_form_id FOREIGN KEY (form_id) REFERENCES platform.form_definitions (id);
ALTER TABLE consent.collection_points ADD CONSTRAINT fk_collection_points_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id);
ALTER TABLE consent.collection_point_purposes ADD CONSTRAINT fk_collection_point_purposes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.collection_point_purposes ADD CONSTRAINT fk_collection_point_purposes_collection_point_id FOREIGN KEY (collection_point_id) REFERENCES consent.collection_points (id) ON DELETE CASCADE;
ALTER TABLE consent.collection_point_purposes ADD CONSTRAINT fk_collection_point_purposes_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id) ON DELETE CASCADE;
ALTER TABLE consent.data_subjects ADD CONSTRAINT fk_data_subjects_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.data_subjects ADD CONSTRAINT fk_data_subjects_guardian_subject_id FOREIGN KEY (guardian_subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.subject_identifiers ADD CONSTRAINT fk_subject_identifiers_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.subject_identifiers ADD CONSTRAINT fk_subject_identifiers_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_collection_point_id FOREIGN KEY (collection_point_id) REFERENCES consent.collection_points (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_captured_by_user_id FOREIGN KEY (captured_by_user_id) REFERENCES iam.users (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_branch_org_unit_id FOREIGN KEY (branch_org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_notice_version_id FOREIGN KEY (notice_version_id) REFERENCES notice.notice_versions (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_verification_id FOREIGN KEY (verification_id) REFERENCES iam.subject_verifications (id);
ALTER TABLE consent.consent_receipts ADD CONSTRAINT fk_consent_receipts_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE consent.consent_transactions ADD CONSTRAINT fk_consent_transactions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.consent_transactions ADD CONSTRAINT fk_consent_transactions_receipt_id FOREIGN KEY (receipt_id) REFERENCES consent.consent_receipts (id);
ALTER TABLE consent.consent_transactions ADD CONSTRAINT fk_consent_transactions_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.consent_transactions ADD CONSTRAINT fk_consent_transactions_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE consent.consent_transactions ADD CONSTRAINT fk_consent_transactions_purpose_version_id FOREIGN KEY (purpose_version_id) REFERENCES consent.purpose_versions (id);
ALTER TABLE consent.consent_status ADD CONSTRAINT fk_consent_status_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.consent_status ADD CONSTRAINT fk_consent_status_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id) ON DELETE CASCADE;
ALTER TABLE consent.consent_status ADD CONSTRAINT fk_consent_status_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id) ON DELETE CASCADE;
ALTER TABLE consent.consent_status ADD CONSTRAINT fk_consent_status_purpose_version_id FOREIGN KEY (purpose_version_id) REFERENCES consent.purpose_versions (id);
ALTER TABLE consent.double_optin_requests ADD CONSTRAINT fk_double_optin_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.guardian_approvals ADD CONSTRAINT fk_guardian_approvals_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.guardian_approvals ADD CONSTRAINT fk_guardian_approvals_minor_subject_id FOREIGN KEY (minor_subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.guardian_approvals ADD CONSTRAINT fk_guardian_approvals_guardian_subject_id FOREIGN KEY (guardian_subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.guardian_approvals ADD CONSTRAINT fk_guardian_approvals_receipt_id FOREIGN KEY (receipt_id) REFERENCES consent.consent_receipts (id);
ALTER TABLE consent.campaigns ADD CONSTRAINT fk_campaigns_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.campaigns ADD CONSTRAINT fk_campaigns_template_id FOREIGN KEY (template_id) REFERENCES platform.notification_templates (id);
ALTER TABLE consent.campaign_recipients ADD CONSTRAINT fk_campaign_recipients_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.campaign_recipients ADD CONSTRAINT fk_campaign_recipients_campaign_id FOREIGN KEY (campaign_id) REFERENCES consent.campaigns (id);
ALTER TABLE consent.campaign_recipients ADD CONSTRAINT fk_campaign_recipients_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.reconcile_runs ADD CONSTRAINT fk_reconcile_runs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.reconcile_runs ADD CONSTRAINT fk_reconcile_runs_target_api_client_id FOREIGN KEY (target_api_client_id) REFERENCES iam.api_clients (id);
ALTER TABLE consent.reconcile_runs ADD CONSTRAINT fk_reconcile_runs_connector_id FOREIGN KEY (connector_id) REFERENCES platform.connectors (id);
ALTER TABLE consent.reconcile_runs ADD CONSTRAINT fk_reconcile_runs_report_file_id FOREIGN KEY (report_file_id) REFERENCES platform.files (id);
ALTER TABLE consent.reconcile_items ADD CONSTRAINT fk_reconcile_items_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.reconcile_items ADD CONSTRAINT fk_reconcile_items_run_id FOREIGN KEY (run_id) REFERENCES consent.reconcile_runs (id);
ALTER TABLE consent.reconcile_items ADD CONSTRAINT fk_reconcile_items_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE consent.reconcile_items ADD CONSTRAINT fk_reconcile_items_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE consent.downstream_syncs ADD CONSTRAINT fk_downstream_syncs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE consent.downstream_syncs ADD CONSTRAINT fk_downstream_syncs_webhook_delivery_id FOREIGN KEY (webhook_delivery_id) REFERENCES platform.webhook_deliveries (id);
ALTER TABLE consent.downstream_syncs ADD CONSTRAINT fk_downstream_syncs_target_api_client_id FOREIGN KEY (target_api_client_id) REFERENCES iam.api_clients (id);
ALTER TABLE cookie.domains ADD CONSTRAINT fk_domains_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.domains ADD CONSTRAINT fk_domains_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE cookie.apps ADD CONSTRAINT fk_apps_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.banner_configs ADD CONSTRAINT fk_banner_configs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.banner_configs ADD CONSTRAINT fk_banner_configs_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.banner_configs ADD CONSTRAINT fk_banner_configs_published_by FOREIGN KEY (published_by) REFERENCES iam.users (id);
ALTER TABLE cookie.categories ADD CONSTRAINT fk_categories_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.categories ADD CONSTRAINT fk_categories_purpose_id FOREIGN KEY (purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE cookie.cookies ADD CONSTRAINT fk_cookies_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.cookies ADD CONSTRAINT fk_cookies_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.cookies ADD CONSTRAINT fk_cookies_category_id FOREIGN KEY (category_id) REFERENCES cookie.categories (id);
ALTER TABLE cookie.cookies ADD CONSTRAINT fk_cookies_kb_id FOREIGN KEY (kb_id) REFERENCES cookie.cookie_kb (id);
ALTER TABLE cookie.cookies ADD CONSTRAINT fk_cookies_first_seen_scan_id FOREIGN KEY (first_seen_scan_id) REFERENCES cookie.scans (id);
ALTER TABLE cookie.scans ADD CONSTRAINT fk_scans_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.scans ADD CONSTRAINT fk_scans_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.scans ADD CONSTRAINT fk_scans_report_file_id FOREIGN KEY (report_file_id) REFERENCES platform.files (id);
ALTER TABLE cookie.scan_findings ADD CONSTRAINT fk_scan_findings_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.scan_findings ADD CONSTRAINT fk_scan_findings_scan_id FOREIGN KEY (scan_id) REFERENCES cookie.scans (id);
ALTER TABLE cookie.scan_findings ADD CONSTRAINT fk_scan_findings_matched_cookie_id FOREIGN KEY (matched_cookie_id) REFERENCES cookie.cookies (id);
ALTER TABLE cookie.script_rules ADD CONSTRAINT fk_script_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.script_rules ADD CONSTRAINT fk_script_rules_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.script_rules ADD CONSTRAINT fk_script_rules_category_id FOREIGN KEY (category_id) REFERENCES cookie.categories (id);
ALTER TABLE cookie.ab_variants ADD CONSTRAINT fk_ab_variants_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.ab_variants ADD CONSTRAINT fk_ab_variants_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.ab_variants ADD CONSTRAINT fk_ab_variants_banner_config_id FOREIGN KEY (banner_config_id) REFERENCES cookie.banner_configs (id);
ALTER TABLE cookie.consent_records ADD CONSTRAINT fk_consent_records_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE cookie.consent_records ADD CONSTRAINT fk_consent_records_domain_id FOREIGN KEY (domain_id) REFERENCES cookie.domains (id);
ALTER TABLE cookie.consent_records ADD CONSTRAINT fk_consent_records_banner_config_id FOREIGN KEY (banner_config_id) REFERENCES cookie.banner_configs (id);
ALTER TABLE cookie.consent_records ADD CONSTRAINT fk_consent_records_ab_variant_id FOREIGN KEY (ab_variant_id) REFERENCES cookie.ab_variants (id);
ALTER TABLE cookie.consent_records ADD CONSTRAINT fk_consent_records_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE notice.notices ADD CONSTRAINT fk_notices_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.notices ADD CONSTRAINT fk_notices_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE notice.notices ADD CONSTRAINT fk_notices_subject_type_id FOREIGN KEY (subject_type_id) REFERENCES org.data_subject_types (id);
ALTER TABLE notice.notices ADD CONSTRAINT fk_notices_document_id FOREIGN KEY (document_id) REFERENCES platform.documents (id);
ALTER TABLE notice.notices ADD CONSTRAINT fk_notices_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE notice.notice_versions ADD CONSTRAINT fk_notice_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.notice_versions ADD CONSTRAINT fk_notice_versions_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id);
ALTER TABLE notice.notice_versions ADD CONSTRAINT fk_notice_versions_document_version_id FOREIGN KEY (document_version_id) REFERENCES platform.document_versions (id);
ALTER TABLE notice.notice_versions ADD CONSTRAINT fk_notice_versions_published_by FOREIGN KEY (published_by) REFERENCES iam.users (id);
ALTER TABLE notice.notice_activity_links ADD CONSTRAINT fk_notice_activity_links_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.notice_activity_links ADD CONSTRAINT fk_notice_activity_links_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id) ON DELETE CASCADE;
ALTER TABLE notice.notice_activity_links ADD CONSTRAINT fk_notice_activity_links_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id) ON DELETE CASCADE;
ALTER TABLE notice.acknowledgements ADD CONSTRAINT fk_acknowledgements_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.acknowledgements ADD CONSTRAINT fk_acknowledgements_notice_version_id FOREIGN KEY (notice_version_id) REFERENCES notice.notice_versions (id);
ALTER TABLE notice.acknowledgements ADD CONSTRAINT fk_acknowledgements_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE notice.acknowledgements ADD CONSTRAINT fk_acknowledgements_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE notice.acknowledgements ADD CONSTRAINT fk_acknowledgements_receipt_id FOREIGN KEY (receipt_id) REFERENCES consent.consent_receipts (id);
ALTER TABLE notice.indirect_collections ADD CONSTRAINT fk_indirect_collections_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.indirect_collections ADD CONSTRAINT fk_indirect_collections_source_party_id FOREIGN KEY (source_party_id) REFERENCES org.external_parties (id);
ALTER TABLE notice.indirect_collections ADD CONSTRAINT fk_indirect_collections_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE notice.indirect_collections ADD CONSTRAINT fk_indirect_collections_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id);
ALTER TABLE notice.indirect_collections ADD CONSTRAINT fk_indirect_collections_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE notice.embeds ADD CONSTRAINT fk_embeds_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.embeds ADD CONSTRAINT fk_embeds_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id);
ALTER TABLE notice.linked_documents ADD CONSTRAINT fk_linked_documents_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.linked_documents ADD CONSTRAINT fk_linked_documents_notice_id FOREIGN KEY (notice_id) REFERENCES notice.notices (id);
ALTER TABLE notice.linked_documents ADD CONSTRAINT fk_linked_documents_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE notice.wizard_templates ADD CONSTRAINT fk_wizard_templates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE notice.wizard_templates ADD CONSTRAINT fk_wizard_templates_template_id FOREIGN KEY (template_id) REFERENCES platform.templates (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_controller_party_id FOREIGN KEY (controller_party_id) REFERENCES org.external_parties (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_template_id FOREIGN KEY (template_id) REFERENCES ropa.activity_templates (id);
ALTER TABLE ropa.processing_activities ADD CONSTRAINT fk_processing_activities_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_purpose_id FOREIGN KEY (purpose_id) REFERENCES org.processing_purposes (id);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_lawful_basis_code FOREIGN KEY (lawful_basis_code) REFERENCES org.lawful_bases (code);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_consent_purpose_id FOREIGN KEY (consent_purpose_id) REFERENCES consent.purposes (id);
ALTER TABLE ropa.activity_purposes ADD CONSTRAINT fk_activity_purposes_lia_assessment_id FOREIGN KEY (lia_assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE ropa.activity_data ADD CONSTRAINT fk_activity_data_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_data ADD CONSTRAINT fk_activity_data_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_data ADD CONSTRAINT fk_activity_data_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE ropa.activity_data ADD CONSTRAINT fk_activity_data_subject_type_id FOREIGN KEY (subject_type_id) REFERENCES org.data_subject_types (id);
ALTER TABLE ropa.activity_data ADD CONSTRAINT fk_activity_data_source_party_id FOREIGN KEY (source_party_id) REFERENCES org.external_parties (id);
ALTER TABLE ropa.activity_systems ADD CONSTRAINT fk_activity_systems_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_systems ADD CONSTRAINT fk_activity_systems_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id) ON DELETE CASCADE;
ALTER TABLE ropa.activity_systems ADD CONSTRAINT fk_activity_systems_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id) ON DELETE CASCADE;
ALTER TABLE ropa.activity_recipients ADD CONSTRAINT fk_activity_recipients_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_recipients ADD CONSTRAINT fk_activity_recipients_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_recipients ADD CONSTRAINT fk_activity_recipients_party_id FOREIGN KEY (party_id) REFERENCES org.external_parties (id);
ALTER TABLE ropa.activity_recipients ADD CONSTRAINT fk_activity_recipients_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE ropa.activity_transfers ADD CONSTRAINT fk_activity_transfers_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_transfers ADD CONSTRAINT fk_activity_transfers_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_transfers ADD CONSTRAINT fk_activity_transfers_recipient_id FOREIGN KEY (recipient_id) REFERENCES ropa.activity_recipients (id);
ALTER TABLE ropa.activity_transfers ADD CONSTRAINT fk_activity_transfers_country_code FOREIGN KEY (country_code) REFERENCES org.countries (code);
ALTER TABLE ropa.activity_transfers ADD CONSTRAINT fk_activity_transfers_tia_assessment_id FOREIGN KEY (tia_assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE ropa.retention_rules ADD CONSTRAINT fk_retention_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.retention_rules ADD CONSTRAINT fk_retention_rules_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.retention_rules ADD CONSTRAINT fk_retention_rules_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE ropa.activity_controls ADD CONSTRAINT fk_activity_controls_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_controls ADD CONSTRAINT fk_activity_controls_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id) ON DELETE CASCADE;
ALTER TABLE ropa.activity_controls ADD CONSTRAINT fk_activity_controls_control_id FOREIGN KEY (control_id) REFERENCES risk.controls (id) ON DELETE CASCADE;
ALTER TABLE ropa.activity_controls ADD CONSTRAINT fk_activity_controls_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE ropa.activity_rejections ADD CONSTRAINT fk_activity_rejections_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_rejections ADD CONSTRAINT fk_activity_rejections_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_rejections ADD CONSTRAINT fk_activity_rejections_dsar_request_id FOREIGN KEY (dsar_request_id) REFERENCES dsar.requests (id);
ALTER TABLE ropa.assets ADD CONSTRAINT fk_assets_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.assets ADD CONSTRAINT fk_assets_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE ropa.assets ADD CONSTRAINT fk_assets_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE ropa.assets ADD CONSTRAINT fk_assets_provider_party_id FOREIGN KEY (provider_party_id) REFERENCES org.external_parties (id);
ALTER TABLE ropa.assets ADD CONSTRAINT fk_assets_hosting_country_code FOREIGN KEY (hosting_country_code) REFERENCES org.countries (code);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE ropa.data_inventory ADD CONSTRAINT fk_data_inventory_discovered_by_finding_id FOREIGN KEY (discovered_by_finding_id) REFERENCES dataflow.discovery_findings (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_respondent_user_id FOREIGN KEY (respondent_user_id) REFERENCES iam.users (id);
ALTER TABLE ropa.questionnaires ADD CONSTRAINT fk_questionnaires_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE ropa.sme_exemption_checks ADD CONSTRAINT fk_sme_exemption_checks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.sme_exemption_checks ADD CONSTRAINT fk_sme_exemption_checks_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE ropa.sme_exemption_checks ADD CONSTRAINT fk_sme_exemption_checks_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE ropa.template_sets ADD CONSTRAINT fk_template_sets_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_templates ADD CONSTRAINT fk_activity_templates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_templates ADD CONSTRAINT fk_activity_templates_template_set_id FOREIGN KEY (template_set_id) REFERENCES ropa.template_sets (id);
ALTER TABLE ropa.generation_runs ADD CONSTRAINT fk_generation_runs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.generation_runs ADD CONSTRAINT fk_generation_runs_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE ropa.wizard_sessions ADD CONSTRAINT fk_wizard_sessions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.wizard_sessions ADD CONSTRAINT fk_wizard_sessions_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE ropa.wizard_sessions ADD CONSTRAINT fk_wizard_sessions_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE dataflow.layouts ADD CONSTRAINT fk_layouts_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dataflow.layouts ADD CONSTRAINT fk_layouts_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE dataflow.snapshots ADD CONSTRAINT fk_snapshots_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dataflow.snapshots ADD CONSTRAINT fk_snapshots_image_file_id FOREIGN KEY (image_file_id) REFERENCES platform.files (id);
ALTER TABLE dataflow.snapshots ADD CONSTRAINT fk_snapshots_published_by FOREIGN KEY (published_by) REFERENCES iam.users (id);
ALTER TABLE dataflow.classifiers ADD CONSTRAINT fk_classifiers_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dataflow.classifiers ADD CONSTRAINT fk_classifiers_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE dataflow.discovery_scans ADD CONSTRAINT fk_discovery_scans_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dataflow.discovery_scans ADD CONSTRAINT fk_discovery_scans_connector_id FOREIGN KEY (connector_id) REFERENCES platform.connectors (id);
ALTER TABLE dataflow.discovery_scans ADD CONSTRAINT fk_discovery_scans_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE dataflow.discovery_findings ADD CONSTRAINT fk_discovery_findings_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dataflow.discovery_findings ADD CONSTRAINT fk_discovery_findings_scan_id FOREIGN KEY (scan_id) REFERENCES dataflow.discovery_scans (id);
ALTER TABLE dataflow.discovery_findings ADD CONSTRAINT fk_discovery_findings_classifier_id FOREIGN KEY (classifier_id) REFERENCES dataflow.classifiers (id);
ALTER TABLE dataflow.discovery_findings ADD CONSTRAINT fk_discovery_findings_suggested_category_id FOREIGN KEY (suggested_category_id) REFERENCES org.data_categories (id);
ALTER TABLE dataflow.discovery_findings ADD CONSTRAINT fk_discovery_findings_reviewed_by FOREIGN KEY (reviewed_by) REFERENCES iam.users (id);
ALTER TABLE risk.risk_matrices ADD CONSTRAINT fk_risk_matrices_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.risk_factors ADD CONSTRAINT fk_risk_factors_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.controls ADD CONSTRAINT fk_controls_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.activity_scores ADD CONSTRAINT fk_activity_scores_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.activity_scores ADD CONSTRAINT fk_activity_scores_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE risk.activity_scores ADD CONSTRAINT fk_activity_scores_matrix_id FOREIGN KEY (matrix_id) REFERENCES risk.risk_matrices (id);
ALTER TABLE risk.risks ADD CONSTRAINT fk_risks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.risks ADD CONSTRAINT fk_risks_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE risk.risks ADD CONSTRAINT fk_risks_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE risk.risks ADD CONSTRAINT fk_risks_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE risk.risks ADD CONSTRAINT fk_risks_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE risk.risk_controls ADD CONSTRAINT fk_risk_controls_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.risk_controls ADD CONSTRAINT fk_risk_controls_risk_id FOREIGN KEY (risk_id) REFERENCES risk.risks (id) ON DELETE CASCADE;
ALTER TABLE risk.risk_controls ADD CONSTRAINT fk_risk_controls_control_id FOREIGN KEY (control_id) REFERENCES risk.controls (id) ON DELETE CASCADE;
ALTER TABLE risk.risk_controls ADD CONSTRAINT fk_risk_controls_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE risk.risk_controls ADD CONSTRAINT fk_risk_controls_task_id FOREIGN KEY (task_id) REFERENCES dpo.tasks (id);
ALTER TABLE risk.acceptances ADD CONSTRAINT fk_acceptances_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.acceptances ADD CONSTRAINT fk_acceptances_risk_id FOREIGN KEY (risk_id) REFERENCES risk.risks (id);
ALTER TABLE risk.acceptances ADD CONSTRAINT fk_acceptances_approval_id FOREIGN KEY (approval_id) REFERENCES platform.approvals (id);
ALTER TABLE risk.acceptances ADD CONSTRAINT fk_acceptances_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE risk.gap_rules ADD CONSTRAINT fk_gap_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.gap_findings ADD CONSTRAINT fk_gap_findings_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE risk.gap_findings ADD CONSTRAINT fk_gap_findings_rule_id FOREIGN KEY (rule_id) REFERENCES risk.gap_rules (id);
ALTER TABLE risk.gap_findings ADD CONSTRAINT fk_gap_findings_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE risk.gap_findings ADD CONSTRAINT fk_gap_findings_task_id FOREIGN KEY (task_id) REFERENCES dpo.tasks (id);
ALTER TABLE risk.compliance_scores ADD CONSTRAINT fk_compliance_scores_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.templates ADD CONSTRAINT fk_templates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.templates ADD CONSTRAINT fk_templates_form_id FOREIGN KEY (form_id) REFERENCES platform.form_definitions (id);
ALTER TABLE assess.screening_rules ADD CONSTRAINT fk_screening_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_template_id FOREIGN KEY (template_id) REFERENCES assess.templates (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_form_version_id FOREIGN KEY (form_version_id) REFERENCES platform.form_versions (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_previous_id FOREIGN KEY (previous_id) REFERENCES assess.assessments (id);
ALTER TABLE assess.assessments ADD CONSTRAINT fk_assessments_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE assess.sections ADD CONSTRAINT fk_sections_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.sections ADD CONSTRAINT fk_sections_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE assess.sections ADD CONSTRAINT fk_sections_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE assess.sections ADD CONSTRAINT fk_sections_guest_token_id FOREIGN KEY (guest_token_id) REFERENCES iam.guest_tokens (id);
ALTER TABLE assess.answers ADD CONSTRAINT fk_answers_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.answers ADD CONSTRAINT fk_answers_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE assess.answers ADD CONSTRAINT fk_answers_section_id FOREIGN KEY (section_id) REFERENCES assess.sections (id);
ALTER TABLE assess.answers ADD CONSTRAINT fk_answers_confirmed_by FOREIGN KEY (confirmed_by) REFERENCES iam.users (id);
ALTER TABLE assess.assessment_risks ADD CONSTRAINT fk_assessment_risks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.assessment_risks ADD CONSTRAINT fk_assessment_risks_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id) ON DELETE CASCADE;
ALTER TABLE assess.assessment_risks ADD CONSTRAINT fk_assessment_risks_risk_id FOREIGN KEY (risk_id) REFERENCES risk.risks (id) ON DELETE CASCADE;
ALTER TABLE assess.dpo_opinions ADD CONSTRAINT fk_dpo_opinions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.dpo_opinions ADD CONSTRAINT fk_dpo_opinions_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE assess.dpo_opinions ADD CONSTRAINT fk_dpo_opinions_dpo_user_id FOREIGN KEY (dpo_user_id) REFERENCES iam.users (id);
ALTER TABLE assess.consultations ADD CONSTRAINT fk_consultations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE assess.consultations ADD CONSTRAINT fk_consultations_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE dsar.request_types ADD CONSTRAINT fk_request_types_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.request_types ADD CONSTRAINT fk_request_types_workflow_definition_id FOREIGN KEY (workflow_definition_id) REFERENCES platform.workflow_definitions (id);
ALTER TABLE dsar.request_types ADD CONSTRAINT fk_request_types_response_template_id FOREIGN KEY (response_template_id) REFERENCES platform.templates (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_request_type_id FOREIGN KEY (request_type_id) REFERENCES dsar.request_types (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_workflow_instance_id FOREIGN KEY (workflow_instance_id) REFERENCES platform.workflow_instances (id);
ALTER TABLE dsar.requests ADD CONSTRAINT fk_requests_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE dsar.agents ADD CONSTRAINT fk_agents_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.agents ADD CONSTRAINT fk_agents_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.agents ADD CONSTRAINT fk_agents_authority_file_id FOREIGN KEY (authority_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.verifications ADD CONSTRAINT fk_verifications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.verifications ADD CONSTRAINT fk_verifications_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.verifications ADD CONSTRAINT fk_verifications_subject_verification_id FOREIGN KEY (subject_verification_id) REFERENCES iam.subject_verifications (id);
ALTER TABLE dsar.verifications ADD CONSTRAINT fk_verifications_masked_id_file_id FOREIGN KEY (masked_id_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.verifications ADD CONSTRAINT fk_verifications_verified_by FOREIGN KEY (verified_by) REFERENCES iam.users (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_assignee_group_id FOREIGN KEY (assignee_group_id) REFERENCES iam.groups (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_assignee_party_id FOREIGN KEY (assignee_party_id) REFERENCES org.external_parties (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_guest_token_id FOREIGN KEY (guest_token_id) REFERENCES iam.guest_tokens (id);
ALTER TABLE dsar.subtasks ADD CONSTRAINT fk_subtasks_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.search_results ADD CONSTRAINT fk_search_results_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.search_results ADD CONSTRAINT fk_search_results_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.search_results ADD CONSTRAINT fk_search_results_connector_id FOREIGN KEY (connector_id) REFERENCES platform.connectors (id);
ALTER TABLE dsar.search_results ADD CONSTRAINT fk_search_results_result_file_id FOREIGN KEY (result_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.legal_holds ADD CONSTRAINT fk_legal_holds_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.legal_holds ADD CONSTRAINT fk_legal_holds_data_category_id FOREIGN KEY (data_category_id) REFERENCES org.data_categories (id);
ALTER TABLE dsar.legal_holds ADD CONSTRAINT fk_legal_holds_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE dsar.exemption_checks ADD CONSTRAINT fk_exemption_checks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.exemption_checks ADD CONSTRAINT fk_exemption_checks_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.exemption_checks ADD CONSTRAINT fk_exemption_checks_legal_hold_id FOREIGN KEY (legal_hold_id) REFERENCES dsar.legal_holds (id);
ALTER TABLE dsar.packages ADD CONSTRAINT fk_packages_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.packages ADD CONSTRAINT fk_packages_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.packages ADD CONSTRAINT fk_packages_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.redactions ADD CONSTRAINT fk_redactions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.redactions ADD CONSTRAINT fk_redactions_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.redactions ADD CONSTRAINT fk_redactions_source_file_id FOREIGN KEY (source_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.redactions ADD CONSTRAINT fk_redactions_output_file_id FOREIGN KEY (output_file_id) REFERENCES platform.files (id);
ALTER TABLE dsar.redactions ADD CONSTRAINT fk_redactions_reviewed_by FOREIGN KEY (reviewed_by) REFERENCES iam.users (id);
ALTER TABLE dsar.communications ADD CONSTRAINT fk_communications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.communications ADD CONSTRAINT fk_communications_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.communications ADD CONSTRAINT fk_communications_document_version_id FOREIGN KEY (document_version_id) REFERENCES platform.document_versions (id);
ALTER TABLE dsar.communications ADD CONSTRAINT fk_communications_notification_id FOREIGN KEY (notification_id) REFERENCES platform.notifications (id);
ALTER TABLE dsar.downstream_notices ADD CONSTRAINT fk_downstream_notices_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dsar.downstream_notices ADD CONSTRAINT fk_downstream_notices_request_id FOREIGN KEY (request_id) REFERENCES dsar.requests (id);
ALTER TABLE dsar.downstream_notices ADD CONSTRAINT fk_downstream_notices_party_id FOREIGN KEY (party_id) REFERENCES org.external_parties (id);
ALTER TABLE dsar.downstream_notices ADD CONSTRAINT fk_downstream_notices_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_reporter_user_id FOREIGN KEY (reporter_user_id) REFERENCES iam.users (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_processor_party_id FOREIGN KEY (processor_party_id) REFERENCES org.external_parties (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_decided_by FOREIGN KEY (decided_by) REFERENCES iam.users (id);
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_workflow_instance_id FOREIGN KEY (workflow_instance_id) REFERENCES platform.workflow_instances (id);
ALTER TABLE breach.incident_assets ADD CONSTRAINT fk_incident_assets_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.incident_assets ADD CONSTRAINT fk_incident_assets_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.incident_assets ADD CONSTRAINT fk_incident_assets_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE breach.incident_assets ADD CONSTRAINT fk_incident_assets_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE breach.assessments ADD CONSTRAINT fk_assessments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.assessments ADD CONSTRAINT fk_assessments_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.assessments ADD CONSTRAINT fk_assessments_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE breach.assessments ADD CONSTRAINT fk_assessments_assessed_by FOREIGN KEY (assessed_by) REFERENCES iam.users (id);
ALTER TABLE breach.pdpc_notifications ADD CONSTRAINT fk_pdpc_notifications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.pdpc_notifications ADD CONSTRAINT fk_pdpc_notifications_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.pdpc_notifications ADD CONSTRAINT fk_pdpc_notifications_document_version_id FOREIGN KEY (document_version_id) REFERENCES platform.document_versions (id);
ALTER TABLE breach.pdpc_notifications ADD CONSTRAINT fk_pdpc_notifications_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE breach.pdpc_notifications ADD CONSTRAINT fk_pdpc_notifications_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE breach.subject_notifications ADD CONSTRAINT fk_subject_notifications_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.subject_notifications ADD CONSTRAINT fk_subject_notifications_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.subject_notifications ADD CONSTRAINT fk_subject_notifications_template_id FOREIGN KEY (template_id) REFERENCES platform.notification_templates (id);
ALTER TABLE breach.notification_recipients ADD CONSTRAINT fk_notification_recipients_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.notification_recipients ADD CONSTRAINT fk_notification_recipients_subject_notification_id FOREIGN KEY (subject_notification_id) REFERENCES breach.subject_notifications (id);
ALTER TABLE breach.notification_recipients ADD CONSTRAINT fk_notification_recipients_subject_id FOREIGN KEY (subject_id) REFERENCES consent.data_subjects (id);
ALTER TABLE breach.playbooks ADD CONSTRAINT fk_playbooks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.response_tasks ADD CONSTRAINT fk_response_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.response_tasks ADD CONSTRAINT fk_response_tasks_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.response_tasks ADD CONSTRAINT fk_response_tasks_playbook_id FOREIGN KEY (playbook_id) REFERENCES breach.playbooks (id);
ALTER TABLE breach.response_tasks ADD CONSTRAINT fk_response_tasks_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE breach.response_tasks ADD CONSTRAINT fk_response_tasks_assignee_group_id FOREIGN KEY (assignee_group_id) REFERENCES iam.groups (id);
ALTER TABLE breach.evidence ADD CONSTRAINT fk_evidence_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.evidence ADD CONSTRAINT fk_evidence_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.evidence ADD CONSTRAINT fk_evidence_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE breach.evidence ADD CONSTRAINT fk_evidence_collected_by FOREIGN KEY (collected_by) REFERENCES iam.users (id);
ALTER TABLE breach.timeline_events ADD CONSTRAINT fk_timeline_events_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.timeline_events ADD CONSTRAINT fk_timeline_events_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.root_causes ADD CONSTRAINT fk_root_causes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.root_causes ADD CONSTRAINT fk_root_causes_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE breach.root_causes ADD CONSTRAINT fk_root_causes_risk_id FOREIGN KEY (risk_id) REFERENCES risk.risks (id);
ALTER TABLE breach.routing_rules ADD CONSTRAINT fk_routing_rules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE breach.routing_rules ADD CONSTRAINT fk_routing_rules_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE breach.routing_rules ADD CONSTRAINT fk_routing_rules_group_id FOREIGN KEY (group_id) REFERENCES iam.groups (id);
ALTER TABLE vendor.vendors ADD CONSTRAINT fk_vendors_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.vendors ADD CONSTRAINT fk_vendors_party_id FOREIGN KEY (party_id) REFERENCES org.external_parties (id);
ALTER TABLE vendor.vendors ADD CONSTRAINT fk_vendors_relationship_owner_id FOREIGN KEY (relationship_owner_id) REFERENCES iam.users (id);
ALTER TABLE vendor.intakes ADD CONSTRAINT fk_intakes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.intakes ADD CONSTRAINT fk_intakes_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.intakes ADD CONSTRAINT fk_intakes_form_submission_id FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE vendor.vendor_assessments ADD CONSTRAINT fk_vendor_assessments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.vendor_assessments ADD CONSTRAINT fk_vendor_assessments_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.vendor_assessments ADD CONSTRAINT fk_vendor_assessments_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE vendor.vendor_assessments ADD CONSTRAINT fk_vendor_assessments_guest_token_id FOREIGN KEY (guest_token_id) REFERENCES iam.guest_tokens (id);
ALTER TABLE vendor.vendor_assessments ADD CONSTRAINT fk_vendor_assessments_decided_by FOREIGN KEY (decided_by) REFERENCES iam.users (id);
ALTER TABLE vendor.sub_processors ADD CONSTRAINT fk_sub_processors_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.sub_processors ADD CONSTRAINT fk_sub_processors_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.sub_processors ADD CONSTRAINT fk_sub_processors_party_id FOREIGN KEY (party_id) REFERENCES org.external_parties (id);
ALTER TABLE vendor.sub_processors ADD CONSTRAINT fk_sub_processors_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE vendor.certificates ADD CONSTRAINT fk_certificates_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.certificates ADD CONSTRAINT fk_certificates_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.certificates ADD CONSTRAINT fk_certificates_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE vendor.certificates ADD CONSTRAINT fk_certificates_verified_by FOREIGN KEY (verified_by) REFERENCES iam.users (id);
ALTER TABLE vendor.remediation_items ADD CONSTRAINT fk_remediation_items_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.remediation_items ADD CONSTRAINT fk_remediation_items_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.remediation_items ADD CONSTRAINT fk_remediation_items_vendor_assessment_id FOREIGN KEY (vendor_assessment_id) REFERENCES vendor.vendor_assessments (id);
ALTER TABLE vendor.remediation_items ADD CONSTRAINT fk_remediation_items_guest_token_id FOREIGN KEY (guest_token_id) REFERENCES iam.guest_tokens (id);
ALTER TABLE vendor.remediation_items ADD CONSTRAINT fk_remediation_items_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE vendor.offboardings ADD CONSTRAINT fk_offboardings_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE vendor.offboardings ADD CONSTRAINT fk_offboardings_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE vendor.offboardings ADD CONSTRAINT fk_offboardings_destruction_certificate_file_id FOREIGN KEY (destruction_certificate_file_id) REFERENCES platform.files (id);
ALTER TABLE agreement.agreements ADD CONSTRAINT fk_agreements_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.agreements ADD CONSTRAINT fk_agreements_counterparty_id FOREIGN KEY (counterparty_id) REFERENCES org.external_parties (id);
ALTER TABLE agreement.agreements ADD CONSTRAINT fk_agreements_vendor_id FOREIGN KEY (vendor_id) REFERENCES vendor.vendors (id);
ALTER TABLE agreement.agreements ADD CONSTRAINT fk_agreements_template_id FOREIGN KEY (template_id) REFERENCES platform.templates (id);
ALTER TABLE agreement.agreements ADD CONSTRAINT fk_agreements_document_id FOREIGN KEY (document_id) REFERENCES platform.documents (id);
ALTER TABLE agreement.parties ADD CONSTRAINT fk_parties_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.parties ADD CONSTRAINT fk_parties_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.parties ADD CONSTRAINT fk_parties_party_id FOREIGN KEY (party_id) REFERENCES org.external_parties (id);
ALTER TABLE agreement.parties ADD CONSTRAINT fk_parties_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE agreement.agreement_activities ADD CONSTRAINT fk_agreement_activities_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.agreement_activities ADD CONSTRAINT fk_agreement_activities_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id) ON DELETE CASCADE;
ALTER TABLE agreement.agreement_activities ADD CONSTRAINT fk_agreement_activities_activity_id FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id) ON DELETE CASCADE;
ALTER TABLE agreement.agreement_activities ADD CONSTRAINT fk_agreement_activities_recipient_id FOREIGN KEY (recipient_id) REFERENCES ropa.activity_recipients (id);
ALTER TABLE agreement.clauses ADD CONSTRAINT fk_clauses_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.clauses ADD CONSTRAINT fk_clauses_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.clauses ADD CONSTRAINT fk_clauses_clause_id FOREIGN KEY (clause_id) REFERENCES platform.clause_library (id);
ALTER TABLE agreement.annexes ADD CONSTRAINT fk_annexes_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.annexes ADD CONSTRAINT fk_annexes_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.annexes ADD CONSTRAINT fk_annexes_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE agreement.signature_requests ADD CONSTRAINT fk_signature_requests_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.signature_requests ADD CONSTRAINT fk_signature_requests_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.signature_requests ADD CONSTRAINT fk_signature_requests_signed_file_id FOREIGN KEY (signed_file_id) REFERENCES platform.files (id);
ALTER TABLE agreement.obligations ADD CONSTRAINT fk_obligations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.obligations ADD CONSTRAINT fk_obligations_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.obligations ADD CONSTRAINT fk_obligations_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE agreement.obligations ADD CONSTRAINT fk_obligations_task_id FOREIGN KEY (task_id) REFERENCES dpo.tasks (id);
ALTER TABLE agreement.return_confirmations ADD CONSTRAINT fk_return_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.return_confirmations ADD CONSTRAINT fk_return_confirmations_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.return_confirmations ADD CONSTRAINT fk_return_confirmations_guest_token_id FOREIGN KEY (guest_token_id) REFERENCES iam.guest_tokens (id);
ALTER TABLE agreement.return_confirmations ADD CONSTRAINT fk_return_confirmations_certificate_file_id FOREIGN KEY (certificate_file_id) REFERENCES platform.files (id);
ALTER TABLE agreement.downloads ADD CONSTRAINT fk_downloads_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE agreement.downloads ADD CONSTRAINT fk_downloads_agreement_id FOREIGN KEY (agreement_id) REFERENCES agreement.agreements (id);
ALTER TABLE agreement.downloads ADD CONSTRAINT fk_downloads_document_version_id FOREIGN KEY (document_version_id) REFERENCES platform.document_versions (id);
ALTER TABLE agreement.downloads ADD CONSTRAINT fk_downloads_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.appointments ADD CONSTRAINT fk_appointments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.appointments ADD CONSTRAINT fk_appointments_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE dpo.appointments ADD CONSTRAINT fk_appointments_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.appointments ADD CONSTRAINT fk_appointments_appointment_file_id FOREIGN KEY (appointment_file_id) REFERENCES platform.files (id);
ALTER TABLE dpo.appointments ADD CONSTRAINT fk_appointments_pdpc_evidence_file_id FOREIGN KEY (pdpc_evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE dpo.requirement_checks ADD CONSTRAINT fk_requirement_checks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.requirement_checks ADD CONSTRAINT fk_requirement_checks_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE dpo.requirement_checks ADD CONSTRAINT fk_requirement_checks_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE dpo.independence_declarations ADD CONSTRAINT fk_independence_declarations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.independence_declarations ADD CONSTRAINT fk_independence_declarations_appointment_id FOREIGN KEY (appointment_id) REFERENCES dpo.appointments (id);
ALTER TABLE dpo.independence_declarations ADD CONSTRAINT fk_independence_declarations_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE dpo.tasks ADD CONSTRAINT fk_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.tasks ADD CONSTRAINT fk_tasks_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.tasks ADD CONSTRAINT fk_tasks_reviewer_user_id FOREIGN KEY (reviewer_user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.tasks ADD CONSTRAINT fk_tasks_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE dpo.advisories ADD CONSTRAINT fk_advisories_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.advisories ADD CONSTRAINT fk_advisories_org_unit_id FOREIGN KEY (org_unit_id) REFERENCES org.org_units (id);
ALTER TABLE dpo.advisories ADD CONSTRAINT fk_advisories_requester_user_id FOREIGN KEY (requester_user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.advisories ADD CONSTRAINT fk_advisories_answered_by FOREIGN KEY (answered_by) REFERENCES iam.users (id);
ALTER TABLE dpo.advisories ADD CONSTRAINT fk_advisories_kb_article_id FOREIGN KEY (kb_article_id) REFERENCES dpo.kb_articles (id);
ALTER TABLE dpo.kb_articles ADD CONSTRAINT fk_kb_articles_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.calendar_events ADD CONSTRAINT fk_calendar_events_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE dpo.calendar_events ADD CONSTRAINT fk_calendar_events_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE dpo.report_schedules ADD CONSTRAINT fk_report_schedules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.courses ADD CONSTRAINT fk_courses_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.courses ADD CONSTRAINT fk_courses_content_file_id FOREIGN KEY (content_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.courses ADD CONSTRAINT fk_courses_quiz_form_id FOREIGN KEY (quiz_form_id) REFERENCES platform.form_definitions (id);
ALTER TABLE gov.training_assignments ADD CONSTRAINT fk_training_assignments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.training_assignments ADD CONSTRAINT fk_training_assignments_course_id FOREIGN KEY (course_id) REFERENCES gov.courses (id);
ALTER TABLE gov.training_assignments ADD CONSTRAINT fk_training_assignments_assigned_by FOREIGN KEY (assigned_by) REFERENCES iam.users (id);
ALTER TABLE gov.training_attempts ADD CONSTRAINT fk_training_attempts_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.training_attempts ADD CONSTRAINT fk_training_attempts_course_id FOREIGN KEY (course_id) REFERENCES gov.courses (id);
ALTER TABLE gov.training_attempts ADD CONSTRAINT fk_training_attempts_assignment_id FOREIGN KEY (assignment_id) REFERENCES gov.training_assignments (id);
ALTER TABLE gov.training_attempts ADD CONSTRAINT fk_training_attempts_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE gov.training_attempts ADD CONSTRAINT fk_training_attempts_certificate_file_id FOREIGN KEY (certificate_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.policies ADD CONSTRAINT fk_policies_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.policies ADD CONSTRAINT fk_policies_document_id FOREIGN KEY (document_id) REFERENCES platform.documents (id);
ALTER TABLE gov.policy_attestations ADD CONSTRAINT fk_policy_attestations_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.policy_attestations ADD CONSTRAINT fk_policy_attestations_policy_id FOREIGN KEY (policy_id) REFERENCES gov.policies (id);
ALTER TABLE gov.policy_attestations ADD CONSTRAINT fk_policy_attestations_user_id FOREIGN KEY (user_id) REFERENCES iam.users (id);
ALTER TABLE gov.audits ADD CONSTRAINT fk_audits_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.audits ADD CONSTRAINT fk_audits_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
ALTER TABLE gov.audits ADD CONSTRAINT fk_audits_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE gov.audit_findings ADD CONSTRAINT fk_audit_findings_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.audit_findings ADD CONSTRAINT fk_audit_findings_audit_id FOREIGN KEY (audit_id) REFERENCES gov.audits (id);
ALTER TABLE gov.audit_findings ADD CONSTRAINT fk_audit_findings_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE gov.audit_findings ADD CONSTRAINT fk_audit_findings_task_id FOREIGN KEY (task_id) REFERENCES dpo.tasks (id);
ALTER TABLE gov.audit_findings ADD CONSTRAINT fk_audit_findings_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.retention_schedules ADD CONSTRAINT fk_retention_schedules_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.retention_schedules ADD CONSTRAINT fk_retention_schedules_retention_rule_id FOREIGN KEY (retention_rule_id) REFERENCES ropa.retention_rules (id);
ALTER TABLE gov.retention_schedules ADD CONSTRAINT fk_retention_schedules_asset_id FOREIGN KEY (asset_id) REFERENCES ropa.assets (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_schedule_id FOREIGN KEY (schedule_id) REFERENCES gov.retention_schedules (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_legal_hold_id FOREIGN KEY (legal_hold_id) REFERENCES dsar.legal_holds (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_assignee_user_id FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE gov.disposal_jobs ADD CONSTRAINT fk_disposal_jobs_evidence_file_id FOREIGN KEY (evidence_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.regulator_letters ADD CONSTRAINT fk_regulator_letters_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.regulator_letters ADD CONSTRAINT fk_regulator_letters_file_id FOREIGN KEY (file_id) REFERENCES platform.files (id);
ALTER TABLE gov.regulator_letters ADD CONSTRAINT fk_regulator_letters_incident_id FOREIGN KEY (incident_id) REFERENCES breach.incidents (id);
ALTER TABLE gov.regulatory_reviews ADD CONSTRAINT fk_regulatory_reviews_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.regulatory_reviews ADD CONSTRAINT fk_regulatory_reviews_update_id FOREIGN KEY (update_id) REFERENCES gov.regulatory_updates (id);
ALTER TABLE gov.regulatory_reviews ADD CONSTRAINT fk_regulatory_reviews_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE gov.regulatory_reviews ADD CONSTRAINT fk_regulatory_reviews_reviewed_by FOREIGN KEY (reviewed_by) REFERENCES iam.users (id);
ALTER TABLE gov.ai_systems ADD CONSTRAINT fk_ai_systems_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.ai_systems ADD CONSTRAINT fk_ai_systems_vendor_party_id FOREIGN KEY (vendor_party_id) REFERENCES org.external_parties (id);
ALTER TABLE gov.ai_systems ADD CONSTRAINT fk_ai_systems_assessment_id FOREIGN KEY (assessment_id) REFERENCES assess.assessments (id);
ALTER TABLE gov.ai_systems ADD CONSTRAINT fk_ai_systems_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE gov.masking_jobs ADD CONSTRAINT fk_masking_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.masking_jobs ADD CONSTRAINT fk_masking_jobs_source_file_id FOREIGN KEY (source_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.masking_jobs ADD CONSTRAINT fk_masking_jobs_output_file_id FOREIGN KEY (output_file_id) REFERENCES platform.files (id);
ALTER TABLE gov.masking_jobs ADD CONSTRAINT fk_masking_jobs_requested_by FOREIGN KEY (requested_by) REFERENCES iam.users (id);
ALTER TABLE gov.kb_chunks ADD CONSTRAINT fk_kb_chunks_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.kb_chunks ADD CONSTRAINT fk_kb_chunks_kb_article_id FOREIGN KEY (kb_article_id) REFERENCES dpo.kb_articles (id);

-- ---------------------------------------------------------------- indexes
CREATE INDEX ix_platform_public_keys_tenant_id ON platform.public_keys (tenant_id);
CREATE INDEX ix_platform_public_keys_entity_id ON platform.public_keys (entity_id);
CREATE INDEX ix_platform_workflow_instances_definition_id ON platform.workflow_instances (tenant_id, definition_id);
CREATE INDEX ix_platform_workflow_instances_entity_type ON platform.workflow_instances (tenant_id, entity_type);
CREATE INDEX ix_platform_workflow_instances_entity_id ON platform.workflow_instances (tenant_id, entity_id);
CREATE INDEX ix_platform_workflow_tasks_instance_id ON platform.workflow_tasks (tenant_id, instance_id);
CREATE INDEX ix_platform_workflow_tasks_assignee_user_id ON platform.workflow_tasks (tenant_id, assignee_user_id);
CREATE INDEX ix_platform_workflow_tasks_assignee_group_id ON platform.workflow_tasks (tenant_id, assignee_group_id);
CREATE INDEX ix_platform_sla_timers_instance_id ON platform.sla_timers (tenant_id, instance_id);
CREATE INDEX ix_platform_sla_timers_calendar_id ON platform.sla_timers (tenant_id, calendar_id);
CREATE INDEX ix_platform_sla_timers_due_at ON platform.sla_timers (tenant_id, due_at);
CREATE INDEX ix_platform_form_versions_form_id ON platform.form_versions (form_id);
CREATE INDEX ix_platform_form_versions_published_by ON platform.form_versions (published_by);
CREATE INDEX ix_platform_form_submissions_form_version_id ON platform.form_submissions (tenant_id, form_version_id);
CREATE INDEX ix_platform_form_submissions_entity_type ON platform.form_submissions (tenant_id, entity_type);
CREATE INDEX ix_platform_form_submissions_entity_id ON platform.form_submissions (tenant_id, entity_id);
CREATE INDEX ix_platform_record_versions_entity_type ON platform.record_versions (tenant_id, entity_type);
CREATE INDEX ix_platform_record_versions_entity_id ON platform.record_versions (tenant_id, entity_id);
CREATE INDEX ix_platform_approvals_entity_type ON platform.approvals (tenant_id, entity_type);
CREATE INDEX ix_platform_approvals_entity_id ON platform.approvals (tenant_id, entity_id);
CREATE INDEX ix_platform_approvals_record_version_id ON platform.approvals (tenant_id, record_version_id);
CREATE INDEX ix_platform_approvals_requested_by ON platform.approvals (tenant_id, requested_by);
CREATE INDEX ix_platform_approvals_approver_user_id ON platform.approvals (tenant_id, approver_user_id);
CREATE INDEX ix_platform_comments_entity_type ON platform.comments (tenant_id, entity_type);
CREATE INDEX ix_platform_comments_entity_id ON platform.comments (tenant_id, entity_id);
CREATE INDEX ix_platform_comments_parent_id ON platform.comments (tenant_id, parent_id);
CREATE INDEX ix_platform_files_entity_type ON platform.files (tenant_id, entity_type);
CREATE INDEX ix_platform_files_entity_id ON platform.files (tenant_id, entity_id);
CREATE INDEX ix_platform_documents_entity_type ON platform.documents (tenant_id, entity_type);
CREATE INDEX ix_platform_documents_entity_id ON platform.documents (tenant_id, entity_id);
CREATE INDEX ix_platform_documents_template_id ON platform.documents (tenant_id, template_id);
CREATE INDEX ix_platform_document_versions_document_id ON platform.document_versions (tenant_id, document_id);
CREATE INDEX ix_platform_document_versions_pdf_file_id ON platform.document_versions (tenant_id, pdf_file_id);
CREATE INDEX ix_platform_document_versions_docx_file_id ON platform.document_versions (tenant_id, docx_file_id);
CREATE INDEX ix_platform_document_versions_approved_by ON platform.document_versions (tenant_id, approved_by);
CREATE INDEX ix_platform_notifications_template_id ON platform.notifications (tenant_id, template_id);
CREATE INDEX ix_platform_notifications_recipient_user_id ON platform.notifications (tenant_id, recipient_user_id);
CREATE INDEX ix_platform_notifications_entity_type ON platform.notifications (tenant_id, entity_type);
CREATE INDEX ix_platform_notifications_entity_id ON platform.notifications (tenant_id, entity_id);
CREATE INDEX ix_platform_outbox_events_event_type ON platform.outbox_events (tenant_id, event_type);
CREATE INDEX ix_platform_outbox_events_published_at ON platform.outbox_events (tenant_id, published_at);
CREATE INDEX ix_platform_webhook_subscriptions_api_client_id ON platform.webhook_subscriptions (tenant_id, api_client_id);
CREATE INDEX ix_platform_webhook_deliveries_subscription_id ON platform.webhook_deliveries (tenant_id, subscription_id);
CREATE INDEX ix_platform_webhook_deliveries_event_id ON platform.webhook_deliveries (tenant_id, event_id);
CREATE INDEX ix_platform_webhook_deliveries_next_retry_at ON platform.webhook_deliveries (tenant_id, next_retry_at);
CREATE INDEX ix_platform_audit_log_entity_type ON platform.audit_log (tenant_id, entity_type);
CREATE INDEX ix_platform_audit_log_entity_id ON platform.audit_log (tenant_id, entity_id);
CREATE INDEX ix_platform_import_jobs_file_id ON platform.import_jobs (tenant_id, file_id);
CREATE INDEX ix_platform_import_jobs_error_file_id ON platform.import_jobs (tenant_id, error_file_id);
CREATE INDEX ix_platform_export_jobs_file_id ON platform.export_jobs (tenant_id, file_id);
CREATE INDEX ix_platform_export_jobs_requested_by ON platform.export_jobs (tenant_id, requested_by);
CREATE INDEX ix_platform_connectors_asset_id ON platform.connectors (tenant_id, asset_id);
CREATE INDEX ix_platform_ai_requests_confirmed_by ON platform.ai_requests (tenant_id, confirmed_by);
CREATE INDEX ix_iam_users_primary_org_unit_id ON iam.users (tenant_id, primary_org_unit_id);
CREATE INDEX ix_iam_users_avatar_file_id ON iam.users (tenant_id, avatar_file_id);
CREATE INDEX ix_iam_roles_cloned_from_id ON iam.roles (cloned_from_id);
CREATE INDEX ix_iam_role_permissions_permission_code ON iam.role_permissions (permission_code);
CREATE INDEX ix_iam_group_members_user_id ON iam.group_members (tenant_id, user_id);
CREATE INDEX ix_iam_role_assignments_user_id ON iam.role_assignments (tenant_id, user_id);
CREATE INDEX ix_iam_role_assignments_group_id ON iam.role_assignments (tenant_id, group_id);
CREATE INDEX ix_iam_role_assignments_role_id ON iam.role_assignments (tenant_id, role_id);
CREATE INDEX ix_iam_role_assignments_legal_entity_id ON iam.role_assignments (tenant_id, legal_entity_id);
CREATE INDEX ix_iam_role_assignments_org_unit_id ON iam.role_assignments (tenant_id, org_unit_id);
CREATE INDEX ix_iam_role_assignments_granted_by ON iam.role_assignments (tenant_id, granted_by);
CREATE INDEX ix_iam_role_assignments_approval_id ON iam.role_assignments (tenant_id, approval_id);
CREATE INDEX ix_iam_api_clients_owner_user_id ON iam.api_clients (tenant_id, owner_user_id);
CREATE INDEX ix_iam_api_clients_system_asset_id ON iam.api_clients (tenant_id, system_asset_id);
CREATE INDEX ix_iam_guest_tokens_issued_by ON iam.guest_tokens (tenant_id, issued_by);
CREATE INDEX ix_iam_security_events_user_id ON iam.security_events (tenant_id, user_id);
CREATE INDEX ix_iam_access_review_items_review_id ON iam.access_review_items (tenant_id, review_id);
CREATE INDEX ix_iam_access_review_items_role_assignment_id ON iam.access_review_items (tenant_id, role_assignment_id);
CREATE INDEX ix_iam_access_review_items_reviewer_user_id ON iam.access_review_items (tenant_id, reviewer_user_id);
CREATE INDEX ix_iam_breakglass_requests_requested_by ON iam.breakglass_requests (tenant_id, requested_by);
CREATE INDEX ix_iam_breakglass_requests_approved_by ON iam.breakglass_requests (tenant_id, approved_by);
CREATE INDEX ix_iam_delegations_from_user_id ON iam.delegations (tenant_id, from_user_id);
CREATE INDEX ix_iam_delegations_to_user_id ON iam.delegations (tenant_id, to_user_id);
CREATE INDEX ix_iam_delegations_role_id ON iam.delegations (tenant_id, role_id);
CREATE INDEX ix_iam_field_masking_rules_unmask_permission ON iam.field_masking_rules (unmask_permission);
CREATE INDEX ix_iam_unmask_logs_user_id ON iam.unmask_logs (tenant_id, user_id);
CREATE INDEX ix_iam_subject_verifications_subject_id ON iam.subject_verifications (tenant_id, subject_id);
CREATE INDEX ix_iam_subject_verifications_identifier_blind_index ON iam.subject_verifications (tenant_id, identifier_blind_index);
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
CREATE INDEX ix_consent_data_elements_data_category_id ON consent.data_elements (tenant_id, data_category_id);
CREATE INDEX ix_consent_purposes_legal_entity_id ON consent.purposes (tenant_id, legal_entity_id);
CREATE INDEX ix_consent_purposes_lawful_basis_code ON consent.purposes (tenant_id, lawful_basis_code);
CREATE INDEX ix_consent_purpose_versions_purpose_id ON consent.purpose_versions (tenant_id, purpose_id);
CREATE INDEX ix_consent_purpose_versions_approved_by ON consent.purpose_versions (tenant_id, approved_by);
CREATE INDEX ix_consent_purpose_preferences_purpose_id ON consent.purpose_preferences (tenant_id, purpose_id);
CREATE INDEX ix_consent_purpose_data_elements_data_element_id ON consent.purpose_data_elements (tenant_id, data_element_id);
CREATE INDEX ix_consent_collection_points_legal_entity_id ON consent.collection_points (tenant_id, legal_entity_id);
CREATE INDEX ix_consent_collection_points_form_id ON consent.collection_points (tenant_id, form_id);
CREATE INDEX ix_consent_collection_points_notice_id ON consent.collection_points (tenant_id, notice_id);
CREATE INDEX ix_consent_collection_point_purposes_purpose_id ON consent.collection_point_purposes (tenant_id, purpose_id);
CREATE INDEX ix_consent_data_subjects_guardian_subject_id ON consent.data_subjects (tenant_id, guardian_subject_id);
CREATE INDEX ix_consent_subject_identifiers_subject_id ON consent.subject_identifiers (tenant_id, subject_id);
CREATE INDEX ix_consent_subject_identifiers_blind_index ON consent.subject_identifiers (tenant_id, blind_index);
CREATE INDEX ix_consent_consent_receipts_subject_id ON consent.consent_receipts (tenant_id, subject_id);
CREATE INDEX ix_consent_consent_receipts_collection_point_id ON consent.consent_receipts (tenant_id, collection_point_id);
CREATE INDEX ix_consent_consent_receipts_captured_by_user_id ON consent.consent_receipts (tenant_id, captured_by_user_id);
CREATE INDEX ix_consent_consent_receipts_branch_org_unit_id ON consent.consent_receipts (tenant_id, branch_org_unit_id);
CREATE INDEX ix_consent_consent_receipts_notice_version_id ON consent.consent_receipts (tenant_id, notice_version_id);
CREATE INDEX ix_consent_consent_receipts_form_submission_id ON consent.consent_receipts (tenant_id, form_submission_id);
CREATE INDEX ix_consent_consent_receipts_verification_id ON consent.consent_receipts (tenant_id, verification_id);
CREATE INDEX ix_consent_consent_receipts_evidence_file_id ON consent.consent_receipts (tenant_id, evidence_file_id);
CREATE INDEX ix_consent_consent_transactions_receipt_id ON consent.consent_transactions (tenant_id, receipt_id);
CREATE INDEX ix_consent_consent_transactions_subject_id ON consent.consent_transactions (tenant_id, subject_id);
CREATE INDEX ix_consent_consent_transactions_purpose_id ON consent.consent_transactions (tenant_id, purpose_id);
CREATE INDEX ix_consent_consent_transactions_purpose_version_id ON consent.consent_transactions (tenant_id, purpose_version_id);
CREATE INDEX ix_consent_consent_status_purpose_id ON consent.consent_status (tenant_id, purpose_id);
CREATE INDEX ix_consent_consent_status_purpose_version_id ON consent.consent_status (tenant_id, purpose_version_id);
CREATE INDEX ix_consent_consent_status_last_transaction_id ON consent.consent_status (tenant_id, last_transaction_id);
CREATE INDEX ix_consent_consent_status_expires_at ON consent.consent_status (tenant_id, expires_at);
CREATE INDEX ix_consent_double_optin_requests_transaction_id ON consent.double_optin_requests (tenant_id, transaction_id);
CREATE INDEX ix_consent_guardian_approvals_minor_subject_id ON consent.guardian_approvals (tenant_id, minor_subject_id);
CREATE INDEX ix_consent_guardian_approvals_guardian_subject_id ON consent.guardian_approvals (tenant_id, guardian_subject_id);
CREATE INDEX ix_consent_guardian_approvals_receipt_id ON consent.guardian_approvals (tenant_id, receipt_id);
CREATE INDEX ix_consent_campaigns_template_id ON consent.campaigns (tenant_id, template_id);
CREATE INDEX ix_consent_campaign_recipients_campaign_id ON consent.campaign_recipients (tenant_id, campaign_id);
CREATE INDEX ix_consent_campaign_recipients_subject_id ON consent.campaign_recipients (tenant_id, subject_id);
CREATE INDEX ix_consent_reconcile_runs_target_api_client_id ON consent.reconcile_runs (tenant_id, target_api_client_id);
CREATE INDEX ix_consent_reconcile_runs_connector_id ON consent.reconcile_runs (tenant_id, connector_id);
CREATE INDEX ix_consent_reconcile_runs_report_file_id ON consent.reconcile_runs (tenant_id, report_file_id);
CREATE INDEX ix_consent_reconcile_items_run_id ON consent.reconcile_items (tenant_id, run_id);
CREATE INDEX ix_consent_reconcile_items_subject_id ON consent.reconcile_items (tenant_id, subject_id);
CREATE INDEX ix_consent_reconcile_items_purpose_id ON consent.reconcile_items (tenant_id, purpose_id);
CREATE INDEX ix_consent_downstream_syncs_transaction_id ON consent.downstream_syncs (tenant_id, transaction_id);
CREATE INDEX ix_consent_downstream_syncs_webhook_delivery_id ON consent.downstream_syncs (tenant_id, webhook_delivery_id);
CREATE INDEX ix_consent_downstream_syncs_target_api_client_id ON consent.downstream_syncs (tenant_id, target_api_client_id);
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
CREATE INDEX ix_ropa_processing_activities_legal_entity_id ON ropa.processing_activities (tenant_id, legal_entity_id);
CREATE INDEX ix_ropa_processing_activities_org_unit_id ON ropa.processing_activities (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_processing_activities_controller_party_id ON ropa.processing_activities (tenant_id, controller_party_id);
CREATE INDEX ix_ropa_processing_activities_owner_user_id ON ropa.processing_activities (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_processing_activities_template_id ON ropa.processing_activities (tenant_id, template_id);
CREATE INDEX ix_ropa_processing_activities_approved_by ON ropa.processing_activities (tenant_id, approved_by);
CREATE INDEX ix_ropa_activity_purposes_activity_id ON ropa.activity_purposes (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_purposes_purpose_id ON ropa.activity_purposes (tenant_id, purpose_id);
CREATE INDEX ix_ropa_activity_purposes_lawful_basis_code ON ropa.activity_purposes (tenant_id, lawful_basis_code);
CREATE INDEX ix_ropa_activity_purposes_consent_purpose_id ON ropa.activity_purposes (tenant_id, consent_purpose_id);
CREATE INDEX ix_ropa_activity_purposes_lia_assessment_id ON ropa.activity_purposes (tenant_id, lia_assessment_id);
CREATE INDEX ix_ropa_activity_data_activity_id ON ropa.activity_data (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_data_data_category_id ON ropa.activity_data (tenant_id, data_category_id);
CREATE INDEX ix_ropa_activity_data_subject_type_id ON ropa.activity_data (tenant_id, subject_type_id);
CREATE INDEX ix_ropa_activity_data_source_party_id ON ropa.activity_data (tenant_id, source_party_id);
CREATE INDEX ix_ropa_activity_systems_asset_id ON ropa.activity_systems (tenant_id, asset_id);
CREATE INDEX ix_ropa_activity_recipients_activity_id ON ropa.activity_recipients (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_recipients_party_id ON ropa.activity_recipients (tenant_id, party_id);
CREATE INDEX ix_ropa_activity_recipients_agreement_id ON ropa.activity_recipients (tenant_id, agreement_id);
CREATE INDEX ix_ropa_activity_transfers_activity_id ON ropa.activity_transfers (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_transfers_recipient_id ON ropa.activity_transfers (tenant_id, recipient_id);
CREATE INDEX ix_ropa_activity_transfers_country_code ON ropa.activity_transfers (tenant_id, country_code);
CREATE INDEX ix_ropa_activity_transfers_tia_assessment_id ON ropa.activity_transfers (tenant_id, tia_assessment_id);
CREATE INDEX ix_ropa_retention_rules_activity_id ON ropa.retention_rules (tenant_id, activity_id);
CREATE INDEX ix_ropa_retention_rules_data_category_id ON ropa.retention_rules (tenant_id, data_category_id);
CREATE INDEX ix_ropa_activity_controls_control_id ON ropa.activity_controls (tenant_id, control_id);
CREATE INDEX ix_ropa_activity_controls_assessment_id ON ropa.activity_controls (tenant_id, assessment_id);
CREATE INDEX ix_ropa_activity_rejections_activity_id ON ropa.activity_rejections (tenant_id, activity_id);
CREATE INDEX ix_ropa_activity_rejections_dsar_request_id ON ropa.activity_rejections (tenant_id, dsar_request_id);
CREATE INDEX ix_ropa_assets_org_unit_id ON ropa.assets (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_assets_owner_user_id ON ropa.assets (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_assets_provider_party_id ON ropa.assets (tenant_id, provider_party_id);
CREATE INDEX ix_ropa_assets_hosting_country_code ON ropa.assets (tenant_id, hosting_country_code);
CREATE INDEX ix_ropa_data_inventory_asset_id ON ropa.data_inventory (tenant_id, asset_id);
CREATE INDEX ix_ropa_data_inventory_data_category_id ON ropa.data_inventory (tenant_id, data_category_id);
CREATE INDEX ix_ropa_data_inventory_org_unit_id ON ropa.data_inventory (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_data_inventory_owner_user_id ON ropa.data_inventory (tenant_id, owner_user_id);
CREATE INDEX ix_ropa_data_inventory_discovered_by_finding_id ON ropa.data_inventory (tenant_id, discovered_by_finding_id);
CREATE INDEX ix_ropa_questionnaires_org_unit_id ON ropa.questionnaires (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_questionnaires_activity_id ON ropa.questionnaires (tenant_id, activity_id);
CREATE INDEX ix_ropa_questionnaires_form_submission_id ON ropa.questionnaires (tenant_id, form_submission_id);
CREATE INDEX ix_ropa_questionnaires_respondent_user_id ON ropa.questionnaires (tenant_id, respondent_user_id);
CREATE INDEX ix_ropa_questionnaires_approved_by ON ropa.questionnaires (tenant_id, approved_by);
CREATE INDEX ix_ropa_sme_exemption_checks_legal_entity_id ON ropa.sme_exemption_checks (tenant_id, legal_entity_id);
CREATE INDEX ix_ropa_sme_exemption_checks_form_submission_id ON ropa.sme_exemption_checks (tenant_id, form_submission_id);
CREATE INDEX ix_ropa_activity_templates_template_set_id ON ropa.activity_templates (template_set_id);
CREATE INDEX ix_ropa_generation_runs_org_unit_id ON ropa.generation_runs (tenant_id, org_unit_id);
CREATE INDEX ix_ropa_wizard_sessions_user_id ON ropa.wizard_sessions (tenant_id, user_id);
CREATE INDEX ix_ropa_wizard_sessions_activity_id ON ropa.wizard_sessions (tenant_id, activity_id);
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
CREATE INDEX ix_assess_templates_form_id ON assess.templates (form_id);
CREATE INDEX ix_assess_assessments_assessment_type ON assess.assessments (tenant_id, assessment_type);
CREATE INDEX ix_assess_assessments_template_id ON assess.assessments (tenant_id, template_id);
CREATE INDEX ix_assess_assessments_form_version_id ON assess.assessments (tenant_id, form_version_id);
CREATE INDEX ix_assess_assessments_activity_id ON assess.assessments (tenant_id, activity_id);
CREATE INDEX ix_assess_assessments_previous_id ON assess.assessments (tenant_id, previous_id);
CREATE INDEX ix_assess_assessments_owner_user_id ON assess.assessments (tenant_id, owner_user_id);
CREATE INDEX ix_assess_sections_assessment_id ON assess.sections (tenant_id, assessment_id);
CREATE INDEX ix_assess_sections_assignee_user_id ON assess.sections (tenant_id, assignee_user_id);
CREATE INDEX ix_assess_sections_guest_token_id ON assess.sections (tenant_id, guest_token_id);
CREATE INDEX ix_assess_answers_assessment_id ON assess.answers (tenant_id, assessment_id);
CREATE INDEX ix_assess_answers_section_id ON assess.answers (tenant_id, section_id);
CREATE INDEX ix_assess_answers_confirmed_by ON assess.answers (tenant_id, confirmed_by);
CREATE INDEX ix_assess_assessment_risks_risk_id ON assess.assessment_risks (tenant_id, risk_id);
CREATE INDEX ix_assess_dpo_opinions_assessment_id ON assess.dpo_opinions (tenant_id, assessment_id);
CREATE INDEX ix_assess_dpo_opinions_dpo_user_id ON assess.dpo_opinions (tenant_id, dpo_user_id);
CREATE INDEX ix_assess_consultations_assessment_id ON assess.consultations (tenant_id, assessment_id);
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
CREATE INDEX ix_gov_courses_content_file_id ON gov.courses (content_file_id);
CREATE INDEX ix_gov_courses_quiz_form_id ON gov.courses (quiz_form_id);
CREATE INDEX ix_gov_training_assignments_course_id ON gov.training_assignments (tenant_id, course_id);
CREATE INDEX ix_gov_training_assignments_assigned_by ON gov.training_assignments (tenant_id, assigned_by);
CREATE INDEX ix_gov_training_attempts_course_id ON gov.training_attempts (tenant_id, course_id);
CREATE INDEX ix_gov_training_attempts_assignment_id ON gov.training_attempts (tenant_id, assignment_id);
CREATE INDEX ix_gov_training_attempts_user_id ON gov.training_attempts (tenant_id, user_id);
CREATE INDEX ix_gov_training_attempts_certificate_file_id ON gov.training_attempts (tenant_id, certificate_file_id);
CREATE INDEX ix_gov_policies_document_id ON gov.policies (tenant_id, document_id);
CREATE INDEX ix_gov_policy_attestations_policy_id ON gov.policy_attestations (tenant_id, policy_id);
CREATE INDEX ix_gov_policy_attestations_user_id ON gov.policy_attestations (tenant_id, user_id);
CREATE INDEX ix_gov_audits_legal_entity_id ON gov.audits (tenant_id, legal_entity_id);
CREATE INDEX ix_gov_audits_assessment_id ON gov.audits (tenant_id, assessment_id);
CREATE INDEX ix_gov_audit_findings_audit_id ON gov.audit_findings (tenant_id, audit_id);
CREATE INDEX ix_gov_audit_findings_owner_user_id ON gov.audit_findings (tenant_id, owner_user_id);
CREATE INDEX ix_gov_audit_findings_task_id ON gov.audit_findings (tenant_id, task_id);
CREATE INDEX ix_gov_audit_findings_evidence_file_id ON gov.audit_findings (tenant_id, evidence_file_id);
CREATE INDEX ix_gov_retention_schedules_retention_rule_id ON gov.retention_schedules (tenant_id, retention_rule_id);
CREATE INDEX ix_gov_retention_schedules_asset_id ON gov.retention_schedules (tenant_id, asset_id);
CREATE INDEX ix_gov_retention_schedules_next_due_at ON gov.retention_schedules (tenant_id, next_due_at);
CREATE INDEX ix_gov_disposal_jobs_schedule_id ON gov.disposal_jobs (tenant_id, schedule_id);
CREATE INDEX ix_gov_disposal_jobs_legal_hold_id ON gov.disposal_jobs (tenant_id, legal_hold_id);
CREATE INDEX ix_gov_disposal_jobs_assignee_user_id ON gov.disposal_jobs (tenant_id, assignee_user_id);
CREATE INDEX ix_gov_disposal_jobs_approved_by ON gov.disposal_jobs (tenant_id, approved_by);
CREATE INDEX ix_gov_disposal_jobs_evidence_file_id ON gov.disposal_jobs (tenant_id, evidence_file_id);
CREATE INDEX ix_gov_regulator_letters_file_id ON gov.regulator_letters (tenant_id, file_id);
CREATE INDEX ix_gov_regulator_letters_incident_id ON gov.regulator_letters (tenant_id, incident_id);
CREATE INDEX ix_gov_regulatory_reviews_update_id ON gov.regulatory_reviews (tenant_id, update_id);
CREATE INDEX ix_gov_regulatory_reviews_assessment_id ON gov.regulatory_reviews (tenant_id, assessment_id);
CREATE INDEX ix_gov_regulatory_reviews_reviewed_by ON gov.regulatory_reviews (tenant_id, reviewed_by);
CREATE INDEX ix_gov_ai_systems_vendor_party_id ON gov.ai_systems (tenant_id, vendor_party_id);
CREATE INDEX ix_gov_ai_systems_assessment_id ON gov.ai_systems (tenant_id, assessment_id);
CREATE INDEX ix_gov_ai_systems_owner_user_id ON gov.ai_systems (tenant_id, owner_user_id);
CREATE INDEX ix_gov_masking_jobs_source_file_id ON gov.masking_jobs (tenant_id, source_file_id);
CREATE INDEX ix_gov_masking_jobs_output_file_id ON gov.masking_jobs (tenant_id, output_file_id);
CREATE INDEX ix_gov_masking_jobs_requested_by ON gov.masking_jobs (tenant_id, requested_by);
CREATE INDEX ix_gov_kb_chunks_kb_article_id ON gov.kb_chunks (kb_article_id);

-- ---------------------------------------------------------------- row-level security
ALTER TABLE platform.tenant_modules ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.tenant_modules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.tenant_modules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.public_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.public_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY public_read ON platform.public_keys FOR SELECT USING (true);
CREATE POLICY tenant_write ON platform.public_keys FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.workflow_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.workflow_definitions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.workflow_definitions FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.workflow_definitions FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.workflow_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.workflow_instances FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.workflow_instances USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.workflow_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.workflow_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.workflow_tasks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.sla_timers ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.sla_timers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.sla_timers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.form_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.form_definitions FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.form_definitions FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.form_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.form_versions FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.form_versions FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.form_submissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.form_submissions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.form_submissions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.record_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.record_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.record_versions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.approvals USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.comments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.comments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.files ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.files FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.files USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.documents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.documents USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.document_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.document_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.document_versions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.clause_library ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.clause_library FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.clause_library FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.clause_library FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.notification_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON platform.notification_templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON platform.notification_templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.notifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.notifications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.outbox_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.outbox_events USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.webhook_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.webhook_subscriptions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.webhook_subscriptions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.webhook_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.webhook_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.webhook_deliveries USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.audit_log FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.audit_log_default ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.audit_log_default FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.audit_log_default USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON platform.audit_log USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.import_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.import_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.import_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.export_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.export_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.export_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.connectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.connectors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.connectors USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE platform.ai_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.ai_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.ai_requests USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.users ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.users USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON iam.roles FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON iam.roles FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.role_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON iam.role_permissions FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON iam.role_permissions FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.groups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.groups USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.group_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.group_members FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.group_members USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.role_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.role_assignments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.api_clients ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.api_clients FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.api_clients USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.guest_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.guest_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.guest_tokens USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.idp_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.idp_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.idp_configs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.security_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.security_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.security_policies USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.security_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.security_events FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.security_events_default ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.security_events_default FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.security_events_default USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON iam.security_events USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.access_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.access_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.access_reviews USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.access_review_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.access_review_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.access_review_items USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.breakglass_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.breakglass_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.breakglass_requests USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.delegations ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.delegations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.delegations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.field_masking_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.field_masking_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON iam.field_masking_rules FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON iam.field_masking_rules FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.unmask_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.unmask_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.unmask_logs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE iam.subject_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE iam.subject_verifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON iam.subject_verifications USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
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
ALTER TABLE consent.data_elements ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.data_elements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.data_elements USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.purposes ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.purposes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.purposes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.purpose_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.purpose_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.purpose_versions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.purpose_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.purpose_preferences FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.purpose_preferences USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.purpose_data_elements ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.purpose_data_elements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.purpose_data_elements USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.collection_points ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.collection_points FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.collection_points USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.collection_point_purposes ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.collection_point_purposes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.collection_point_purposes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.data_subjects ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.data_subjects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.data_subjects USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.subject_identifiers ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.subject_identifiers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.subject_identifiers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.consent_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.consent_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.consent_receipts USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.consent_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.consent_transactions FORCE ROW LEVEL SECURITY;
ALTER TABLE consent.consent_transactions_default ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.consent_transactions_default FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.consent_transactions_default USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON consent.consent_transactions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.consent_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.consent_status FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.consent_status USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.double_optin_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.double_optin_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.double_optin_requests USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.guardian_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.guardian_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.guardian_approvals USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.campaigns ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.campaigns FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.campaigns USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.campaign_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.campaign_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.campaign_recipients USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.reconcile_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.reconcile_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.reconcile_runs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.reconcile_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.reconcile_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.reconcile_items USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE consent.downstream_syncs ENABLE ROW LEVEL SECURITY;
ALTER TABLE consent.downstream_syncs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON consent.downstream_syncs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
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
ALTER TABLE ropa.processing_activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.processing_activities FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.processing_activities USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_purposes ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_purposes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_purposes USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_data ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_data FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_data USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_systems ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_systems FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_systems USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_recipients USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_transfers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_transfers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.retention_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.retention_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.retention_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_controls FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_controls USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_rejections ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_rejections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_rejections USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.assets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.assets USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.data_inventory ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.data_inventory FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.data_inventory USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.questionnaires ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.questionnaires FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.questionnaires USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.sme_exemption_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.sme_exemption_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.sme_exemption_checks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.template_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.template_sets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ropa.template_sets FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON ropa.template_sets FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.activity_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.activity_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ropa.activity_templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON ropa.activity_templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.generation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.generation_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.generation_runs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE ropa.wizard_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa.wizard_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.wizard_sessions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
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
ALTER TABLE assess.templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON assess.templates FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON assess.templates FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.screening_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.screening_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.screening_rules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.assessments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.assessments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.sections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.sections USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.answers ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.answers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.answers USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.assessment_risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.assessment_risks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.assessment_risks USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.dpo_opinions ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.dpo_opinions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.dpo_opinions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE assess.consultations ENABLE ROW LEVEL SECURITY;
ALTER TABLE assess.consultations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assess.consultations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
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
ALTER TABLE gov.courses ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.courses FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON gov.courses FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON gov.courses FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.training_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.training_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.training_assignments USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.training_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.training_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.training_attempts USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.policies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.policies USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.policy_attestations ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.policy_attestations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.policy_attestations USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.audits ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.audits FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.audits USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.audit_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.audit_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.audit_findings USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.retention_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.retention_schedules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.retention_schedules USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.disposal_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.disposal_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.disposal_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.regulator_letters ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.regulator_letters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.regulator_letters USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.regulatory_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.regulatory_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.regulatory_reviews USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.ai_systems ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.ai_systems FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.ai_systems USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.masking_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.masking_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.masking_jobs USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
ALTER TABLE gov.kb_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE gov.kb_chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON gov.kb_chunks FOR SELECT USING (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_write ON gov.kb_chunks FOR ALL USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---------------------------------------------------------------- updated_at triggers
CREATE TRIGGER trg_tenants_updated BEFORE UPDATE ON platform.tenants FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_tenant_modules_updated BEFORE UPDATE ON platform.tenant_modules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_workflow_definitions_updated BEFORE UPDATE ON platform.workflow_definitions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_workflow_instances_updated BEFORE UPDATE ON platform.workflow_instances FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_workflow_tasks_updated BEFORE UPDATE ON platform.workflow_tasks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sla_timers_updated BEFORE UPDATE ON platform.sla_timers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_form_definitions_updated BEFORE UPDATE ON platform.form_definitions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_form_versions_updated BEFORE UPDATE ON platform.form_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_form_submissions_updated BEFORE UPDATE ON platform.form_submissions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_record_versions_updated BEFORE UPDATE ON platform.record_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_approvals_updated BEFORE UPDATE ON platform.approvals FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_comments_updated BEFORE UPDATE ON platform.comments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_files_updated BEFORE UPDATE ON platform.files FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_documents_updated BEFORE UPDATE ON platform.documents FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_document_versions_updated BEFORE UPDATE ON platform.document_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_templates_updated BEFORE UPDATE ON platform.templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_clause_library_updated BEFORE UPDATE ON platform.clause_library FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_notification_templates_updated BEFORE UPDATE ON platform.notification_templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_notifications_updated BEFORE UPDATE ON platform.notifications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_outbox_events_updated BEFORE UPDATE ON platform.outbox_events FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_webhook_subscriptions_updated BEFORE UPDATE ON platform.webhook_subscriptions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_webhook_deliveries_updated BEFORE UPDATE ON platform.webhook_deliveries FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_import_jobs_updated BEFORE UPDATE ON platform.import_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_export_jobs_updated BEFORE UPDATE ON platform.export_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_connectors_updated BEFORE UPDATE ON platform.connectors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_ai_requests_updated BEFORE UPDATE ON platform.ai_requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_users_updated BEFORE UPDATE ON iam.users FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_roles_updated BEFORE UPDATE ON iam.roles FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_groups_updated BEFORE UPDATE ON iam.groups FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_group_members_updated BEFORE UPDATE ON iam.group_members FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_role_assignments_updated BEFORE UPDATE ON iam.role_assignments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_api_clients_updated BEFORE UPDATE ON iam.api_clients FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_guest_tokens_updated BEFORE UPDATE ON iam.guest_tokens FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_idp_configs_updated BEFORE UPDATE ON iam.idp_configs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_security_policies_updated BEFORE UPDATE ON iam.security_policies FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_access_reviews_updated BEFORE UPDATE ON iam.access_reviews FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_access_review_items_updated BEFORE UPDATE ON iam.access_review_items FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_breakglass_requests_updated BEFORE UPDATE ON iam.breakglass_requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_delegations_updated BEFORE UPDATE ON iam.delegations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_field_masking_rules_updated BEFORE UPDATE ON iam.field_masking_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_subject_verifications_updated BEFORE UPDATE ON iam.subject_verifications FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_legal_entities_updated BEFORE UPDATE ON org.legal_entities FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_org_units_updated BEFORE UPDATE ON org.org_units FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_privacy_champions_updated BEFORE UPDATE ON org.privacy_champions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_external_parties_updated BEFORE UPDATE ON org.external_parties FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_categories_updated BEFORE UPDATE ON org.data_categories FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_subject_types_updated BEFORE UPDATE ON org.data_subject_types FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_processing_purposes_updated BEFORE UPDATE ON org.processing_purposes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_business_calendars_updated BEFORE UPDATE ON org.business_calendars FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_org_settings_updated BEFORE UPDATE ON org.org_settings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_elements_updated BEFORE UPDATE ON consent.data_elements FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_purposes_updated BEFORE UPDATE ON consent.purposes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_purpose_versions_updated BEFORE UPDATE ON consent.purpose_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_purpose_preferences_updated BEFORE UPDATE ON consent.purpose_preferences FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_collection_points_updated BEFORE UPDATE ON consent.collection_points FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_subjects_updated BEFORE UPDATE ON consent.data_subjects FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_subject_identifiers_updated BEFORE UPDATE ON consent.subject_identifiers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_double_optin_requests_updated BEFORE UPDATE ON consent.double_optin_requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_guardian_approvals_updated BEFORE UPDATE ON consent.guardian_approvals FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_campaigns_updated BEFORE UPDATE ON consent.campaigns FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_campaign_recipients_updated BEFORE UPDATE ON consent.campaign_recipients FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_reconcile_runs_updated BEFORE UPDATE ON consent.reconcile_runs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_reconcile_items_updated BEFORE UPDATE ON consent.reconcile_items FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_downstream_syncs_updated BEFORE UPDATE ON consent.downstream_syncs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
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
CREATE TRIGGER trg_notices_updated BEFORE UPDATE ON notice.notices FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_notice_versions_updated BEFORE UPDATE ON notice.notice_versions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_indirect_collections_updated BEFORE UPDATE ON notice.indirect_collections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_embeds_updated BEFORE UPDATE ON notice.embeds FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_linked_documents_updated BEFORE UPDATE ON notice.linked_documents FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_wizard_templates_updated BEFORE UPDATE ON notice.wizard_templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_processing_activities_updated BEFORE UPDATE ON ropa.processing_activities FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_purposes_updated BEFORE UPDATE ON ropa.activity_purposes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_data_updated BEFORE UPDATE ON ropa.activity_data FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_recipients_updated BEFORE UPDATE ON ropa.activity_recipients FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_transfers_updated BEFORE UPDATE ON ropa.activity_transfers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_retention_rules_updated BEFORE UPDATE ON ropa.retention_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_rejections_updated BEFORE UPDATE ON ropa.activity_rejections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_assets_updated BEFORE UPDATE ON ropa.assets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_data_inventory_updated BEFORE UPDATE ON ropa.data_inventory FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_questionnaires_updated BEFORE UPDATE ON ropa.questionnaires FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sme_exemption_checks_updated BEFORE UPDATE ON ropa.sme_exemption_checks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_template_sets_updated BEFORE UPDATE ON ropa.template_sets FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_activity_templates_updated BEFORE UPDATE ON ropa.activity_templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_generation_runs_updated BEFORE UPDATE ON ropa.generation_runs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_wizard_sessions_updated BEFORE UPDATE ON ropa.wizard_sessions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_layouts_updated BEFORE UPDATE ON dataflow.layouts FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_snapshots_updated BEFORE UPDATE ON dataflow.snapshots FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_classifiers_updated BEFORE UPDATE ON dataflow.classifiers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_discovery_scans_updated BEFORE UPDATE ON dataflow.discovery_scans FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_discovery_findings_updated BEFORE UPDATE ON dataflow.discovery_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risk_matrices_updated BEFORE UPDATE ON risk.risk_matrices FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risk_factors_updated BEFORE UPDATE ON risk.risk_factors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_controls_updated BEFORE UPDATE ON risk.controls FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risks_updated BEFORE UPDATE ON risk.risks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_risk_controls_updated BEFORE UPDATE ON risk.risk_controls FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_acceptances_updated BEFORE UPDATE ON risk.acceptances FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_gap_rules_updated BEFORE UPDATE ON risk.gap_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_gap_findings_updated BEFORE UPDATE ON risk.gap_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_templates_updated BEFORE UPDATE ON assess.templates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_screening_rules_updated BEFORE UPDATE ON assess.screening_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_assessments_updated BEFORE UPDATE ON assess.assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sections_updated BEFORE UPDATE ON assess.sections FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_answers_updated BEFORE UPDATE ON assess.answers FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_dpo_opinions_updated BEFORE UPDATE ON assess.dpo_opinions FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_consultations_updated BEFORE UPDATE ON assess.consultations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
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
CREATE TRIGGER trg_vendors_updated BEFORE UPDATE ON vendor.vendors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_intakes_updated BEFORE UPDATE ON vendor.intakes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_vendor_assessments_updated BEFORE UPDATE ON vendor.vendor_assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_sub_processors_updated BEFORE UPDATE ON vendor.sub_processors FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_certificates_updated BEFORE UPDATE ON vendor.certificates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_remediation_items_updated BEFORE UPDATE ON vendor.remediation_items FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_offboardings_updated BEFORE UPDATE ON vendor.offboardings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_agreements_updated BEFORE UPDATE ON agreement.agreements FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_parties_updated BEFORE UPDATE ON agreement.parties FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_clauses_updated BEFORE UPDATE ON agreement.clauses FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_mandatory_rules_updated BEFORE UPDATE ON agreement.mandatory_rules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_annexes_updated BEFORE UPDATE ON agreement.annexes FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_signature_requests_updated BEFORE UPDATE ON agreement.signature_requests FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_obligations_updated BEFORE UPDATE ON agreement.obligations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_return_confirmations_updated BEFORE UPDATE ON agreement.return_confirmations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_appointments_updated BEFORE UPDATE ON dpo.appointments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_requirement_checks_updated BEFORE UPDATE ON dpo.requirement_checks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_independence_declarations_updated BEFORE UPDATE ON dpo.independence_declarations FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_tasks_updated BEFORE UPDATE ON dpo.tasks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_advisories_updated BEFORE UPDATE ON dpo.advisories FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_kb_articles_updated BEFORE UPDATE ON dpo.kb_articles FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_calendar_events_updated BEFORE UPDATE ON dpo.calendar_events FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_report_schedules_updated BEFORE UPDATE ON dpo.report_schedules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_courses_updated BEFORE UPDATE ON gov.courses FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_training_assignments_updated BEFORE UPDATE ON gov.training_assignments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_training_attempts_updated BEFORE UPDATE ON gov.training_attempts FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_policies_updated BEFORE UPDATE ON gov.policies FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_audits_updated BEFORE UPDATE ON gov.audits FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_audit_findings_updated BEFORE UPDATE ON gov.audit_findings FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_retention_schedules_updated BEFORE UPDATE ON gov.retention_schedules FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_disposal_jobs_updated BEFORE UPDATE ON gov.disposal_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulator_letters_updated BEFORE UPDATE ON gov.regulator_letters FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulatory_updates_updated BEFORE UPDATE ON gov.regulatory_updates FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_regulatory_reviews_updated BEFORE UPDATE ON gov.regulatory_reviews FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_ai_systems_updated BEFORE UPDATE ON gov.ai_systems FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
CREATE TRIGGER trg_masking_jobs_updated BEFORE UPDATE ON gov.masking_jobs FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

-- ---------------------------------------------------------------- comments
COMMENT ON TABLE platform.tenants IS 'Tenant (องค์กรลูกค้า)';
COMMENT ON TABLE platform.tenant_modules IS 'โมดูลที่เปิดใช้ต่อ tenant (license)';
COMMENT ON TABLE platform.public_keys IS 'public key สำหรับ /public/v1 และ portal: หา tenant ก่อนเปิด transaction (ทุกคนอ่านได้ · เขียนได้เฉพาะ tenant เจ้าของ)';
COMMENT ON TABLE platform.workflow_definitions IS 'นิยาม workflow (state machine) ต่อประเภทงาน';
COMMENT ON TABLE platform.workflow_instances IS 'workflow ที่กำลังทำงานของแต่ละ record';
COMMENT ON TABLE platform.workflow_tasks IS 'งานในแต่ละขั้นของ workflow';
COMMENT ON TABLE platform.sla_timers IS 'ตัวนับเวลา SLA (วันปฏิทิน / วันทำการ / ชั่วโมง)';
COMMENT ON TABLE platform.form_definitions IS 'ฟอร์ม / แบบประเมิน (form engine)';
COMMENT ON TABLE platform.form_versions IS 'เวอร์ชันของฟอร์ม (schema JSON)';
COMMENT ON TABLE platform.form_submissions IS 'คำตอบของฟอร์ม';
COMMENT ON TABLE platform.record_versions IS 'snapshot และ diff ของ record ที่มีเวอร์ชัน';
COMMENT ON TABLE platform.approvals IS 'ขั้นตอนอนุมัติ (maker-checker / หลายระดับ)';
COMMENT ON TABLE platform.comments IS 'ความเห็น / @mention ต่อ record';
COMMENT ON TABLE platform.files IS 'ไฟล์ใน object storage (เข้ารหัส + สแกนไวรัส)';
COMMENT ON TABLE platform.documents IS 'เอกสารที่สร้างจาก composer (ประกาศ สัญญา หนังสือ แบบแจ้ง)';
COMMENT ON TABLE platform.document_versions IS 'เวอร์ชันของเอกสาร (ProseMirror JSON + ไฟล์ที่ render)';
COMMENT ON TABLE platform.templates IS 'template เอกสาร / ข้อความ (กลางและของ tenant)';
COMMENT ON TABLE platform.clause_library IS 'คลังข้อความสัญญา / clause มาตรฐาน';
COMMENT ON TABLE platform.notification_templates IS 'template อีเมล / SMS / LINE / in-app';
COMMENT ON TABLE platform.notifications IS 'ข้อความที่ส่งและสถานะการส่ง';
COMMENT ON TABLE platform.outbox_events IS 'transactional outbox ของ domain event';
COMMENT ON TABLE platform.webhook_subscriptions IS 'webhook ที่ระบบปลายทางลงทะเบียน';
COMMENT ON TABLE platform.webhook_deliveries IS 'การส่ง webhook แต่ละครั้ง (retry / dead-letter)';
COMMENT ON TABLE platform.audit_log IS 'audit log แบบ append-only + hash chain (partition รายเดือน)';
COMMENT ON TABLE platform.import_jobs IS 'งานนำเข้า Excel / CSV';
COMMENT ON TABLE platform.export_jobs IS 'งานส่งออกแบบ async (ศูนย์ดาวน์โหลด)';
COMMENT ON TABLE platform.connectors IS 'connector ไปยังระบบของลูกค้า (DB / REST / SaaS)';
COMMENT ON TABLE platform.ai_requests IS 'log การเรียก AI (หลังปกปิด PII)';
COMMENT ON TABLE iam.users IS 'ผู้ใช้ระบบ (ผูกกับบัญชี Keycloak)';
COMMENT ON TABLE iam.roles IS 'role มาตรฐาน (tenant_id ว่าง) และ custom role';
COMMENT ON TABLE iam.permissions IS 'catalog สิทธิ์ module.resource.action (สร้างจาก OpenAPI x-permission)';
COMMENT ON TABLE iam.role_permissions IS 'สิทธิ์ของแต่ละ role';
COMMENT ON TABLE iam.groups IS 'กลุ่มผู้ใช้ / ทีม';
COMMENT ON TABLE iam.group_members IS 'สมาชิกของกลุ่ม';
COMMENT ON TABLE iam.role_assignments IS 'การมอบ role ให้ผู้ใช้ / กลุ่ม พร้อมขอบเขตข้อมูล';
COMMENT ON TABLE iam.api_clients IS 'API client / service account (OAuth2 client credentials)';
COMMENT ON TABLE iam.guest_tokens IS 'magic link ของผู้ใช้ภายนอก (คู่ค้า ผู้ประมวลผล ผู้ร่วมประเมิน)';
COMMENT ON TABLE iam.idp_configs IS 'การตั้งค่า SSO / IdP ต่อ tenant (สะท้อนค่าใน Keycloak)';
COMMENT ON TABLE iam.security_policies IS 'นโยบายรหัสผ่าน MFA session ต่อ tenant';
COMMENT ON TABLE iam.security_events IS 'เหตุการณ์ความปลอดภัยของบัญชี (login, lockout, อุปกรณ์ใหม่)';
COMMENT ON TABLE iam.access_reviews IS 'รอบทบทวนสิทธิ์';
COMMENT ON TABLE iam.access_review_items IS 'รายการที่ต้องยืนยันในรอบทบทวน';
COMMENT ON TABLE iam.breakglass_requests IS 'การขอใช้สิทธิ์ฉุกเฉิน';
COMMENT ON TABLE iam.delegations IS 'การมอบอำนาจ / สิทธิ์ชั่วคราว';
COMMENT ON TABLE iam.field_masking_rules IS 'กฎการปกปิดข้อมูลบนหน้าจอ';
COMMENT ON TABLE iam.unmask_logs IS 'การเปิดดูข้อมูลที่ปกปิด (ต้องระบุเหตุผล)';
COMMENT ON TABLE iam.subject_verifications IS 'การยืนยันตัวตนของเจ้าของข้อมูล (OTP / magic link / IdP)';
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
COMMENT ON TABLE consent.data_elements IS 'Data Element: ข้อมูลที่ใช้ในแต่ละวัตถุประสงค์';
COMMENT ON TABLE consent.purposes IS 'Purpose: วัตถุประสงค์ที่ขอความยินยอม';
COMMENT ON TABLE consent.purpose_versions IS 'ข้อความของ Purpose แต่ละเวอร์ชัน';
COMMENT ON TABLE consent.purpose_preferences IS 'Purpose Preference: ตัวเลือกย่อย เช่น ช่องทาง หัวข้อ ความถี่';
COMMENT ON TABLE consent.purpose_data_elements IS 'Data Element ที่ใช้ในแต่ละ Purpose';
COMMENT ON TABLE consent.collection_points IS 'Collection Point: จุดเก็บความยินยอม';
COMMENT ON TABLE consent.collection_point_purposes IS 'Purpose ที่แสดงใน Collection Point';
COMMENT ON TABLE consent.data_subjects IS 'เจ้าของข้อมูล (identifier เข้ารหัส + blind index)';
COMMENT ON TABLE consent.subject_identifiers IS 'identifier ของเจ้าของข้อมูล (อีเมล เบอร์ เลขบัตร รหัสลูกค้า)';
COMMENT ON TABLE consent.consent_receipts IS 'Receipt: หลักฐานการตอบแต่ละครั้ง (hash chain)';
COMMENT ON TABLE consent.consent_transactions IS 'Consent Transaction: รายการต่อ Purpose (partition รายเดือน)';
COMMENT ON TABLE consent.consent_status IS 'สถานะล่าสุดต่อเจ้าของข้อมูล × Purpose (projection)';
COMMENT ON TABLE consent.double_optin_requests IS 'คำขอยืนยัน Double Opt-In';
COMMENT ON TABLE consent.guardian_approvals IS 'การให้ความยินยอมโดยผู้ปกครอง / ผู้อนุบาล (ม.20)';
COMMENT ON TABLE consent.campaigns IS 'แคมเปญขอความยินยอมแบบ bulk';
COMMENT ON TABLE consent.campaign_recipients IS 'ผู้รับของแคมเปญ';
COMMENT ON TABLE consent.reconcile_runs IS 'Reconcile Report: รอบเทียบสถานะกับระบบปลายทาง';
COMMENT ON TABLE consent.reconcile_items IS 'รายการที่สถานะไม่ตรงกัน';
COMMENT ON TABLE consent.downstream_syncs IS 'การยืนยันจากระบบปลายทางหลังเปลี่ยนสถานะ';
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
COMMENT ON TABLE notice.notices IS 'ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV';
COMMENT ON TABLE notice.notice_versions IS 'เวอร์ชันที่เผยแพร่ + ผล checklist ม.23';
COMMENT ON TABLE notice.notice_activity_links IS 'กิจกรรม RoPA ที่ประกาศครอบคลุม';
COMMENT ON TABLE notice.acknowledgements IS 'การรับทราบประกาศ';
COMMENT ON TABLE notice.indirect_collections IS 'การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25)';
COMMENT ON TABLE notice.embeds IS 'โค้ดฝังและลิงก์ของประกาศ';
COMMENT ON TABLE notice.linked_documents IS 'เอกสารอ้างอิงที่ลิงก์ในประกาศ';
COMMENT ON TABLE notice.wizard_templates IS 'template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม';
COMMENT ON TABLE ropa.processing_activities IS 'กิจกรรมการประมวลผล (RoPA ม.39 / ผู้ประมวลผล)';
COMMENT ON TABLE ropa.activity_purposes IS 'วัตถุประสงค์และฐานกฎหมายของกิจกรรม';
COMMENT ON TABLE ropa.activity_data IS 'ข้อมูลที่เก็บในกิจกรรม';
COMMENT ON TABLE ropa.activity_systems IS 'ระบบ / asset ที่ใช้ในกิจกรรม';
COMMENT ON TABLE ropa.activity_recipients IS 'ผู้รับข้อมูลและการเปิดเผย (ม.27)';
COMMENT ON TABLE ropa.activity_transfers IS 'การโอนไปต่างประเทศ (ม.28 / ม.29)';
COMMENT ON TABLE ropa.retention_rules IS 'ระยะเวลาเก็บรักษาและวิธีทำลาย';
COMMENT ON TABLE ropa.activity_controls IS 'มาตรการความปลอดภัยต่อกิจกรรม (ม.37(1))';
COMMENT ON TABLE ropa.activity_rejections IS 'การปฏิเสธคำขอใช้สิทธิที่เกี่ยวข้อง (ม.39(7))';
COMMENT ON TABLE ropa.assets IS 'ทะเบียนระบบ / asset';
COMMENT ON TABLE ropa.data_inventory IS 'ทะเบียนข้อมูลส่วนบุคคลต่อระบบ';
COMMENT ON TABLE ropa.questionnaires IS 'แบบสอบถามเก็บข้อมูลกิจกรรมจากหน่วยงาน';
COMMENT ON TABLE ropa.sme_exemption_checks IS 'ผลตรวจสิทธิ์ยกเว้น RoPA ของกิจการขนาดเล็ก';
COMMENT ON TABLE ropa.template_sets IS 'ชุด template (มาตรฐาน / อุตสาหกรรม / ภาครัฐ / ขององค์กร)';
COMMENT ON TABLE ropa.activity_templates IS 'กิจกรรมมาตรฐานพร้อมค่าตั้งต้นและเหตุผล';
COMMENT ON TABLE ropa.generation_runs IS 'การสร้างร่าง RoPA จาก template ครั้งละหลายกิจกรรม';
COMMENT ON TABLE ropa.wizard_sessions IS 'session ของ wizard ถาม-ตอบภาษาง่าย';
COMMENT ON TABLE dataflow.layouts IS 'ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง';
COMMENT ON TABLE dataflow.snapshots IS 'แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน)';
COMMENT ON TABLE dataflow.classifiers IS 'classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย)';
COMMENT ON TABLE dataflow.discovery_scans IS 'รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector';
COMMENT ON TABLE dataflow.discovery_findings IS 'ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง)';
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
COMMENT ON TABLE assess.templates IS 'template แบบประเมิน (DPIA / LIA / TIA / AI / security / maturity ฯลฯ)';
COMMENT ON TABLE assess.screening_rules IS 'เกณฑ์คัดกรอง / บังคับทำ DPIA';
COMMENT ON TABLE assess.assessments IS 'แบบประเมินแต่ละครั้ง';
COMMENT ON TABLE assess.sections IS 'ส่วนของแบบประเมินที่มอบหมายผู้ตอบ';
COMMENT ON TABLE assess.answers IS 'คำตอบรายข้อ + หลักฐาน';
COMMENT ON TABLE assess.assessment_risks IS 'ความเสี่ยงที่ระบุในแบบประเมิน';
COMMENT ON TABLE assess.dpo_opinions IS 'ความเห็นของ DPO';
COMMENT ON TABLE assess.consultations IS 'บันทึกการปรึกษาผู้มีส่วนได้เสีย';
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
COMMENT ON TABLE vendor.vendors IS 'คู่ค้า / ผู้ประมวลผล (ต่อยอดจาก org.external_parties)';
COMMENT ON TABLE vendor.intakes IS 'แบบ intake สำหรับจัดระดับความเสี่ยง (tier)';
COMMENT ON TABLE vendor.vendor_assessments IS 'รอบประเมินคู่ค้า (ใช้ assessment engine)';
COMMENT ON TABLE vendor.sub_processors IS 'ผู้ประมวลผลช่วง';
COMMENT ON TABLE vendor.certificates IS 'ใบรับรองของคู่ค้า (ISO 27001, SOC 2 ฯลฯ)';
COMMENT ON TABLE vendor.remediation_items IS 'ประเด็นที่คู่ค้าต้องแก้ไข';
COMMENT ON TABLE vendor.offboardings IS 'การยุติการใช้บริการ';
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
COMMENT ON TABLE dpo.appointments IS 'การแต่งตั้ง DPO และการแจ้ง สคส.';
COMMENT ON TABLE dpo.requirement_checks IS 'ผลประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41)';
COMMENT ON TABLE dpo.independence_declarations IS 'คำรับรองความเป็นอิสระ / ผลประโยชน์ทับซ้อน';
COMMENT ON TABLE dpo.tasks IS 'งาน / ticket 5 สถานะ (สร้างอัตโนมัติจาก gap / DPIA / audit)';
COMMENT ON TABLE dpo.advisories IS 'คำปรึกษาจากหน่วยงานถึง DPO';
COMMENT ON TABLE dpo.kb_articles IS 'คลังเอกสารกฎหมายและ FAQ';
COMMENT ON TABLE dpo.calendar_events IS 'ปฏิทินงาน compliance (กิจกรรมที่ไม่ได้มาจากโมดูลอื่น)';
COMMENT ON TABLE dpo.report_schedules IS 'ตั้งเวลาส่งรายงาน DPO / ผู้บริหาร';
COMMENT ON TABLE gov.courses IS 'คอร์สอบรม PDPA';
COMMENT ON TABLE gov.training_assignments IS 'การมอบหมายอบรม';
COMMENT ON TABLE gov.training_attempts IS 'ผลการเรียน / สอบรายบุคคล';
COMMENT ON TABLE gov.policies IS 'นโยบาย / คู่มือภายในที่ต้องรับทราบ';
COMMENT ON TABLE gov.policy_attestations IS 'การรับทราบนโยบาย';
COMMENT ON TABLE gov.audits IS 'การตรวจประเมินความพร้อม PDPA ระดับองค์กร';
COMMENT ON TABLE gov.audit_findings IS 'ข้อตรวจพบและแผนแก้ไข';
COMMENT ON TABLE gov.retention_schedules IS 'ตาราง retention ต่อระบบ (สร้างจาก RoPA)';
COMMENT ON TABLE gov.disposal_jobs IS 'งานลบ / ทำลาย / ทำให้ไม่ระบุตัวตน';
COMMENT ON TABLE gov.regulator_letters IS 'ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส.';
COMMENT ON TABLE gov.regulatory_updates IS 'ฟีดประกาศ / แนวปฏิบัติใหม่ (ทีมเนื้อหาผู้ให้บริการ)';
COMMENT ON TABLE gov.regulatory_reviews IS 'การประเมินผลกระทบของประกาศใหม่ต่อ tenant';
COMMENT ON TABLE gov.ai_systems IS 'ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล';
COMMENT ON TABLE gov.masking_jobs IS 'งาน mask / tokenize ข้อมูลสำหรับทดสอบ';
COMMENT ON TABLE gov.kb_chunks IS 'ชิ้นเอกสารสำหรับ RAG ของผู้ช่วย AI (pgvector)';

