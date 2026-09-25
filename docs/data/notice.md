# schema `notice`

> ประกาศความเป็นส่วนตัว เวอร์ชัน การรับทราบ และการแจ้งตาม ม.25  
> migration: `backend/db/migrations/00007_notice.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [PNG](../modules/PNG.md) (`backend/internal/notice`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-07

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [notices](#notice-notices) | ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [notice_versions](#notice-notice-versions) | เวอร์ชันที่เผยแพร่ + ผล checklist ม.23 | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [notice_activity_links](#notice-notice-activity-links) | กิจกรรม RoPA ที่ประกาศครอบคลุม | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [acknowledgements](#notice-acknowledgements) | การรับทราบประกาศ | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [indirect_collections](#notice-indirect-collections) | การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25) | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [embeds](#notice-embeds) | โค้ดฝังและลิงก์ของประกาศ | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [linked_documents](#notice-linked-documents) | เอกสารอ้างอิงที่ลิงก์ในประกาศ | tenant · RLS `tenant_isolation` |  | ERD-07 | BP-04 |
| [wizard_templates](#notice-wizard-templates) | template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-07 | BP-04 |

<a id="notice-notices"></a>
## notice.notices

ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `subject_type_id` | `uuid` |  |  | FK → [org.data_subject_types](org.md#org-data-subject-types) |  |
| `notice_type` | `text` | ✓ |  |  | ค่า: `privacy_notice`, `privacy_policy`, `cookie_policy`, `cctv`, `layered_short`, `employee` |
| `title` | `text` | ✓ |  |  |  |
| `slug` | `varchar(120)` | ✓ |  |  |  |
| `document_id` | `uuid` | ✓ |  | FK → [platform.documents](platform.md#platform-documents) |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `in_review`, `published`, `retired` · state machine [ST-04](../states/ST-04.md) |
| `current_version_id` | `uuid` |  |  |  |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `review_cycle_months` | `smallint` | ✓ | 12 |  |  |
| `next_review_at` | `date` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_notices_updated`
- PK: `(id)`
- Unique: `uq_notices_slug UNIQUE (tenant_id, slug)`
- Index: `notice.notices (tenant_id, legal_entity_id)` · `notice.notices (tenant_id, subject_type_id)` · `notice.notices (tenant_id, document_id)` · `notice.notices (tenant_id, owner_user_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.collection_points.notice_id`, `notice.notice_versions.notice_id`, `notice.notice_activity_links.notice_id`, `notice.indirect_collections.notice_id`, `notice.embeds.notice_id`, `notice.linked_documents.notice_id`

<a id="notice-notice-versions"></a>
## notice.notice_versions

เวอร์ชันที่เผยแพร่ + ผล checklist ม.23

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `notice_id` | `uuid` | ✓ |  | FK → [notice.notices](#notice-notices) |  |
| `version_no` | `int` | ✓ |  |  |  |
| `document_version_id` | `uuid` | ✓ |  | FK → [platform.document_versions](platform.md#platform-document-versions) |  |
| `languages` | `text[]` | ✓ |  |  |  |
| `effective_from` | `date` | ✓ |  |  |  |
| `is_material_change` | `boolean` | ✓ | false |  |  |
| `changes_purpose` | `boolean` | ✓ | false |  |  |
| `checklist_result` | `jsonb` | ✓ |  |  |  |
| `public_url` | `text` |  |  |  |  |
| `published_at` | `timestamptz` |  |  |  |  |
| `published_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_notice_versions_updated`
- PK: `(id)`
- Unique: `uq_notice_versions_notice_id_version_no UNIQUE (notice_id, version_no)`
- Index: `notice.notice_versions (tenant_id, notice_id)` · `notice.notice_versions (tenant_id, document_version_id)` · `notice.notice_versions (tenant_id, published_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_receipts.notice_version_id`, `notice.acknowledgements.notice_version_id`

<a id="notice-notice-activity-links"></a>
## notice.notice_activity_links

กิจกรรม RoPA ที่ประกาศครอบคลุม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `notice_id` | `uuid` | ✓ |  | PK · FK → [notice.notices](#notice-notices) |  |
| `activity_id` | `uuid` | ✓ |  | PK · FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |

- PK: `(notice_id, activity_id)`
- Index: `notice.notice_activity_links (tenant_id, activity_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="notice-acknowledgements"></a>
## notice.acknowledgements

การรับทราบประกาศ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `notice_version_id` | `uuid` | ✓ |  | FK → [notice.notice_versions](#notice-notice-versions) |  |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](consent.md#consent-data-subjects) |  |
| `user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `channel` | `varchar(20)` | ✓ |  |  |  |
| `receipt_id` | `uuid` |  |  | FK → [consent.consent_receipts](consent.md#consent-consent-receipts) |  |
| `occurred_at` | `timestamptz` | ✓ | now() |  |  |
| `ip` | `inet` |  |  |  |  |

- PK: `(id)`
- Index: `notice.acknowledgements (tenant_id, notice_version_id)` · `notice.acknowledgements (tenant_id, subject_id)` · `notice.acknowledgements (tenant_id, user_id)` · `notice.acknowledgements (tenant_id, receipt_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="notice-indirect-collections"></a>
## notice.indirect_collections

การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `source_party_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `notice_id` | `uuid` |  |  | FK → [notice.notices](#notice-notices) |  |
| `obtained_at` | `date` | ✓ |  |  |  |
| `subject_count` | `int` |  |  |  |  |
| `notify_due_at` | `date` | ✓ |  |  |  |
| `method` | `text` |  |  |  | ค่า: `email`, `sms`, `letter`, `website`, `other` |
| `notified_at` | `timestamptz` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `notified`, `exempted`, `overdue` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_indirect_collections_updated`
- PK: `(id)`
- Index: `notice.indirect_collections (tenant_id, source_party_id)` · `notice.indirect_collections (tenant_id, activity_id)` · `notice.indirect_collections (tenant_id, notice_id)` · `notice.indirect_collections (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="notice-embeds"></a>
## notice.embeds

โค้ดฝังและลิงก์ของประกาศ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `notice_id` | `uuid` | ✓ |  | FK → [notice.notices](#notice-notices) |  |
| `embed_type` | `text` | ✓ |  |  | ค่า: `script`, `iframe`, `link`, `qr` |
| `config` | `jsonb` | ✓ | '{}'::jsonb |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_embeds_updated`
- PK: `(id)`
- Index: `notice.embeds (tenant_id, notice_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="notice-linked-documents"></a>
## notice.linked_documents

เอกสารอ้างอิงที่ลิงก์ในประกาศ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `notice_id` | `uuid` | ✓ |  | FK → [notice.notices](#notice-notices) |  |
| `label` | `text` | ✓ |  |  |  |
| `url` | `text` |  |  |  |  |
| `file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `display_mode` | `text` | ✓ | 'link' |  | ค่า: `inline`, `popup`, `link` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_linked_documents_updated`
- PK: `(id)`
- Index: `notice.linked_documents (tenant_id, notice_id)` · `notice.linked_documents (tenant_id, file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="notice-wizard-templates"></a>
## notice.wizard_templates

template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `subject_type_code` | `varchar(40)` | ✓ |  |  |  |
| `industry` | `varchar(40)` |  |  |  |  |
| `language` | `varchar(5)` | ✓ |  |  |  |
| `template_id` | `uuid` | ✓ |  | FK → [platform.templates](platform.md#platform-templates) |  |
| `questions` | `jsonb` | ✓ |  |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_wizard_templates_updated`
- PK: `(id)`
- Index: `notice.wizard_templates (template_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
