# schema `dsar`

> คำขอใช้สิทธิของเจ้าของข้อมูล  
> migration: `backend/db/migrations/00012_dsar.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DSAR](../modules/DSAR.md) (`backend/internal/dsar`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-11

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [request_types](#dsar-request-types) | ประเภทคำขอ (ม.19, ม.30-36, ร้องเรียน, สอบถาม) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-11 | BP-06 |
| [requests](#dsar-requests) | คำขอใช้สิทธิ | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06, SEQ-05 |
| [agents](#dsar-agents) | ผู้ยื่นแทน / ผู้ใช้อำนาจปกครอง (ม.20) | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06, SEQ-05 |
| [verifications](#dsar-verifications) | การยืนยันตัวตนของผู้ยื่น | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06, SEQ-05 |
| [subtasks](#dsar-subtasks) | งานย่อยต่อระบบ / ทีม / ผู้ประมวลผล | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [search_results](#dsar-search-results) | ผลค้นหาข้อมูลข้ามระบบ (เข้ารหัส ลบเมื่อปิดคำขอ) | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [legal_holds](#dsar-legal-holds) | ข้อมูลที่ต้องเก็บตามกฎหมายอื่น (ข้อยกเว้น ม.33) | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06, BP-11 |
| [exemption_checks](#dsar-exemption-checks) | ผลตรวจข้อยกเว้นก่อนลบ | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [packages](#dsar-packages) | แพ็กเกจข้อมูลที่ส่งคืน (ลิงก์หมดอายุ + รหัสผ่าน) | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [redactions](#dsar-redactions) | งานปกปิดข้อมูลบุคคลอื่นในเอกสาร | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [communications](#dsar-communications) | การติดต่อกับผู้ยื่น (หนังสือตอบ / ขอข้อมูลเพิ่ม) | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |
| [downstream_notices](#dsar-downstream-notices) | การแจ้งผู้รับข้อมูลให้ดำเนินการตามคำขอ | tenant · RLS `tenant_isolation` |  | ERD-11 | BP-06 |

<a id="dsar-request-types"></a>
## dsar.request_types

ประเภทคำขอ (ม.19, ม.30-36, ร้องเรียน, สอบถาม)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `text` | ✓ |  |  | ค่า: `access`, `portability`, `objection`, `erasure`, `restriction`, `rectification`, `withdraw_consent`, `complaint`, `inquiry` |
| `name_th` | `text` | ✓ |  |  |  |
| `legal_ref` | `varchar(40)` |  |  |  |  |
| `workflow_definition_id` | `uuid` |  |  | FK → [platform.workflow_definitions](platform.md#platform-workflow-definitions) |  |
| `sla_days` | `smallint` | ✓ | 30 |  |  |
| `response_template_id` | `uuid` |  |  | FK → [platform.templates](platform.md#platform-templates) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_request_types_updated`
- PK: `(id)`
- Unique: `uq_request_types_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- Index: `dsar.request_types (workflow_definition_id)` · `dsar.request_types (response_template_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `dsar.requests.request_type_id`

<a id="dsar-requests"></a>
## dsar.requests

คำขอใช้สิทธิ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_no` | `varchar(30)` | ✓ |  | UQ |  |
| `request_type_id` | `uuid` | ✓ |  | FK → [dsar.request_types](#dsar-request-types) |  |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `channel` | `text` | ✓ |  |  | ค่า: `web`, `email`, `phone`, `branch`, `letter`, `line`, `api` |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](consent.md#consent-data-subjects) |  |
| `requester_name_enc` | `bytea` | ✓ |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `requester_contact_enc` | `bytea` | ✓ |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `requester_blind_index` | `bytea` | ✓ |  | IX | HMAC-SHA256 ของค่าที่ normalize แล้ว (ค้นหาแบบตรงตัว) |
| `on_behalf` | `boolean` | ✓ | false |  |  |
| `details` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `form_submission_id` | `uuid` |  |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `workflow_instance_id` | `uuid` |  |  | FK → [platform.workflow_instances](platform.md#platform-workflow-instances) |  |
| `status` | `text` | ✓ | 'received' |  | ค่า: `received`, `verifying`, `in_review`, `in_progress`, `awaiting_info`, `completed`, `rejected`, `withdrawn` · state machine [ST-02](../states/ST-02.md) |
| `received_at` | `timestamptz` | ✓ |  |  |  |
| `due_at` | `timestamptz` | ✓ |  | IX |  |
| `verified_at` | `timestamptz` |  |  |  |  |
| `closed_at` | `timestamptz` |  |  |  |  |
| `outcome` | `text` |  |  |  | ค่า: `fulfilled`, `partially_fulfilled`, `rejected`, `withdrawn` |
| `rejection_reason_code` | `varchar(40)` |  |  |  |  |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_requests_updated`
- PK: `(id)`
- Unique: `uq_requests_request_no UNIQUE (tenant_id, request_no)`
- Index: `dsar.requests (tenant_id, request_type_id)` · `dsar.requests (tenant_id, legal_entity_id)` · `dsar.requests (tenant_id, subject_id)` · `dsar.requests (tenant_id, requester_blind_index)` · `dsar.requests (tenant_id, form_submission_id)` · `dsar.requests (tenant_id, workflow_instance_id)` · `dsar.requests (tenant_id, due_at)` · `dsar.requests (tenant_id, assignee_user_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `ropa.activity_rejections.dsar_request_id`, `dsar.agents.request_id`, `dsar.verifications.request_id`, `dsar.subtasks.request_id`, `dsar.search_results.request_id`, `dsar.exemption_checks.request_id`, `dsar.packages.request_id`, `dsar.redactions.request_id`, `dsar.communications.request_id`, `dsar.downstream_notices.request_id`

<a id="dsar-agents"></a>
## dsar.agents

ผู้ยื่นแทน / ผู้ใช้อำนาจปกครอง (ม.20)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `agent_name_enc` | `bytea` | ✓ |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `authority_type` | `text` | ✓ |  |  | ค่า: `power_of_attorney`, `parent`, `guardian`, `curator` |
| `authority_file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `verified_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_agents_updated`
- PK: `(id)`
- Index: `dsar.agents (tenant_id, request_id)` · `dsar.agents (tenant_id, authority_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-verifications"></a>
## dsar.verifications

การยืนยันตัวตนของผู้ยื่น

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `method` | `text` | ✓ |  |  | ค่า: `otp_sms`, `otp_email`, `id_document`, `in_person`, `idp`, `thaid` |
| `subject_verification_id` | `uuid` |  |  | FK → [iam.subject_verifications](iam.md#iam-subject-verifications) |  |
| `masked_id_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `passed`, `failed` |
| `verified_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `verified_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_verifications_updated`
- PK: `(id)`
- Index: `dsar.verifications (tenant_id, request_id)` · `dsar.verifications (tenant_id, subject_verification_id)` · `dsar.verifications (tenant_id, masked_id_file_id)` · `dsar.verifications (tenant_id, verified_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-subtasks"></a>
## dsar.subtasks

งานย่อยต่อระบบ / ทีม / ผู้ประมวลผล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `action` | `text` | ✓ |  |  | ค่า: `search`, `export`, `delete`, `rectify`, `restrict`, `stop_marketing`, `review` |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `assignee_group_id` | `uuid` |  |  | FK → [iam.groups](iam.md#iam-groups) |  |
| `assignee_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `guest_token_id` | `uuid` |  |  | FK → [iam.guest_tokens](iam.md#iam-guest-tokens) |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_progress`, `done`, `not_applicable` |
| `due_at` | `timestamptz` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_subtasks_updated`
- PK: `(id)`
- Index: `dsar.subtasks (tenant_id, request_id)` · `dsar.subtasks (tenant_id, asset_id)` · `dsar.subtasks (tenant_id, assignee_user_id)` · `dsar.subtasks (tenant_id, assignee_group_id)` · `dsar.subtasks (tenant_id, assignee_party_id)` · `dsar.subtasks (tenant_id, guest_token_id)` · `dsar.subtasks (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-search-results"></a>
## dsar.search_results

ผลค้นหาข้อมูลข้ามระบบ (เข้ารหัส ลบเมื่อปิดคำขอ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `connector_id` | `uuid` | ✓ |  | FK → [platform.connectors](platform.md#platform-connectors) |  |
| `found` | `boolean` | ✓ |  |  |  |
| `record_count` | `int` |  |  |  |  |
| `summary_enc` | `bytea` |  |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `result_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `searched_at` | `timestamptz` | ✓ |  |  |  |
| `purged_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_search_results_updated`
- PK: `(id)`
- Index: `dsar.search_results (tenant_id, request_id)` · `dsar.search_results (tenant_id, connector_id)` · `dsar.search_results (tenant_id, result_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-legal-holds"></a>
## dsar.legal_holds

ข้อมูลที่ต้องเก็บตามกฎหมายอื่น (ข้อยกเว้น ม.33)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `data_category_id` | `uuid` |  |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `law_ref` | `text` | ✓ |  |  |  |
| `hold_rule` | `jsonb` | ✓ |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_legal_holds_updated`
- PK: `(id)`
- Index: `dsar.legal_holds (tenant_id, data_category_id)` · `dsar.legal_holds (tenant_id, asset_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `dsar.exemption_checks.legal_hold_id`, `gov.disposal_jobs.legal_hold_id`

<a id="dsar-exemption-checks"></a>
## dsar.exemption_checks

ผลตรวจข้อยกเว้นก่อนลบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `legal_hold_id` | `uuid` |  |  | FK → [dsar.legal_holds](#dsar-legal-holds) |  |
| `result` | `text` | ✓ |  |  | ค่า: `can_delete`, `must_retain`, `partial` |
| `reason` | `text` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_exemption_checks_updated`
- PK: `(id)`
- Index: `dsar.exemption_checks (tenant_id, request_id)` · `dsar.exemption_checks (tenant_id, legal_hold_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-packages"></a>
## dsar.packages

แพ็กเกจข้อมูลที่ส่งคืน (ลิงก์หมดอายุ + รหัสผ่าน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `format` | `text` | ✓ |  |  | ค่า: `pdf`, `csv`, `json`, `xml`, `zip` |
| `token_hash` | `char(64)` | ✓ |  | UQ |  |
| `password_protected` | `boolean` | ✓ | true |  |  |
| `expires_at` | `timestamptz` | ✓ |  |  |  |
| `download_count` | `int` | ✓ | 0 |  |  |
| `last_downloaded_at` | `timestamptz` |  |  |  |  |
| `purged_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_packages_updated`
- PK: `(id)`
- Unique: `uq_packages_token_hash UNIQUE (tenant_id, token_hash)`
- Index: `dsar.packages (tenant_id, request_id)` · `dsar.packages (tenant_id, file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-redactions"></a>
## dsar.redactions

งานปกปิดข้อมูลบุคคลอื่นในเอกสาร

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `source_file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `output_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `regions` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `status` | `text` | ✓ | 'detecting' |  | ค่า: `detecting`, `review`, `done` |
| `reviewed_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_redactions_updated`
- PK: `(id)`
- Index: `dsar.redactions (tenant_id, request_id)` · `dsar.redactions (tenant_id, source_file_id)` · `dsar.redactions (tenant_id, output_file_id)` · `dsar.redactions (tenant_id, reviewed_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-communications"></a>
## dsar.communications

การติดต่อกับผู้ยื่น (หนังสือตอบ / ขอข้อมูลเพิ่ม)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `direction` | `text` | ✓ |  |  | ค่า: `inbound`, `outbound` |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms`, `portal`, `letter` |
| `purpose` | `text` | ✓ |  |  | ค่า: `acknowledge`, `request_info`, `result`, `rejection`, `extension_notice`, `other` |
| `document_version_id` | `uuid` |  |  | FK → [platform.document_versions](platform.md#platform-document-versions) |  |
| `notification_id` | `uuid` |  |  | FK → [platform.notifications](platform.md#platform-notifications) |  |
| `occurred_at` | `timestamptz` | ✓ | now() |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_communications_updated`
- PK: `(id)`
- Index: `dsar.communications (tenant_id, request_id)` · `dsar.communications (tenant_id, document_version_id)` · `dsar.communications (tenant_id, notification_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dsar-downstream-notices"></a>
## dsar.downstream_notices

การแจ้งผู้รับข้อมูลให้ดำเนินการตามคำขอ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `request_id` | `uuid` | ✓ |  | FK → [dsar.requests](#dsar-requests) |  |
| `party_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `method` | `text` | ✓ |  |  | ค่า: `webhook`, `connector`, `guest_link`, `email` |
| `status` | `text` | ✓ | 'sent' |  | ค่า: `sent`, `confirmed`, `failed` |
| `sent_at` | `timestamptz` |  |  |  |  |
| `confirmed_at` | `timestamptz` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_downstream_notices_updated`
- PK: `(id)`
- Index: `dsar.downstream_notices (tenant_id, request_id)` · `dsar.downstream_notices (tenant_id, party_id)` · `dsar.downstream_notices (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`
