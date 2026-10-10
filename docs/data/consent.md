# schema `consent`

> ความยินยอมตาม FSD V3.2: Data Element, Purpose, Purpose Preference, Collection Point, Consent Transaction, Reconcile  
> migration: `backend/db/migrations/00005_consent.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [CON](../modules/CON.md) (`backend/internal/consent · backend/internal/cookie`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-05

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [data_elements](#consent-data-elements) | Data Element: ข้อมูลที่ใช้ในแต่ละวัตถุประสงค์ | tenant · RLS `tenant_isolation` |  | ERD-05 |  |
| [purposes](#consent-purposes) | Purpose: วัตถุประสงค์ที่ขอความยินยอม | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [purpose_versions](#consent-purpose-versions) | ข้อความของ Purpose แต่ละเวอร์ชัน | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01, SEQ-04 |
| [purpose_preferences](#consent-purpose-preferences) | Purpose Preference: ตัวเลือกย่อย เช่น ช่องทาง หัวข้อ ความถี่ | tenant · RLS `tenant_isolation` |  | ERD-05 |  |
| [purpose_data_elements](#consent-purpose-data-elements) | Data Element ที่ใช้ในแต่ละ Purpose | tenant · RLS `tenant_isolation` |  | ERD-05 |  |
| [collection_points](#consent-collection-points) | Collection Point: จุดเก็บความยินยอม | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [collection_point_purposes](#consent-collection-point-purposes) | Purpose ที่แสดงใน Collection Point | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [data_subjects](#consent-data-subjects) | เจ้าของข้อมูล (identifier เข้ารหัส + blind index) | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [subject_identifiers](#consent-subject-identifiers) | identifier ของเจ้าของข้อมูล (อีเมล เบอร์ เลขบัตร รหัสลูกค้า) | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [consent_receipts](#consent-consent-receipts) | Receipt: หลักฐานการตอบแต่ละครั้ง (hash chain) | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01, SEQ-04 |
| [consent_transactions](#consent-consent-transactions) | Consent Transaction: รายการต่อ Purpose (partition รายเดือน) | tenant · RLS `tenant_isolation` | ⟨P⟩ occurred_at | ERD-05 | BP-01, BP-02, SEQ-04 |
| [consent_status](#consent-consent-status) | สถานะล่าสุดต่อเจ้าของข้อมูล × Purpose (projection) | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01, BP-02, SEQ-04 |
| [double_optin_requests](#consent-double-optin-requests) | คำขอยืนยัน Double Opt-In | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [guardian_approvals](#consent-guardian-approvals) | การให้ความยินยอมโดยผู้ปกครอง / ผู้อนุบาล (ม.20) | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01 |
| [campaigns](#consent-campaigns) | แคมเปญขอความยินยอมแบบ bulk | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-02 |
| [campaign_recipients](#consent-campaign-recipients) | ผู้รับของแคมเปญ | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-02 |
| [reconcile_runs](#consent-reconcile-runs) | Reconcile Report: รอบเทียบสถานะกับระบบปลายทาง | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-02 |
| [reconcile_items](#consent-reconcile-items) | รายการที่สถานะไม่ตรงกัน | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-02 |
| [downstream_syncs](#consent-downstream-syncs) | การยืนยันจากระบบปลายทางหลังเปลี่ยนสถานะ | tenant · RLS `tenant_isolation` |  | ERD-05 | BP-01, BP-02 |

<a id="consent-data-elements"></a>
## consent.data_elements

Data Element: ข้อมูลที่ใช้ในแต่ละวัตถุประสงค์

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `data_category_id` | `uuid` |  |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `is_identifier` | `boolean` | ✓ | false |  |  |
| `identifier_type` | `text` |  |  |  | ค่า: `email`, `phone`, `national_id`, `customer_id`, `passport`, `other` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_data_elements_updated`
- PK: `(id)`
- Unique: `uq_data_elements_code UNIQUE (tenant_id, code)`
- Index: `consent.data_elements (tenant_id, data_category_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.purpose_data_elements.data_element_id`

<a id="consent-purposes"></a>
## consent.purposes

