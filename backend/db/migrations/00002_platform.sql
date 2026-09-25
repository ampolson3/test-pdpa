-- +goose Up
-- schema platform: บริการกลางของแพลตฟอร์ม: tenant, workflow, form, เอกสาร, ไฟล์, แจ้งเตือน, event, audit
-- 28 tables · docs: docs/data/platform.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

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

-- indexes
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

-- row-level security (tenant isolation)
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

-- updated_at / row_version triggers
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

-- comments
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

-- +goose Down
DROP TABLE IF EXISTS platform.ai_requests;
DROP TABLE IF EXISTS platform.connectors;
DROP TABLE IF EXISTS platform.export_jobs;
DROP TABLE IF EXISTS platform.import_jobs;
DROP TABLE IF EXISTS platform.audit_log;
DROP TABLE IF EXISTS platform.webhook_deliveries;
DROP TABLE IF EXISTS platform.webhook_subscriptions;
DROP TABLE IF EXISTS platform.outbox_events;
DROP TABLE IF EXISTS platform.notifications;
DROP TABLE IF EXISTS platform.notification_templates;
DROP TABLE IF EXISTS platform.clause_library;
DROP TABLE IF EXISTS platform.templates;
DROP TABLE IF EXISTS platform.document_versions;
DROP TABLE IF EXISTS platform.documents;
DROP TABLE IF EXISTS platform.files;
DROP TABLE IF EXISTS platform.comments;
DROP TABLE IF EXISTS platform.approvals;
DROP TABLE IF EXISTS platform.record_versions;
DROP TABLE IF EXISTS platform.form_submissions;
DROP TABLE IF EXISTS platform.form_versions;
DROP TABLE IF EXISTS platform.form_definitions;
DROP TABLE IF EXISTS platform.sla_timers;
DROP TABLE IF EXISTS platform.workflow_tasks;
DROP TABLE IF EXISTS platform.workflow_instances;
DROP TABLE IF EXISTS platform.workflow_definitions;
DROP TABLE IF EXISTS platform.public_keys;
DROP TABLE IF EXISTS platform.tenant_modules;
DROP TABLE IF EXISTS platform.tenants;
