-- +goose Up
-- schema consent: ความยินยอมตาม FSD V3.2: Data Element, Purpose, Purpose Preference, Collection Point, Consent Transaction, Reconcile
-- 19 tables · docs: docs/data/consent.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

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

-- indexes
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

-- row-level security (tenant isolation)
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

-- updated_at / row_version triggers
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

-- comments
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

-- +goose Down
DROP TABLE IF EXISTS consent.downstream_syncs;
DROP TABLE IF EXISTS consent.reconcile_items;
DROP TABLE IF EXISTS consent.reconcile_runs;
DROP TABLE IF EXISTS consent.campaign_recipients;
DROP TABLE IF EXISTS consent.campaigns;
DROP TABLE IF EXISTS consent.guardian_approvals;
DROP TABLE IF EXISTS consent.double_optin_requests;
DROP TABLE IF EXISTS consent.consent_status;
DROP TABLE IF EXISTS consent.consent_transactions;
DROP TABLE IF EXISTS consent.consent_receipts;
DROP TABLE IF EXISTS consent.subject_identifiers;
DROP TABLE IF EXISTS consent.data_subjects;
DROP TABLE IF EXISTS consent.collection_point_purposes;
DROP TABLE IF EXISTS consent.collection_points;
DROP TABLE IF EXISTS consent.purpose_data_elements;
DROP TABLE IF EXISTS consent.purpose_preferences;
DROP TABLE IF EXISTS consent.purpose_versions;
DROP TABLE IF EXISTS consent.purposes;
DROP TABLE IF EXISTS consent.data_elements;
