# schema `org`

> นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก และข้อมูลตั้งต้น  
> migration: `backend/db/migrations/00004_org.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [ORG](../modules/ORG.md) (`backend/internal/org`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-04

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [legal_entities](#org-legal-entities) | นิติบุคคล (บริษัทในกลุ่ม) | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [org_units](#org-org-units) | โครงสร้างหน่วยงาน (ltree) | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [privacy_champions](#org-privacy-champions) | ผู้ประสานงาน PDPA ประจำหน่วยงาน | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [external_parties](#org-external-parties) | ทะเบียนหน่วยงานภายนอก (ผู้ประมวลผล ผู้รับ หน่วยงานรัฐ) | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [data_categories](#org-data-categories) | หมวดข้อมูลส่วนบุคคล (ทั่วไป / อ่อนไหว ม.26) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-04 |  |
| [data_subject_types](#org-data-subject-types) | กลุ่มเจ้าของข้อมูล | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-04 |  |
| [processing_purposes](#org-processing-purposes) | วัตถุประสงค์การประมวลผล (master) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-04 |  |
| [lawful_bases](#org-lawful-bases) | ฐานทางกฎหมาย ม.24 / ม.26 / ม.19 | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-04 |  |
| [countries](#org-countries) | ประเทศและสถานะมาตรฐานการคุ้มครองที่เพียงพอ | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-04 |  |
| [business_calendars](#org-business-calendars) | ปฏิทินวันทำการ | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [holidays](#org-holidays) | วันหยุดในปฏิทิน | tenant · RLS `tenant_isolation` |  | ERD-04 |  |
| [org_settings](#org-org-settings) | ค่าตั้งค่าองค์กร (ภาษา แบรนด์ รูปแบบวันที่) | tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation` |  | ERD-04 |  |

<a id="org-legal-entities"></a>
## org.legal_entities

