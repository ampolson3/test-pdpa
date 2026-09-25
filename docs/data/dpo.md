# schema `dpo`

> งานของ DPO: การแต่งตั้ง งาน คำปรึกษา คลังความรู้  
> migration: `backend/db/migrations/00016_dpo.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DPO](../modules/DPO.md) (`backend/internal/dpo`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-14

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [appointments](#dpo-appointments) | การแต่งตั้ง DPO และการแจ้ง สคส. | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [requirement_checks](#dpo-requirement-checks) | ผลประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41) | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [independence_declarations](#dpo-independence-declarations) | คำรับรองความเป็นอิสระ / ผลประโยชน์ทับซ้อน | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [tasks](#dpo-tasks) | งาน / ticket 5 สถานะ (สร้างอัตโนมัติจาก gap / DPIA / audit) | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [advisories](#dpo-advisories) | คำปรึกษาจากหน่วยงานถึง DPO | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [kb_articles](#dpo-kb-articles) | คลังเอกสารกฎหมายและ FAQ | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-14 |  |
| [calendar_events](#dpo-calendar-events) | ปฏิทินงาน compliance (กิจกรรมที่ไม่ได้มาจากโมดูลอื่น) | tenant · RLS `tenant_isolation` |  | ERD-14 |  |
| [report_schedules](#dpo-report-schedules) | ตั้งเวลาส่งรายงาน DPO / ผู้บริหาร | tenant · RLS `tenant_isolation` |  | ERD-14 |  |

<a id="dpo-appointments"></a>
## dpo.appointments

การแต่งตั้ง DPO และการแจ้ง สคส.

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `dpo_type` | `text` | ✓ |  |  | ค่า: `internal`, `external`, `group` |
| `user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `external_name` | `text` |  |  |  |  |
| `external_company` | `text` |  |  |  |  |
| `contact_email` | `citext` | ✓ |  |  |  |
| `contact_phone` | `varchar(30)` |  |  |  |  |
| `appointed_at` | `date` | ✓ |  |  |  |
| `appointment_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `pdpc_notified_at` | `date` |  |  |  |  |
| `pdpc_evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `ended_at` | `date` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_appointments_updated`
- PK: `(id)`
- Index: `dpo.appointments (tenant_id, legal_entity_id)` · `dpo.appointments (tenant_id, user_id)` · `dpo.appointments (tenant_id, appointment_file_id)` · `dpo.appointments (tenant_id, pdpc_evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `dpo.independence_declarations.appointment_id`

<a id="dpo-requirement-checks"></a>
## dpo.requirement_checks

ผลประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](assess.md#assess-assessments) |  |
| `result` | `text` | ✓ |  |  | ค่า: `required`, `not_required`, `recommended` |
| `basis` | `text` | ✓ |  |  |  |
| `assessed_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_requirement_checks_updated`
- PK: `(id)`
- Index: `dpo.requirement_checks (tenant_id, legal_entity_id)` · `dpo.requirement_checks (tenant_id, assessment_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dpo-independence-declarations"></a>
## dpo.independence_declarations

คำรับรองความเป็นอิสระ / ผลประโยชน์ทับซ้อน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `appointment_id` | `uuid` | ✓ |  | FK → [dpo.appointments](#dpo-appointments) |  |
| `year` | `smallint` | ✓ |  |  |  |
| `other_duties` | `text` |  |  |  |  |
| `has_conflict` | `boolean` | ✓ | false |  |  |
| `signed_at` | `timestamptz` |  |  |  |  |
| `file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_independence_declarations_updated`
- PK: `(id)`
- Index: `dpo.independence_declarations (tenant_id, appointment_id)` · `dpo.independence_declarations (tenant_id, file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dpo-tasks"></a>
## dpo.tasks

งาน / ticket 5 สถานะ (สร้างอัตโนมัติจาก gap / DPIA / audit)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `task_no` | `varchar(30)` | ✓ |  | UQ |  |
| `title` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `source_type` | `text` | ✓ |  |  | ค่า: `ropa_gap`, `dpia`, `audit`, `breach`, `risk`, `vendor`, `agreement`, `manual` |
| `source_id` | `uuid` |  |  |  |  |
| `status` | `text` | ✓ | 'created' |  | ค่า: `created`, `assigned`, `in_review`, `done`, `closed` |
| `priority` | `text` | ✓ | 'medium' |  | ค่า: `low`, `medium`, `high`, `urgent` |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `reviewer_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `due_at` | `date` |  |  | IX |  |
| `completed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_tasks_updated`
- PK: `(id)`
- Unique: `uq_tasks_task_no UNIQUE (tenant_id, task_no)`
- Index: `dpo.tasks (tenant_id, assignee_user_id)` · `dpo.tasks (tenant_id, reviewer_user_id)` · `dpo.tasks (tenant_id, org_unit_id)` · `dpo.tasks (tenant_id, due_at)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `risk.risk_controls.task_id`, `risk.gap_findings.task_id`, `agreement.obligations.task_id`, `gov.audit_findings.task_id`

<a id="dpo-advisories"></a>
## dpo.advisories

คำปรึกษาจากหน่วยงานถึง DPO

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `advisory_no` | `varchar(30)` | ✓ |  | UQ |  |
| `org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `requester_user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `subject` | `text` | ✓ |  |  |  |
| `question` | `text` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `answered`, `closed` |
| `answer` | `text` |  |  |  |  |
| `answered_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `answered_at` | `timestamptz` |  |  |  |  |
| `kb_article_id` | `uuid` |  |  | FK → [dpo.kb_articles](#dpo-kb-articles) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_advisories_updated`
- PK: `(id)`
- Unique: `uq_advisories_advisory_no UNIQUE (tenant_id, advisory_no)`
- Index: `dpo.advisories (tenant_id, org_unit_id)` · `dpo.advisories (tenant_id, requester_user_id)` · `dpo.advisories (tenant_id, answered_by)` · `dpo.advisories (tenant_id, kb_article_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dpo-kb-articles"></a>
## dpo.kb_articles

คลังเอกสารกฎหมายและ FAQ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `category` | `varchar(40)` | ✓ |  |  |  |
| `title` | `text` | ✓ |  |  |  |
| `body` | `jsonb` | ✓ |  |  |  |
| `language` | `varchar(5)` | ✓ | 'th' |  |  |
| `tags` | `text[]` | ✓ | '{}' |  |  |
| `source` | `text` | ✓ |  |  | ค่า: `law`, `pdpc`, `policy`, `faq`, `internal` |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `archived` |
| `version_no` | `int` | ✓ | 1 |  |  |
| `published_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_kb_articles_updated`
- PK: `(id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `dpo.advisories.kb_article_id`, `gov.kb_chunks.kb_article_id`

<a id="dpo-calendar-events"></a>
## dpo.calendar_events

ปฏิทินงาน compliance (กิจกรรมที่ไม่ได้มาจากโมดูลอื่น)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `title` | `text` | ✓ |  |  |  |
| `event_type` | `varchar(40)` | ✓ |  |  |  |
| `due_at` | `date` | ✓ |  |  |  |
| `recurrence` | `varchar(60)` |  |  |  |  |
| `source_type` | `varchar(40)` |  |  |  |  |
| `source_id` | `uuid` |  |  |  |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_calendar_events_updated`
- PK: `(id)`
- Index: `dpo.calendar_events (tenant_id, owner_user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dpo-report-schedules"></a>
## dpo.report_schedules

ตั้งเวลาส่งรายงาน DPO / ผู้บริหาร

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `report_type` | `varchar(40)` | ✓ |  |  |  |
| `recipients` | `uuid[]` | ✓ |  |  |  |
| `cron` | `varchar(40)` | ✓ |  |  |  |
| `format` | `text` | ✓ | 'pdf' |  | ค่า: `pdf`, `xlsx` |
| `last_run_at` | `timestamptz` |  |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_report_schedules_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