Purpose: วัตถุประสงค์ที่ขอความยินยอม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `lawful_basis_code` | `varchar(20)` | ✓ |  | FK → [org.lawful_bases](org.md#org-lawful-bases) |  |
| `is_sensitive` | `boolean` | ✓ | false |  |  |
| `requires_explicit` | `boolean` | ✓ | false |  |  |
| `min_age` | `smallint` |  |  |  |  |
| `lifespan_days` | `int` |  |  |  |  |
| `double_opt_in` | `boolean` | ✓ | false |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `active`, `retired` |
| `current_version_id` | `uuid` |  |  |  |  |
| `description_th` | `text` |  |  |  | คำอธิบายที่แสดงบนฟอร์ม (migration 00034) |
| `description_en` | `text` |  |  |  | (00034) |
| `data_category_codes` | `text[]` | ✓ | '{}' |  | รหัสหมวดข้อมูล ORG-07 ที่วัตถุประสงค์ใช้ — มีหมวดอ่อนไหว = `is_sensitive` + ต้องยินยอมโดยชัดแจ้ง (CON-10, 00034) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_purposes_updated`
- PK: `(id)`
- Unique: `uq_purposes_code UNIQUE (tenant_id, code)`
- Index: `consent.purposes (tenant_id, legal_entity_id)` · `consent.purposes (tenant_id, lawful_basis_code)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.purpose_versions.purpose_id`, `consent.purpose_preferences.purpose_id`, `consent.purpose_data_elements.purpose_id`, `consent.collection_point_purposes.purpose_id`, `consent.consent_transactions.purpose_id`, `consent.consent_status.purpose_id`, `consent.reconcile_items.purpose_id`, `cookie.categories.purpose_id`, `ropa.activity_purposes.consent_purpose_id`

<a id="consent-purpose-versions"></a>
## consent.purpose_versions

ข้อความของ Purpose แต่ละเวอร์ชัน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `purpose_id` | `uuid` | ✓ |  | FK → [consent.purposes](#consent-purposes) |  |
| `version_no` | `int` | ✓ |  |  |  |
| `text_th` | `text` | ✓ |  |  |  |
| `text_en` | `text` |  |  |  |  |
| `change_type` | `text` | ✓ | 'initial' |  | ค่า: `initial`, `minor`, `material` |
| `requires_reconsent` | `boolean` | ✓ | false |  |  |
| `published_at` | `timestamptz` |  |  |  |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `explicit_text_th` | `text` |  |  |  | ข้อความยินยอมโดยชัดแจ้งของวัตถุประสงค์ที่อ่อนไหว (ม.26, CON-10, 00034) |
| `explicit_text_en` | `text` |  |  |  | (00034) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_purpose_versions_updated`
- PK: `(id)`
- Unique: `uq_purpose_versions_purpose_id_version_no UNIQUE (purpose_id, version_no)`
- Index: `consent.purpose_versions (tenant_id, purpose_id)` · `consent.purpose_versions (tenant_id, approved_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_transactions.purpose_version_id`, `consent.consent_status.purpose_version_id`

<a id="consent-purpose-preferences"></a>
## consent.purpose_preferences

Purpose Preference: ตัวเลือกย่อย เช่น ช่องทาง หัวข้อ ความถี่

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `purpose_id` | `uuid` | ✓ |  | FK → [consent.purposes](#consent-purposes) |  |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name_th` | `text` | ✓ |  |  |  |
| `name_en` | `text` |  |  |  |  |
| `pref_type` | `text` | ✓ |  |  | ค่า: `channel`, `topic`, `frequency`, `other` |
| `options` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `display_order` | `smallint` | ✓ | 0 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_purpose_preferences_updated`
- PK: `(id)`
- Unique: `uq_purpose_preferences_purpose_id_code UNIQUE (purpose_id, code)`
- Index: `consent.purpose_preferences (tenant_id, purpose_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-purpose-data-elements"></a>
## consent.purpose_data_elements

Data Element ที่ใช้ในแต่ละ Purpose

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `purpose_id` | `uuid` | ✓ |  | PK · FK → [consent.purposes](#consent-purposes) |  |
| `data_element_id` | `uuid` | ✓ |  | PK · FK → [consent.data_elements](#consent-data-elements) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |

- PK: `(purpose_id, data_element_id)`
- Index: `consent.purpose_data_elements (tenant_id, data_element_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-collection-points"></a>
## consent.collection_points

Collection Point: จุดเก็บความยินยอม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `channel` | `text` | ✓ |  |  | ค่า: `web`, `app`, `pos`, `call_center`, `kiosk`, `paper`, `line`, `api`, `import`, `cookie` |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `form_id` | `uuid` |  |  | FK → [platform.form_definitions](platform.md#platform-form-definitions) |  |
| `notice_id` | `uuid` |  |  | FK → [notice.notices](notice.md#notice-notices) |  |
| `verification_method` | `text` | ✓ | 'none' |  | ค่า: `none`, `otp`, `magic_link`, `idp` |
| `double_opt_in` | `boolean` | ✓ | false |  |  |
| `age_gate` | `boolean` | ✓ | false |  |  |
| `qr_token` | `varchar(64)` |  |  | UQ |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `active`, `retired` |
| `public_key` | `varchar(64)` |  |  |  | คีย์ใน [platform.public_keys](platform.md#platform-public-keys) ที่ออกเมื่อ publish ครั้งแรก, ใช้ในลิงก์ฟอร์มและ `X-Public-Key` (CON-09, 00034) |
| `allowed_origins` | `text[]` | ✓ | '{}' |  | origin ที่ใช้คีย์ได้ (ว่าง = ทุกที่); คัดลอกไปที่ `platform.public_keys` (00034) |
| `publish_checklist` | `jsonb` |  |  |  | checklist ม.19 ที่ผู้ publish ยืนยัน (00034) |
| `published_at` | `timestamptz` |  |  |  | (00034) |
| `published_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) | (00034) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_collection_points_updated`
- PK: `(id)`
- Unique: `uq_collection_points_qr_token UNIQUE (tenant_id, qr_token)` · `uq_collection_points_code UNIQUE (tenant_id, code)`
- Index: `consent.collection_points (tenant_id, legal_entity_id)` · `consent.collection_points (tenant_id, form_id)` · `consent.collection_points (tenant_id, notice_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.collection_point_purposes.collection_point_id`, `consent.consent_receipts.collection_point_id`

<a id="consent-collection-point-purposes"></a>
## consent.collection_point_purposes

Purpose ที่แสดงใน Collection Point

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `collection_point_id` | `uuid` | ✓ |  | PK · FK → [consent.collection_points](#consent-collection-points) |  |
| `purpose_id` | `uuid` | ✓ |  | PK · FK → [consent.purposes](#consent-purposes) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `is_required` | `boolean` | ✓ | false |  |  |
| `display_order` | `smallint` | ✓ | 0 |  |  |

- PK: `(collection_point_id, purpose_id)`
- Index: `consent.collection_point_purposes (tenant_id, purpose_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-data-subjects"></a>
## consent.data_subjects

เจ้าของข้อมูล (identifier เข้ารหัส + blind index)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `subject_key` | `varchar(80)` | ✓ |  |  |  |
| `display_name_enc` | `bytea` |  |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `birth_date_enc` | `bytea` |  |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `is_minor` | `boolean` | ✓ | false |  |  |
| `guardian_subject_id` | `uuid` |  |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `legal_capacity` | `text` | ✓ | 'full' |  | ค่า: `full`, `minor`, `incompetent`, `quasi_incompetent` |
| `last_activity_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_data_subjects_updated`
- PK: `(id)`
- Unique: `uq_data_subjects_subject_key UNIQUE (tenant_id, subject_key)`
- Index: `consent.data_subjects (tenant_id, guardian_subject_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `iam.subject_verifications.subject_id`, `consent.data_subjects.guardian_subject_id`, `consent.subject_identifiers.subject_id`, `consent.consent_receipts.subject_id`, `consent.consent_transactions.subject_id`, `consent.consent_status.subject_id`, `consent.guardian_approvals.minor_subject_id`, `consent.guardian_approvals.guardian_subject_id`, `consent.campaign_recipients.subject_id`, `consent.reconcile_items.subject_id`, `cookie.consent_records.subject_id`, `notice.acknowledgements.subject_id`, `dsar.requests.subject_id`, `breach.notification_recipients.subject_id`

<a id="consent-subject-identifiers"></a>
## consent.subject_identifiers

identifier ของเจ้าของข้อมูล (อีเมล เบอร์ เลขบัตร รหัสลูกค้า)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `identifier_type` | `text` | ✓ |  |  | ค่า: `email`, `phone`, `national_id`, `customer_id`, `passport`, `line_uid`, `other` |
| `value_enc` | `bytea` | ✓ |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `blind_index` | `bytea` | ✓ |  | IX | HMAC-SHA256 ของค่าที่ normalize แล้ว (ค้นหาแบบตรงตัว) |
| `is_primary` | `boolean` | ✓ | false |  |  |
| `verified_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_subject_identifiers_updated`
- PK: `(id)`
- Unique: `uq_subject_identifiers_identifier_type_blind_index UNIQUE (tenant_id, identifier_type, blind_index)`
- Index: `consent.subject_identifiers (tenant_id, subject_id)` · `consent.subject_identifiers (tenant_id, blind_index)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-consent-receipts"></a>
## consent.consent_receipts

Receipt: หลักฐานการตอบแต่ละครั้ง (hash chain)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `receipt_no` | `varchar(40)` | ✓ |  | UQ |  |
| `subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `collection_point_id` | `uuid` | ✓ |  | FK → [consent.collection_points](#consent-collection-points) |  |
| `channel` | `varchar(20)` | ✓ |  |  |  |
| `captured_by_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `branch_org_unit_id` | `uuid` |  |  | FK → [org.org_units](org.md#org-org-units) |  |
| `notice_version_id` | `uuid` |  |  | FK → [notice.notice_versions](notice.md#notice-notice-versions) |  |
| `form_submission_id` | `uuid` |  |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `verification_id` | `uuid` |  |  | FK → [iam.subject_verifications](iam.md#iam-subject-verifications) |  |
| `ip` | `inet` |  |  |  |  |
| `user_agent` | `text` |  |  |  |  |
| `country` | `char(2)` |  |  |  |  |
| `language` | `varchar(5)` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `occurred_at` | `timestamptz` | ✓ |  |  |  |
| `prev_hash` | `char(64)` |  |  |  |  |
| `hash` | `char(64)` | ✓ |  |  |  |

- PK: `(id)`
- Unique: `uq_consent_receipts_receipt_no UNIQUE (tenant_id, receipt_no)`
- Index: `consent.consent_receipts (tenant_id, subject_id)` · `consent.consent_receipts (tenant_id, collection_point_id)` · `consent.consent_receipts (tenant_id, captured_by_user_id)` · `consent.consent_receipts (tenant_id, branch_org_unit_id)` · `consent.consent_receipts (tenant_id, notice_version_id)` · `consent.consent_receipts (tenant_id, form_submission_id)` · `consent.consent_receipts (tenant_id, verification_id)` · `consent.consent_receipts (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_transactions.receipt_id`, `consent.guardian_approvals.receipt_id`, `notice.acknowledgements.receipt_id`

<a id="consent-consent-transactions"></a>
## consent.consent_transactions

Consent Transaction: รายการต่อ Purpose (partition รายเดือน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `occurred_at` | `timestamptz` | ✓ |  | PK |  |
| `receipt_id` | `uuid` | ✓ |  | FK → [consent.consent_receipts](#consent-consent-receipts) |  |
| `subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `purpose_id` | `uuid` | ✓ |  | FK → [consent.purposes](#consent-purposes) |  |
| `purpose_version_id` | `uuid` | ✓ |  | FK → [consent.purpose_versions](#consent-purpose-versions) |  |
| `transaction_type` | `text` | ✓ |  |  | ค่า: `CONSENTED`, `NOT_CONSENTED`, `WITHDRAWN`, `EXPIRED`, `EXTENDED`, `PENDING`, `CONFIRMED`, `CANCELLED`, `CHANGED_PREFERENCES` |
| `preferences` | `jsonb` |  |  |  |  |
| `reason_code` | `varchar(40)` |  |  |  |  |
| `expires_at` | `timestamptz` |  |  |  |  |
| `source` | `text` | ✓ |  |  | ค่า: `web`, `app`, `api`, `staff`, `import`, `campaign`, `dsar`, `system` |
| `idempotency_key` | `varchar(80)` |  |  |  |  |

- PK: `(id, occurred_at)` — รวมคอลัมน์ partition
- Index: `consent.consent_transactions (tenant_id, receipt_id)` · `consent.consent_transactions (tenant_id, subject_id)` · `consent.consent_transactions (tenant_id, purpose_id)` · `consent.consent_transactions (tenant_id, purpose_version_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.consent_status.last_transaction_id` (LFK), `consent.double_optin_requests.transaction_id` (LFK), `consent.downstream_syncs.transaction_id` (LFK)

<a id="consent-consent-status"></a>
## consent.consent_status

สถานะล่าสุดต่อเจ้าของข้อมูล × Purpose (projection)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `subject_id` | `uuid` | ✓ |  | PK · FK → [consent.data_subjects](#consent-data-subjects) |  |
| `purpose_id` | `uuid` | ✓ |  | PK · FK → [consent.purposes](#consent-purposes) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `status` | `text` | ✓ |  |  | ค่า: `ACTIVE`, `NOT_GIVEN`, `WITHDRAWN`, `EXPIRED`, `PENDING` · state machine [ST-01](../states/ST-01.md) |
| `purpose_version_id` | `uuid` | ✓ |  | FK → [consent.purpose_versions](#consent-purpose-versions) |  |
| `last_transaction_id` | `uuid` | ✓ |  | LFK → `consent.consent_transactions` (ไม่มี constraint) |  |
| `preferences` | `jsonb` |  |  |  |  |
| `expires_at` | `timestamptz` |  |  | IX |  |
| `updated_at` | `timestamptz` | ✓ | now() |  |  |

- PK: `(subject_id, purpose_id)`
- Index: `consent.consent_status (tenant_id, purpose_id)` · `consent.consent_status (tenant_id, purpose_version_id)` · `consent.consent_status (tenant_id, last_transaction_id)` · `consent.consent_status (tenant_id, expires_at)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-double-optin-requests"></a>
## consent.double_optin_requests

คำขอยืนยัน Double Opt-In

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `transaction_id` | `uuid` | ✓ |  | LFK → `consent.consent_transactions` (ไม่มี constraint) |  |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms` |
| `token_hash` | `char(64)` | ✓ |  | UQ |  |
| `sent_at` | `timestamptz` |  |  |  |  |
| `expires_at` | `timestamptz` | ✓ |  |  |  |
| `confirmed_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'sent' |  | ค่า: `sent`, `confirmed`, `cancelled`, `expired` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_double_optin_requests_updated`
- PK: `(id)`
- Unique: `uq_double_optin_requests_token_hash UNIQUE (tenant_id, token_hash)`
- Index: `consent.double_optin_requests (tenant_id, transaction_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-guardian-approvals"></a>
## consent.guardian_approvals

การให้ความยินยอมโดยผู้ปกครอง / ผู้อนุบาล (ม.20)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `minor_subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `guardian_subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `receipt_id` | `uuid` |  |  | FK → [consent.consent_receipts](#consent-consent-receipts) |  |
| `relationship` | `text` | ✓ |  |  | ค่า: `parent`, `legal_guardian`, `curator`, `custodian` |
| `status` | `text` | ✓ | 'requested' |  | ค่า: `requested`, `approved`, `rejected`, `expired` |
| `requested_at` | `timestamptz` | ✓ |  |  |  |
| `approved_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_guardian_approvals_updated`
- PK: `(id)`
- Index: `consent.guardian_approvals (tenant_id, minor_subject_id)` · `consent.guardian_approvals (tenant_id, guardian_subject_id)` · `consent.guardian_approvals (tenant_id, receipt_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-campaigns"></a>
## consent.campaigns

แคมเปญขอความยินยอมแบบ bulk

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `name` | `text` | ✓ |  |  |  |
| `purpose_ids` | `uuid[]` | ✓ |  |  |  |
| `audience_query` | `jsonb` | ✓ |  |  |  |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms`, `line` |
| `template_id` | `uuid` |  |  | FK → [platform.notification_templates](platform.md#platform-notification-templates) |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `scheduled`, `sending`, `done`, `cancelled` |
| `scheduled_at` | `timestamptz` |  |  |  |  |
| `sent_count` | `int` | ✓ | 0 |  |  |
| `response_count` | `int` | ✓ | 0 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_campaigns_updated`
- PK: `(id)`
- Index: `consent.campaigns (tenant_id, template_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.campaign_recipients.campaign_id`

<a id="consent-campaign-recipients"></a>
## consent.campaign_recipients

ผู้รับของแคมเปญ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `campaign_id` | `uuid` | ✓ |  | FK → [consent.campaigns](#consent-campaigns) |  |
| `subject_id` | `uuid` | ✓ |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `token_hash` | `char(64)` |  |  | UQ |  |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `sent`, `opened`, `responded`, `bounced` |
| `sent_at` | `timestamptz` |  |  |  |  |
| `responded_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_campaign_recipients_updated`
- PK: `(id)`
- Unique: `uq_campaign_recipients_token_hash UNIQUE (tenant_id, token_hash)`
- Index: `consent.campaign_recipients (tenant_id, campaign_id)` · `consent.campaign_recipients (tenant_id, subject_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-reconcile-runs"></a>
## consent.reconcile_runs

Reconcile Report: รอบเทียบสถานะกับระบบปลายทาง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `target_api_client_id` | `uuid` |  |  | FK → [iam.api_clients](iam.md#iam-api-clients) |  |
| `connector_id` | `uuid` |  |  | FK → [platform.connectors](platform.md#platform-connectors) |  |
| `started_at` | `timestamptz` | ✓ |  |  |  |
| `finished_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'running' |  | ค่า: `running`, `done`, `failed` |
| `total` | `int` |  |  |  |  |
| `mismatched` | `int` |  |  |  |  |
| `report_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_reconcile_runs_updated`
- PK: `(id)`
- Index: `consent.reconcile_runs (tenant_id, target_api_client_id)` · `consent.reconcile_runs (tenant_id, connector_id)` · `consent.reconcile_runs (tenant_id, report_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `consent.reconcile_items.run_id`

<a id="consent-reconcile-items"></a>
## consent.reconcile_items

รายการที่สถานะไม่ตรงกัน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `run_id` | `uuid` | ✓ |  | FK → [consent.reconcile_runs](#consent-reconcile-runs) |  |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](#consent-data-subjects) |  |
| `purpose_id` | `uuid` |  |  | FK → [consent.purposes](#consent-purposes) |  |
| `platform_status` | `varchar(20)` |  |  |  |  |
| `target_status` | `varchar(20)` |  |  |  |  |
| `mismatch_type` | `text` | ✓ |  |  | ค่า: `missing_in_target`, `missing_in_platform`, `status_diff` |
| `resolved_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_reconcile_items_updated`
- PK: `(id)`
- Index: `consent.reconcile_items (tenant_id, run_id)` · `consent.reconcile_items (tenant_id, subject_id)` · `consent.reconcile_items (tenant_id, purpose_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="consent-downstream-syncs"></a>
## consent.downstream_syncs

การยืนยันจากระบบปลายทางหลังเปลี่ยนสถานะ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `transaction_id` | `uuid` | ✓ |  | LFK → `consent.consent_transactions` (ไม่มี constraint) |  |
| `webhook_delivery_id` | `uuid` |  |  | FK → [platform.webhook_deliveries](platform.md#platform-webhook-deliveries) |  |
| `target_api_client_id` | `uuid` |  |  | FK → [iam.api_clients](iam.md#iam-api-clients) |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `acknowledged`, `failed` |
| `acknowledged_at` | `timestamptz` |  |  |  |  |
| `error` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_downstream_syncs_updated`
- PK: `(id)`
- Index: `consent.downstream_syncs (tenant_id, transaction_id)` · `consent.downstream_syncs (tenant_id, webhook_delivery_id)` · `consent.downstream_syncs (tenant_id, target_api_client_id)`
- RLS: tenant · RLS `tenant_isolation`
