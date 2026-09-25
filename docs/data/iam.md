# schema `iam`

> ผู้ใช้ สิทธิ์ ขอบเขตข้อมูล API client และการยืนยันตัวตน  
> migration: `backend/db/migrations/00003_iam.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [IAM](../modules/IAM.md) (`backend/internal/iam`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-03

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [users](#iam-users) | ผู้ใช้ระบบ (ผูกกับบัญชี Keycloak) | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12, SEQ-01 |
| [roles](#iam-roles) | role มาตรฐาน (tenant_id ว่าง) และ custom role | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-03 | BP-12 |
| [permissions](#iam-permissions) | catalog สิทธิ์ module.resource.action (สร้างจาก OpenAPI x-permission) | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-03 | BP-12, SEQ-01, SEQ-02 |
| [role_permissions](#iam-role-permissions) | สิทธิ์ของแต่ละ role | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-03 | BP-12, SEQ-02 |
| [groups](#iam-groups) | กลุ่มผู้ใช้ / ทีม | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12 |
| [group_members](#iam-group-members) | สมาชิกของกลุ่ม | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12 |
| [role_assignments](#iam-role-assignments) | การมอบ role ให้ผู้ใช้ / กลุ่ม พร้อมขอบเขตข้อมูล | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12, SEQ-01, SEQ-02 |
| [api_clients](#iam-api-clients) | API client / service account (OAuth2 client credentials) | tenant · RLS `tenant_isolation` |  | ERD-03 | SEQ-04 |
| [guest_tokens](#iam-guest-tokens) | magic link ของผู้ใช้ภายนอก (คู่ค้า ผู้ประมวลผล ผู้ร่วมประเมิน) | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-09 |
| [idp_configs](#iam-idp-configs) | การตั้งค่า SSO / IdP ต่อ tenant (สะท้อนค่าใน Keycloak) | tenant · RLS `tenant_isolation` |  | ERD-03 |  |
| [security_policies](#iam-security-policies) | นโยบายรหัสผ่าน MFA session ต่อ tenant | tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation` |  | ERD-03 | BP-12, SEQ-01 |
| [security_events](#iam-security-events) | เหตุการณ์ความปลอดภัยของบัญชี (login, lockout, อุปกรณ์ใหม่) | tenant · RLS `tenant_isolation` | ⟨P⟩ occurred_at | ERD-03 | BP-12, SEQ-01 |
| [access_reviews](#iam-access-reviews) | รอบทบทวนสิทธิ์ | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12 |
| [access_review_items](#iam-access-review-items) | รายการที่ต้องยืนยันในรอบทบทวน | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12 |
| [breakglass_requests](#iam-breakglass-requests) | การขอใช้สิทธิ์ฉุกเฉิน | tenant · RLS `tenant_isolation` |  | ERD-03 |  |
| [delegations](#iam-delegations) | การมอบอำนาจ / สิทธิ์ชั่วคราว | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-12, SEQ-02 |
| [field_masking_rules](#iam-field-masking-rules) | กฎการปกปิดข้อมูลบนหน้าจอ | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-03 | SEQ-02 |
| [unmask_logs](#iam-unmask-logs) | การเปิดดูข้อมูลที่ปกปิด (ต้องระบุเหตุผล) | tenant · RLS `tenant_isolation` |  | ERD-03 | SEQ-02 |
| [subject_verifications](#iam-subject-verifications) | การยืนยันตัวตนของเจ้าของข้อมูล (OTP / magic link / IdP) | tenant · RLS `tenant_isolation` |  | ERD-03 | BP-02, BP-06 |

<a id="iam-users"></a>
## iam.users

ผู้ใช้ระบบ (ผูกกับบัญชี Keycloak)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `keycloak_user_id` | `uuid` |  |  | UQ |  |
| `email` | `citext` | ✓ |  |  |  |
| `display_name` | `text` | ✓ |  |  |  |
| `phone` | `varchar(30)` |  |  |  |  |
| `status` | `text` | ✓ | 'invited' |  | ค่า: `invited`, `active`, `disabled`, `locked` · state machine [ST-07](../states/ST-07.md) |
| `locale` | `varchar(5)` | ✓ | 'th' |  |  |
| `primary_org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `mfa_required` | `boolean` | ✓ | false |  |  |
| `notification_prefs` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `avatar_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `last_login_at` | `timestamptz` |  |  |  |  |
| `disabled_at` | `timestamptz` |  |  |  |  |
| `source` | `text` | ✓ | 'local' |  | ค่า: `local`, `sso_jit`, `scim`, `import` |
| `external_id` | `varchar(200)` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_users_updated`
- PK: `(id)`
- Unique: `uq_users_keycloak_user_id UNIQUE (tenant_id, keycloak_user_id)`
- Index: `iam.users (tenant_id, primary_org_unit_id)` · `iam.users (tenant_id, avatar_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.workflow_tasks.assignee_user_id`, `platform.form_versions.published_by`, `platform.approvals.requested_by`, `platform.approvals.approver_user_id`, `platform.document_versions.approved_by`, `platform.notifications.recipient_user_id`, `platform.export_jobs.requested_by`, `platform.ai_requests.confirmed_by`, `iam.group_members.user_id`, `iam.role_assignments.user_id`, `iam.role_assignments.granted_by`, `iam.api_clients.owner_user_id`, `iam.guest_tokens.issued_by`, `iam.security_events.user_id`, `iam.access_review_items.reviewer_user_id`, `iam.breakglass_requests.requested_by`, `iam.breakglass_requests.approved_by`, `iam.delegations.from_user_id`, `iam.delegations.to_user_id`, `iam.unmask_logs.user_id`, `org.privacy_champions.user_id`, `consent.purpose_versions.approved_by`, `consent.consent_receipts.captured_by_user_id`, `cookie.banner_configs.published_by`, `notice.notices.owner_user_id`, `notice.notice_versions.published_by`, `notice.acknowledgements.user_id`, `ropa.processing_activities.owner_user_id`, `ropa.processing_activities.approved_by`, `ropa.assets.owner_user_id`, `ropa.data_inventory.owner_user_id`, `ropa.questionnaires.respondent_user_id`, `ropa.questionnaires.approved_by`, `ropa.wizard_sessions.user_id`, `dataflow.layouts.user_id`, `dataflow.snapshots.published_by`, `dataflow.discovery_findings.reviewed_by`, `risk.risks.owner_user_id`, `risk.risk_controls.owner_user_id`, `risk.acceptances.approved_by`, `assess.assessments.owner_user_id`, `assess.sections.assignee_user_id`, `assess.answers.confirmed_by`, `assess.dpo_opinions.dpo_user_id`, `dsar.requests.assignee_user_id`, `dsar.verifications.verified_by`, `dsar.subtasks.assignee_user_id`, `dsar.redactions.reviewed_by`, `breach.incidents.reporter_user_id`, `breach.incidents.decided_by`, `breach.assessments.assessed_by`, `breach.pdpc_notifications.approved_by`, `breach.response_tasks.assignee_user_id`, `breach.evidence.collected_by`, `vendor.vendors.relationship_owner_id`, `vendor.vendor_assessments.decided_by`, `vendor.sub_processors.approved_by`, `vendor.certificates.verified_by`, `agreement.obligations.owner_user_id`, `agreement.downloads.user_id`, `dpo.appointments.user_id`, `dpo.tasks.assignee_user_id`, `dpo.tasks.reviewer_user_id`, `dpo.advisories.requester_user_id`, `dpo.advisories.answered_by`, `dpo.calendar_events.owner_user_id`, `gov.training_assignments.assigned_by`, `gov.training_attempts.user_id`, `gov.policy_attestations.user_id`, `gov.audit_findings.owner_user_id`, `gov.disposal_jobs.assignee_user_id`, `gov.disposal_jobs.approved_by`, `gov.regulatory_reviews.reviewed_by`, `gov.ai_systems.owner_user_id`, `gov.masking_jobs.requested_by`

<a id="iam-roles"></a>
## iam.roles

role มาตรฐาน (tenant_id ว่าง) และ custom role

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `is_system` | `boolean` | ✓ | false |  |  |
| `requires_mfa` | `boolean` | ✓ | false |  |  |
| `cloned_from_id` | `uuid` |  |  | FK → [iam.roles](#iam-roles) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_roles_updated`
- PK: `(id)`
- Unique: `uq_roles_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- Index: `iam.roles (cloned_from_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `iam.roles.cloned_from_id`, `iam.role_permissions.role_id`, `iam.role_assignments.role_id`, `iam.delegations.role_id`

<a id="iam-permissions"></a>
## iam.permissions

catalog สิทธิ์ module.resource.action (สร้างจาก OpenAPI x-permission)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `code` | `varchar(80)` | ✓ |  | PK |  |
| `module` | `varchar(20)` | ✓ |  |  |  |
| `resource` | `varchar(40)` | ✓ |  |  |  |
| `action` | `text` | ✓ |  |  | ค่า: `C`, `R`, `U`, `D`, `A`, `P`, `E`, `X` |
| `description` | `text` |  |  |  |  |
| `is_sensitive` | `boolean` | ✓ | false |  |  |

- PK: `(code)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `iam.role_permissions.permission_code`, `iam.field_masking_rules.unmask_permission`

<a id="iam-role-permissions"></a>
## iam.role_permissions

สิทธิ์ของแต่ละ role

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `role_id` | `uuid` | ✓ |  | PK · FK → [iam.roles](#iam-roles) |  |
| `permission_code` | `varchar(80)` | ✓ |  | PK · FK → [iam.permissions](#iam-permissions) |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |

- PK: `(role_id, permission_code)`
- Index: `iam.role_permissions (permission_code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`

<a id="iam-groups"></a>
## iam.groups

กลุ่มผู้ใช้ / ทีม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `source` | `text` | ✓ | 'local' |  | ค่า: `local`, `scim`, `idp` |
| `external_id` | `varchar(200)` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_groups_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.workflow_tasks.assignee_group_id`, `iam.group_members.group_id`, `iam.role_assignments.group_id`, `dsar.subtasks.assignee_group_id`, `breach.response_tasks.assignee_group_id`, `breach.routing_rules.group_id`

<a id="iam-group-members"></a>
## iam.group_members

สมาชิกของกลุ่ม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `group_id` | `uuid` | ✓ |  | PK · FK → [iam.groups](#iam-groups) |  |
| `user_id` | `uuid` | ✓ |  | PK · FK → [iam.users](#iam-users) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_group_members_updated`
- PK: `(group_id, user_id)`
- Index: `iam.group_members (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-role-assignments"></a>
## iam.role_assignments

การมอบ role ให้ผู้ใช้ / กลุ่ม พร้อมขอบเขตข้อมูล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `user_id` | `uuid` |  |  | FK → [iam.users](#iam-users) · IX |  |
| `group_id` | `uuid` |  |  | FK → [iam.groups](#iam-groups) · IX |  |
| `role_id` | `uuid` | ✓ |  | FK → [iam.roles](#iam-roles) |  |
| `scope_type` | `text` | ✓ |  |  | ค่า: `tenant`, `legal_entity`, `org_unit`, `self` |
| `legal_entity_id` | `uuid` |  |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `include_descendants` | `boolean` | ✓ | true |  |  |
| `valid_from` | `timestamptz` | ✓ | now() |  |  |
| `valid_to` | `timestamptz` |  |  |  |  |
| `granted_by` | `uuid` |  |  | FK → [iam.users](#iam-users) |  |
| `approval_id` | `uuid` |  |  | FK → [platform.approvals](platform.md#platform-approvals) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_role_assignments_updated`
- PK: `(id)`
- Index: `iam.role_assignments (tenant_id, user_id)` · `iam.role_assignments (tenant_id, group_id)` · `iam.role_assignments (tenant_id, role_id)` · `iam.role_assignments (tenant_id, legal_entity_id)` · `iam.role_assignments (tenant_id, org_unit_id)` · `iam.role_assignments (tenant_id, granted_by)` · `iam.role_assignments (tenant_id, approval_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.access_review_items.role_assignment_id`

<a id="iam-api-clients"></a>
## iam.api_clients

API client / service account (OAuth2 client credentials)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `keycloak_client_id` | `varchar(120)` | ✓ |  | UQ |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](#iam-users) |  |
| `system_asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `scopes` | `text[]` | ✓ |  |  |  |
| `ip_allowlist` | `cidr[]` | ✓ | '{}' |  |  |
| `rate_limit_per_min` | `int` | ✓ | 600 |  |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `disabled`, `revoked` |
| `secret_rotated_at` | `timestamptz` |  |  |  |  |
| `expires_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_api_clients_updated`
- PK: `(id)`
- Unique: `uq_api_clients_keycloak_client_id UNIQUE (tenant_id, keycloak_client_id)`
- Index: `iam.api_clients (tenant_id, owner_user_id)` · `iam.api_clients (tenant_id, system_asset_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.webhook_subscriptions.api_client_id`, `consent.reconcile_runs.target_api_client_id`, `consent.downstream_syncs.target_api_client_id`

<a id="iam-guest-tokens"></a>
## iam.guest_tokens

magic link ของผู้ใช้ภายนอก (คู่ค้า ผู้ประมวลผล ผู้ร่วมประเมิน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `purpose` | `text` | ✓ |  |  | ค่า: `vendor_assessment`, `assessment_section`, `breach_report`, `dsar_subtask`, `agreement_review`, `remediation`, `deletion_proof` |
| `entity_type` | `varchar(60)` | ✓ |  |  |  |
| `entity_id` | `uuid` | ✓ |  |  |  |
| `email` | `citext` | ✓ |  |  |  |
| `token_hash` | `char(64)` | ✓ |  | UQ |  |
| `otp_required` | `boolean` | ✓ | true |  |  |
| `expires_at` | `timestamptz` | ✓ |  |  |  |
| `revoked_at` | `timestamptz` |  |  |  |  |
| `last_used_at` | `timestamptz` |  |  |  |  |
| `issued_by` | `uuid` |  |  | FK → [iam.users](#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_guest_tokens_updated`
- PK: `(id)`
- Unique: `uq_guest_tokens_token_hash UNIQUE (tenant_id, token_hash)`
- Index: `iam.guest_tokens (tenant_id, issued_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `assess.sections.guest_token_id`, `dsar.subtasks.guest_token_id`, `vendor.vendor_assessments.guest_token_id`, `vendor.remediation_items.guest_token_id`, `agreement.return_confirmations.guest_token_id`

<a id="iam-idp-configs"></a>
## iam.idp_configs

การตั้งค่า SSO / IdP ต่อ tenant (สะท้อนค่าใน Keycloak)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `idp_type` | `text` | ✓ |  |  | ค่า: `oidc`, `saml`, `ldap`, `thaid`, `google` |
| `alias` | `varchar(60)` | ✓ |  |  |  |
| `display_name` | `text` | ✓ |  |  |  |
| `keycloak_alias` | `varchar(60)` | ✓ |  |  |  |
| `group_mappings` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `jit_enabled` | `boolean` | ✓ | true |  |  |
| `enforced` | `boolean` | ✓ | false |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `active`, `disabled` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_idp_configs_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-security-policies"></a>
## iam.security_policies

นโยบายรหัสผ่าน MFA session ต่อ tenant

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `tenant_id` | `uuid` | ✓ |  | PK · FK → [platform.tenants](platform.md#platform-tenants) |  |
| `password_policy` | `jsonb` | ✓ |  |  |  |
| `lockout_policy` | `jsonb` | ✓ |  |  |  |
| `mfa_role_codes` | `text[]` | ✓ | '{}' |  |  |
| `session_idle_min` | `int` | ✓ | 30 |  |  |
| `session_max_hours` | `int` | ✓ | 12 |  |  |
| `admin_ip_allowlist` | `cidr[]` | ✓ | '{}' |  |  |
| `access_review_months` | `int` | ✓ | 6 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_security_policies_updated`
- PK: `(tenant_id)`
- RLS: tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation`

<a id="iam-security-events"></a>
## iam.security_events

เหตุการณ์ความปลอดภัยของบัญชี (login, lockout, อุปกรณ์ใหม่)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `occurred_at` | `timestamptz` | ✓ | now() | PK |  |
| `user_id` | `uuid` |  |  | FK → [iam.users](#iam-users) |  |
| `event_type` | `text` | ✓ |  |  | ค่า: `login`, `login_failed`, `logout`, `lockout`, `mfa_enrolled`, `mfa_reset`, `new_device`, `breakglass`, `session_revoked` |
| `ip` | `inet` |  |  |  |  |
| `country` | `char(2)` |  |  |  |  |
| `user_agent` | `text` |  |  |  |  |
| `details` | `jsonb` |  |  |  |  |

- PK: `(id, occurred_at)` — รวมคอลัมน์ partition
- Index: `iam.security_events (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-access-reviews"></a>
## iam.access_reviews

รอบทบทวนสิทธิ์

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `scope` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `open`, `closed` |
| `due_at` | `timestamptz` | ✓ |  |  |  |
| `auto_revoke` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_access_reviews_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.access_review_items.review_id`

<a id="iam-access-review-items"></a>
## iam.access_review_items

รายการที่ต้องยืนยันในรอบทบทวน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `review_id` | `uuid` | ✓ |  | FK → [iam.access_reviews](#iam-access-reviews) |  |
| `role_assignment_id` | `uuid` | ✓ |  | FK → [iam.role_assignments](#iam-role-assignments) |  |
| `reviewer_user_id` | `uuid` | ✓ |  | FK → [iam.users](#iam-users) |  |
| `decision` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `keep`, `revoke` |
| `decided_at` | `timestamptz` |  |  |  |  |
| `comment` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_access_review_items_updated`
- PK: `(id)`
- Index: `iam.access_review_items (tenant_id, review_id)` · `iam.access_review_items (tenant_id, role_assignment_id)` · `iam.access_review_items (tenant_id, reviewer_user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-breakglass-requests"></a>
## iam.breakglass_requests

การขอใช้สิทธิ์ฉุกเฉิน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `requested_by` | `uuid` | ✓ |  | FK → [iam.users](#iam-users) |  |
| `reason` | `text` | ✓ |  |  |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](#iam-users) |  |
| `starts_at` | `timestamptz` |  |  |  |  |
| `ends_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'requested' |  | ค่า: `requested`, `active`, `expired`, `rejected` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_breakglass_requests_updated`
- PK: `(id)`
- Index: `iam.breakglass_requests (tenant_id, requested_by)` · `iam.breakglass_requests (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-delegations"></a>
## iam.delegations

การมอบอำนาจ / สิทธิ์ชั่วคราว

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `from_user_id` | `uuid` | ✓ |  | FK → [iam.users](#iam-users) |  |
| `to_user_id` | `uuid` | ✓ |  | FK → [iam.users](#iam-users) |  |
| `role_id` | `uuid` |  |  | FK → [iam.roles](#iam-roles) |  |
| `starts_at` | `timestamptz` | ✓ |  |  |  |
| `ends_at` | `timestamptz` | ✓ |  |  |  |
| `reason` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_delegations_updated`
- PK: `(id)`
- Index: `iam.delegations (tenant_id, from_user_id)` · `iam.delegations (tenant_id, to_user_id)` · `iam.delegations (tenant_id, role_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-field-masking-rules"></a>
## iam.field_masking_rules

กฎการปกปิดข้อมูลบนหน้าจอ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `entity_type` | `varchar(60)` | ✓ |  |  |  |
| `field_name` | `varchar(60)` | ✓ |  |  |  |
| `mask_pattern` | `varchar(60)` | ✓ |  |  |  |
| `unmask_permission` | `varchar(80)` | ✓ |  | FK → [iam.permissions](#iam-permissions) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_field_masking_rules_updated`
- PK: `(id)`
- Index: `iam.field_masking_rules (unmask_permission)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`

<a id="iam-unmask-logs"></a>
## iam.unmask_logs

การเปิดดูข้อมูลที่ปกปิด (ต้องระบุเหตุผล)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `user_id` | `uuid` | ✓ |  | FK → [iam.users](#iam-users) |  |
| `entity_type` | `varchar(60)` | ✓ |  |  |  |
| `entity_id` | `uuid` | ✓ |  |  |  |
| `field_name` | `varchar(60)` | ✓ |  |  |  |
| `reason` | `text` | ✓ |  |  |  |
| `occurred_at` | `timestamptz` | ✓ | now() |  |  |

- PK: `(id)`
- Index: `iam.unmask_logs (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="iam-subject-verifications"></a>
## iam.subject_verifications

การยืนยันตัวตนของเจ้าของข้อมูล (OTP / magic link / IdP)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](consent.md#consent-data-subjects) |  |
| `identifier_blind_index` | `bytea` | ✓ |  | IX | HMAC-SHA256 ของค่าที่ normalize แล้ว (ค้นหาแบบตรงตัว) |
| `purpose` | `text` | ✓ |  |  | ค่า: `preference_center`, `consent`, `dsar`, `double_opt_in`, `guardian` |
| `method` | `text` | ✓ |  |  | ค่า: `otp_sms`, `otp_email`, `magic_link`, `idp`, `thaid` |
| `otp_hash` | `char(64)` |  |  |  |  |
| `attempts` | `int` | ✓ | 0 |  |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `verified`, `failed`, `expired` |
| `assurance_level` | `smallint` | ✓ | 1 |  |  |
| `expires_at` | `timestamptz` | ✓ |  |  |  |
| `verified_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_subject_verifications_updated`
- PK: `(id)`
- Index: `iam.subject_verifications (tenant_id, subject_id)` · `iam.subject_verifications (tenant_id, identifier_blind_index)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_receipts.verification_id`, `dsar.verifications.subject_verification_id`
