# schema `platform`

> บริการกลางของแพลตฟอร์ม: tenant, workflow, form, เอกสาร, ไฟล์, แจ้งเตือน, event, audit  
> migration: `backend/db/migrations/00002_platform.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [PLT](../modules/PLT.md) (`backend/internal/platform/<service>`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-01, ERD-02

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [tenants](#platform-tenants) | Tenant (องค์กรลูกค้า) | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-01 |  |
| [tenant_modules](#platform-tenant-modules) | โมดูลที่เปิดใช้ต่อ tenant (license) | tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation` |  | ERD-01 |  |
| [public_keys](#platform-public-keys) | public key สำหรับ /public/v1 และ portal: หา tenant ก่อนเปิด transaction (ทุกคนอ่านได้ · เขียนได้เฉพาะ tenant เจ้าของ) | lookup สาธารณะ · RLS `public_read` (อ่านได้ก่อนรู้ tenant) + `tenant_write` |  | ERD-01 |  |
| [workflow_definitions](#platform-workflow-definitions) | นิยาม workflow (state machine) ต่อประเภทงาน | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-01 |  |
| [workflow_instances](#platform-workflow-instances) | workflow ที่กำลังทำงานของแต่ละ record | tenant · RLS `tenant_isolation` |  | ERD-01 |  |
| [workflow_tasks](#platform-workflow-tasks) | งานในแต่ละขั้นของ workflow | tenant · RLS `tenant_isolation` |  | ERD-01 |  |
| [sla_timers](#platform-sla-timers) | ตัวนับเวลา SLA (วันปฏิทิน / วันทำการ / ชั่วโมง) | tenant · RLS `tenant_isolation` |  | ERD-01 | BP-06, SEQ-05 |
| [form_definitions](#platform-form-definitions) | ฟอร์ม / แบบประเมิน (form engine) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-01 |  |
| [form_versions](#platform-form-versions) | เวอร์ชันของฟอร์ม (schema JSON) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-01 |  |
| [form_submissions](#platform-form-submissions) | คำตอบของฟอร์ม | tenant · RLS `tenant_isolation` |  | ERD-01 |  |
| [record_versions](#platform-record-versions) | snapshot และ diff ของ record ที่มีเวอร์ชัน | tenant · RLS `tenant_isolation` |  | ERD-01 | BP-05, SEQ-02 |
| [approvals](#platform-approvals) | ขั้นตอนอนุมัติ (maker-checker / หลายระดับ) | tenant · RLS `tenant_isolation` |  | ERD-01 | BP-05, BP-12 |
| [comments](#platform-comments) | ความเห็น / @mention ต่อ record | tenant · RLS `tenant_isolation` |  | ERD-01 |  |
| [files](#platform-files) | ไฟล์ใน object storage (เข้ารหัส + สแกนไวรัส) | tenant · RLS `tenant_isolation` |  | ERD-01 | SEQ-06, SEQ-07 |
| [documents](#platform-documents) | เอกสารที่สร้างจาก composer (ประกาศ สัญญา หนังสือ แบบแจ้ง) | tenant · RLS `tenant_isolation` |  | ERD-02 | BP-10, SEQ-07 |
| [document_versions](#platform-document-versions) | เวอร์ชันของเอกสาร (ProseMirror JSON + ไฟล์ที่ render) | tenant · RLS `tenant_isolation` |  | ERD-02 | BP-10, SEQ-07 |
| [templates](#platform-templates) | template เอกสาร / ข้อความ (กลางและของ tenant) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-02 | BP-10 |
| [clause_library](#platform-clause-library) | คลังข้อความสัญญา / clause มาตรฐาน | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-02 | BP-10 |
| [notification_templates](#platform-notification-templates) | template อีเมล / SMS / LINE / in-app | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-02 |  |
| [notifications](#platform-notifications) | ข้อความที่ส่งและสถานะการส่ง | tenant · RLS `tenant_isolation` |  | ERD-02 |  |
| [outbox_events](#platform-outbox-events) | transactional outbox ของ domain event | tenant · RLS `tenant_isolation` |  | ERD-02 | BP-01, SEQ-02, SEQ-04 |
| [webhook_subscriptions](#platform-webhook-subscriptions) | webhook ที่ระบบปลายทางลงทะเบียน | tenant · RLS `tenant_isolation` |  | ERD-02 |  |
| [webhook_deliveries](#platform-webhook-deliveries) | การส่ง webhook แต่ละครั้ง (retry / dead-letter) | tenant · RLS `tenant_isolation` |  | ERD-02 | SEQ-04 |
| [audit_log](#platform-audit-log) | audit log แบบ append-only + hash chain (partition รายเดือน) | tenant · RLS `tenant_isolation` | ⟨P⟩ occurred_at | ERD-02 | BP-11, SEQ-02 |
| [import_jobs](#platform-import-jobs) | งานนำเข้า Excel / CSV | tenant · RLS `tenant_isolation` |  | ERD-02 |  |
| [export_jobs](#platform-export-jobs) | งานส่งออกแบบ async (ศูนย์ดาวน์โหลด) | tenant · RLS `tenant_isolation` |  | ERD-02 |  |
| [connectors](#platform-connectors) | connector ไปยังระบบของลูกค้า (DB / REST / SaaS) | tenant · RLS `tenant_isolation` |  | ERD-02 |  |
| [ai_requests](#platform-ai-requests) | log การเรียก AI (หลังปกปิด PII) | tenant · RLS `tenant_isolation` |  | ERD-02 |  |

<a id="platform-tenants"></a>
## platform.tenants

Tenant (องค์กรลูกค้า)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `code` | `varchar(40)` | ✓ |  | UQ |  |
| `name` | `text` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `trial`, `active`, `suspended`, `terminated` |
| `plan_code` | `varchar(40)` | ✓ |  |  |  |
| `deployment` | `text` | ✓ | 'saas_shared' |  | ค่า: `saas_shared`, `saas_dedicated`, `on_prem` |
| `data_region` | `varchar(20)` | ✓ | 'TH' |  |  |
| `keycloak_org_id` | `varchar(100)` |  |  | UQ |  |
| `primary_domain` | `varchar(255)` |  |  | UQ |  |
| `settings` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `suspended_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_tenants_updated`
- PK: `(id)`
- Unique: `uq_tenants_code UNIQUE (code)` · `uq_tenants_keycloak_org_id UNIQUE (keycloak_org_id)` · `uq_tenants_primary_domain UNIQUE (primary_domain)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `platform.tenant_modules.tenant_id`, `platform.public_keys.tenant_id`, `iam.security_policies.tenant_id`, `org.org_settings.tenant_id`

<a id="platform-tenant-modules"></a>
## platform.tenant_modules

โมดูลที่เปิดใช้ต่อ tenant (license)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `tenant_id` | `uuid` | ✓ |  | PK · FK → [platform.tenants](#platform-tenants) |  |
| `module_code` | `varchar(20)` | ✓ |  | PK |  |
| `enabled` | `boolean` | ✓ | true |  |  |
| `license_expires_at` | `date` |  |  |  |  |
| `quota` | `jsonb` | ✓ | '{}'::jsonb |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_tenant_modules_updated`
- PK: `(tenant_id, module_code)`
- RLS: tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation`

<a id="platform-public-keys"></a>
## platform.public_keys

public key สำหรับ /public/v1 และ portal: หา tenant ก่อนเปิด transaction (ทุกคนอ่านได้ · เขียนได้เฉพาะ tenant เจ้าของ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `key` | `varchar(64)` | ✓ |  | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) |  |
| `entity_type` | `text` | ✓ |  |  | ค่า: `collection_point`, `cookie_domain`, `portal`, `notice`, `dsar_form`, `breach_form` |
| `entity_id` | `uuid` | ✓ |  | IX |  |
| `allowed_origins` | `text[]` | ✓ | '{}' |  |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `revoked` |
| `created_at` | `timestamptz` | ✓ | now() |  |  |
| `revoked_at` | `timestamptz` |  |  |  |  |

- PK: `(key)`
- Index: `platform.public_keys (tenant_id)` · `platform.public_keys (entity_id)`
- RLS: lookup สาธารณะ · RLS `public_read` (อ่านได้ก่อนรู้ tenant) + `tenant_write`

<a id="platform-workflow-definitions"></a>
## platform.workflow_definitions

นิยาม workflow (state machine) ต่อประเภทงาน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `entity_type` | `varchar(60)` | ✓ |  |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `definition` | `jsonb` | ✓ |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_workflow_definitions_updated`
- PK: `(id)`
- Unique: `uq_workflow_definitions_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, code, version_no)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `platform.workflow_instances.definition_id`, `dsar.request_types.workflow_definition_id`

<a id="platform-workflow-instances"></a>
## platform.workflow_instances

workflow ที่กำลังทำงานของแต่ละ record

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `definition_id` | `uuid` | ✓ |  | FK → [platform.workflow_definitions](#platform-workflow-definitions) |  |
| `entity_type` | `varchar(60)` | ✓ |  | IX |  |
| `entity_id` | `uuid` | ✓ |  | IX |  |
| `current_state` | `varchar(60)` | ✓ |  |  |  |
| `started_at` | `timestamptz` | ✓ | now() |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `sla_status` | `text` | ✓ | 'on_track' |  | ค่า: `on_track`, `at_risk`, `overdue`, `paused`, `done` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_workflow_instances_updated`
- PK: `(id)`
- Index: `platform.workflow_instances (tenant_id, definition_id)` · `platform.workflow_instances (tenant_id, entity_type)` · `platform.workflow_instances (tenant_id, entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.workflow_tasks.instance_id`, `platform.sla_timers.instance_id`, `dsar.requests.workflow_instance_id`, `breach.incidents.workflow_instance_id`

<a id="platform-workflow-tasks"></a>
## platform.workflow_tasks

งานในแต่ละขั้นของ workflow

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `instance_id` | `uuid` | ✓ |  | FK → [platform.workflow_instances](#platform-workflow-instances) |  |
| `state` | `varchar(60)` | ✓ |  |  |  |
| `title` | `text` | ✓ |  |  |  |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `assignee_group_id` | `uuid` |  |  | FK → [iam.groups](iam.md#iam-groups) |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_progress`, `done`, `cancelled` |
| `due_at` | `timestamptz` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `outcome` | `varchar(40)` |  |  |  |  |
| `comment` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_workflow_tasks_updated`
- PK: `(id)`
- Index: `platform.workflow_tasks (tenant_id, instance_id)` · `platform.workflow_tasks (tenant_id, assignee_user_id)` · `platform.workflow_tasks (tenant_id, assignee_group_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="platform-sla-timers"></a>
## platform.sla_timers

ตัวนับเวลา SLA (วันปฏิทิน / วันทำการ / ชั่วโมง)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `instance_id` | `uuid` | ✓ |  | FK → [platform.workflow_instances](#platform-workflow-instances) |  |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `mode` | `text` | ✓ |  |  | ค่า: `calendar_days`, `business_days`, `hours` |
| `calendar_id` | `uuid` |  |  | FK → [org.business_calendars](org.md#org-business-calendars) |  |
| `started_at` | `timestamptz` | ✓ |  |  |  |
| `due_at` | `timestamptz` | ✓ |  | IX |  |
| `reminders` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `escalated_at` | `timestamptz` |  |  |  |  |
| `stopped_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'running' |  | ค่า: `running`, `met`, `breached`, `stopped` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_sla_timers_updated`
- PK: `(id)`
- Index: `platform.sla_timers (tenant_id, instance_id)` · `platform.sla_timers (tenant_id, calendar_id)` · `platform.sla_timers (tenant_id, due_at)`
- RLS: tenant · RLS `tenant_isolation`

<a id="platform-form-definitions"></a>
## platform.form_definitions

ฟอร์ม / แบบประเมิน (form engine)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `form_type` | `text` | ✓ |  |  | ค่า: `consent`, `dsar`, `breach`, `assessment`, `questionnaire`, `intake`, `quiz` |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |
| `current_version_id` | `uuid` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_form_definitions_updated`
- PK: `(id)`
- Unique: `uq_form_definitions_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `platform.form_versions.form_id`, `consent.collection_points.form_id`, `assess.templates.form_id`, `gov.courses.quiz_form_id`

<a id="platform-form-versions"></a>
## platform.form_versions

เวอร์ชันของฟอร์ม (schema JSON)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `form_id` | `uuid` | ✓ |  | FK → [platform.form_definitions](#platform-form-definitions) |  |
| `version_no` | `int` | ✓ |  |  |  |
| `schema` | `jsonb` | ✓ |  |  |  |
| `scoring` | `jsonb` |  |  |  |  |
| `languages` | `text[]` | ✓ | '{th,en}' |  |  |
| `published_at` | `timestamptz` |  |  |  |  |
| `published_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_form_versions_updated`
- PK: `(id)`
- Index: `platform.form_versions (form_id)` · `platform.form_versions (published_by)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `platform.form_submissions.form_version_id`, `assess.assessments.form_version_id`

<a id="platform-form-submissions"></a>
## platform.form_submissions

คำตอบของฟอร์ม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `form_version_id` | `uuid` | ✓ |  | FK → [platform.form_versions](#platform-form-versions) |  |
| `entity_type` | `varchar(60)` |  |  | IX |  |
| `entity_id` | `uuid` |  |  | IX |  |
| `submitted_by_type` | `text` | ✓ |  |  | ค่า: `user`, `guest`, `data_subject`, `system` |
| `submitted_by` | `uuid` |  |  |  |  |
| `answers` | `jsonb` | ✓ |  |  |  |
| `score` | `numeric(8,2)` |  |  |  |  |
| `submitted_at` | `timestamptz` | ✓ | now() |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_form_submissions_updated`
- PK: `(id)`
- Index: `platform.form_submissions (tenant_id, form_version_id)` · `platform.form_submissions (tenant_id, entity_type)` · `platform.form_submissions (tenant_id, entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_receipts.form_submission_id`, `ropa.questionnaires.form_submission_id`, `ropa.sme_exemption_checks.form_submission_id`, `dsar.requests.form_submission_id`, `breach.assessments.form_submission_id`, `vendor.intakes.form_submission_id`

<a id="platform-record-versions"></a>
## platform.record_versions

snapshot และ diff ของ record ที่มีเวอร์ชัน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `entity_type` | `varchar(60)` | ✓ |  | IX |  |
| `entity_id` | `uuid` | ✓ |  | IX |  |
| `version_no` | `int` | ✓ |  |  |  |
| `snapshot` | `jsonb` | ✓ |  |  |  |
| `diff` | `jsonb` |  |  |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `in_review`, `approved`, `published`, `superseded` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_record_versions_updated`
- PK: `(id)`
- Unique: `uq_record_versions_entity_type_entity_id_version_no UNIQUE (tenant_id, entity_type, entity_id, version_no)`
- Index: `platform.record_versions (tenant_id, entity_type)` · `platform.record_versions (tenant_id, entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.approvals.record_version_id`

<a id="platform-approvals"></a>
## platform.approvals

ขั้นตอนอนุมัติ (maker-checker / หลายระดับ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `entity_type` | `varchar(60)` | ✓ |  | IX |  |
| `entity_id` | `uuid` | ✓ |  | IX |  |
| `record_version_id` | `uuid` |  |  | FK → [platform.record_versions](#platform-record-versions) |  |
| `step_no` | `int` | ✓ | 1 |  |  |
| `requested_by` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `approver_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approver_role` | `varchar(40)` |  |  |  |  |
| `decision` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `approved`, `rejected`, `returned` |
| `reason` | `text` |  |  |  |  |
| `decided_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_approvals_updated`
- PK: `(id)`
- Index: `platform.approvals (tenant_id, entity_type)` · `platform.approvals (tenant_id, entity_id)` · `platform.approvals (tenant_id, record_version_id)` · `platform.approvals (tenant_id, requested_by)` · `platform.approvals (tenant_id, approver_user_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.role_assignments.approval_id`, `risk.acceptances.approval_id`

<a id="platform-comments"></a>
## platform.comments

ความเห็น / @mention ต่อ record

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `entity_type` | `varchar(60)` | ✓ |  | IX |  |
| `entity_id` | `uuid` | ✓ |  | IX |  |
| `parent_id` | `uuid` |  |  | FK → [platform.comments](#platform-comments) |  |
| `author_type` | `text` | ✓ |  |  | ค่า: `user`, `guest` |
| `author_id` | `uuid` | ✓ |  |  |  |
| `body` | `text` | ✓ |  |  |  |
| `mentions` | `uuid[]` | ✓ | '{}' |  |  |
| `resolved_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_comments_updated`
- PK: `(id)`
- Index: `platform.comments (tenant_id, entity_type)` · `platform.comments (tenant_id, entity_id)` · `platform.comments (tenant_id, parent_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.comments.parent_id`

<a id="platform-files"></a>
## platform.files

ไฟล์ใน object storage (เข้ารหัส + สแกนไวรัส)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `bucket` | `varchar(63)` | ✓ |  |  |  |
| `object_key` | `text` | ✓ |  | UQ |  |
| `file_name` | `text` | ✓ |  |  |  |
| `mime_type` | `varchar(120)` | ✓ |  |  |  |
| `size_bytes` | `bigint` | ✓ |  |  |  |
| `sha256` | `char(64)` | ✓ |  |  |  |
| `encryption_key_id` | `varchar(120)` |  |  |  |  |
| `av_status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `clean`, `infected`, `error` |
| `entity_type` | `varchar(60)` |  |  | IX |  |
| `entity_id` | `uuid` |  |  | IX |  |
| `retention_until` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_files_updated`
- PK: `(id)`
- Unique: `uq_files_object_key UNIQUE (tenant_id, object_key)`
- PLT-09 (`backend/internal/platform/files`): `object_key` = `<tenant>/<yyyy>/<mm>/<file id>` (ไม่มีชื่อไฟล์ — ชื่ออาจเป็นข้อมูลส่วนบุคคล) · `mime_type` มาจากการ sniff เนื้อหา ไม่เชื่อค่าที่ client ส่ง · `encryption_key_id` = `sse-s3` เมื่อเปิด SSE · `av_status` เปลี่ยนตาม state machine `PLT-09` ใน `docs/states/state-machines.yaml` โดย job `files.scan` พร้อม audit (`platform.file.scan`) · infected → ลบ object เก็บแถวไว้เป็นหลักฐาน · `retention_until` = กำหนดลบไฟล์ที่ยังไม่ถูกผูกกับ record (orphan, ค่าเริ่มต้น 24 ชม., job `files.expire`) และถูกล้างเมื่อ `Attach`
- Index: `platform.files (tenant_id, entity_type)` · `platform.files (tenant_id, entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.document_versions.pdf_file_id`, `platform.document_versions.docx_file_id`, `platform.import_jobs.file_id`, `platform.import_jobs.error_file_id`, `platform.export_jobs.file_id`, `iam.users.avatar_file_id`, `org.legal_entities.logo_file_id`, `consent.consent_receipts.evidence_file_id`, `consent.reconcile_runs.report_file_id`, `cookie.scans.report_file_id`, `notice.indirect_collections.evidence_file_id`, `notice.linked_documents.file_id`, `dataflow.snapshots.image_file_id`, `dsar.agents.authority_file_id`, `dsar.verifications.masked_id_file_id`, `dsar.subtasks.evidence_file_id`, `dsar.search_results.result_file_id`, `dsar.packages.file_id`, `dsar.redactions.source_file_id`, `dsar.redactions.output_file_id`, `dsar.downstream_notices.evidence_file_id`, `breach.pdpc_notifications.evidence_file_id`, `breach.evidence.file_id`, `vendor.certificates.file_id`, `vendor.remediation_items.evidence_file_id`, `vendor.offboardings.destruction_certificate_file_id`, `agreement.annexes.file_id`, `agreement.signature_requests.signed_file_id`, `agreement.return_confirmations.certificate_file_id`, `dpo.appointments.appointment_file_id`, `dpo.appointments.pdpc_evidence_file_id`, `dpo.independence_declarations.file_id`, `gov.courses.content_file_id`, `gov.training_attempts.certificate_file_id`, `gov.audit_findings.evidence_file_id`, `gov.disposal_jobs.evidence_file_id`, `gov.regulator_letters.file_id`, `gov.masking_jobs.source_file_id`, `gov.masking_jobs.output_file_id`

<a id="platform-documents"></a>
## platform.documents

เอกสารที่สร้างจาก composer (ประกาศ สัญญา หนังสือ แบบแจ้ง)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `doc_type` | `text` | ✓ |  |  | ค่า: `notice`, `policy`, `dpa`, `dsa`, `dsar_letter`, `pdpc_form`, `breach_letter`, `report`, `other` |
| `entity_type` | `varchar(60)` |  |  | IX |  |
| `entity_id` | `uuid` |  |  | IX |  |
| `title` | `text` | ✓ |  |  |  |
| `template_id` | `uuid` |  |  | FK → [platform.templates](#platform-templates) |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `in_review`, `approved`, `published`, `archived` |
| `current_version_id` | `uuid` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_documents_updated`
- PK: `(id)`
- Index: `platform.documents (tenant_id, entity_type)` · `platform.documents (tenant_id, entity_id)` · `platform.documents (tenant_id, template_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.document_versions.document_id`, `notice.notices.document_id`, `agreement.agreements.document_id`, `gov.policies.document_id`

<a id="platform-document-versions"></a>
## platform.document_versions

เวอร์ชันของเอกสาร (ProseMirror JSON + ไฟล์ที่ render)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `document_id` | `uuid` | ✓ |  | FK → [platform.documents](#platform-documents) |  |
| `version_no` | `int` | ✓ |  |  |  |
| `content` | `jsonb` | ✓ |  |  |  |
| `languages` | `text[]` | ✓ | '{th}' |  |  |
| `pdf_file_id` | `uuid` |  |  | FK → [platform.files](#platform-files) |  |
| `docx_file_id` | `uuid` |  |  | FK → [platform.files](#platform-files) |  |
| `change_summary` | `text` |  |  |  |  |
| `effective_from` | `date` |  |  |  |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approved_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_document_versions_updated`
- PK: `(id)`
- Unique: `uq_document_versions_document_id_version_no UNIQUE (document_id, version_no)`
- Index: `platform.document_versions (tenant_id, document_id)` · `platform.document_versions (tenant_id, pdf_file_id)` · `platform.document_versions (tenant_id, docx_file_id)` · `platform.document_versions (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `notice.notice_versions.document_version_id`, `dsar.communications.document_version_id`, `breach.pdpc_notifications.document_version_id`, `agreement.downloads.document_version_id`

<a id="platform-templates"></a>
## platform.templates

template เอกสาร / ข้อความ (กลางและของ tenant)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `template_type` | `varchar(40)` | ✓ |  |  |  |
| `code` | `varchar(80)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `language` | `varchar(5)` | ✓ |  |  |  |
| `industry` | `varchar(40)` |  |  |  |  |
| `content` | `jsonb` | ✓ |  |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_templates_updated`
- PK: `(id)`
- Unique: `uq_templates_template_type_code_language_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, template_type, code, language, version_no)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `platform.documents.template_id`, `notice.wizard_templates.template_id`, `dsar.request_types.response_template_id`, `agreement.agreements.template_id`

<a id="platform-clause-library"></a>
## platform.clause_library

คลังข้อความสัญญา / clause มาตรฐาน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(80)` | ✓ |  |  |  |
| `category` | `varchar(60)` | ✓ |  |  |  |
| `title` | `text` | ✓ |  |  |  |
| `body_th` | `jsonb` | ✓ |  |  |  |
| `body_en` | `jsonb` |  |  |  |  |
| `legal_ref` | `text` |  |  |  |  |
| `applies_to` | `text[]` | ✓ | '{dpa,dsa}' |  |  |
| `is_mandatory` | `boolean` | ✓ | false |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_clause_library_updated`
- PK: `(id)`
- Unique: `uq_clause_library_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, code, version_no)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `agreement.clauses.clause_id`

<a id="platform-notification-templates"></a>
## platform.notification_templates

template อีเมล / SMS / LINE / in-app

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(80)` | ✓ |  |  |  |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms`, `line`, `in_app` |
| `language` | `varchar(5)` | ✓ |  |  |  |
| `subject` | `text` |  |  |  |  |
| `body` | `text` | ✓ |  |  |  |
| `variables` | `jsonb` | ✓ | '[]'::jsonb |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_notification_templates_updated`
- PK: `(id)`
- Unique: `uq_notification_templates_code_channel_language UNIQUE NULLS NOT DISTINCT (tenant_id, code, channel, language)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `platform.notifications.template_id`, `consent.campaigns.template_id`, `breach.subject_notifications.template_id`

<a id="platform-notifications"></a>
## platform.notifications

ข้อความที่ส่งและสถานะการส่ง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `template_id` | `uuid` |  |  | FK → [platform.notification_templates](#platform-notification-templates) |  |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms`, `line`, `in_app` |
| `recipient_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `recipient_address_enc` | `bytea` |  |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `payload` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `sent`, `delivered`, `failed`, `cancelled` |
| `attempts` | `int` | ✓ | 0 |  |  |
| `provider_message_id` | `varchar(120)` |  |  |  |  |
| `sent_at` | `timestamptz` |  |  |  |  |
| `error` | `text` |  |  |  |  |
| `entity_type` | `varchar(60)` |  |  | IX |  |
| `entity_id` | `uuid` |  |  | IX |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_notifications_updated`
- PK: `(id)`
- Index: `platform.notifications (tenant_id, template_id)` · `platform.notifications (tenant_id, recipient_user_id)` · `platform.notifications (tenant_id, entity_type)` · `platform.notifications (tenant_id, entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `dsar.communications.notification_id`

<a id="platform-outbox-events"></a>
## platform.outbox_events

transactional outbox ของ domain event

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `aggregate_type` | `varchar(60)` | ✓ |  |  |  |
| `aggregate_id` | `uuid` | ✓ |  |  |  |
| `event_type` | `varchar(80)` | ✓ |  | IX |  |
| `payload` | `jsonb` | ✓ |  |  |  |
| `occurred_at` | `timestamptz` | ✓ | now() |  |  |
| `published_at` | `timestamptz` |  |  | IX |  |
| `attempts` | `int` | ✓ | 0 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_outbox_events_updated`
- PK: `(id)`
- Index: `platform.outbox_events (tenant_id, event_type)` · `platform.outbox_events (tenant_id, published_at)` · `ix_platform_outbox_events_unpublished (tenant_id, occurred_at, id) WHERE published_at IS NULL` (migration 00022 — dispatcher)
- `payload` = `{"version": <n>, "data": {...}}` · `data` มีฟิลด์ตาม `docs/architecture/events.yaml` ครบและไม่เกิน (ตรวจใน `events.Publisher`) · envelope `subject` = `aggregate_type` + `aggregate_id` · `attempts` นับทุกครั้งที่ dispatch (สำเร็จหรือล้มเหลว) (PLT-11)
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.webhook_deliveries.event_id`

<a id="platform-webhook-subscriptions"></a>
## platform.webhook_subscriptions

webhook ที่ระบบปลายทางลงทะเบียน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `api_client_id` | `uuid` | ✓ |  | FK → [iam.api_clients](iam.md#iam-api-clients) |  |
| `url` | `text` | ✓ |  |  |  |
| `event_types` | `text[]` | ✓ |  |  |  |
| `secret_ref` | `varchar(200)` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `paused`, `disabled` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_webhook_subscriptions_updated`
- PK: `(id)`
- Index: `platform.webhook_subscriptions (tenant_id, api_client_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.webhook_deliveries.subscription_id`

<a id="platform-webhook-deliveries"></a>
## platform.webhook_deliveries

การส่ง webhook แต่ละครั้ง (retry / dead-letter)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `subscription_id` | `uuid` | ✓ |  | FK → [platform.webhook_subscriptions](#platform-webhook-subscriptions) |  |
| `event_id` | `uuid` | ✓ |  | FK → [platform.outbox_events](#platform-outbox-events) |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `delivered`, `failed`, `dead` · state machine [ST-07](../states/ST-07.md) |
| `http_status` | `int` |  |  |  |  |
| `attempts` | `int` | ✓ | 0 |  |  |
| `next_retry_at` | `timestamptz` |  |  | IX |  |
| `last_error` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_webhook_deliveries_updated`
- PK: `(id)`
- Index: `platform.webhook_deliveries (tenant_id, subscription_id)` · `platform.webhook_deliveries (tenant_id, event_id)` · `platform.webhook_deliveries (tenant_id, next_retry_at)`
- Unique: `uq_platform_webhook_deliveries_subscription_event (tenant_id, subscription_id, event_id)` (migration 00022) — dispatch ซ้ำไม่สร้าง delivery ซ้ำ · แถวถูกสร้างเป็น `pending` โดย `outbox.dispatch` (PLT-11) และส่งจริงโดย `webhook.deliver` (PLT-15)
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.downstream_syncs.webhook_delivery_id`

<a id="platform-tenant-keys"></a>
## platform.tenant_keys

กุญแจข้อมูล (DEK / blind index key) ต่อ tenant ที่ห่อด้วย KEK ใน OpenBao Transit — ห้ามลบ (PLT-13, migration 00024)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `purpose` | `text` | ✓ |  |  | ค่า: `dek` (AES-256-GCM สำหรับ `*_enc`), `blind_index` (HMAC-SHA256) |
| `data_class` | `varchar(60)` | ✓ |  |  | ประเภทข้อมูลของ DEK เช่น `subject_identifier`, `contact` · blind index ใช้ `default` (1 key ต่อ tenant) |
| `version` | `int` | ✓ |  |  | เริ่ม 1 · `RotateDEK` เพิ่มทีละ 1 |
| `wrapped_key` | `bytea` | ✓ |  |  | key ที่ห่อด้วย KEK ของ tenant (Transit ciphertext) |
| `kek_ref` | `varchar(200)` | ✓ |  |  | KEK version ที่ห่อ เช่น `transit:tenant-<id>:v2` · เปลี่ยนเมื่อ `RotateKEK` rewrap |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `retired` (version เก่ายังใช้ถอดรหัสได้) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_tenant_keys_updated`
- Unique: `uq_tenant_keys_purpose_class_version (tenant_id, purpose, data_class, version)` · `uq_tenant_keys_active (tenant_id, purpose, data_class) WHERE status = 'active'`
- RLS: tenant · RLS `tenant_isolation` · `pdpa_app` / `pdpa_platform` ไม่มีสิทธิ์ DELETE / TRUNCATE (`deploy/db/10-grants.sql`) — ลบ key = ข้อมูลอ่านไม่ได้ถาวร (crypto-shredding ตอนเลิกใช้ tenant)
- รูปแบบค่าใน `*_enc`: `0x01 | DEK version (uint32) | nonce | AES-256-GCM ciphertext+tag` · associated data = tenant_id + data_class + ชื่อคอลัมน์ (ย้าย ciphertext ไปคอลัมน์ / tenant อื่นแล้วถอดไม่ได้) · `blind_index` = HMAC-SHA256(key, identifier_type ‖ ค่าที่ normalize) — normalize: อีเมล lower-case, เบอร์ E.164 (ค่าเริ่มต้น +66), เลขบัตร 13 หลัก, เลขไทยแปลงเป็นอารบิก

<a id="platform-audit-log"></a>
## platform.audit_log

audit log แบบ append-only + hash chain (partition รายเดือน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `bigint identity` | ✓ |  | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `occurred_at` | `timestamptz` | ✓ | now() | PK |  |
| `actor_type` | `text` | ✓ |  |  | ค่า: `user`, `api_client`, `guest`, `data_subject`, `system` |
| `actor_id` | `uuid` |  |  |  |  |
| `action` | `varchar(80)` | ✓ |  |  |  |
| `entity_type` | `varchar(60)` |  |  | IX |  |
| `entity_id` | `uuid` |  |  | IX |  |
| `before` | `jsonb` |  |  |  |  |
| `after` | `jsonb` |  |  |  |  |
| `ip` | `inet` |  |  |  |  |
| `user_agent` | `text` |  |  |  |  |
| `prev_hash` | `char(64)` |  |  |  |  |
| `hash` | `char(64)` | ✓ |  |  |  |

- PK: `(id, occurred_at)` — รวมคอลัมน์ partition
- Index: `platform.audit_log (tenant_id, entity_type)` · `platform.audit_log (tenant_id, entity_id)` · `ix_platform_audit_log_chain (tenant_id, occurred_at, id)` (migration 00023, Go migration: ON ONLY + CONCURRENTLY ต่อ partition + ATTACH)
- hash chain ต่อ tenant (PLT-12, `internal/platform/audit/service`): `hash` = SHA-256 ของทุกคอลัมน์ยกเว้น `id`/`hash` (แต่ละฟิลด์มี length prefix, tag `audit/v2`, `before`/`after` เป็น canonical JSON, `occurred_at` ระดับ microsecond UTC) · `prev_hash` = `hash` ของแถวก่อนหน้า (แถวแรก NULL) · ลำดับ chain = `(occurred_at, id)` · ผู้เขียนถือ advisory lock ต่อ tenant ถึง COMMIT และ `occurred_at` = นาฬิกา DB แต่ไม่น้อยกว่าแถวก่อน + 1µs · `Verify` ไล่ตรวจทั้ง chain · job `audit.verify` ทุกวันต่อ tenant → `alert=audit_chain_broken`
- RLS: tenant · RLS `tenant_isolation`

<a id="platform-import-jobs"></a>
## platform.import_jobs

งานนำเข้า Excel / CSV

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `import_type` | `varchar(60)` | ✓ |  |  |  |
| `file_id` | `uuid` | ✓ |  | FK → [platform.files](#platform-files) |  |
| `mapping` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `dry_run` | `boolean` | ✓ | true |  |  |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `validating`, `ready`, `importing`, `done`, `failed` |
| `total_rows` | `int` |  |  |  |  |
| `success_rows` | `int` |  |  |  |  |
| `error_rows` | `int` |  |  |  |  |
| `error_file_id` | `uuid` |  |  | FK → [platform.files](#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_import_jobs_updated`
- PK: `(id)`
- Index: `platform.import_jobs (tenant_id, file_id)` · `platform.import_jobs (tenant_id, error_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="platform-export-jobs"></a>
## platform.export_jobs

งานส่งออกแบบ async (ศูนย์ดาวน์โหลด)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `export_type` | `varchar(60)` | ✓ |  |  |  |
| `params` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `format` | `text` | ✓ |  |  | ค่า: `csv`, `xlsx`, `pdf`, `json` |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `running`, `done`, `failed`, `expired` |
| `file_id` | `uuid` |  |  | FK → [platform.files](#platform-files) |  |
| `requested_by` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `expires_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_export_jobs_updated`
- PK: `(id)`
- Index: `platform.export_jobs (tenant_id, file_id)` · `platform.export_jobs (tenant_id, requested_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="platform-connectors"></a>
## platform.connectors

connector ไปยังระบบของลูกค้า (DB / REST / SaaS)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `connector_type` | `varchar(40)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `config` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `secret_ref` | `varchar(200)` |  |  |  |  |
| `capabilities` | `text[]` | ✓ | '{}' |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `active`, `error`, `disabled` |
| `last_tested_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_connectors_updated`
- PK: `(id)`
- Index: `platform.connectors (tenant_id, asset_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.reconcile_runs.connector_id`, `dataflow.discovery_scans.connector_id`, `dsar.search_results.connector_id`

<a id="platform-ai-requests"></a>
## platform.ai_requests

log การเรียก AI (หลังปกปิด PII)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](#platform-tenants) | RLS |
| `feature_code` | `varchar(40)` | ✓ |  |  |  |
| `provider` | `varchar(40)` | ✓ |  |  |  |
| `model` | `varchar(80)` | ✓ |  |  |  |
| `prompt_template_code` | `varchar(80)` | ✓ |  |  |  |
| `redacted` | `boolean` | ✓ | true |  |  |
| `tokens_in` | `int` |  |  |  |  |
| `tokens_out` | `int` |  |  |  |  |
| `cost` | `numeric(12,4)` |  |  |  |  |
| `status` | `text` | ✓ |  |  | ค่า: `ok`, `error`, `blocked` |
| `entity_type` | `varchar(60)` |  |  |  |  |
| `entity_id` | `uuid` |  |  |  |  |
| `confirmed_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `confirmed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_ai_requests_updated`
- PK: `(id)`
- Index: `platform.ai_requests (tenant_id, confirmed_by)`
- RLS: tenant · RLS `tenant_isolation`
