# schema `cookie`

> โดเมน แบนเนอร์ คุกกี้ ผลสแกน และหลักฐานความยินยอมคุกกี้  
> migration: `backend/db/migrations/00006_cookie.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [CON](../modules/CON.md) (`backend/internal/consent · backend/internal/cookie`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-06

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [domains](#cookie-domains) | โดเมน / เว็บไซต์ที่ใช้แบนเนอร์ | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [apps](#cookie-apps) | แอป / LINE LIFF ที่ใช้ SDK | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [banner_configs](#cookie-banner-configs) | config แบนเนอร์ (มีเวอร์ชัน publish ไป CDN) | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [categories](#cookie-categories) | หมวดคุกกี้ | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03, SEQ-03 |
| [cookies](#cookie-cookies) | คุกกี้ที่พบ / ประกาศไว้ | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [cookie_kb](#cookie-cookie-kb) | ฐานข้อมูลคุกกี้กลาง (ใช้จัดหมวดอัตโนมัติ) | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-06 | BP-03 |
| [scans](#cookie-scans) | รอบสแกนเว็บไซต์ | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [scan_findings](#cookie-scan-findings) | คุกกี้ / tracker ที่พบในแต่ละรอบ | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [script_rules](#cookie-script-rules) | กฎบล็อกสคริปต์ก่อนได้รับความยินยอม | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03, SEQ-03 |
| [ab_variants](#cookie-ab-variants) | variant แบนเนอร์สำหรับ A/B test | tenant · RLS `tenant_isolation` |  | ERD-06 | BP-03 |
| [consent_records](#cookie-consent-records) | หลักฐานความยินยอมคุกกี้ของผู้เข้าชม (partition รายเดือน) | tenant · RLS `tenant_isolation` | ⟨P⟩ occurred_at | ERD-06 | BP-03, SEQ-03 |

<a id="cookie-domains"></a>
## cookie.domains

โดเมน / เว็บไซต์ที่ใช้แบนเนอร์

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain` | `varchar(255)` | ✓ |  |  |  |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `verification_token` | `varchar(64)` | ✓ |  |  |  |
| `verified_at` | `timestamptz` |  |  |  |  |
| `scan_schedule` | `varchar(40)` |  |  |  |  |
| `status` | `text` | ✓ | 'pending' |  | ค่า: `pending`, `active`, `disabled` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_domains_updated`
- PK: `(id)`
- Unique: `uq_domains_domain UNIQUE (tenant_id, domain)`
- Index: `cookie.domains (tenant_id, legal_entity_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.banner_configs.domain_id`, `cookie.cookies.domain_id`, `cookie.scans.domain_id`, `cookie.script_rules.domain_id`, `cookie.ab_variants.domain_id`, `cookie.consent_records.domain_id`

<a id="cookie-apps"></a>
## cookie.apps

แอป / LINE LIFF ที่ใช้ SDK

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `platform` | `text` | ✓ |  |  | ค่า: `ios`, `android`, `web`, `liff`, `ctv` |
| `app_identifier` | `varchar(200)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `config` | `jsonb` | ✓ | '{}'::jsonb |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_apps_updated`
- PK: `(id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="cookie-banner-configs"></a>
## cookie.banner_configs

config แบนเนอร์ (มีเวอร์ชัน publish ไป CDN)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `version_no` | `int` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `published`, `archived` |
| `layout` | `varchar(20)` | ✓ |  |  |  |
| `theme` | `jsonb` | ✓ |  |  |  |
| `texts` | `jsonb` | ✓ |  |  |  |
| `geo_rules` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `consent_mode_v2` | `boolean` | ✓ | true |  |  |
| `tcf_enabled` | `boolean` | ✓ | false |  |  |
| `cdn_path` | `text` |  |  |  |  |
| `published_at` | `timestamptz` |  |  |  |  |
| `published_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_banner_configs_updated`
- PK: `(id)`
- Unique: `uq_banner_configs_domain_id_version_no UNIQUE (domain_id, version_no)`
- Index: `cookie.banner_configs (tenant_id, domain_id)` · `cookie.banner_configs (tenant_id, published_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.ab_variants.banner_config_id`, `cookie.consent_records.banner_config_id`

<a id="cookie-categories"></a>
## cookie.categories

หมวดคุกกี้

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `code` | `varchar(40)` | ✓ |  |  |  |
| `names` | `jsonb` | ✓ |  |  |  |
| `descriptions` | `jsonb` | ✓ |  |  |  |
| `is_required` | `boolean` | ✓ | false |  |  |
| `display_order` | `smallint` | ✓ | 0 |  |  |
| `purpose_id` | `uuid` |  |  | FK → [consent.purposes](consent.md#consent-purposes) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_categories_updated`
- PK: `(id)`
- Unique: `uq_categories_code UNIQUE (tenant_id, code)`
- Index: `cookie.categories (tenant_id, purpose_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.cookies.category_id`, `cookie.script_rules.category_id`

<a id="cookie-cookies"></a>
## cookie.cookies

คุกกี้ที่พบ / ประกาศไว้

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `name` | `varchar(255)` | ✓ |  |  |  |
| `provider_domain` | `varchar(255)` | ✓ |  |  |  |
| `category_id` | `uuid` |  |  | FK → [cookie.categories](#cookie-categories) |  |
| `storage_type` | `text` | ✓ |  |  | ค่า: `http_cookie`, `js_cookie`, `local_storage`, `session_storage`, `pixel` |
| `duration` | `varchar(40)` |  |  |  |  |
| `descriptions` | `jsonb` |  |  |  |  |
| `source` | `text` | ✓ |  |  | ค่า: `scan`, `manual`, `kb` |
| `kb_id` | `uuid` |  |  | FK → [cookie.cookie_kb](#cookie-cookie-kb) |  |
| `first_seen_scan_id` | `uuid` |  |  | FK → [cookie.scans](#cookie-scans) |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_cookies_updated`
- PK: `(id)`
- Index: `cookie.cookies (tenant_id, domain_id)` · `cookie.cookies (tenant_id, category_id)` · `cookie.cookies (tenant_id, kb_id)` · `cookie.cookies (tenant_id, first_seen_scan_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.scan_findings.matched_cookie_id`

<a id="cookie-cookie-kb"></a>
## cookie.cookie_kb

ฐานข้อมูลคุกกี้กลาง (ใช้จัดหมวดอัตโนมัติ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `name_pattern` | `varchar(255)` | ✓ |  |  |  |
| `provider` | `varchar(200)` |  |  |  |  |
| `category_code` | `varchar(40)` | ✓ |  |  |  |
| `description_th` | `text` |  |  |  |  |
| `description_en` | `text` |  |  |  |  |
| `source` | `varchar(60)` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_cookie_kb_updated`
- PK: `(id)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)
- ถูกอ้างถึงโดย: `cookie.cookies.kb_id`

<a id="cookie-scans"></a>
## cookie.scans

รอบสแกนเว็บไซต์

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `trigger` | `text` | ✓ |  |  | ค่า: `manual`, `schedule`, `publish` |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `running`, `done`, `failed` |
| `started_at` | `timestamptz` |  |  |  |  |
| `finished_at` | `timestamptz` |  |  |  |  |
| `pages_scanned` | `int` |  |  |  |  |
| `cookies_found` | `int` |  |  |  |  |
| `new_cookies` | `int` |  |  |  |  |
| `report_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_scans_updated`
- PK: `(id)`
- Index: `cookie.scans (tenant_id, domain_id)` · `cookie.scans (tenant_id, report_file_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.cookies.first_seen_scan_id`, `cookie.scan_findings.scan_id`

<a id="cookie-scan-findings"></a>
## cookie.scan_findings

คุกกี้ / tracker ที่พบในแต่ละรอบ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `scan_id` | `uuid` | ✓ |  | FK → [cookie.scans](#cookie-scans) |  |
| `cookie_name` | `varchar(255)` | ✓ |  |  |  |
| `cookie_domain` | `varchar(255)` | ✓ |  |  |  |
| `page_url` | `text` | ✓ |  |  |  |
| `set_before_consent` | `boolean` | ✓ | false |  |  |
| `suggested_category` | `varchar(40)` |  |  |  |  |
| `matched_cookie_id` | `uuid` |  |  | FK → [cookie.cookies](#cookie-cookies) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_scan_findings_updated`
- PK: `(id)`
- Index: `cookie.scan_findings (tenant_id, scan_id)` · `cookie.scan_findings (tenant_id, matched_cookie_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="cookie-script-rules"></a>
## cookie.script_rules

กฎบล็อกสคริปต์ก่อนได้รับความยินยอม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `pattern` | `text` | ✓ |  |  |  |
| `category_id` | `uuid` | ✓ |  | FK → [cookie.categories](#cookie-categories) |  |
| `action` | `text` | ✓ | 'block' |  | ค่า: `block`, `allow` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_script_rules_updated`
- PK: `(id)`
- Index: `cookie.script_rules (tenant_id, domain_id)` · `cookie.script_rules (tenant_id, category_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="cookie-ab-variants"></a>
## cookie.ab_variants

variant แบนเนอร์สำหรับ A/B test

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `banner_config_id` | `uuid` | ✓ |  | FK → [cookie.banner_configs](#cookie-banner-configs) |  |
| `name` | `varchar(60)` | ✓ |  |  |  |
| `traffic_pct` | `smallint` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `running`, `stopped` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_ab_variants_updated`
- PK: `(id)`
- Index: `cookie.ab_variants (tenant_id, domain_id)` · `cookie.ab_variants (tenant_id, banner_config_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `cookie.consent_records.ab_variant_id`

<a id="cookie-consent-records"></a>
## cookie.consent_records

หลักฐานความยินยอมคุกกี้ของผู้เข้าชม (partition รายเดือน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `occurred_at` | `timestamptz` | ✓ |  | PK |  |
| `domain_id` | `uuid` | ✓ |  | FK → [cookie.domains](#cookie-domains) |  |
| `visitor_id` | `varchar(64)` | ✓ |  | IX |  |
| `banner_config_id` | `uuid` | ✓ |  | FK → [cookie.banner_configs](#cookie-banner-configs) |  |
| `ab_variant_id` | `uuid` |  |  | FK → [cookie.ab_variants](#cookie-ab-variants) |  |
| `action` | `text` | ✓ |  |  | ค่า: `accept_all`, `reject_all`, `custom`, `withdraw` |
| `choices` | `jsonb` | ✓ |  |  |  |
| `ip_hash` | `char(64)` |  |  |  |  |
| `country` | `char(2)` |  |  |  |  |
| `user_agent` | `text` |  |  |  |  |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](consent.md#consent-data-subjects) |  |

- PK: `(id, occurred_at)` — รวมคอลัมน์ partition
- Index: `cookie.consent_records (tenant_id, domain_id)` · `cookie.consent_records (tenant_id, visitor_id)` · `cookie.consent_records (tenant_id, banner_config_id)` · `cookie.consent_records (tenant_id, ab_variant_id)` · `cookie.consent_records (tenant_id, subject_id)`
- RLS: tenant · RLS `tenant_isolation`
