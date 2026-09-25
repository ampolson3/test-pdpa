-- +goose Up
-- schema iam: ผู้ใช้ สิทธิ์ ขอบเขตข้อมูล API client และการยืนยันตัวตน
-- 19 tables · docs: docs/data/iam.md · foreign keys live in 00018_foreign_keys.sql
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

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

-- indexes
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

-- row-level security (tenant isolation)
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

-- updated_at / row_version triggers
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

-- comments
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

-- +goose Down
DROP TABLE IF EXISTS iam.subject_verifications;
DROP TABLE IF EXISTS iam.unmask_logs;
DROP TABLE IF EXISTS iam.field_masking_rules;
DROP TABLE IF EXISTS iam.delegations;
DROP TABLE IF EXISTS iam.breakglass_requests;
DROP TABLE IF EXISTS iam.access_review_items;
DROP TABLE IF EXISTS iam.access_reviews;
DROP TABLE IF EXISTS iam.security_events;
DROP TABLE IF EXISTS iam.security_policies;
DROP TABLE IF EXISTS iam.idp_configs;
DROP TABLE IF EXISTS iam.guest_tokens;
DROP TABLE IF EXISTS iam.api_clients;
DROP TABLE IF EXISTS iam.role_assignments;
DROP TABLE IF EXISTS iam.group_members;
DROP TABLE IF EXISTS iam.groups;
DROP TABLE IF EXISTS iam.role_permissions;
DROP TABLE IF EXISTS iam.permissions;
DROP TABLE IF EXISTS iam.roles;
DROP TABLE IF EXISTS iam.users;
