# schema `risk`

> risk matrix ทะเบียนความเสี่ยง control และการวิเคราะห์ช่องว่าง  
> migration: `backend/db/migrations/00010_risk.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [RRA](../modules/RRA.md) (`backend/internal/risk`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-10

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [risk_matrices](#risk-risk-matrices) | risk matrix ต่อ tenant | tenant · RLS `tenant_isolation` |  | ERD-10 |  |
| [risk_factors](#risk-risk-factors) | ปัจจัยความเสี่ยงและน้ำหนัก (ใช้คำนวณจาก RoPA / คู่ค้า / เหตุ) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-10 |  |
| [controls](#risk-controls) | คลังมาตรการ / control (ประกาศมาตรการความปลอดภัย, ISO) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-10 | BP-08 |
| [activity_scores](#risk-activity-scores) | คะแนนความเสี่ยงรายกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-05 |
| [risks](#risk-risks) | ทะเบียนความเสี่ยง (จาก DPIA / RoPA / คู่ค้า / เหตุละเมิด / audit) | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [risk_controls](#risk-risk-controls) | มาตรการที่ผูกกับความเสี่ยง | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [acceptances](#risk-acceptances) | การยอมรับความเสี่ยงคงเหลือ | tenant · RLS `tenant_isolation` |  | ERD-10 | BP-08 |
| [gap_rules](#risk-gap-rules) | กฎวิเคราะห์ช่องว่างทางกฎหมายของ RoPA | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-10 |  |
| [gap_findings](#risk-gap-findings) | ช่องว่างที่พบรายกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-10 |  |
| [compliance_scores](#risk-compliance-scores) | คะแนนความพร้อมรายกิจกรรม / หน่วยงาน / บริษัท | tenant · RLS `tenant_isolation` |  | ERD-10 |  |

<a id="risk-risk-matrices"></a>
## risk.risk_matrices

risk matrix ต่อ tenant

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `likelihood_levels` | `jsonb` | ✓ |  |  |  |
| `impact_levels` | `jsonb` | ✓ |  |  |  |
| `thresholds` | `jsonb` | ✓ |  |  |  |
| `is_default` | `boolean` | ✓ | false |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_risk_matrices_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `risk.activity_scores.matrix_id`

<a id="risk-risk-factors"></a>
## risk.risk_factors

ปัจจัยความเสี่ยงและน้ำหนัก (ใช้คำนวณจาก RoPA / คู่ค้า / เหตุ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `applies_to` | `text` | ✓ |  |  | ค่า: `activity`, `vendor`, `breach`, `dpia` |
| `weight` | `numeric(5,2)` | ✓ | 1 |  |  |
| `rule` | `jsonb` | ✓ |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_risk_factors_updated`
- PK: `(id)`
- Unique: `uq_risk_factors_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`

<a id="risk-controls"></a>
## risk.controls

คลังมาตรการ / control (ประกาศมาตรการความปลอดภัย, ISO)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `category` | `text` | ✓ |  |  | ค่า: `organizational`, `technical`, `physical`, `access_control`, `legal` |
| `description` | `text` |  |  |  |  |
| `framework_refs` | `text[]` | ✓ | '{}' |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_controls_updated`
- PK: `(id)`
- Unique: `uq_controls_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `ropa.activity_controls.control_id`, `risk.risk_controls.control_id`

<a id="risk-activity-scores"></a>
## risk.activity_scores

