# schema `vendor`

> คู่ค้าและผู้ประมวลผล  
> migration: `backend/db/migrations/00014_vendor.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [VEN](../modules/VEN.md) (`backend/internal/vendor`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-13

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [vendors](#vendor-vendors) | คู่ค้า / ผู้ประมวลผล (ต่อยอดจาก org.external_parties) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [intakes](#vendor-intakes) | แบบ intake สำหรับจัดระดับความเสี่ยง (tier) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [vendor_assessments](#vendor-vendor-assessments) | รอบประเมินคู่ค้า (ใช้ assessment engine) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [sub_processors](#vendor-sub-processors) | ผู้ประมวลผลช่วง | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [certificates](#vendor-certificates) | ใบรับรองของคู่ค้า (ISO 27001, SOC 2 ฯลฯ) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [remediation_items](#vendor-remediation-items) | ประเด็นที่คู่ค้าต้องแก้ไข | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |
| [offboardings](#vendor-offboardings) | การยุติการใช้บริการ | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09 |

<a id="vendor-vendors"></a>
## vendor.vendors

คู่ค้า / ผู้ประมวลผล (ต่อยอดจาก org.external_parties)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `party_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) · UQ |  |
| `service_description` | `text` | ✓ |  |  |  |
| `relationship_owner_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `is_processor` | `boolean` | ✓ | true |  |  |
| `tier` | `text` |  |  |  | ค่า: `low`, `medium`, `high`, `critical` |
| `data_access` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `processing_countries` | `char(2)[]` | ✓ | '{}' |  |  |
| `status` | `text` | ✓ | 'prospect' |  | ค่า: `prospect`, `onboarding`, `approved`, `conditional`, `rejected`, `offboarding`, `terminated` · state machine [ST-06](../states/ST-06.md) |
| `next_assessment_at` | `date` |  |  |  |  |
| `approved_at` | `timestamptz` |  |  |  |  |
| `offboarded_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_vendors_updated`
- PK: `(id)`
- Unique: `uq_vendors_party_id UNIQUE (tenant_id, party_id)`
- Index: `vendor.vendors (tenant_id, party_id)` · `vendor.vendors (tenant_id, relationship_owner_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `risk.risks.vendor_id`, `vendor.intakes.vendor_id`, `vendor.vendor_assessments.vendor_id`, `vendor.sub_processors.vendor_id`, `vendor.certificates.vendor_id`, `vendor.remediation_items.vendor_id`, `vendor.offboardings.vendor_id`, `agreement.agreements.vendor_id`

<a id="vendor-intakes"></a>
## vendor.intakes

แบบ intake สำหรับจัดระดับความเสี่ยง (tier)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `form_submission_id` | `uuid` | ✓ |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `inherent_score` | `numeric(6,2)` | ✓ |  |  |  |
| `tier_result` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high`, `critical` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_intakes_updated`
- PK: `(id)`
- Index: `vendor.intakes (tenant_id, vendor_id)` · `vendor.intakes (tenant_id, form_submission_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="vendor-vendor-assessments"></a>
## vendor.vendor_assessments

รอบประเมินคู่ค้า (ใช้ assessment engine)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `assessment_id` | `uuid` | ✓ |  | FK → [assess.assessments](assess.md#assess-assessments) |  |
| `cycle_no` | `smallint` | ✓ | 1 |  |  |
| `guest_token_id` | `uuid` |  |  | FK → [iam.guest_tokens](iam.md#iam-guest-tokens) |  |
| `sent_at` | `timestamptz` |  |  |  |  |
| `due_at` | `date` |  |  |  |  |
| `submitted_at` | `timestamptz` |  |  |  |  |
| `score` | `numeric(6,2)` |  |  |  |  |
| `residual_level` | `text` |  |  |  | ค่า: `low`, `medium`, `high`, `critical` |
| `decision` | `text` |  |  |  | ค่า: `approved`, `conditional`, `rejected` |
| `decided_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `decided_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_vendor_assessments_updated`
- PK: `(id)`
- Unique: `uq_vendor_assessments_vendor_id_cycle_no UNIQUE (vendor_id, cycle_no)`
- Index: `vendor.vendor_assessments (tenant_id, vendor_id)` · `vendor.vendor_assessments (tenant_id, assessment_id)` · `vendor.vendor_assessments (tenant_id, guest_token_id)` · `vendor.vendor_assessments (tenant_id, decided_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `vendor.remediation_items.vendor_assessment_id`

<a id="vendor-sub-processors"></a>
## vendor.sub_processors

ผู้ประมวลผลช่วง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `party_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `service` | `text` | ✓ |  |  |  |
| `countries` | `char(2)[]` | ✓ | '{}' |  |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `approved`, `rejected` |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approved_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_sub_processors_updated`
- PK: `(id)`
- Index: `vendor.sub_processors (tenant_id, vendor_id)` · `vendor.sub_processors (tenant_id, party_id)` · `vendor.sub_processors (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="vendor-certificates"></a>
## vendor.certificates

ใบรับรองของคู่ค้า (ISO 27001, SOC 2 ฯลฯ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `cert_type` | `varchar(40)` | ✓ |  |  |  |
| `issuer` | `text` |  |  |  |  |
| `file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `valid_from` | `date` |  |  |  |  |
| `valid_to` | `date` |  |  | IX |  |
| `verified_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_certificates_updated`
- PK: `(id)`
- Index: `vendor.certificates (tenant_id, vendor_id)` · `vendor.certificates (tenant_id, file_id)` · `vendor.certificates (tenant_id, valid_to)` · `vendor.certificates (tenant_id, verified_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="vendor-remediation-items"></a>
## vendor.remediation_items

ประเด็นที่คู่ค้าต้องแก้ไข

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `vendor_assessment_id` | `uuid` |  |  | FK → [vendor.vendor_assessments](#vendor-vendor-assessments) |  |
| `title` | `text` | ✓ |  |  |  |
| `severity` | `text` | ✓ |  |  | ค่า: `low`, `medium`, `high` |
| `guest_token_id` | `uuid` |  |  | FK → [iam.guest_tokens](iam.md#iam-guest-tokens) |  |
| `due_at` | `date` |  |  |  |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_progress`, `done`, `accepted_risk` |
| `closed_at` | `timestamptz` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_remediation_items_updated`
- PK: `(id)`
- Index: `vendor.remediation_items (tenant_id, vendor_id)` · `vendor.remediation_items (tenant_id, vendor_assessment_id)` · `vendor.remediation_items (tenant_id, guest_token_id)` · `vendor.remediation_items (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="vendor-offboardings"></a>
## vendor.offboardings

การยุติการใช้บริการ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `vendor_id` | `uuid` | ✓ |  | FK → [vendor.vendors](#vendor-vendors) |  |
| `data_return_status` | `text` | ✓ |  |  | ค่า: `pending`, `returned`, `destroyed`, `not_applicable` |
| `destruction_certificate_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `access_revoked_at` | `timestamptz` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_offboardings_updated`
- PK: `(id)`
- Index: `vendor.offboardings (tenant_id, vendor_id)` · `vendor.offboardings (tenant_id, destruction_certificate_file_id)`
- RLS: tenant · RLS `tenant_isolation`
