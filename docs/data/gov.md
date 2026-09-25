# schema `gov`

> อบรม นโยบาย การตรวจประเมิน retention การติดต่อ สคส. และทะเบียน AI  
> migration: `backend/db/migrations/00017_gov.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DPX](../modules/DPX.md) (`backend/internal/gov`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-14, ERD-15

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [courses](#gov-courses) | คอร์สอบรม PDPA | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-15 |  |
| [training_assignments](#gov-training-assignments) | การมอบหมายอบรม | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [training_attempts](#gov-training-attempts) | ผลการเรียน / สอบรายบุคคล | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [policies](#gov-policies) | นโยบาย / คู่มือภายในที่ต้องรับทราบ | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [policy_attestations](#gov-policy-attestations) | การรับทราบนโยบาย | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [audits](#gov-audits) | การตรวจประเมินความพร้อม PDPA ระดับองค์กร | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [audit_findings](#gov-audit-findings) | ข้อตรวจพบและแผนแก้ไข | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [retention_schedules](#gov-retention-schedules) | ตาราง retention ต่อระบบ (สร้างจาก RoPA) | tenant · RLS `tenant_isolation` |  | ERD-15 | BP-11 |
| [disposal_jobs](#gov-disposal-jobs) | งานลบ / ทำลาย / ทำให้ไม่ระบุตัวตน | tenant · RLS `tenant_isolation` |  | ERD-15 | BP-11 |
| [regulator_letters](#gov-regulator-letters) | ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส. | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [regulatory_updates](#gov-regulatory-updates) | ฟีดประกาศ / แนวปฏิบัติใหม่ (ทีมเนื้อหาผู้ให้บริการ) | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-15 |  |
| [regulatory_reviews](#gov-regulatory-reviews) | การประเมินผลกระทบของประกาศใหม่ต่อ tenant | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [ai_systems](#gov-ai-systems) | ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [masking_jobs](#gov-masking-jobs) | งาน mask / tokenize ข้อมูลสำหรับทดสอบ | tenant · RLS `tenant_isolation` |  | ERD-15 |  |
| [kb_chunks](#gov-kb-chunks) | ชิ้นเอกสารสำหรับ RAG ของผู้ช่วย AI (pgvector) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-14 |  |

<a id="gov-courses"></a>
## gov.courses

คอร์สอบรม PDPA

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `title` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `content_type` | `text` | ✓ |  |  | ค่า: `video`, `slides`, `scorm`, `xapi` |
| `content_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `quiz_form_id` | `uuid` |  |  | FK → [platform.form_definitions](platform.md#platform-form-definitions) |  |
| `passing_score` | `smallint` | ✓ | 80 |  |  |
| `validity_months` | `smallint` | ✓ | 12 |  |  |
| `language` | `varchar(5)` | ✓ | 'th' |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_courses_updated`
- PK: `(id)`
- Index: `gov.courses (content_file_id)` · `gov.courses (quiz_form_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `gov.training_assignments.course_id`, `gov.training_attempts.course_id`

<a id="gov-training-assignments"></a>
## gov.training_assignments

การมอบหมายอบรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `course_id` | `uuid` | ✓ |  | FK → [gov.courses](#gov-courses) |  |
| `target_type` | `text` | ✓ |  |  | ค่า: `all`, `org_unit`, `group`, `user` |
| `target_id` | `uuid` |  |  |  |  |
| `due_at` | `date` | ✓ |  |  |  |
| `assigned_by` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_training_assignments_updated`
- PK: `(id)`
- Index: `gov.training_assignments (tenant_id, course_id)` · `gov.training_assignments (tenant_id, assigned_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `gov.training_attempts.assignment_id`

<a id="gov-training-attempts"></a>
## gov.training_attempts

ผลการเรียน / สอบรายบุคคล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `course_id` | `uuid` | ✓ |  | FK → [gov.courses](#gov-courses) |  |
| `assignment_id` | `uuid` |  |  | FK → [gov.training_assignments](#gov-training-assignments) |  |
| `user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `started_at` | `timestamptz` | ✓ |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `score` | `smallint` |  |  |  |  |
| `passed` | `boolean` |  |  |  |  |
| `certificate_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_training_attempts_updated`
- PK: `(id)`
- Index: `gov.training_attempts (tenant_id, course_id)` · `gov.training_attempts (tenant_id, assignment_id)` · `gov.training_attempts (tenant_id, user_id)` · `gov.training_attempts (tenant_id, certificate_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-policies"></a>
## gov.policies

นโยบาย / คู่มือภายในที่ต้องรับทราบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `title` | `text` | ✓ |  |  |  |
| `document_id` | `uuid` | ✓ |  | FK → [platform.documents](platform.md#platform-documents) |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `requires_attestation` | `boolean` | ✓ | true |  |  |
| `published_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_policies_updated`
- PK: `(id)`
- Unique: `uq_policies_document_id_version_no UNIQUE (tenant_id, document_id, version_no)`
- Index: `gov.policies (tenant_id, document_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `gov.policy_attestations.policy_id`

<a id="gov-policy-attestations"></a>
## gov.policy_attestations

การรับทราบนโยบาย

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `policy_id` | `uuid` | ✓ |  | FK → [gov.policies](#gov-policies) |  |
| `policy_version_no` | `int` | ✓ |  |  |  |
| `user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `attested_at` | `timestamptz` | ✓ | now() |  |  |
| `ip` | `inet` |  |  |  |  |

- PK: `(id)`
- Index: `gov.policy_attestations (tenant_id, policy_id)` · `gov.policy_attestations (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-audits"></a>
## gov.audits

การตรวจประเมินความพร้อม PDPA ระดับองค์กร

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `audit_type` | `text` | ✓ |  |  | ค่า: `maturity`, `pdpa_compliance`, `security`, `related_law` |
| `legal_entity_id` | `uuid` |  |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](assess.md#assess-assessments) |  |
| `period` | `varchar(20)` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'planned' |  | ค่า: `planned`, `in_progress`, `completed` |
| `overall_score` | `numeric(5,2)` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_audits_updated`
- PK: `(id)`
- Index: `gov.audits (tenant_id, legal_entity_id)` · `gov.audits (tenant_id, assessment_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `gov.audit_findings.audit_id`

<a id="gov-audit-findings"></a>
## gov.audit_findings

ข้อตรวจพบและแผนแก้ไข

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `audit_id` | `uuid` | ✓ |  | FK → [gov.audits](#gov-audits) |  |
| `title` | `text` | ✓ |  |  |  |
| `severity` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high`, `critical` |
| `requirement_ref` | `text` |  |  |  |  |
| `recommendation` | `text` |  |  |  |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `due_at` | `date` |  |  |  |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_progress`, `done`, `accepted` |
| `task_id` | `uuid` |  |  | FK → [dpo.tasks](dpo.md#dpo-tasks) |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_audit_findings_updated`
- PK: `(id)`
- Index: `gov.audit_findings (tenant_id, audit_id)` · `gov.audit_findings (tenant_id, owner_user_id)` · `gov.audit_findings (tenant_id, task_id)` · `gov.audit_findings (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-retention-schedules"></a>
## gov.retention_schedules

ตาราง retention ต่อระบบ (สร้างจาก RoPA)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `retention_rule_id` | `uuid` | ✓ |  | FK → [ropa.retention_rules](ropa.md#ropa-retention-rules) |  |
| `asset_id` | `uuid` | ✓ |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `next_due_at` | `date` | ✓ |  | IX |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `paused`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_retention_schedules_updated`
- PK: `(id)`
- Index: `gov.retention_schedules (tenant_id, retention_rule_id)` · `gov.retention_schedules (tenant_id, asset_id)` · `gov.retention_schedules (tenant_id, next_due_at)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `gov.disposal_jobs.schedule_id`

<a id="gov-disposal-jobs"></a>
## gov.disposal_jobs

งานลบ / ทำลาย / ทำให้ไม่ระบุตัวตน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `schedule_id` | `uuid` | ✓ |  | FK → [gov.retention_schedules](#gov-retention-schedules) |  |
| `due_at` | `date` | ✓ |  |  |  |
| `method` | `text` | ✓ |  |  | ค่า: `delete`, `destroy`, `anonymize`, `return` |
| `legal_hold_id` | `uuid` |  |  | FK → [dsar.legal_holds](dsar.md#dsar-legal-holds) |  |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `approved`, `in_progress`, `done`, `on_hold` |
| `record_count` | `int` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_disposal_jobs_updated`
- PK: `(id)`
- Index: `gov.disposal_jobs (tenant_id, schedule_id)` · `gov.disposal_jobs (tenant_id, legal_hold_id)` · `gov.disposal_jobs (tenant_id, assignee_user_id)` · `gov.disposal_jobs (tenant_id, approved_by)` · `gov.disposal_jobs (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-regulator-letters"></a>
## gov.regulator_letters

ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส.

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `direction` | `text` | ✓ |  |  | ค่า: `incoming`, `outgoing` |
| `letter_no` | `varchar(60)` |  |  |  |  |
| `subject` | `text` | ✓ |  |  |  |
| `received_at` | `date` |  |  |  |  |
| `due_at` | `date` |  |  |  |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `responded`, `closed` |
| `file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `incident_id` | `uuid` |  |  | FK → [breach.incidents](breach.md#breach-incidents) |  |
| `responded_at` | `date` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_regulator_letters_updated`
- PK: `(id)`
- Index: `gov.regulator_letters (tenant_id, file_id)` · `gov.regulator_letters (tenant_id, incident_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-regulatory-updates"></a>
## gov.regulatory_updates

ฟีดประกาศ / แนวปฏิบัติใหม่ (ทีมเนื้อหาผู้ให้บริการ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `title` | `text` | ✓ |  |  |  |
| `source_url` | `text` |  |  |  |  |
| `published_at` | `date` | ✓ |  |  |  |
| `summary` | `text` | ✓ |  |  |  |
| `impact_areas` | `text[]` | ✓ | '{}' |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_regulatory_updates_updated`
- PK: `(id)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `gov.regulatory_reviews.update_id`

<a id="gov-regulatory-reviews"></a>
## gov.regulatory_reviews

การประเมินผลกระทบของประกาศใหม่ต่อ tenant

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `update_id` | `uuid` | ✓ |  | FK → [gov.regulatory_updates](#gov-regulatory-updates) |  |
| `assessment_id` | `uuid` |  |  | FK → [assess.assessments](assess.md#assess-assessments) |  |
| `status` | `text` | ✓ | 'new' |  | ค่า: `new`, `reviewing`, `done`, `not_applicable` |
| `reviewed_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `reviewed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_regulatory_reviews_updated`
- PK: `(id)`
- Index: `gov.regulatory_reviews (tenant_id, update_id)` · `gov.regulatory_reviews (tenant_id, assessment_id)` · `gov.regulatory_reviews (tenant_id, reviewed_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-ai-systems"></a>
## gov.ai_systems

ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `vendor_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `purpose` | `text` | ✓ |  |  |  |
| `model_type` | `varchar(60)` |  |  |  |  |
| `data_category_ids` | `uuid[]` | ✓ | '{}' |  |  |
| `automated_decision` | `boolean` | ✓ | false |  |  |
| `risk_level` | `text` |  |  |  | ค่า: `low`, `medium`, `high`, `unacceptable` |
| `assessment_id` | `uuid` |  |  | FK → [assess.assessments](assess.md#assess-assessments) |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `planned`, `active`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_ai_systems_updated`
- PK: `(id)`
- Index: `gov.ai_systems (tenant_id, vendor_party_id)` · `gov.ai_systems (tenant_id, assessment_id)` · `gov.ai_systems (tenant_id, owner_user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-masking-jobs"></a>
## gov.masking_jobs

งาน mask / tokenize ข้อมูลสำหรับทดสอบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `source_file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `output_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `rules` | `jsonb` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `running`, `done`, `failed` |
| `requested_by` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_masking_jobs_updated`
- PK: `(id)`
- Index: `gov.masking_jobs (tenant_id, source_file_id)` · `gov.masking_jobs (tenant_id, output_file_id)` · `gov.masking_jobs (tenant_id, requested_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="gov-kb-chunks"></a>
## gov.kb_chunks

ชิ้นเอกสารสำหรับ RAG ของผู้ช่วย AI (pgvector)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `kb_article_id` | `uuid` | ✓ |  | FK → [dpo.kb_articles](dpo.md#dpo-kb-articles) |  |
| `chunk_no` | `int` | ✓ |  |  |  |
| `content` | `text` | ✓ |  |  |  |
| `embedding` | `vector(1024)` | ✓ |  |  |  |

- PK: `(id)`
- Index: `gov.kb_chunks (kb_article_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