คะแนนความเสี่ยงรายกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `matrix_id` | `uuid` | ✓ |  | FK → [risk.risk_matrices](#risk-risk-matrices) |  |
| `likelihood` | `smallint` | ✓ |  |  |  |
| `impact` | `smallint` | ✓ |  |  |  |
| `inherent_score` | `numeric(6,2)` | ✓ |  |  |  |
| `residual_score` | `numeric(6,2)` |  |  |  |  |
| `level` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high`, `very_high` |
| `factor_breakdown` | `jsonb` | ✓ |  |  |  |
| `computed_at` | `timestamptz` | ✓ | now() |  |  |

- PK: `(id)`
- Index: `risk.activity_scores (tenant_id, activity_id)` · `risk.activity_scores (tenant_id, matrix_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="risk-risks"></a>
## risk.risks

ทะเบียนความเสี่ยง (จาก DPIA / RoPA / คู่ค้า / เหตุละเมิด / audit)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `source_type` | `text` | ✓ |  |  | ค่า: `dpia`, `activity`, `vendor`, `breach`, `audit`, `manual` |
| `source_id` | `uuid` |  |  |  |  |
| `title` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `vendor_id` | `uuid` |  |  | FK → [vendor.vendors](vendor.md#vendor-vendors) |  |
| `likelihood` | `smallint` | ✓ |  |  |  |
| `impact` | `smallint` | ✓ |  |  |  |
| `inherent_score` | `numeric(6,2)` | ✓ |  |  |  |
| `residual_likelihood` | `smallint` |  |  |  |  |
| `residual_impact` | `smallint` |  |  |  |  |
| `residual_score` | `numeric(6,2)` |  |  |  |  |
| `level` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high`, `very_high` |
| `treatment` | `text` |  |  |  | ค่า: `mitigate`, `accept`, `transfer`, `avoid` |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_treatment`, `accepted`, `closed` |
| `review_at` | `date` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_risks_updated`
- PK: `(id)`
- Index: `risk.risks (tenant_id, owner_user_id)` · `risk.risks (tenant_id, activity_id)` · `risk.risks (tenant_id, asset_id)` · `risk.risks (tenant_id, vendor_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `risk.risk_controls.risk_id`, `risk.acceptances.risk_id`, `assess.assessment_risks.risk_id`, `breach.root_causes.risk_id`

<a id="risk-risk-controls"></a>
## risk.risk_controls

มาตรการที่ผูกกับความเสี่ยง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `risk_id` | `uuid` | ✓ |  | PK · FK → [risk.risks](#risk-risks) |  |
| `control_id` | `uuid` | ✓ |  | PK · FK → [risk.controls](#risk-controls) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `status` | `text` | ✓ | 'planned' |  | ค่า: `existing`, `planned`, `implemented`, `not_effective` |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `due_at` | `date` |  |  |  |  |
| `task_id` | `uuid` |  |  | FK → [dpo.tasks](dpo.md#dpo-tasks) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_risk_controls_updated`
- PK: `(risk_id, control_id)`
- Index: `risk.risk_controls (tenant_id, control_id)` · `risk.risk_controls (tenant_id, owner_user_id)` · `risk.risk_controls (tenant_id, task_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="risk-acceptances"></a>
## risk.acceptances

การยอมรับความเสี่ยงคงเหลือ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `risk_id` | `uuid` | ✓ |  | FK → [risk.risks](#risk-risks) |  |
| `reason` | `text` | ✓ |  |  |  |
| `approval_id` | `uuid` |  |  | FK → [platform.approvals](platform.md#platform-approvals) |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approved_at` | `timestamptz` |  |  |  |  |
| `expires_at` | `date` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_acceptances_updated`
- PK: `(id)`
- Index: `risk.acceptances (tenant_id, risk_id)` · `risk.acceptances (tenant_id, approval_id)` · `risk.acceptances (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="risk-gap-rules"></a>
## risk.gap_rules

กฎวิเคราะห์ช่องว่างทางกฎหมายของ RoPA

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `expression` | `jsonb` | ✓ |  |  |  |
| `severity` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high` |
| `legal_ref` | `text` | ✓ |  |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_gap_rules_updated`
- PK: `(id)`
- Unique: `uq_gap_rules_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `risk.gap_findings.rule_id`

<a id="risk-gap-findings"></a>
## risk.gap_findings

ช่องว่างที่พบรายกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `rule_id` | `uuid` | ✓ |  | FK → [risk.gap_rules](#risk-gap-rules) |  |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `resolved`, `waived` |
| `detected_at` | `timestamptz` | ✓ | now() |  |  |
| `resolved_at` | `timestamptz` |  |  |  |  |
| `task_id` | `uuid` |  |  | FK → [dpo.tasks](dpo.md#dpo-tasks) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_gap_findings_updated`
- PK: `(id)`
- Index: `risk.gap_findings (tenant_id, rule_id)` · `risk.gap_findings (tenant_id, activity_id)` · `risk.gap_findings (tenant_id, task_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="risk-compliance-scores"></a>
## risk.compliance_scores

คะแนนความพร้อมรายกิจกรรม / หน่วยงาน / บริษัท

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `scope_type` | `text` | ✓ |  |  | ค่า: `activity`, `org_unit`, `legal_entity`, `tenant` |
| `scope_id` | `uuid` |  |  |  |  |
| `score` | `numeric(5,2)` | ✓ |  |  |  |
| `breakdown` | `jsonb` | ✓ |  |  |  |
| `computed_at` | `timestamptz` | ✓ | now() |  |  |

- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
