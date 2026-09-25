# schema `agreement`

> ข้อตกลง DPA / DSA (agreement engine)  
> migration: `backend/db/migrations/00015_agreement.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [DPA](../modules/DPA.md) (`backend/internal/agreement`), [DSA](../modules/DSA.md) (`backend/internal/agreement`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-13

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [agreements](#agreement-agreements) | ข้อตกลง DPA / DSA / ผู้ควบคุมร่วม / DPA ขาเข้า | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-09, BP-10, SEQ-07 |
| [parties](#agreement-parties) | คู่สัญญาในข้อตกลง (หลายฝ่าย) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |
| [agreement_activities](#agreement-agreement-activities) | กิจกรรม RoPA และเส้นทางข้อมูลที่ข้อตกลงครอบคลุม | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |
| [clauses](#agreement-clauses) | clause ในข้อตกลง (อ้างอิงคลังกลาง + ข้อความที่ปรับ) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10, SEQ-07 |
| [mandatory_rules](#agreement-mandatory-rules) | กฎ clause บังคับ (ม.27, ม.28-29, ม.37(2), ม.40) | global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป) |  | ERD-13 | BP-10, SEQ-07 |
| [annexes](#agreement-annexes) | ภาคผนวก (รายละเอียดการประมวลผล รายการข้อมูล มาตรการ แผนผัง) | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10, SEQ-07 |
| [signature_requests](#agreement-signature-requests) | การส่งลงนามอิเล็กทรอนิกส์ | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |
| [obligations](#agreement-obligations) | ภาระผูกพันตามข้อตกลง | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |
| [return_confirmations](#agreement-return-confirmations) | ใบยืนยันการคืน / ทำลายข้อมูลเมื่อสิ้นสุด | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |
| [downloads](#agreement-downloads) | ประวัติการดาวน์โหลดเอกสารสัญญา | tenant · RLS `tenant_isolation` |  | ERD-13 | BP-10 |

<a id="agreement-agreements"></a>
## agreement.agreements

ข้อตกลง DPA / DSA / ผู้ควบคุมร่วม / DPA ขาเข้า

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_type` | `text` | ✓ |  |  | ค่า: `dpa`, `dsa`, `joint_controller`, `inbound_dpa` |
| `agreement_no` | `varchar(40)` | ✓ |  | UQ |  |
| `title` | `text` | ✓ |  |  |  |
| `our_role` | `text` | ✓ |  |  | ค่า: `controller`, `processor`, `joint_controller` |
| `counterparty_id` | `uuid` | ✓ |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `vendor_id` | `uuid` |  |  | FK → [vendor.vendors](vendor.md#vendor-vendors) |  |
| `template_id` | `uuid` |  |  | FK → [platform.templates](platform.md#platform-templates) |  |
| `document_id` | `uuid` | ✓ |  | FK → [platform.documents](platform.md#platform-documents) |  |
| `sharing_direction` | `text` |  |  |  | ค่า: `one_way`, `two_way` |
| `is_government` | `boolean` | ✓ | false |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `in_review`, `approved`, `out_for_signature`, `active`, `expired`, `terminated` · state machine [ST-04](../states/ST-04.md) |
| `effective_from` | `date` |  |  |  |  |
| `effective_to` | `date` |  |  | IX |  |
| `auto_renew` | `boolean` | ✓ | false |  |  |
| `renewal_notice_days` | `smallint` | ✓ | 60 |  |  |
| `signed_at` | `timestamptz` |  |  |  |  |
| `terminated_at` | `timestamptz` |  |  |  |  |
| `termination_reason` | `text` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_agreements_updated`
- PK: `(id)`
- Unique: `uq_agreements_agreement_no UNIQUE (tenant_id, agreement_no)`
- Index: `agreement.agreements (tenant_id, counterparty_id)` · `agreement.agreements (tenant_id, vendor_id)` · `agreement.agreements (tenant_id, template_id)` · `agreement.agreements (tenant_id, document_id)` · `agreement.agreements (tenant_id, effective_to)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `ropa.activity_recipients.agreement_id`, `agreement.parties.agreement_id`, `agreement.agreement_activities.agreement_id`, `agreement.clauses.agreement_id`, `agreement.annexes.agreement_id`, `agreement.signature_requests.agreement_id`, `agreement.obligations.agreement_id`, `agreement.return_confirmations.agreement_id`, `agreement.downloads.agreement_id`

<a id="agreement-parties"></a>
## agreement.parties

คู่สัญญาในข้อตกลง (หลายฝ่าย)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `legal_entity_id` | `uuid` |  |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `party_role` | `text` | ✓ |  |  | ค่า: `disclosing`, `receiving`, `joint_controller`, `controller`, `processor` |
| `signatory_name` | `text` |  |  |  |  |
| `signatory_email` | `citext` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_parties_updated`
- PK: `(id)`
- Index: `agreement.parties (tenant_id, agreement_id)` · `agreement.parties (tenant_id, party_id)` · `agreement.parties (tenant_id, legal_entity_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-agreement-activities"></a>
## agreement.agreement_activities

กิจกรรม RoPA และเส้นทางข้อมูลที่ข้อตกลงครอบคลุม

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `agreement_id` | `uuid` | ✓ |  | PK · FK → [agreement.agreements](#agreement-agreements) |  |
| `activity_id` | `uuid` | ✓ |  | PK · FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `recipient_id` | `uuid` |  |  | FK → [ropa.activity_recipients](ropa.md#ropa-activity-recipients) |  |

- PK: `(agreement_id, activity_id)`
- Index: `agreement.agreement_activities (tenant_id, activity_id)` · `agreement.agreement_activities (tenant_id, recipient_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-clauses"></a>
## agreement.clauses

clause ในข้อตกลง (อ้างอิงคลังกลาง + ข้อความที่ปรับ)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `clause_id` | `uuid` |  |  | FK → [platform.clause_library](platform.md#platform-clause-library) |  |
| `clause_version_no` | `int` |  |  |  |  |
| `position` | `smallint` | ✓ |  |  |  |
| `is_mandatory` | `boolean` | ✓ | false |  |  |
| `customized_body` | `jsonb` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_clauses_updated`
- PK: `(id)`
- Index: `agreement.clauses (tenant_id, agreement_id)` · `agreement.clauses (tenant_id, clause_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-mandatory-rules"></a>
## agreement.mandatory_rules

กฎ clause บังคับ (ม.27, ม.28-29, ม.37(2), ม.40)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `agreement_type` | `varchar(20)` | ✓ |  |  |  |
| `clause_code` | `varchar(80)` | ✓ |  |  |  |
| `legal_ref` | `text` | ✓ |  |  |  |
| `condition` | `jsonb` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_mandatory_rules_updated`
- PK: `(id)`
- RLS: global · ไม่มี RLS (อ่านอย่างเดียวสำหรับแอป)

<a id="agreement-annexes"></a>
## agreement.annexes

ภาคผนวก (รายละเอียดการประมวลผล รายการข้อมูล มาตรการ แผนผัง)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `annex_type` | `text` | ✓ |  |  | ค่า: `processing_schedule`, `data_list`, `security_measures`, `request_form`, `flow_diagram`, `transfer_clauses` |
| `content` | `jsonb` |  |  |  |  |
| `file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_annexes_updated`
- PK: `(id)`
- Index: `agreement.annexes (tenant_id, agreement_id)` · `agreement.annexes (tenant_id, file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-signature-requests"></a>
## agreement.signature_requests

การส่งลงนามอิเล็กทรอนิกส์

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `provider` | `varchar(40)` | ✓ |  |  |  |
| `envelope_id` | `varchar(120)` | ✓ |  |  |  |
| `status` | `text` | ✓ | 'sent' |  | ค่า: `sent`, `viewed`, `signed`, `declined`, `expired` |
| `sent_at` | `timestamptz` | ✓ |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `signed_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_signature_requests_updated`
- PK: `(id)`
- Index: `agreement.signature_requests (tenant_id, agreement_id)` · `agreement.signature_requests (tenant_id, signed_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-obligations"></a>
## agreement.obligations

ภาระผูกพันตามข้อตกลง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `clause_ref` | `varchar(80)` |  |  |  |  |
| `obligation_type` | `text` | ✓ |  |  | ค่า: `delete_on_termination`, `usage_report`, `periodic_review`, `breach_notice`, `audit_right`, `other` |
| `next_due_at` | `date` |  |  | IX |  |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `done`, `waived` |
| `task_id` | `uuid` |  |  | FK → [dpo.tasks](dpo.md#dpo-tasks) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_obligations_updated`
- PK: `(id)`
- Index: `agreement.obligations (tenant_id, agreement_id)` · `agreement.obligations (tenant_id, next_due_at)` · `agreement.obligations (tenant_id, owner_user_id)` · `agreement.obligations (tenant_id, task_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-return-confirmations"></a>
## agreement.return_confirmations

ใบยืนยันการคืน / ทำลายข้อมูลเมื่อสิ้นสุด

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `requested_at` | `timestamptz` | ✓ |  |  |  |
| `guest_token_id` | `uuid` |  |  | FK → [iam.guest_tokens](iam.md#iam-guest-tokens) |  |
| `certificate_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |
| `confirmed_at` | `timestamptz` |  |  |  |  |
| `status` | `text` | ✓ | 'requested' |  | ค่า: `requested`, `received`, `overdue` |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_return_confirmations_updated`
- PK: `(id)`
- Index: `agreement.return_confirmations (tenant_id, agreement_id)` · `agreement.return_confirmations (tenant_id, guest_token_id)` · `agreement.return_confirmations (tenant_id, certificate_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="agreement-downloads"></a>
## agreement.downloads

ประวัติการดาวน์โหลดเอกสารสัญญา

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `agreement_id` | `uuid` | ✓ |  | FK → [agreement.agreements](#agreement-agreements) |  |
| `document_version_id` | `uuid` | ✓ |  | FK → [platform.document_versions](platform.md#platform-document-versions) |  |
| `user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `downloaded_at` | `timestamptz` | ✓ | now() |  |  |
| `ip` | `inet` |  |  |  |  |

- PK: `(id)`
- Index: `agreement.downloads (tenant_id, agreement_id)` · `agreement.downloads (tenant_id, document_version_id)` · `agreement.downloads (tenant_id, user_id)`
- RLS: tenant · RLS `tenant_isolation`
