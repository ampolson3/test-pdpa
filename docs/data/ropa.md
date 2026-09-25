# schema `ropa`

> RoPA ทะเบียนข้อมูล asset การโอน retention และคลังกิจกรรมมาตรฐาน  
> migration: `backend/db/migrations/00008_ropa.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [ROPA](../modules/ROPA.md) (`backend/internal/ropa`), [RTG](../modules/RTG.md) (`backend/internal/ropa/templates`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-08, ERD-09

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [processing_activities](#ropa-processing-activities) | กิจกรรมการประมวลผล (RoPA ม.39 / ผู้ประมวลผล) | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-04, BP-05, BP-08 |
| [activity_purposes](#ropa-activity-purposes) | วัตถุประสงค์และฐานกฎหมายของกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [activity_data](#ropa-activity-data) | ข้อมูลที่เก็บในกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [activity_systems](#ropa-activity-systems) | ระบบ / asset ที่ใช้ในกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [activity_recipients](#ropa-activity-recipients) | ผู้รับข้อมูลและการเปิดเผย (ม.27) | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [activity_transfers](#ropa-activity-transfers) | การโอนไปต่างประเทศ (ม.28 / ม.29) | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [retention_rules](#ropa-retention-rules) | ระยะเวลาเก็บรักษาและวิธีทำลาย | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05, BP-11 |
| [activity_controls](#ropa-activity-controls) | มาตรการความปลอดภัยต่อกิจกรรม (ม.37(1)) | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [activity_rejections](#ropa-activity-rejections) | การปฏิเสธคำขอใช้สิทธิที่เกี่ยวข้อง (ม.39(7)) | tenant · RLS `tenant_isolation` |  | ERD-08 | BP-05 |
| [assets](#ropa-assets) | ทะเบียนระบบ / asset | tenant · RLS `tenant_isolation` |  | ERD-08 | SEQ-06 |
| [data_inventory](#ropa-data-inventory) | ทะเบียนข้อมูลส่วนบุคคลต่อระบบ | tenant · RLS `tenant_isolation` |  | ERD-08 |  |
| [questionnaires](#ropa-questionnaires) | แบบสอบถามเก็บข้อมูลกิจกรรมจากหน่วยงาน | tenant · RLS `tenant_isolation` |  | ERD-08 |  |
| [sme_exemption_checks](#ropa-sme-exemption-checks) | ผลตรวจสิทธิ์ยกเว้น RoPA ของกิจการขนาดเล็ก | tenant · RLS `tenant_isolation` |  | ERD-08 |  |
| [template_sets](#ropa-template-sets) | ชุด template (มาตรฐาน / อุตสาหกรรม / ภาครัฐ / ขององค์กร) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-09 | BP-05 |
| [activity_templates](#ropa-activity-templates) | กิจกรรมมาตรฐานพร้อมค่าตั้งต้นและเหตุผล | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-09 | BP-05 |
| [generation_runs](#ropa-generation-runs) | การสร้างร่าง RoPA จาก template ครั้งละหลายกิจกรรม | tenant · RLS `tenant_isolation` |  | ERD-09 | BP-05 |
| [wizard_sessions](#ropa-wizard-sessions) | session ของ wizard ถาม-ตอบภาษาง่าย | tenant · RLS `tenant_isolation` |  | ERD-09 | BP-05 |

<a id="ropa-processing-activities"></a>
## ropa.processing_activities

กิจกรรมการประมวลผล (RoPA ม.39 / ผู้ประมวลผล)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `org_unit_id` | `uuid` | ✓ |  | FK → [org.org_units](org.md#org-org-units) |  |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `description` | `text` |  |  |  |  |
| `role` | `text` | ✓ |  |  | ค่า: `controller`, `processor` |
| `controller_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `template_id` | `uuid` |  |  | FK → [ropa.activity_templates](#ropa-activity-templates) |  |
| `template_version_no` | `int` |  |  |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `pending_approval`, `active`, `under_review`, `ended` · state machine [ST-05](../states/ST-05.md) |
| `completeness` | `smallint` | ✓ | 0 |  |  |
| `risk_level` | `text` |  |  |  | ค่า: `low`, `medium`, `high`, `very_high` |
| `rights_and_access` | `text` |  |  |  |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `approved_at` | `timestamptz` |  |  |  |  |
| `next_review_at` | `date` |  |  |  |  |
| `ended_at` | `timestamptz` |  |  |  |  |
| `end_reason` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_processing_activities_updated`
- PK: `(id)`
- Unique: `uq_processing_activities_code UNIQUE (tenant_id, code)`
- Index: `ropa.processing_activities (tenant_id, legal_entity_id)` · `ropa.processing_activities (tenant_id, org_unit_id)` · `ropa.processing_activities (tenant_id, controller_party_id)` · `ropa.processing_activities (tenant_id, owner_user_id)` · `ropa.processing_activities (tenant_id, template_id)` · `ropa.processing_activities (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `notice.notice_activity_links.activity_id`, `notice.indirect_collections.activity_id`, `ropa.activity_purposes.activity_id`, `ropa.activity_data.activity_id`, `ropa.activity_systems.activity_id`, `ropa.activity_recipients.activity_id`, `ropa.activity_transfers.activity_id`, `ropa.retention_rules.activity_id`, `ropa.activity_controls.activity_id`, `ropa.activity_rejections.activity_id`, `ropa.questionnaires.activity_id`, `ropa.wizard_sessions.activity_id`, `risk.activity_scores.activity_id`, `risk.risks.activity_id`, `risk.gap_findings.activity_id`, `assess.assessments.activity_id`, `breach.incident_assets.activity_id`, `agreement.agreement_activities.activity_id`

<a id="ropa-activity-purposes"></a>
## ropa.activity_purposes

วัตถุประสงค์และฐานกฎหมายของกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `purpose_id` | `uuid` |  |  | FK → [org.processing_purposes](org.md#org-processing-purposes) |  |
| `purpose_text` | `text` | ✓ |  |  |  |
| `lawful_basis_code` | `varchar(20)` | ✓ |  | FK → [org.lawful_bases](org.md#org-lawful-bases) |  |
| `consent_purpose_id` | `uuid` |  |  | FK → [consent.purposes](consent.md#consent-purposes) |  |
| `lia_assessment_id` | `uuid` |  |  | FK → [assess.assessments](assess.md#assess-assessments) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_purposes_updated`
- PK: `(id)`
- Index: `ropa.activity_purposes (tenant_id, activity_id)` · `ropa.activity_purposes (tenant_id, purpose_id)` · `ropa.activity_purposes (tenant_id, lawful_basis_code)` · `ropa.activity_purposes (tenant_id, consent_purpose_id)` · `ropa.activity_purposes (tenant_id, lia_assessment_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-activity-data"></a>
## ropa.activity_data

ข้อมูลที่เก็บในกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `data_category_id` | `uuid` | ✓ |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `subject_type_id` | `uuid` | ✓ |  | FK → [org.data_subject_types](org.md#org-data-subject-types) |  |
| `source` | `text` | ✓ |  |  | ค่า: `direct`, `indirect` |
| `source_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `is_sensitive` | `boolean` | ✓ | false |  |  |
| `volume_band` | `text` |  |  |  | ค่า: `lt_1k`, `1k_10k`, `10k_100k`, `gt_100k` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_data_updated`
- PK: `(id)`
- Index: `ropa.activity_data (tenant_id, activity_id)` · `ropa.activity_data (tenant_id, data_category_id)` · `ropa.activity_data (tenant_id, subject_type_id)` · `ropa.activity_data (tenant_id, source_party_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-activity-systems"></a>
## ropa.activity_systems

ระบบ / asset ที่ใช้ในกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `activity_id` | `uuid` | ✓ |  | PK · FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `asset_id` | `uuid` | ✓ |  | PK · FK → [ropa.assets](#ropa-assets) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `usage` | `text` | ✓ |  |  | ค่า: `collect`, `store`, `process`, `transfer`, `archive` |

- PK: `(activity_id, asset_id)`
- Index: `ropa.activity_systems (tenant_id, asset_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-activity-recipients"></a>
## ropa.activity_recipients

ผู้รับข้อมูลและการเปิดเผย (ม.27)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `party_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `recipient_role` | `text` | ✓ |  |  | ค่า: `processor`, `controller`, `joint_controller`, `government` |
| `disclosure_basis` | `varchar(40)` |  |  |  |  |
| `data_category_ids` | `uuid[]` | ✓ | '{}' |  |  |
| `agreement_id` | `uuid` |  |  | FK → [agreement.agreements](agreement.md#agreement-agreements) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_recipients_updated`
- PK: `(id)`
- Index: `ropa.activity_recipients (tenant_id, activity_id)` · `ropa.activity_recipients (tenant_id, party_id)` · `ropa.activity_recipients (tenant_id, agreement_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `ropa.activity_transfers.recipient_id`, `agreement.agreement_activities.recipient_id`

<a id="ropa-activity-transfers"></a>
## ropa.activity_transfers

การโอนไปต่างประเทศ (ม.28 / ม.29)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `recipient_id` | `uuid` |  |  | FK → [ropa.activity_recipients](#ropa-activity-recipients) |  |
| `country_code` | `char(2)` | ✓ |  | FK → [org.countries](org.md#org-countries) |  |
| `transfer_basis` | `text` | ✓ |  |  | ค่า: `adequacy`, `bcr`, `standard_clauses`, `certification`, `exemption`, `consent` |
| `safeguards` | `text` |  |  |  |  |
| `tia_assessment_id` | `uuid` |  |  | FK → [assess.assessments](assess.md#assess-assessments) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_transfers_updated`
- PK: `(id)`
- Index: `ropa.activity_transfers (tenant_id, activity_id)` · `ropa.activity_transfers (tenant_id, recipient_id)` · `ropa.activity_transfers (tenant_id, country_code)` · `ropa.activity_transfers (tenant_id, tia_assessment_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-retention-rules"></a>
## ropa.retention_rules

ระยะเวลาเก็บรักษาและวิธีทำลาย

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `data_category_id` | `uuid` |  |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `retention_months` | `int` |  |  |  |  |
| `retention_basis` | `text` | ✓ |  |  |  |
| `trigger_event` | `varchar(60)` | ✓ |  |  |  |
| `disposal_method` | `text` | ✓ |  |  | ค่า: `delete`, `destroy`, `anonymize`, `return` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_retention_rules_updated`
- PK: `(id)`
- Index: `ropa.retention_rules (tenant_id, activity_id)` · `ropa.retention_rules (tenant_id, data_category_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `gov.retention_schedules.retention_rule_id`

<a id="ropa-activity-controls"></a>
## ropa.activity_controls

มาตรการความปลอดภัยต่อกิจกรรม (ม.37(1))

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `activity_id` | `uuid` | ✓ |  | PK · FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `control_id` | `uuid` | ✓ |  | PK · FK → [risk.controls](risk.md#risk-controls) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `description` | `text` |  |  |  |  |
| `assessment_id` | `uuid` |  |  | FK → [assess.assessments](assess.md#assess-assessments) |  |

- PK: `(activity_id, control_id)`
- Index: `ropa.activity_controls (tenant_id, control_id)` · `ropa.activity_controls (tenant_id, assessment_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-activity-rejections"></a>
## ropa.activity_rejections

การปฏิเสธคำขอใช้สิทธิที่เกี่ยวข้อง (ม.39(7))

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `activity_id` | `uuid` | ✓ |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `dsar_request_id` | `uuid` | ✓ |  | FK → [dsar.requests](dsar.md#dsar-requests) |  |
| `reason_code` | `varchar(40)` | ✓ |  |  |  |
| `rejected_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_rejections_updated`
- PK: `(id)`
- Index: `ropa.activity_rejections (tenant_id, activity_id)` · `ropa.activity_rejections (tenant_id, dsar_request_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-assets"></a>
## ropa.assets

ทะเบียนระบบ / asset

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `asset_type` | `text` | ✓ |  |  | ค่า: `application`, `database`, `file_share`, `saas`, `paper`, `device`, `other` |
| `org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `provider_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `hosting_country_code` | `char(2)` |  |  | FK → [org.countries](org.md#org-countries) |  |
| `hosting_type` | `text` |  |  |  | ค่า: `on_prem`, `cloud`, `hybrid` |
| `classification` | `text` |  |  |  | ค่า: `public`, `internal`, `confidential`, `restricted` |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `retired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_assets_updated`
- PK: `(id)`
- Index: `ropa.assets (tenant_id, org_unit_id)` · `ropa.assets (tenant_id, owner_user_id)` · `ropa.assets (tenant_id, provider_party_id)` · `ropa.assets (tenant_id, hosting_country_code)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.connectors.asset_id`, `iam.api_clients.system_asset_id`, `ropa.activity_systems.asset_id`, `ropa.data_inventory.asset_id`, `dataflow.discovery_scans.asset_id`, `risk.risks.asset_id`, `dsar.subtasks.asset_id`, `dsar.legal_holds.asset_id`, `breach.incident_assets.asset_id`, `gov.retention_schedules.asset_id`

<a id="ropa-data-inventory"></a>
## ropa.data_inventory

ทะเบียนข้อมูลส่วนบุคคลต่อระบบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `asset_id` | `uuid` | ✓ |  | FK → [ropa.assets](#ropa-assets) |  |
| `data_category_id` | `uuid` | ✓ |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `source` | `text` |  |  |  | ค่า: `direct`, `indirect`, `derived` |
| `location_detail` | `text` |  |  |  |  |
| `discovered_by_finding_id` | `uuid` |  |  | FK → [dataflow.discovery_findings](dataflow.md#dataflow-discovery-findings) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_data_inventory_updated`
- PK: `(id)`
- Index: `ropa.data_inventory (tenant_id, asset_id)` · `ropa.data_inventory (tenant_id, data_category_id)` · `ropa.data_inventory (tenant_id, org_unit_id)` · `ropa.data_inventory (tenant_id, owner_user_id)` · `ropa.data_inventory (tenant_id, discovered_by_finding_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-questionnaires"></a>
## ropa.questionnaires

แบบสอบถามเก็บข้อมูลกิจกรรมจากหน่วยงาน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `org_unit_id` | `uuid` | ✓ |  | FK → [org.org_units](org.md#org-org-units) |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `form_submission_id` | `uuid` |  |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `respondent_user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `due_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'sent' |  | ค่า: `sent`, `answered`, `approved`, `rejected` |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_questionnaires_updated`
- PK: `(id)`
- Index: `ropa.questionnaires (tenant_id, org_unit_id)` · `ropa.questionnaires (tenant_id, activity_id)` · `ropa.questionnaires (tenant_id, form_submission_id)` · `ropa.questionnaires (tenant_id, respondent_user_id)` · `ropa.questionnaires (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-sme-exemption-checks"></a>
## ropa.sme_exemption_checks

ผลตรวจสิทธิ์ยกเว้น RoPA ของกิจการขนาดเล็ก

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `form_submission_id` | `uuid` | ✓ |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `result` | `text` | ✓ |  |  | ค่า: `exempt`, `not_exempt`, `partial` |
| `basis` | `text` | ✓ |  |  |  |
| `assessed_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_sme_exemption_checks_updated`
- PK: `(id)`
- Index: `ropa.sme_exemption_checks (tenant_id, legal_entity_id)` · `ropa.sme_exemption_checks (tenant_id, form_submission_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-template-sets"></a>
## ropa.template_sets

ชุด template (มาตรฐาน / อุตสาหกรรม / ภาครัฐ / ขององค์กร)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `name` | `text` | ✓ |  |  |  |
| `set_type` | `text` | ✓ |  |  | ค่า: `standard`, `industry`, `government`, `processor`, `custom` |
| `industry` | `varchar(40)` |  |  |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `retired` |
| `published_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_template_sets_updated`
- PK: `(id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `ropa.activity_templates.template_set_id`

<a id="ropa-activity-templates"></a>
## ropa.activity_templates

กิจกรรมมาตรฐานพร้อมค่าตั้งต้นและเหตุผล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `template_set_id` | `uuid` | ✓ |  | FK → [ropa.template_sets](#ropa-template-sets) |  |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `job_category` | `varchar(40)` | ✓ |  |  |  |
| `role` | `text` | ✓ |  |  | ค่า: `controller`, `processor` |
| `defaults` | `jsonb` | ✓ |  |  |  |
| `rationale` | `jsonb` | ✓ |  |  |  |
| `legal_refs` | `text[]` | ✓ | '{}' |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_activity_templates_updated`
- PK: `(id)`
- Unique: `uq_activity_templates_template_set_id_code_version_no UNIQUE NULLS NOT DISTINCT (tenant_id, template_set_id, code, version_no)`
- Index: `ropa.activity_templates (template_set_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `ropa.processing_activities.template_id`

<a id="ropa-generation-runs"></a>
## ropa.generation_runs

การสร้างร่าง RoPA จาก template ครั้งละหลายกิจกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `org_unit_id` | `uuid` | ✓ |  | FK → [org.org_units](org.md#org-org-units) |  |
| `template_ids` | `uuid[]` | ✓ |  |  |  |
| `created_activity_ids` | `uuid[]` | ✓ | '{}' |  |  |
| `task_ids` | `uuid[]` | ✓ | '{}' |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_generation_runs_updated`
- PK: `(id)`
- Index: `ropa.generation_runs (tenant_id, org_unit_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="ropa-wizard-sessions"></a>
## ropa.wizard_sessions

session ของ wizard ถาม-ตอบภาษาง่าย

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `answers` | `jsonb` | ✓ |  |  |  |
| `mapped_fields` | `jsonb` |  |  |  |  |
| `ai_suggestions` | `jsonb` |  |  |  |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](#ropa-processing-activities) |  |
| `status` | `text` | ✓ | 'in_progress' |  | ค่า: `in_progress`, `completed`, `abandoned` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_wizard_sessions_updated`
- PK: `(id)`
- Index: `ropa.wizard_sessions (tenant_id, user_id)` · `ropa.wizard_sessions (tenant_id, activity_id)`
- RLS: tenant · RLS `tenant_isolation`