นิติบุคคล (บริษัทในกลุ่ม)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `parent_id` | `uuid` |  |  | FK → [org.legal_entities](#org-legal-entities) |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `registration_no` | `char(13)` |  |  |  |  |
| `tax_id` | `char(13)` |  |  |  |  |
| `address` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `contact_email` | `citext` |  |  |  |  |
| `contact_phone` | `varchar(30)` |  |  |  |  |
| `logo_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `is_controller` | `boolean` | ✓ | true |  |  |
| `is_processor` | `boolean` | ✓ | false |  |  |
| `representative` | `jsonb` |  |  |  |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `inactive` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_legal_entities_updated`
- PK: `(id)`
- Index: `org.legal_entities (tenant_id, parent_id)` · `org.legal_entities (tenant_id, logo_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.role_assignments.legal_entity_id`, `org.legal_entities.parent_id`, `org.org_units.legal_entity_id`, `consent.purposes.legal_entity_id`, `consent.collection_points.legal_entity_id`, `cookie.domains.legal_entity_id`, `notice.notices.legal_entity_id`, `ropa.processing_activities.legal_entity_id`, `ropa.sme_exemption_checks.legal_entity_id`, `dsar.requests.legal_entity_id`, `breach.incidents.legal_entity_id`, `breach.routing_rules.legal_entity_id`, `agreement.parties.legal_entity_id`, `dpo.appointments.legal_entity_id`, `dpo.requirement_checks.legal_entity_id`, `gov.audits.legal_entity_id`
- ORG-01 (migration 00030): unique `(tenant_id, registration_no)` เมื่อไม่ว่าง · `registration_no` / `tax_id` ผ่านการตรวจ check digit 13 หลัก · `address` = `{"line1", "line2", "subdistrict", "district", "province", "postal_code", "country_code"}` · `logo_file_id` = ไฟล์ PLT-09 ที่ผูกกับ entity `legal_entity`

<a id="org-org-units"></a>
## org.org_units

โครงสร้างหน่วยงาน (ltree)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](#org-legal-entities) |  |
| `parent_id` | `uuid` |  |  | FK → [org.org_units](#org-org-units) |  |
| `path` | `ltree` | ✓ |  | IX |  |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `unit_type` | `text` | ✓ |  |  | ค่า: `group`, `company`, `division`, `department`, `branch`, `team` |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `closed` |
| `closed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_org_units_updated`
- PK: `(id)`
- Unique: `uq_org_units_legal_entity_id_code UNIQUE (tenant_id, legal_entity_id, code)`
- Index: `org.org_units (tenant_id, legal_entity_id)` · `org.org_units (tenant_id, parent_id)` · `org.org_units (tenant_id, path)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.users.primary_org_unit_id`, `iam.role_assignments.org_unit_id`, `org.org_units.parent_id`, `org.privacy_champions.org_unit_id`, `consent.consent_receipts.branch_org_unit_id`, `ropa.processing_activities.org_unit_id`, `ropa.assets.org_unit_id`, `ropa.data_inventory.org_unit_id`, `ropa.questionnaires.org_unit_id`, `ropa.generation_runs.org_unit_id`, `dpo.tasks.org_unit_id`, `dpo.advisories.org_unit_id`
- ORG-04: `path` = label ของหน่วยงานแม่ต่อกันจนถึงตัวเอง แต่ละ label = `u` + id ไม่มีขีด (ไม่เปลี่ยนเมื่อแก้ชื่อ) · ย้ายหน่วยงานเขียน path ใหม่ของทั้ง subtree ในคำสั่งเดียว · GiST index `ix_org_org_units_path_gist` (migration 00030) สำหรับ `<@` / `@>` · ปิดแล้ว (`status = closed`) ยังอยู่ใน tree

<a id="org-privacy-champions"></a>
## org.privacy_champions

ผู้ประสานงาน PDPA ประจำหน่วยงาน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `org_unit_id` | `uuid` | ✓ |  | FK → [org.org_units](#org-org-units) |  |
| `user_id` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `champion_role` | `text` | ✓ | 'primary' |  | ค่า: `primary`, `backup` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_privacy_champions_updated`
- PK: `(id)`
- Index: `org.privacy_champions (tenant_id, org_unit_id)` · `org.privacy_champions (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="org-external-parties"></a>
## org.external_parties

ทะเบียนหน่วยงานภายนอก (ผู้ประมวลผล ผู้รับ หน่วยงานรัฐ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `party_type` | `text` | ✓ |  |  | ค่า: `processor`, `recipient`, `controller`, `joint_controller`, `government`, `other` |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `registration_no` | `varchar(30)` |  |  |  |  |
| `country_code` | `char(2)` | ✓ |  | FK → [org.countries](#org-countries) |  |
| `contact` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `website` | `text` |  |  |  |  |
| `dedupe_key` | `varchar(120)` |  |  | IX |  |
| `status` | `text` | ✓ | 'active' |  | ค่า: `active`, `inactive` |
| `merged_into_id` | `uuid` |  |  | FK → [org.external_parties](#org-external-parties) | ORG-06 migration 00038: ผลของการรวมรายการซ้ำ — ต้อง `<> id` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_external_parties_updated`
- PK: `(id)`
- Index: `org.external_parties (tenant_id, country_code)` · `org.external_parties (tenant_id, dedupe_key)` · `org.external_parties (tenant_id, merged_into_id) WHERE merged_into_id IS NOT NULL`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `notice.indirect_collections.source_party_id`, `ropa.processing_activities.controller_party_id`, `ropa.activity_data.source_party_id`, `ropa.activity_recipients.party_id`, `ropa.assets.provider_party_id`, `dsar.subtasks.assignee_party_id`, `dsar.downstream_notices.party_id`, `breach.incidents.processor_party_id`, `vendor.vendors.party_id`, `vendor.sub_processors.party_id`, `agreement.agreements.counterparty_id`, `agreement.parties.party_id`, `gov.ai_systems.vendor_party_id`

<a id="org-data-categories"></a>
## org.data_categories

หมวดข้อมูลส่วนบุคคล (ทั่วไป / อ่อนไหว ม.26)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `is_sensitive` | `boolean` | ✓ | false |  |  |
| `sensitive_type` | `varchar(40)` |  |  |  |  |
| `parent_id` | `uuid` |  |  | FK → [org.data_categories](#org-data-categories) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_data_categories_updated`
- PK: `(id)`
- Unique: `uq_data_categories_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- Index: `org.data_categories (parent_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `org.data_categories.parent_id`, `consent.data_elements.data_category_id`, `ropa.activity_data.data_category_id`, `ropa.retention_rules.data_category_id`, `ropa.data_inventory.data_category_id`, `dataflow.classifiers.data_category_id`, `dataflow.discovery_findings.suggested_category_id`, `dsar.legal_holds.data_category_id`

- ORG-07: ค่าตั้งต้น (tenant_id NULL) seed ใน migration 00031 เป็นร่างรอฝ่ายกฎหมาย (decisions Q-20) · tenant แก้/ลบได้เฉพาะแถวของตน · รหัสของ tenant ซ้ำรหัสค่าตั้งต้นไม่ได้

<a id="org-data-subject-types"></a>
## org.data_subject_types

กลุ่มเจ้าของข้อมูล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `is_vulnerable` | `boolean` | ✓ | false |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_data_subject_types_updated`
- PK: `(id)`
- Unique: `uq_data_subject_types_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `notice.notices.subject_type_id`, `ropa.activity_data.subject_type_id`

<a id="org-processing-purposes"></a>
## org.processing_purposes

วัตถุประสงค์การประมวลผล (master)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `category` | `varchar(40)` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_processing_purposes_updated`
- PK: `(id)`
- Unique: `uq_processing_purposes_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `ropa.activity_purposes.purpose_id`

<a id="org-lawful-bases"></a>
## org.lawful_bases

ฐานทางกฎหมาย ม.24 / ม.26 / ม.19

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `code` | `varchar(20)` | ✓ |  | PK |  |
| `section_ref` | `varchar(30)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `for_sensitive` | `boolean` | ✓ | false |  |  |
| `requires_consent` | `boolean` | ✓ | false |  |  |
| `requires_lia` | `boolean` | ✓ | false |  |  |

- PK: `(code)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `consent.purposes.lawful_basis_code`, `ropa.activity_purposes.lawful_basis_code`

- ORG-07: seed ครบ 249 ประเทศ (migration 00031) ทุกแถว `adequacy_status = unknown` จนกว่าฝ่ายกฎหมายกำหนด

<a id="org-countries"></a>
## org.countries

ประเทศและสถานะมาตรฐานการคุ้มครองที่เพียงพอ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `code` | `char(2)` | ✓ |  | PK |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` | ✓ |  |  |  |
| `adequacy_status` | `text` | ✓ | 'unknown' |  | ค่า: `adequate`, `not_adequate`, `unknown` |
| `region` | `varchar(40)` |  |  |  |  |

- PK: `(code)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `org.external_parties.country_code`, `ropa.activity_transfers.country_code`, `ropa.assets.hosting_country_code`

<a id="org-business-calendars"></a>
## org.business_calendars

ปฏิทินวันทำการ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `timezone` | `varchar(40)` | ✓ | 'Asia/Bangkok' |  |  |
| `workdays` | `smallint[]` | ✓ | '{1,2,3,4,5}' |  |  |
| `is_default` | `boolean` | ✓ | false |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_business_calendars_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `platform.sla_timers.calendar_id`, `org.holidays.calendar_id`, `org.org_settings.default_calendar_id`
- Unique (migration 00027): `ux_org_business_calendars_default (tenant_id) WHERE is_default` — ปฏิทินหลักได้หนึ่งเดียวต่อ tenant · `ux_org_business_calendars_name (tenant_id, lower(name))`
- `workdays` = ISO weekday (1 = จันทร์ … 7 = อาทิตย์) · `is_default` คือแหล่งจริง; `org.org_settings.default_calendar_id` ถูกอัปเดตใน transaction เดียวกันทุกครั้งที่ปฏิทินหลักเปลี่ยน (ORG-20)

<a id="org-holidays"></a>
## org.holidays

วันหยุดในปฏิทิน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `calendar_id` | `uuid` | ✓ |  | PK · FK → [org.business_calendars](#org-business-calendars) |  |
| `holiday_date` | `date` | ✓ |  | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |

- PK: `(calendar_id, holiday_date)`
- RLS: tenant · RLS `tenant_isolation`

<a id="org-org-settings"></a>
## org.org_settings

ค่าตั้งค่าองค์กร (ภาษา แบรนด์ รูปแบบวันที่)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `tenant_id` | `uuid` | ✓ |  | PK · FK → [platform.tenants](platform.md#platform-tenants) |  |
| `default_language` | `varchar(5)` | ✓ | 'th' |  |  |
| `date_era` | `text` | ✓ | 'BE' |  | ค่า: `BE`, `CE` |
| `branding` | `jsonb` | ✓ | '{}'::jsonb |  |  |
| `default_calendar_id` | `uuid` |  |  | FK → [org.business_calendars](#org-business-calendars) |  |
| `notification_defaults` | `jsonb` | ✓ | '{}'::jsonb |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_org_settings_updated`
- PK: `(tenant_id)`
- Index: `org.org_settings (default_calendar_id)`
- RLS: tenant (tenant_id อยู่ใน PK) · RLS `tenant_isolation`
