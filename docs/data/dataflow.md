# schema `dataflow`

> แผนผังการไหลของข้อมูลและ data discovery  
> migration: `backend/db/migrations/00009_dataflow.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DFG](../modules/DFG.md) (`backend/internal/dataflow`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-09

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [layouts](#dataflow-layouts) | ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง | tenant · RLS `tenant_isolation` |  | ERD-09 |  |
| [snapshots](#dataflow-snapshots) | แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน) | tenant · RLS `tenant_isolation` |  | ERD-09 | BP-05 |
| [classifiers](#dataflow-classifiers) | classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย) | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-09 |  |
| [discovery_scans](#dataflow-discovery-scans) | รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector | tenant · RLS `tenant_isolation` |  | ERD-09 |  |
| [discovery_findings](#dataflow-discovery-findings) | ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง) | tenant · RLS `tenant_isolation` |  | ERD-09 |  |

<a id="dataflow-layouts"></a>
## dataflow.layouts

ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `view_type` | `text` | ✓ |  |  | ค่า: `org_unit`, `system`, `legal_entity`, `data_category`, `lineage`, `world` |
| `scope_id` | `uuid` |  |  |  |  |
| `layout` | `jsonb` | ✓ |  |  |  |
| `user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_layouts_updated`
- PK: `(id)`
- Index: `dataflow.layouts (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dataflow-snapshots"></a>
## dataflow.snapshots

แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `view_type` | `varchar(20)` | ✓ |  |  |  |
| `scope_id` | `uuid` |  |  |  |  |
| `version_no` | `int` | ✓ |  |  |  |
| `graph` | `jsonb` | ✓ |  |  |  |
| `image_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `published_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `published_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_snapshots_updated`
- PK: `(id)`
- Unique: `uq_snapshots_view_type_scope_id_version_no UNIQUE (tenant_id, view_type, scope_id, version_no)`
- Index: `dataflow.snapshots (tenant_id, image_file_id)` · `dataflow.snapshots (tenant_id, published_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="dataflow-classifiers"></a>
## dataflow.classifiers

classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `code` | `varchar(60)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `classifier_type` | `text` | ✓ |  |  | ค่า: `regex`, `checksum`, `dictionary`, `ml` |
| `pattern` | `text` |  |  |  |  |
| `data_category_id` | `uuid` |  |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_classifiers_updated`
- PK: `(id)`
- Unique: `uq_classifiers_code UNIQUE NULLS NOT DISTINCT (tenant_id, code)`
- Index: `dataflow.classifiers (data_category_id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `dataflow.discovery_findings.classifier_id`

<a id="dataflow-discovery-scans"></a>
## dataflow.discovery_scans

รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `connector_id` | `uuid` | ✓ |  | FK → [platform.connectors](platform.md#platform-connectors) |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `running`, `done`, `failed` |
| `started_at` | `timestamptz` |  |  |  |  |
| `finished_at` | `timestamptz` |  |  |  |  |
| `objects_scanned` | `int` |  |  |  |  |
| `findings_count` | `int` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_discovery_scans_updated`
- PK: `(id)`
- Index: `dataflow.discovery_scans (tenant_id, connector_id)` · `dataflow.discovery_scans (tenant_id, asset_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `dataflow.discovery_findings.scan_id`

<a id="dataflow-discovery-findings"></a>
## dataflow.discovery_findings

ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `scan_id` | `uuid` | ✓ |  | FK → [dataflow.discovery_scans](#dataflow-discovery-scans) |  |
| `object_path` | `text` | ✓ |  |  |  |
| `classifier_id` | `uuid` | ✓ |  | FK → [dataflow.classifiers](#dataflow-classifiers) |  |
| `match_ratio` | `numeric(5,2)` | ✓ |  |  |  |
| `sample_count` | `int` | ✓ |  |  |  |
| `suggested_category_id` | `uuid` |  |  | FK → [org.data_categories](org.md#org-data-categories) |  |
| `status` | `text` | ✓ | 'proposed' |  | ค่า: `proposed`, `accepted`, `rejected` |
| `reviewed_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_discovery_findings_updated`
- PK: `(id)`
- Index: `dataflow.discovery_findings (tenant_id, scan_id)` · `dataflow.discovery_findings (tenant_id, classifier_id)` · `dataflow.discovery_findings (tenant_id, suggested_category_id)` · `dataflow.discovery_findings (tenant_id, reviewed_by)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `ropa.data_inventory.discovered_by_finding_id`
