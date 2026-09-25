# schema `assess`

> แบบประเมิน DPIA / LIA / TIA / security / maturity (assessment engine)  
> migration: `backend/db/migrations/00011_assess.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DPIA](../modules/DPIA.md) (`backend/internal/assess`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-10

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [templates](#assess-templates) | template แบบประเมิน (DPIA / LIA / TIA / AI / security / maturity ฯลฯ) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-10 | BP-08 |
| [screening_rules](#assess-screening-rules) | เกณฑ์คัดกรอง / บังคับทำ DPIA | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [assessments](#assess-assessments) | แบบประเมินแต่ละครั้ง | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-05, BP-08, BP-09 |
| [sections](#assess-sections) | ส่วนของแบบประเมินที่มอบหมายผู้ตอบ | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [answers](#assess-answers) | คำตอบรายข้อ + หลักฐาน | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [assessment_risks](#assess-assessment-risks) | ความเสี่ยงที่ระบุในแบบประเมิน | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [dpo_opinions](#assess-dpo-opinions) | ความเห็นของ DPO | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [consultations](#assess-consultations) | บันทึกการปรึกษาผู้มีส่วนได้เสีย | tenant · RLS `tenant_isolation` |  | ERD-10 |  |

<a id="assess-templates"></a>
## assess.templates

template แบบประเมิน (DPIA / LIA / TIA / AI / security / maturity ฯลฯ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `assessment_type` | `text` | ✓ |  |  | ค่า: `dpia`, `pia`, `lia`, `tia`, `ai`, `security`, `maturity`, `dpo_check`, `sme_check`, `vendor`, `inbound_dpa`, `independence` |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `form_id` | `uuid` | ✓ |  | FK → [platform.form_definitions](platform.md#platform-form-definitions) |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `legal_refs` | `text[]` | ✓ | '{}' |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_templates_updated`
- PK: `(id)`
- Unique: `uq_templates_assessment_type_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, assessment_type, code, version_no)`
- Index: `assess.templates (form_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `assess.assessments.template_id`

<a id="assess-screening-rules"></a>
## assess.screening_rules

เกณฑ์คัดกรอง / บังคับทำ DPIA

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `criteria` | `jsonb` | ✓ |  |  |  |
| `min_factors` | `smallint` | ✓ | 2 |  |  |
| `min_score` | `numeric(6,2)` |  |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_screening_rules_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="assess-assessments"></a>
## assess.assessments

แบบประเมินแต่ละครั้ง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `assessment_type` | `varchar(20)` | ✓ |  | IX |  |
| `template_id` | `uuid` | ✓ |  | FK → [assess.templates](#assess-templates) |  |
| `form_version_id` | `uuid` | ✓ |  | FK → [platform.form_versions](platform.md#platform-form-versions) |  |
| `title` | `text` | ✓ |  |  |  |
| `subject_type` | `text` | ✓ |  |  | ค่า: `activity`, `vendor`, `system`, `legal_entity`, `ai_system`, `transfer`, `project` |
| `subject_id` | `uuid` |  |  |  |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `round_no` | `smallint` | ✓ | 1 |  |  |
| `previous_id` | `uuid` |  |  | FK → [assess.assessments](#assess-assessments) |  |
| `status` | `text` | ✓ | 'screening' |  | ค่า: `screening`, `not_required`, `in_progress`, `in_review`, `approved`, `rejected`, `needs_review`, `closed` · state machine [ST-05](../states/ST-05.md) |
| `screening_result` | `text` |  |  |  | ค่า: `required`, `recommended`, `not_required` |
| `screening_reason` | `text` |  |  |  |  |
| `score` | `numeric(8,2)` |  |  |  |  |
| `risk_level` | `text` |  |  |  | ค่า: `low`, `medium`, `high`, `very_high` |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `due_at` | `date` |  |  |  |  |
| `approved_at` | `timestamptz` |  |  |  |  |
| `next_review_at` | `date` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_assessments_updated`
- PK: `(id)`
- Index: `assess.assessments (tenant_id, assessment_type)` · `assess.assessments (tenant_id, template_id)` · `assess.assessments (tenant_id, form_version_id)` · `assess.assessments (tenant_id, activity_id)` · `assess.assessments (tenant_id, previous_id)` · `assess.assessments (tenant_id, owner_user_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `ropa.activity_purposes.lia_assessment_id`, `ropa.activity_transfers.tia_assessment_id`, `ropa.activity_controls.assessment_id`, `assess.assessments.previous_id`, `assess.sections.assessment_id`, `assess.answers.assessment_id`, `assess.assessment_risks.assessment_id`, `assess.dpo_opinions.assessment_id`, `assess.consultations.assessment_id`, `vendor.vendor_assessments.assessment_id`, `dpo.requirement_checks.assessment_id`, `gov.audits.assessment_id`, `gov.regulatory_reviews.assessment_id`, `gov.ai_systems.assessment_id`

<a id="assess-sections"></a>
## assess.sections

ส่วนของแบบประเมินที่มอบหมายผู้ตอบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](#assess-assessments) |  |
| `section_code` | `varchar(60)` | ✓ |  |  |  |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `guest_token_id` | `uuid` |  |  | FK → [iam.guest_tokens](iam.md#iam-guest-tokens) |  |
| `status` | `text` | ✓ | 'not_started' |  | ค่า: `not_started`, `in_progress`, `submitted`, `needs_info` |
| `submitted_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_sections_updated`
- PK: `(id)`
- Index: `assess.sections (tenant_id, assessment_id)` · `assess.sections (tenant_id, assignee_user_id)` · `assess.sections (tenant_id, guest_token_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `assess.answers.section_id`

<a id="assess-answers"></a>
## assess.answers

คำตอบรายข้อ + หลักฐาน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](#assess-assessments) |  |
| `section_id` | `uuid` |  |  | FK → [assess.sections](#assess-sections) |  |
| `question_code` | `varchar(80)` | ✓ |  |  |  |
| `answer` | `jsonb` | ✓ |  |  |  |
| `evidence_file_ids` | `uuid[]` | ✓ | '{}' |  |  |
| `answered_by` | `uuid` |  |  |  |  |
| `ai_suggested` | `boolean` | ✓ | false |  |  |
| `confirmed_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_answers_updated`
- PK: `(id)`
- Index: `assess.answers (tenant_id, assessment_id)` · `assess.answers (tenant_id, section_id)` · `assess.answers (tenant_id, confirmed_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="assess-assessment-risks"></a>
## assess.assessment_risks

ความเสี่ยงที่ระบุในแบบประเมิน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `assessment_id` | `uuid` | ✓ |  | PK · FK → [assess.assessments](#assess-assessments) |  |
| `risk_id` | `uuid` | ✓ |  | PK · FK → [risk.risks](risk.md#risk-risks) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |

- PK: `(assessment_id, risk_id)`
- Index: `assess.assessment_risks (tenant_id, risk_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="assess-dpo-opinions"></a>
## assess.dpo_opinions

ความเห็นของ DPO

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](#assess-assessments) |  |
| `dpo_user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `opinion` | `text` | ✓ |  |  |  |
| `recommendation` | `text` | ✓ |  |  | ค่า: `proceed`, `proceed_with_conditions`, `do_not_proceed`, `consult_pdpc` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_dpo_opinions_updated`
- PK: `(id)`
- Index: `assess.dpo_opinions (tenant_id, assessment_id)` · `assess.dpo_opinions (tenant_id, dpo_user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="assess-consultations"></a>
## assess.consultations

บันทึกการปรึกษาผู้มีส่วนได้เสีย

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](#assess-assessments) |  |
| `stakeholder` | `text` | ✓ |  |  |  |
| `stakeholder_type` | `text` | ✓ |  |  | ค่า: `data_subject`, `processor`, `expert`, `internal`, `regulator` |
| `consulted_at` | `date` | ✓ |  |  |  |
| `summary` | `text` | ✓ |  |  |  |
| `response` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_consultations_updated`
- PK: `(id)`
- Index: `assess.consultations (tenant_id, assessment_id)`
- RLS: tenant · RLS `tenant_isolation`
