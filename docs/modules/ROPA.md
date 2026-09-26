# ROPA — บันทึกกิจกรรมการประมวลผล (RoPA)

> ระบบบริหารบันทึกกิจกรรมการประมวลผลข้อมูลส่วนบุคคล (Record of Processing Activities (RoPA)) · ขอบเขต: ทะเบียนข้อมูล ทะเบียนระบบ และบันทึกรายการตาม ม.39 ทั้งผู้ควบคุมและผู้ประมวลผล  
> 20 features · Must 11 / Should 8 / Nice 1 · phase: P1 (13), P3 (6), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/ropa` |
| PostgreSQL schema | [`ropa`](../data/ropa.md) (15 ตาราง) |
| Admin API prefix | `/admin/v1/ropa` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /admin/v1/ropa/activities` — สร้างกิจกรรม RoPA (BP-05)<br>`PATCH /admin/v1/ropa/activities/{id}` — แก้ไขกิจกรรม (If-Match) (SEQ-02)<br>`POST /admin/v1/ropa/activities/{id}/submit` — ส่งอนุมัติ (BP-05)<br>`POST /admin/v1/ropa/imports` — นำเข้า RoPA จาก Excel (BP-05) |
| หน้าจอ (Next.js) | admin: /ropa/* |
| พึ่งพาบริการ | forms, importer, report, events |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-06 ROPA, BP-05, BP-11, DFD-1, ERD-08, ERD-09, SEQ-02, ST-05 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| AUDIT | ผู้ตรวจสอบ | Auditor | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [ROPA-01](#ropa-01) | ทะเบียนข้อมูลส่วนบุคคล | Must | P1 | OWNER IT | M | M | Y | BP-05 |
| [ROPA-02](#ropa-02) | ทะเบียนระบบและทรัพย์สิน | Must | P1 | IT | M | M | Y | BP-05 |
| [ROPA-03](#ropa-03) | RoPA ของผู้ควบคุมข้อมูล | Must | P1 | OWNER DPO | L | L | Y | BP-05 |
| [ROPA-04](#ropa-04) | RoPA ของผู้ประมวลผลข้อมูล | Must | P1 | OWNER DPO | M | M | N | BP-05 |
| [ROPA-05](#ropa-05) | เพิ่มกิจกรรมแบบปกติและแบบมาตรฐาน | Must | P1 | OWNER | S | S | N | BP-05 |
| [ROPA-06](#ropa-06) | ฐานกฎหมายต่อวัตถุประสงค์ | Must | P1 | OWNER DPO | M | S | N | BP-05 |
| [ROPA-07](#ropa-07) | ระยะเวลาเก็บรักษาและวิธีทำลาย | Must | P1 | OWNER | S | S | N | BP-05 |
| [ROPA-08](#ropa-08) | ผู้รับข้อมูลและการโอนต่างประเทศ | Must | P1 | OWNER | M | M | N | BP-05 |
| [ROPA-09](#ropa-09) | มาตรการความปลอดภัยต่อกิจกรรม | Must | P1 | IT DPO | S | S | N | BP-05 |
| [ROPA-10](#ropa-10) | บันทึกการปฏิเสธคำขอใช้สิทธิ | Must | P1 | DPO SCHED | S | S | N | BP-05, BP-11 |
| [ROPA-13](#ropa-13) | เวอร์ชันและการอนุมัติ | Should | P1 | DPO | S | S | N | BP-05 |
| [ROPA-16](#ropa-16) | ออกรายงาน RoPA | Must | P1 | DPO AUDIT | M | S | N | BP-05 |
| [ROPA-18](#ropa-18) | นำเข้า RoPA จาก Excel เดิม | Should | P1 | DPO | M | S | N | BP-05 |
| [ROPA-11](#ropa-11) | แบบสอบถามเก็บข้อมูลจากหน่วยงาน | Should | P3 | DPO OWNER | M | S | N | BP-05 |
| [ROPA-12](#ropa-12) | ถาม-ตอบกับ DPO หรือที่ปรึกษา | Should | P3 | OWNER DPO | XS | S | N | BP-05 |
| [ROPA-14](#ropa-14) | วงจรชีวิตของกิจกรรม | Should | P3 | OWNER | S | XS | N | BP-05 |
| [ROPA-15](#ropa-15) | แจ้งเตือนทบทวน RoPA ตามรอบ | Should | P3 | SCHED OWNER | S | XS | N | BP-05 |
| [ROPA-17](#ropa-17) | แดชบอร์ด RoPA | Should | P3 | DPO EXEC | S | M | Y | BP-05 |
| [ROPA-19](#ropa-19) | เชื่อม RoPA กับโมดูลอื่น | Should | P3 | SCHED | M | S | N | BP-05 |
| [ROPA-20](#ropa-20) | ตรวจสิทธิ์ยกเว้นกิจการขนาดเล็ก | Nice | P4 | DPO | S | S | N | BP-05 |

### ความสัมพันธ์ระหว่าง use case

- ROPA-05 «extend» ROPA-03 (ROPA-05 เป็นทางเลือก/ส่วนขยายของ ROPA-03)
- ROPA-03 «include» ROPA-06 (ทุกครั้งที่ทำ ROPA-03 ต้องทำ ROPA-06)
- ROPA-03 «include» ROPA-08 (ทุกครั้งที่ทำ ROPA-03 ต้องทำ ROPA-08)
- ROPA-11 «extend» ROPA-03 (ROPA-11 เป็นทางเลือก/ส่วนขยายของ ROPA-03)

## กระบวนการ / sequence / state machine

- [BP-05 จัดทำและอนุมัติ RoPA (Record of Processing Activities)](../processes/BP-05.md)
- [BP-11 ระยะเวลาเก็บรักษาและการทำลายข้อมูล (Retention & disposal)](../processes/BP-11.md)
- [SEQ-02 การตรวจสิทธิ์ต่อ request (x-permission + data scope + RLS + optimistic lock)](../sequences/SEQ-02.md)
- [ST-05 กิจกรรม RoPA และแบบประเมิน (DPIA / LIA / TIA)](../states/ST-05.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [ropa.processing_activities](../data/ropa.md#ropa-processing-activities) | กิจกรรมการประมวลผล (RoPA ม.39 / ผู้ประมวลผล) |
| [ropa.activity_purposes](../data/ropa.md#ropa-activity-purposes) | วัตถุประสงค์และฐานกฎหมายของกิจกรรม |
| [ropa.activity_data](../data/ropa.md#ropa-activity-data) | ข้อมูลที่เก็บในกิจกรรม |
| [ropa.activity_systems](../data/ropa.md#ropa-activity-systems) | ระบบ / asset ที่ใช้ในกิจกรรม |
| [ropa.activity_recipients](../data/ropa.md#ropa-activity-recipients) | ผู้รับข้อมูลและการเปิดเผย (ม.27) |
| [ropa.activity_transfers](../data/ropa.md#ropa-activity-transfers) | การโอนไปต่างประเทศ (ม.28 / ม.29) |
| [ropa.retention_rules](../data/ropa.md#ropa-retention-rules) | ระยะเวลาเก็บรักษาและวิธีทำลาย |
| [ropa.activity_controls](../data/ropa.md#ropa-activity-controls) | มาตรการความปลอดภัยต่อกิจกรรม (ม.37(1)) |
| [ropa.activity_rejections](../data/ropa.md#ropa-activity-rejections) | การปฏิเสธคำขอใช้สิทธิที่เกี่ยวข้อง (ม.39(7)) |
| [ropa.assets](../data/ropa.md#ropa-assets) | ทะเบียนระบบ / asset |
| [ropa.data_inventory](../data/ropa.md#ropa-data-inventory) | ทะเบียนข้อมูลส่วนบุคคลต่อระบบ |
| [ropa.questionnaires](../data/ropa.md#ropa-questionnaires) | แบบสอบถามเก็บข้อมูลกิจกรรมจากหน่วยงาน |
| [ropa.sme_exemption_checks](../data/ropa.md#ropa-sme-exemption-checks) | ผลตรวจสิทธิ์ยกเว้น RoPA ของกิจการขนาดเล็ก |
| [ropa.generation_runs](../data/ropa.md#ropa-generation-runs) | การสร้างร่าง RoPA จาก template ครั้งละหลายกิจกรรม |
| [ropa.wizard_sessions](../data/ropa.md#ropa-wizard-sessions) | session ของ wizard ถาม-ตอบภาษาง่าย |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `ropa.inventory.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `ropa.inventory` | ทะเบียนข้อมูลและ asset | DPO `CRUDA` · PRIVACY `CRUD` · LEGAL `R` · OWNER `CRU` · IT `CRU` · SEC `R` · AUDIT `R` | Process Owner เฉพาะแผนกตนเอง |
| `ropa.activity` | กิจกรรมการประมวลผล (RoPA) | DPO `CRUDAE` · PRIVACY `CRUE` · LEGAL `R` · OWNER `CRU` · IT `R` · MKT `R` · SEC `R` · AUDIT `RE` · EXEC `R` | Process Owner เฉพาะแผนกตนเอง |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `ropa.activity_submitted` | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |
| `ropa.activity_approved` | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |
| `ropa.activity_changed` | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** ROPA-01, ROPA-02, ROPA-03, ROPA-04, ROPA-05, ROPA-06, ROPA-07, ROPA-08, ROPA-09, ROPA-10, ROPA-16, ROPA-13, ROPA-18
- **P3:** ROPA-11, ROPA-12, ROPA-14, ROPA-15, ROPA-17, ROPA-19
- **P4:** ROPA-20

## รายละเอียด feature

<a id="ropa-01"></a>
### ROPA-01 ทะเบียนข้อมูลส่วนบุคคล

*Personal data inventory*

- **Priority / Phase:** Must · P1 · กลุ่ม: ทะเบียน
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ORG-07
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ประเภทข้อมูล หมวดทั่วไป/อ่อนไหว (ม.26) แหล่งที่มา ระบบที่จัดเก็บ และผู้รับผิดชอบ

**Backend (Go):** ทะเบียนข้อมูล: ประเภท / หมวดทั่วไป-อ่อนไหว (ม.26), แหล่งที่มา, ระบบที่จัดเก็บ, ผู้รับผิดชอบ; scope ตามหน่วยงาน

**Frontend (Next.js):** หน้ารายการ + ฟอร์ม + ตัวกรองข้อมูลอ่อนไหว

**Acceptance criteria:** ข้อมูลอ่อนไหวถูกระบุและกรองได้ทุกหน่วยงาน

**Implementation (ROPA-01):** `backend/internal/ropa/service/inventory.go` (`ropa.inventory.*`, shared with
ROPA-02) — CRUD on `ropa.data_inventory`, already fully specified in the baseline migrations (asset_id/
data_category_id NOT NULL, org_unit_id/owner_user_id/discovered_by_finding_id nullable) — no new migration.
The acceptance criterion's sensitive-data flag comes straight from ORG-07's `org.data_categories.is_sensitive`
(no new column): `ListDataInventory`'s query joins it in the same transaction (its RLS already allows global
defaults + the tenant's own rows), so every row carries `is_sensitive`/`sensitive_type`/`category_name_*`
without a second round trip. `sensitive_only=true` on `GET /admin/v1/ropa/data-inventory` searches every
department at once (no `org_unit_id` filter applied) — the literal "filterable across every department"; an
`org_unit_id` filter narrows to one department when wanted. Duplicate detection isn't part of this
acceptance criterion, so unlike ORG-06 there's no dedupe step. FK visibility checks follow the ROPA-02
pattern: `asset_id` through `Service.GetAsset` (same package), `data_category_id` through a newly exported
`orgservice.GetMaster` (was a private `masterItem()` helper — same "export what another module needs" move as
ROPA-02's `GetOrgUnit`), `org_unit_id`/`owner_user_id` reusing the exact same checks ROPA-02 already has.
`discovered_by_finding_id` (FK to `dataflow.discovery_findings`) is left alone — the `dataflow` module (automated
discovery scans) doesn't exist yet, so there's nothing to link to; add it when that module ships. API
`/admin/v1/ropa/data-inventory` (cursor pagination), `/{id}`. UI `/ropa/data-inventory` (sensitive-only
toggle, department filter, form with an asset/category/unit picker — the sensitive flag shows inline next to
each category option and as a badge on sensitive rows). Tests: unit (validation, the four FK-visibility
checks, update, the acceptance criterion directly — two departments each with a sensitive entry, confirming
`sensitive_only` returns both — two-tenant isolation), HTTP contract (401/403/400 schema/422/412/428).

<a id="ropa-02"></a>
### ROPA-02 ทะเบียนระบบและทรัพย์สิน

*System / asset register*

- **Priority / Phase:** Must · P1 · กลุ่ม: ทะเบียน
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1), ม.39
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ORG-06
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ระบบ แอปพลิเคชัน ผู้ให้บริการ และประเทศที่ตั้งเซิร์ฟเวอร์

**Backend (Go):** asset: ระบบ แอป ฐานข้อมูล ผู้ให้บริการ ประเทศที่ตั้งเซิร์ฟเวอร์ เจ้าของระบบ

**Frontend (Next.js):** หน้าทะเบียนระบบ + ฟอร์ม

**Acceptance criteria:** กิจกรรมอ้างถึงระบบจากทะเบียนเดียวกัน

**Implementation (ROPA-02):** `backend/internal/ropa` — the first ropa-schema module built (`ropa.inventory.*`,
a permission code already shared with ROPA-01's future data inventory). CRUD on `ropa.assets`, which the
baseline migrations already had (asset_type, org_unit_id/owner_user_id/provider_party_id/hosting_country_code
all real FKs) — no new migration needed. Built ahead of ROPA-01 even though the backlog's `depends_on` only
lists ORG-07 for it: `ropa.data_inventory.asset_id` is a NOT NULL FK to `ropa.assets`, so ROPA-01 cannot be
built first — this asset register has to exist before there is anything for a data-inventory row to point at.
`org_unit_id` and `provider_party_id` are FKs that bypass RLS, so `SaveAsset` verifies each is visible under
the caller's RLS before writing it (rule 1) through org's own exported service (`orgservice.Service.GetOrgUnit`
— newly exported, was a private `unit()` helper — and the already-exported `GetExternalParty`, rule 9);
`owner_user_id` is checked through `iamservice.Names`, the same cross-module helper BRE already uses.
`hosting_country_code`'s FK violation is mapped to a friendly 422 the same way ORG-06 does for its own country
code. API `/admin/v1/ropa/assets` (cursor pagination, same shape as ORG-06/PLT-16's lists), `/{id}`. UI
`/ropa/assets` (list + filter + form; the org-unit picker needs a legal entity chosen first, same two-step
pattern as `/settings/organization`; owner_user_id has no field yet — no user directory UI exists until
IAM-01/ORG-09). Tests: unit (validation, FK visibility checks for org_unit_id/provider_party_id/owner_user_id,
update, two-tenant isolation), HTTP contract (401/403/400 schema/422/412/428).

<a id="ropa-03"></a>
### ROPA-03 RoPA ของผู้ควบคุมข้อมูล

*RoPA – controller*

- **Priority / Phase:** Must · P1 · กลุ่ม: บันทึกรายการ
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-01, ROPA-02, PLT-06
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** บันทึกครบ 8 รายการตาม ม.39: ข้อมูลที่เก็บ วัตถุประสงค์ ข้อมูลผู้ควบคุม ระยะเวลาเก็บ สิทธิและวิธีเข้าถึง การใช้/เปิดเผยที่ยกเว้นความยินยอม การปฏิเสธคำขอ และมาตรการความปลอดภัย

**Backend (Go):** processing activity ครบ 8 หัวข้อ ม.39 เป็นข้อมูลโครงสร้าง ผูก master data, scope ตามแผนก, คะแนนความครบถ้วน

**Frontend (Next.js):** ฟอร์มหลายขั้นตอน + มุมมองตาราง + ตัวบ่งชี้ความครบถ้วน

**Acceptance criteria:** กิจกรรมที่ขาดหัวข้อบังคับแสดงสถานะ 'ไม่ครบ' พร้อมรายการที่ขาด

**Implementation (ROPA-03):** `backend/internal/ropa/service/activities.go` (`ropa.activity.*`, permission codes already
seeded in the baseline migrations) — CRUD on `ropa.processing_activities` plus four child tables
(`activity_purposes`, `activity_data`, `retention_rules`, `activity_recipients`), all already fully specified
in the baseline migrations — no new migration. Scoped to exactly this feature's acceptance criterion and BP-05
rules 1–2: full ST-05 approval (`pending_approval` → `active`) is ROPA-13's job (versioning & approval, PLT-08,
a separate Should feature this doesn't depend on); recipients/transfers with country-adequacy checks is ROPA-08's
job (this builds the recipients table generically, ROPA-08 adds transfers on top); retention policy automation is
ROPA-07's; security-control linking (`activity_controls` → `risk.controls`) and DSAR-linked rejections
(`activity_rejections` → `dsar.requests`) are deferred entirely — neither the risk-control library nor the DSAR
module exists yet (same "don't build against tables nothing can populate" reasoning as ROPA-01's
`discovered_by_finding_id`).

Completeness (the acceptance criterion) is computed live on every `GetActivity` — never persisted from a plain
read, only from mutations (see below) — against 5 fixed items (data, purpose, controller, retention,
rights_and_access; "controller" only applies when `role=processor` and no `controller_party_id`) plus two
conditional ones counted in the missing-item list but not the score denominator: a recipient missing
`disclosure_basis`, and sensitive data (`activity_data.is_sensitive`, from ORG-07 as in ROPA-01) with no purpose
carrying `consent_purpose_id` (BP-05 rule 2's explicit-consent evidence — checked via a newly exported
`consentservice.GetPurpose`, the first cross-module read from `ropa` into `consent`). `POST …/submit` (draft/
under_review → pending_approval, ST-05) refuses with the itemized list (`ropa.activity_incomplete`, 422, same
`FieldError` pattern as PLT-16's `docs.incomplete`) while anything is missing, and 409 `ropa.invalid_transition`
from any other status.

Real bug found and fixed while testing: `SetActivityCompleteness`'s UPDATE ran through the table's ordinary
`row_version`-bumping trigger, so a plain `GetActivity` (no user edit) silently invalidated the caller's ETag —
fixed by making the completeness computation pure and persisting it only from mutation paths (`SaveActivity`,
and each child add/delete), which already legitimately bump the version. Second bug: a duplicate `code` hit the
table's real unique constraint and aborted the whole request transaction (no savepoint), corrupting every later
statement in the same tx until commit failed with `ErrTxCommitRollback` — fixed by wrapping the insert/update in
`pdb.Savepoint`, the same pattern `org.SaveLegalEntity` already uses for its own unique-constraint check.

API `/admin/v1/ropa/activities` (cursor pagination), `/{id}`, `/{id}/submit`, and one list+create+delete triple
per child table (`/purposes`, `/data`, `/retention-rules`, `/recipients` — no per-row update; editing a child is
delete+recreate, keeping the sub-resource surface small). UI `/ropa/activities` (list with a completeness badge)
and `/ropa/activities/{id}` (core-field form, missing-items banner, one section per child table with inline
add/remove, submit button — editable only while `status=draft`). Tests: unit (validation, all four FK-visibility
checks, the acceptance criterion — an activity missing items shows them and clears them one at a time as each is
filled in — processor-needs-controller, sensitive-data-needs-consent-evidence, submit blocked while incomplete
with the itemized list, two-tenant isolation), HTTP contract (401/403/400 schema/422/428).

<a id="ropa-04"></a>
### ROPA-04 RoPA ของผู้ประมวลผลข้อมูล

*RoPA – processor*

- **Priority / Phase:** Must · P1 · กลุ่ม: บันทึกรายการ
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40(3); ประกาศ สคส. RoPA ผู้ประมวลผล พ.ศ. 2565
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ROPA-03
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** บันทึกกิจกรรมของผู้ประมวลผลตามประกาศ สคส.

**Backend (Go):** โหมดผู้ประมวลผล: ฟิลด์ตามประกาศ สคส. RoPA ผู้ประมวลผล พ.ศ. 2565

**Frontend (Next.js):** ฟอร์มและรายการ RoPA ผู้ประมวลผล

**Acceptance criteria:** ส่งออก RoPA ผู้ประมวลผลได้ครบหัวข้อตามประกาศ

<a id="ropa-05"></a>
### ROPA-05 เพิ่มกิจกรรมแบบปกติและแบบมาตรฐาน

*Add activity (blank or template)*

- **Priority / Phase:** Must · P1 · กลุ่ม: บันทึกรายการ
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03, RTG-01
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** สร้างกิจกรรมเอง หรือเลือกจากคลังกิจกรรมมาตรฐานในโมดูล ROPA Template Generator

**Backend (Go):** สร้างกิจกรรมเปล่าหรือเลือกจากคลัง RTG

**Frontend (Next.js):** ปุ่มสร้างแบบเปล่า / จาก template

**Acceptance criteria:** สร้างจาก template แล้วได้ค่าตั้งต้นครบทุกหัวข้อ

<a id="ropa-06"></a>
### ROPA-06 ฐานกฎหมายต่อวัตถุประสงค์

*Lawful basis mapping*

- **Priority / Phase:** Must · P1 · กลุ่ม: ฐานกฎหมาย
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.24, ม.26
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03, CON-09
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ระบุฐาน ม.24/26 ต่อวัตถุประสงค์ และผูกกับ Purpose ในระบบ Consent กรณีใช้ฐานความยินยอม

**Backend (Go):** ฐาน ม.24/26 ต่อวัตถุประสงค์; ถ้าเป็นฐานความยินยอม → ผูก Purpose ใน CON และแสดงสถิติความยินยอม

**Frontend (Next.js):** เลือกฐานต่อวัตถุประสงค์ + ลิงก์ Purpose

**Acceptance criteria:** วัตถุประสงค์ที่ใช้ฐานความยินยอมต้องผูก Purpose ก่อนบันทึก

<a id="ropa-07"></a>
### ROPA-07 ระยะเวลาเก็บรักษาและวิธีทำลาย

*Retention & disposal method*

- **Priority / Phase:** Must · P1 · กลุ่ม: ฐานกฎหมาย
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(3), ม.39(4)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** กำหนดระยะเวลาเก็บและวิธีลบ/ทำลายต่อข้อมูลหรือกิจกรรม เชื่อมกับการแจ้งเตือนครบกำหนด

**Backend (Go):** ระยะเวลาเก็บ + เหตุผล / กฎหมายอ้างอิง + วิธีทำลาย ต่อข้อมูล/กิจกรรม; ส่งต่อ DPX-05

**Frontend (Next.js):** ส่วนระยะเวลาเก็บในฟอร์มกิจกรรม

**Acceptance criteria:** ทุกกิจกรรมมีระยะเวลาเก็บและวิธีทำลาย

<a id="ropa-08"></a>
### ROPA-08 ผู้รับข้อมูลและการโอนต่างประเทศ

*Recipients & cross-border transfers*

- **Priority / Phase:** Must · P1 · กลุ่ม: ฐานกฎหมาย
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.27-29, ม.39(6)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ORG-06, ORG-07
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ระบุผู้รับข้อมูล การเปิดเผยที่ยกเว้นความยินยอม และการโอนไปต่างประเทศพร้อมฐานการโอน

**Backend (Go):** ผู้รับ (จาก ORG-06), การเปิดเผยที่ยกเว้นความยินยอม (ม.27), การโอนต่างประเทศ + ฐานการโอน ม.28/29

**Frontend (Next.js):** ส่วนผู้รับและการโอนในฟอร์ม

**Acceptance criteria:** การโอนที่ไม่มีฐานการโอนถูกเตือน

**Implementation (ROPA-08):** `backend/internal/ropa/service/transfers.go` (`ropa.activity.*`, shared with ROPA-03)
— CRUD on `ropa.activity_transfers`, already fully specified in the baseline migrations — no new migration.
Recipients themselves (`activity_recipients`, with `disclosure_basis` for ม.27's consent-exempt disclosures)
were already built in ROPA-03 (documented there as "generic now, ROPA-08 adds transfers on top"); this feature
adds only the transfer half: `country_code` (validated against ORG-07's countries list — a new `Org.ListMaster`
lookup follows the exact `validLawfulBasis` pattern ROPA-03 already established for `lawful_basis_code`, since
both are code-keyed, not id-keyed), `transfer_basis` (ม.28/29's six mechanisms), `safeguards` free text, and an
optional link to one of the activity's own recipients (checked by scanning `ListActivityRecipients`, not a new
FK-visibility query).

Since `transfer_basis` is a NOT NULL enum column, an actual transfer row can never lack a basis — the acceptance
criterion ("a transfer without a basis is warned") is about the *implicit* transfer that isn't logged at all:
`docs/legal/pdpa-rules.md`'s ม.28 row is explicit that every real transfer in the RoPA needs its country and
mechanism recorded. So `completeness()` (ROPA-03's live, non-persisted check) gained one more conditional item:
for each recipient, look up its party's `country_code` via the already-shared `Org.GetExternalParty`, and if
that's a real country other than `TH` with no `activity_transfers` row referencing that recipient, flag
`transfer_basis` (once per activity, same one-flag-not-one-per-row pattern as `recipient_basis`) — it blocks
`/submit` exactly like ROPA-03's other conditional items. A party with an empty `country_code` (not required at
ORG-06) is treated as domestic rather than guessed at.

API `/admin/v1/ropa/activities/{id}/transfers` (list+create) and `/{transferId}` (delete) — same
list+create+delete shape as ROPA-03's other child tables, no per-row update. UI: a "การโอนข้อมูลไปต่างประเทศ"
section on the activity detail page (country/basis/safeguards + an optional recipient picker scoped to the
activity's own recipients). Tests: unit (validation — bad country code, unknown country, bad transfer basis,
a recipient from another activity refused — the acceptance criterion directly: a foreign recipient with no
transfer flags `transfer_basis`, adding one clears it, deleting the only one brings it back — two-tenant
isolation), HTTP contract (401/403/400 schema/422).

<a id="ropa-09"></a>
### ROPA-09 มาตรการความปลอดภัยต่อกิจกรรม

*Security measures per activity*

- **Priority / Phase:** Must · P1 · กลุ่ม: ฐานกฎหมาย
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1), ม.39(8)
- **Actor:** IT (เจ้าของระบบ / IT), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPO-09
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** คำอธิบายมาตรการตาม ม.37(1) ที่ใช้กับกิจกรรมนั้น อ้างอิงผลประเมินมาตรการ

**Backend (Go):** เลือกมาตรการจากคลัง control / ผลประเมิน DPO-09 + คำอธิบาย

**Frontend (Next.js):** ส่วนมาตรการความปลอดภัยในฟอร์ม

**Acceptance criteria:** ทุกกิจกรรมอ้างอิงมาตรการตาม ม.37(1)

<a id="ropa-10"></a>
### ROPA-10 บันทึกการปฏิเสธคำขอใช้สิทธิ

*Log of rejected requests*

- **Priority / Phase:** Must · P1 · กลุ่ม: ฐานกฎหมาย
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39(7)
- **Actor:** DPO (DPO / Privacy Team), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSAR-11
- **Process:** [BP-05](../processes/BP-05.md), [BP-11](../processes/BP-11.md)

**คำอธิบาย:** ดึงรายการปฏิเสธคำขอจากโมดูล DSAR มาแสดงใน RoPA ของกิจกรรมที่เกี่ยวข้อง

**Backend (Go):** event dsar.rejected → ผูกกับกิจกรรมที่เกี่ยวข้อง แสดงในหัวข้อ ม.39(7) และในรายงาน

**Frontend (Next.js):** แท็บการปฏิเสธคำขอในหน้ากิจกรรม

**Acceptance criteria:** คำขอที่ถูกปฏิเสธปรากฏใน RoPA ของกิจกรรมที่เกี่ยวข้องอัตโนมัติ

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="ropa-13"></a>
### ROPA-13 เวอร์ชันและการอนุมัติ

*Version control & approval*

- **Priority / Phase:** Should · P1 · กลุ่ม: Workflow
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** เก็บประวัติการแก้ไข เปรียบเทียบเวอร์ชัน และ workflow อนุมัติโดย DPO

**Backend (Go):** เวอร์ชัน + diff + อนุมัติโดย DPO

**Frontend (Next.js):** หน้าประวัติเวอร์ชัน + ปุ่มส่งอนุมัติ

**Acceptance criteria:** กิจกรรมที่อนุมัติแล้วแก้ได้ผ่านเวอร์ชันใหม่เท่านั้น

**หมายเหตุ:** ดึงเข้า P1 (ใช้ PLT-08)

<a id="ropa-16"></a>
### ROPA-16 ออกรายงาน RoPA

*RoPA export*

- **Priority / Phase:** Must · P1 · กลุ่ม: รายงาน
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), AUDIT (ผู้ตรวจสอบ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ส่งออก Excel/PDF/CSV ครบ 8 หัวข้อตาม ม.39 และพร้อมให้ สคส. ตรวจสอบเมื่อร้องขอ

**Backend (Go):** export Excel / PDF / CSV ครบ 8 หัวข้อ ม.39 (layout แบบที่ใช้ในไทย), เลือกบริษัท / แผนก, รูปแบบผู้ประมวลผลแยก

**Frontend (Next.js):** ปุ่มส่งออก + เลือกขอบเขต

**Acceptance criteria:** ไฟล์ที่ส่งออกพร้อมส่ง สคส. เมื่อร้องขอ

<a id="ropa-18"></a>
### ROPA-18 นำเข้า RoPA จาก Excel เดิม

*Bulk import*

- **Priority / Phase:** Should · P1 · กลุ่ม: นำเข้า
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-14
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** นำเข้า RoPA ที่องค์กรทำไว้ใน Excel พร้อมตรวจความครบถ้วนก่อนบันทึก

**Backend (Go):** template Excel ภาษาไทย, map คอลัมน์ ↔ ฟิลด์ RoPA, จับคู่ master data แบบ fuzzy, รายงานแถวที่ไม่ครบ

**Frontend (Next.js):** wizard นำเข้า RoPA

**Acceptance criteria:** นำเข้า RoPA 500 กิจกรรมจาก Excel ได้ และรายงานรายการที่ต้องแก้

**หมายเหตุ:** ดึงเข้า P1 เพื่อให้ลูกค้าย้ายจาก Excel ได้ทันที (T26)

<a id="ropa-11"></a>
### ROPA-11 แบบสอบถามเก็บข้อมูลจากหน่วยงาน

*Data mapping questionnaire*

- **Priority / Phase:** Should · P3 · กลุ่ม: Workflow
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06, ROPA-03
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ส่งแบบสอบถามให้แผนกกรอก แล้วนำคำตอบที่ DPO อนุมัติเข้าสู่ RoPA อัตโนมัติ

**Backend (Go):** ส่งแบบสอบถาม (PLT-06) ให้แผนก, map คำตอบ → ฟิลด์ RoPA, DPO อนุมัติก่อนบันทึก

**Frontend (Next.js):** หน้าส่งแบบสอบถามและติดตามผู้ตอบ

**Acceptance criteria:** คำตอบที่อนุมัติแล้วเข้า RoPA อัตโนมัติ

<a id="ropa-12"></a>
### ROPA-12 ถาม-ตอบกับ DPO หรือที่ปรึกษา

*Activity Q&A*

- **Priority / Phase:** Should · P3 · กลุ่ม: Workflow
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-07
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ส่งคำถามเพิ่มเติมในแต่ละกิจกรรม ตอบกลับเป็นข้อความ และตั้งค่าการแจ้งเตือนสถานะ

**Backend (Go):** ใช้ comment / mention กลาง + สถานะคำถาม (เปิด / ตอบแล้ว)

**Frontend (Next.js):** แท็บถาม-ตอบในหน้ากิจกรรม

**Acceptance criteria:** คำถามที่ยังไม่ตอบแสดงในรายการงานของผู้รับ

<a id="ropa-14"></a>
### ROPA-14 วงจรชีวิตของกิจกรรม

*Activity lifecycle*

- **Priority / Phase:** Should · P3 · กลุ่ม: Workflow
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** ROPA-03
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** สถานะ ร่าง → ใช้งาน → สิ้นสุดการประมวลผล ปิดกิจกรรมที่เลิกใช้โดยเก็บประวัติไว้

**Backend (Go):** สถานะ ร่าง → ใช้งาน → สิ้นสุด, ปิดโดยเก็บประวัติและเหตุผล

**Frontend (Next.js):** ปุ่มเปลี่ยนสถานะ + ตัวกรอง

**Acceptance criteria:** กิจกรรมที่สิ้นสุดไม่ปรากฏในรายงานปัจจุบันแต่ค้นย้อนหลังได้

<a id="ropa-15"></a>
### ROPA-15 แจ้งเตือนทบทวน RoPA ตามรอบ

*Periodic review reminder*

- **Priority / Phase:** Should · P3 · กลุ่ม: Workflow
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** SCHED (ระบบ: Scheduler / Event), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-10
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** แจ้งเจ้าของกิจกรรมให้ทบทวนตามรอบ หรือเมื่อกระบวนการเปลี่ยน

**Backend (Go):** รอบทบทวนต่อกิจกรรม, River cron แจ้งเจ้าของ

**Frontend (Next.js):** ตั้งรอบทบทวนในหน้ากิจกรรม

**Acceptance criteria:** เจ้าของกิจกรรมได้รับงานทบทวนตามรอบ

<a id="ropa-17"></a>
### ROPA-17 แดชบอร์ด RoPA

*RoPA dashboard*

- **Priority / Phase:** Should · P3 · กลุ่ม: รายงาน
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** จำนวนกิจกรรมตามหน่วยงาน สถานะ ฐานกฎหมาย และข้อมูลอ่อนไหว

**Backend (Go):** API สถิติกิจกรรม / สถานะ / ฐาน / ข้อมูลอ่อนไหว ตามหน่วยงาน

**Frontend (Next.js):** dashboard RoPA

**Acceptance criteria:** ตัวเลขใน dashboard ตรงกับทะเบียน

<a id="ropa-19"></a>
### ROPA-19 เชื่อม RoPA กับโมดูลอื่น

*Cross-module links*

- **Priority / Phase:** Should · P3 · กลุ่ม: เชื่อมต่อ
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-11
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** เมื่อ RoPA เปลี่ยน แจ้งให้ทบทวนประกาศ DPIA แผนผัง และ Purpose ที่เกี่ยวข้อง

**Backend (Go):** event ropa.updated → งานทบทวนใน Notice / DPIA / Data Flow / Purpose ที่เกี่ยวข้อง

**Frontend (Next.js):** แท็บความเชื่อมโยงในหน้ากิจกรรม

**Acceptance criteria:** แก้ RoPA แล้วเจ้าของเอกสารที่เกี่ยวข้องได้รับงานทบทวน

<a id="ropa-20"></a>
### ROPA-20 ตรวจสิทธิ์ยกเว้นกิจการขนาดเล็ก

*SME exemption check*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ยกเว้น
- **ที่มา:** Function List: 03_RoPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศ สคส. ยกเว้นกิจการขนาดเล็ก พ.ศ. 2565 และ พ.ศ. 2567
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ประเมินว่าได้รับยกเว้นการทำ RoPA หรือไม่ ตามประกาศยกเว้นของผู้ควบคุม (2565) และผู้ประมวลผล (2567)

**Backend (Go):** แบบประเมินตามประกาศยกเว้นกิจการขนาดเล็ก 2565 / 2567 (PLT-06) + ผลและเหตุผล

**Frontend (Next.js):** wizard ตรวจสิทธิ์ยกเว้น

**Acceptance criteria:** ผลประเมินระบุว่าได้รับยกเว้นหรือไม่พร้อมเหตุผลอ้างประกาศ

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)
