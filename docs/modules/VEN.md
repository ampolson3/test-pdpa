# VEN — ประเมินคู่ค้า (Vendor Assessment)

> ระบบแบบประเมินคู่ค้า/คู่สัญญา (Vendor Assessment Module) · ขอบเขต: ทะเบียนและประเมินคู่ค้า/ผู้ประมวลผลตลอดวงจร ตั้งแต่คัดเลือกจนยุติการใช้บริการ  
> 15 features · Must 7 / Should 7 / Nice 1 · phase: P2 (7), P3 (7), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/vendormgmt` — **not** `backend/internal/vendor` as originally planned: a directory literally named `vendor` anywhere under the module triggers Go's own vendoring-directory import rewriting (`pdpa-platform/internal/vendor/store` fails to build with "must be imported as store"), discovered while building VEN-01. The PostgreSQL schema, permission codes (`vendor.*`) and API prefix (`/admin/v1/vendors`) are unaffected — only the Go source directory moved. |
| PostgreSQL schema | [`vendor`](../data/vendor.md) (7 ตาราง) |
| Admin API prefix | `/admin/v1/vendors` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /admin/v1/vendors/intakes` — คำขอรับคู่ค้าใหม่ (BP-09) |
| หน้าจอ (Next.js) | admin: /vendors/*; portal: /guest/vendor/[token] |
| พึ่งพาบริการ | forms, risk, guest access |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-14 VEN, BP-09, DFD-1, ERD-13, ST-06 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| PROC | จัดซื้อ / ผู้ดูแลคู่ค้า | Procurement / Vendor Manager | OIDC SSO · Admin app |
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | Guest link (token หมดอายุ) + OTP |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| SEC | ทีม Security / Incident | Security / Incident Response | OIDC SSO + MFA · Admin app |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [VEN-01](#ven-01) | ทะเบียนคู่ค้าและผู้ประมวลผล | Must | P2 | PROC OWNER | M | M | Y | BP-09 |
| [VEN-02](#ven-02) | จัดระดับความเสี่ยงคู่ค้า | Must | P2 | PROC DPO | S | S | N | BP-09 |
| [VEN-04](#ven-04) | คลังแบบประเมินคู่ค้า | Must | P2 | DPO SEC | S | S | N | BP-09 |
| [VEN-05](#ven-05) | พอร์ทัลให้คู่ค้าตอบแบบประเมิน | Must | P2 | VENDOR | M | M | Y | BP-09 |
| [VEN-07](#ven-07) | คำนวณคะแนนความเสี่ยงอัตโนมัติ | Must | P2 | SEC DPO | M | S | N | BP-09 |
| [VEN-08](#ven-08) | อนุมัติหรือปฏิเสธคู่ค้า | Must | P2 | DPO EXEC | S | S | N | BP-09 |
| [VEN-11](#ven-11) | ผูกคู่ค้ากับสัญญาและกิจกรรม | Must | P2 | PROC DPO | S | S | N | BP-09 |
| [VEN-03](#ven-03) | ผู้ประมวลผลช่วง | Should | P3 | VENDOR DPO | S | S | N | BP-09 |
| [VEN-06](#ven-06) | ตรวจหลักฐานและใบรับรอง | Should | P3 | VENDOR SEC | S | S | N | BP-09 |
| [VEN-09](#ven-09) | แผนแก้ไขข้อบกพร่องของคู่ค้า | Should | P3 | VENDOR PROC | S | S | N | BP-09 |
| [VEN-10](#ven-10) | ประเมินซ้ำตามรอบ | Should | P3 | SCHED | S | XS | N | BP-09 |
| [VEN-12](#ven-12) | เหตุละเมิดที่เกี่ยวกับคู่ค้า | Should | P3 | VENDOR SEC | S | XS | N | BP-09 |
| [VEN-13](#ven-13) | แดชบอร์ดความเสี่ยงคู่ค้า | Should | P3 | DPO EXEC | S | M | N | BP-09 |
| [VEN-14](#ven-14) | ยุติการใช้บริการ | Should | P3 | PROC VENDOR | S | S | N | BP-09 |
| [VEN-15](#ven-15) | AI ช่วยประเมินคู่ค้า | Nice | P4 | SEC LLM | M | S | N | BP-09 |

### ความสัมพันธ์ระหว่าง use case

- VEN-01 «include» VEN-02 (ทุกครั้งที่ทำ VEN-01 ต้องทำ VEN-02)
- VEN-05 «include» VEN-07 (ทุกครั้งที่ทำ VEN-05 ต้องทำ VEN-07)
- VEN-09 «extend» VEN-08 (VEN-09 เป็นทางเลือก/ส่วนขยายของ VEN-08)
- VEN-15 «extend» VEN-07 (VEN-15 เป็นทางเลือก/ส่วนขยายของ VEN-07)

## กระบวนการ / sequence / state machine

- [BP-09 รับคู่ค้าใหม่และติดตามความเสี่ยงคู่ค้า (Vendor onboarding & monitoring)](../processes/BP-09.md)
- [ST-06 คู่ค้า / ผู้ประมวลผล (vendor.vendors.status)](../states/ST-06.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [vendor.vendors](../data/vendor.md#vendor-vendors) | คู่ค้า / ผู้ประมวลผล (ต่อยอดจาก org.external_parties) |
| [vendor.intakes](../data/vendor.md#vendor-intakes) | แบบ intake สำหรับจัดระดับความเสี่ยง (tier) |
| [vendor.vendor_assessments](../data/vendor.md#vendor-vendor-assessments) | รอบประเมินคู่ค้า (ใช้ assessment engine) |
| [vendor.sub_processors](../data/vendor.md#vendor-sub-processors) | ผู้ประมวลผลช่วง |
| [vendor.certificates](../data/vendor.md#vendor-certificates) | ใบรับรองของคู่ค้า (ISO 27001, SOC 2 ฯลฯ) |
| [vendor.remediation_items](../data/vendor.md#vendor-remediation-items) | ประเด็นที่คู่ค้าต้องแก้ไข |
| [vendor.offboardings](../data/vendor.md#vendor-offboardings) | การยุติการใช้บริการ |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `vendor.vendor.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `vendor.vendor` | คู่ค้า / ผู้ประมวลผล | DPO `CRUDA` · PRIVACY `CRU` · LEGAL `R` · OWNER `CR` · IT `R` · SEC `RU` · PROC `CRUD` · AUDIT `R` | Owner ขอเพิ่มคู่ค้าใหม่ได้ |
| `vendor.assessment` | แบบประเมินคู่ค้า | DPO `CRUDA` · PRIVACY `CRUX` · SEC `RUX` · PROC `CRUX` · AUDIT `R` · GUEST `RU` | GUEST ตอบเฉพาะแบบประเมินของบริษัทตนเอง |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `vendor.assessment_completed` | vendor_id · tier · expires_at | จัดซื้อ · agreement |
| `vendor.approved` | vendor_id · tier · expires_at | จัดซื้อ · agreement |
| `vendor.certificate_expiring` | vendor_id · tier · expires_at | จัดซื้อ · agreement |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** VEN-01, VEN-02, VEN-04, VEN-05, VEN-07, VEN-08, VEN-11
- **P3:** VEN-03, VEN-06, VEN-09, VEN-10, VEN-12, VEN-13, VEN-14
- **P4:** VEN-15

## รายละเอียด feature

<a id="ven-01"></a>
### VEN-01 ทะเบียนคู่ค้าและผู้ประมวลผล

*Vendor registry*

- **Priority / Phase:** Must · P2 · กลุ่ม: ทะเบียน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ORG-06
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** ข้อมูลบริษัท ผู้ติดต่อ บริการ ข้อมูลที่เข้าถึง ประเทศที่ประมวลผล และสัญญา

**Backend (Go):** vendor profile ต่อยอด ORG-06: บริการ ข้อมูลที่เข้าถึง ประเทศที่ประมวลผล ผู้ติดต่อ สัญญา เจ้าของความสัมพันธ์ภายใน

**Frontend (Next.js):** หน้าทะเบียนคู่ค้า + หน้ารายละเอียด

**Acceptance criteria:** คู่ค้าหนึ่งรายมีหน้าเดียวที่รวมข้อมูลทุกโมดูล

**Implementation (VEN-01) — done:** The first feature on the `vendor` schema — `vendor.vendors` was already
fully specified in the baseline migrations (party_id → `org.external_parties` UQ, relationship_owner_id →
`iam.users`, is_processor, tier, data_access jsonb, processing_countries char(2)[], ST-06's own status column
defaulting to `prospect`) — no new migration. `internal/vendormgmt/service` (see the Go-package note above for
why the directory isn't literally `vendor`) is plain CRUD on the profile fields plus the FK-visibility checks
rule 1 requires: `party_id` via a newly-reused `orgservice.GetExternalParty` (ORG-06), `relationship_owner_id`
via `iamservice.Names`. `SaveVendor` deliberately never touches `status`/`tier`/`next_assessment_at`/
`approved_at`/`offboarded_at` — a new vendor always lands `prospect` (the column's own DB default, ST-06's
`[*] → prospect` edge) and every other ST-06 transition belongs to a sibling feature not built yet (VEN-02
tiering, VEN-05/07 assessment, VEN-08 approval, VEN-09 remediation, VEN-14 offboarding). The insert is wrapped
in `pdb.Savepoint` since `uq_vendors_party_id` is a real unique constraint (one vendor row per external party)
— the same pattern `org.SaveLegalEntity`/ROPA-03's own unique-constraint checks already use, so a duplicate
party_id doesn't abort the whole request transaction.

The acceptance criterion ("คู่ค้าหนึ่งรายมีหน้าเดียวที่รวมข้อมูลทุกโมดูล") is read literally as "the page is the
single place every later VEN/DPA/DSA feature adds its section to," not as a claim that those sibling modules
already exist to aggregate from — today the page shows exactly the profile this feature owns (status badge,
tier when set, processing countries, service description, relationship owner) with nothing yet to show from
VEN-02/04/05/07/08/09/11/14 (none built). API: `GET`/`POST /admin/v1/vendors` (cursor pagination, same shape as
every other module's list), `GET`/`PATCH /admin/v1/vendors/{id}` (ETag/If-Match) — all on the already-seeded
`vendor.vendor.*` permissions, no new code. UI: `/vendors` (list + create form, a party picker from ORG-06's
own `useExternalParties`, an owner picker reusing PLT-07's `useMentionSearch`) and `/vendors/{id}` (the single
profile page the acceptance criterion names, with its own inline edit). Tests: unit (validation, both
FK-visibility checks, duplicate-party refused without aborting the transaction, update never changes status,
two-tenant isolation incl. one tenant's party_id refused for another tenant's vendor), HTTP contract
(401/403/400 schema/404/412/422/428) through the real validator + AuthZ. `pnpm --filter @pdpa/admin build`/
`tsc` and the `@pdpa/i18n` ICU message tests both verified clean; not verified in a live browser session (the
dev-login server action didn't produce a session cookie against this session's local stack — the same
Keycloak-less limitation several earlier features, e.g. ROPA-02/DPO-01, already flagged rather than a bug
newly introduced here).

<a id="ven-02"></a>
### VEN-02 จัดระดับความเสี่ยงคู่ค้า

*Vendor tiering*

- **Priority / Phase:** Must · P2 · กลุ่ม: ทะเบียน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1), ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RRA-02
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** แบ่งระดับตามประเภทและปริมาณข้อมูล ข้อมูลอ่อนไหว และการโอนต่างประเทศ เพื่อกำหนดความเข้มของการประเมิน

**Backend (Go):** แบบ intake → คำนวณ tier อัตโนมัติ → กำหนดชุดแบบประเมินและรอบ

**Frontend (Next.js):** ส่วน intake ตอนเพิ่มคู่ค้า

**Acceptance criteria:** tier ถูกคำนวณตามเกณฑ์และกำหนดแบบประเมินที่ต้องส่ง

**Implementation — done.** `internal/vendormgmt/service/intake.go` (`vendor.vendor.*`, shared with VEN-01 — no
new permission code) reuses PLT-06 exactly the way the module doc's own backend note calls for ("แบบ intake →
คำนวณ tier อัตโนมัติ → กำหนดชุดแบบประเมิน"): `platform.form_definitions.form_type` already allowed `'intake'`
and `internal/wiring/forms.go`'s own comment had named it "reserved but unregistered" since PLT-06 shipped —
this feature registers it (`vendor.vendor.approve` to design/publish, matching DPO-09's own "the DPO designs
and publishes, staff record a run" split; `vendor.vendor.update` to respond, matching VEN-01's own actor line
"PROC DPO"). `vendor.intakes`/`vendor.vendors.tier` were already fully specified in the baseline migrations —
no new table. Migration 00058 (`docs/decisions.md` Q-33) seeds one global `"intake"`-type form, code
`vendor_intake`: four questions (`data_volume`/`sensitive_data`/`system_access_level`/`cross_border_transfer`,
the four factors ม.37(1)/ม.40 and the module doc's own "ประเภทและปริมาณข้อมูล ข้อมูลอ่อนไหว และการโอนต่างประเทศ"
name directly) scored into bands keyed exactly `low`/`medium`/`high`/`critical` — matching
`vendor.intakes.tier_result`'s own CHECK constraint, so `forms.Result.Band` lands on `tier_result` with no
separate mapping step (the same CHECK-matching move DPIA-01's own screening bands used for
`assess.assessments.status`).

`RecordIntake` is the one-shot path every intake answers through — `forms.Service.Record` (the mechanism
breach risk assessments and DPO-09's security checklist already use for a form filled in one atomic call, not
through PLT-06's draft/section-assignment admin flow), resolving the published `vendor_intake` form via a new
`publishedIntakeVersion` (mirrors DPIA-01's own `screeningVersion`: current *published* version only, a
tenant's own same-coded override preferred over the global default). The resulting `vendor.intakes` row and
`vendor.vendors.tier`/`next_assessment_at` update happen in the same call; `RequiredAssessmentCodes(tier,
crossBorderTransfer)` is a pure function naming VEN-04's own seeded template codes
(`vendor_pdpa`/`vendor_security`/`vendor_transfer`, migration 00053) a tier now calls for — a tunable business
default, not a legally-mandated rule (no `docs/decisions.md` entry: changing which tier requires which
template changes no data model or legally-required behaviour), documented inline: every tier but `low` gets
`vendor_pdpa`, `high`/`critical` also get `vendor_security`, and `vendor_transfer` is required whenever the
intake's own `cross_border_transfer` answer was `"yes"` regardless of tier — an otherwise low-risk engagement
that still moves data abroad still needs that specific check. `reassessmentInterval(tier)` is a second tunable
default (6/12/18/24 months by tier) feeding `next_assessment_at` — a scheduling convenience, not the legal
deadline VEN-10's own periodic re-assessment feature (not built) will eventually drive from a real schedule.

API: `GET`/`POST /admin/v1/vendors/{id}/intakes` (list — newest first — and record; no pagination, a vendor's
own tiering history is short), same shape as every other module's child-table endpoints. UI: a "Risk tiering"
section on `/vendors/{id}` (VEN-01's own single profile page, per its "include" relationship with this
feature) — past rounds with a tier badge, an inline intake form (the four questions above), and the computed
tier + required-template codes shown right after submitting. Tests: unit (the acceptance criterion directly —
a low-risk answer set lands `low` with no required template, a maximally-risky one lands `critical` with all
three VEN-04 codes required; `RequiredAssessmentCodes`'s boundary cases per tier/cross-border combination;
missing required answers refused; unknown vendor refused; two-tenant isolation of both the vendor and its
intake history — the seeded global form itself stays visible to every tenant by design, the same ORG-07
master-data pattern), HTTP contract (401/403/404/422/201/200) through the real validator + AuthZ.
`pnpm --filter @pdpa/admin build`/`tsc` and the `@pdpa/i18n` ICU message tests both verified clean; not
verified against a real Postgres in this pass (no reachable database in this environment) — `sqlc generate`
and `oapi-codegen` were run for real and produced the exact store/HTTP code checked in (confirmed byte-identical
to what was hand-written first), so only the DB-backed test run itself is unverified, the same gap the
session's own earlier notes on Docker/Keycloak-less local stacks already flag elsewhere.

<a id="ven-04"></a>
### VEN-04 คลังแบบประเมินคู่ค้า

*Questionnaire templates*

- **Priority / Phase:** Must · P2 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40; ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** DPO (DPO / Privacy Team), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06, T34
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** แบบประเมิน PDPA ความปลอดภัยสารสนเทศ (อ้างอิง ISO 27001/27701) และการโอนต่างประเทศ

**Backend (Go):** template แบบประเมิน PDPA / ความปลอดภัย (mapping ISO 27001 / 27701) / การโอน บน PLT-06

**Frontend (Next.js):** หน้าคลังแบบประเมินคู่ค้า

**Acceptance criteria:** มี template พร้อมใช้อย่างน้อย 3 ชุด

**Implementation — done.** No new Go code at all: DPIA-03's own generic template library (`internal/dpia/
service/templates.go`'s `ListTemplates`/`GetTemplateByID`/`CreateTemplate`/`CloneTemplate`/
`PublishTemplate`/`RetireTemplate`, all on `assess.templates` + a PLT-06 "assessment" form, gated on the
already-seeded `assessment.template.*` codes) was never hardcoded to `assessment_type='dpia'` — it takes the
type as a plain parameter, and `'vendor'` was already in `assess.templates`' own CHECK constraint, unused
until now. VEN-04 is purely data: migration 00053 (`docs/decisions.md` Q-31) seeds 3 global
(`tenant_id NULL`) published templates — PDPA compliance (ม.40), information security mapped to ISO/IEC
27001/27701 control areas, and cross-border data transfer — the same "seed a draft flagged for legal/security
review" move ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01 already made; each template's own `name` carries the
"(ร่าง — รอฝ่ายกฎหมายตรวจ)" marker directly in the data, since the shared template-library page (built for
DPIA-03, used by every assessment_type) has no per-row banner mechanism of its own to hook into. Frontend:
the existing `/settings/dpia-templates` page gained one hook — its `filterType` state now reads an initial
`?type=` query param (`useSearchParams`) — so `/vendors` can deep-link straight to the vendor-filtered view
instead of a new, duplicate screen; `/vendors`'s header gained a plain link there. Tests:
`TestVendorAssessmentTemplates_SeededReady` (the acceptance criterion directly — at least 3 published vendor
templates, each with real `legal_refs` and a resolvable published form version, including the three specific
seeded codes) and `TestVendorAssessmentTemplates_VisibleToAnyTenant` (a second, entirely separate tenant sees
them too, with no provisioning of its own — the same global-visibility pattern ORG-07's master data already
proved) — both run against the real seeded migration on a real Postgres. Migration verified both directions
(`up`/`down`/`up` against the real local Postgres) before committing.

<a id="ven-05"></a>
### VEN-05 พอร์ทัลให้คู่ค้าตอบแบบประเมิน

*Vendor portal*

- **Priority / Phase:** Must · P2 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest))
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-04, PLT-17
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** คู่ค้าตอบแบบประเมินและแนบหลักฐานผ่านลิงก์ โดยไม่ต้องมีบัญชีผู้ใช้เต็มรูปแบบ

**Backend (Go):** คู่ค้าตอบผ่าน guest link: บันทึกร่าง มอบให้เพื่อนร่วมงาน แนบหลักฐาน ถาม-ตอบกับผู้ประเมิน ส่ง

**Frontend (Next.js):** พอร์ทัลคู่ค้าใน portal

**Acceptance criteria:** คู่ค้าตอบได้โดยไม่ต้องมีบัญชีผู้ใช้เต็มรูปแบบ

<a id="ven-07"></a>
### VEN-07 คำนวณคะแนนความเสี่ยงอัตโนมัติ

*Automated scoring*

- **Priority / Phase:** Must · P2 · กลุ่ม: ผลประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SEC (ทีม Security / Incident), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-04, RRA-02
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** คิดคะแนนจากคำตอบและน้ำหนักคำถาม สรุประดับความเสี่ยงและความเสี่ยงคงเหลือ

**Backend (Go):** คะแนนจากน้ำหนักคำถาม → ระดับความเสี่ยง + ความเสี่ยงคงเหลือหลังมาตรการ

**Frontend (Next.js):** สรุปคะแนนในหน้าการประเมิน

**Acceptance criteria:** คะแนนคำนวณถูกต้องตามน้ำหนักทุกกรณีทดสอบ

<a id="ven-08"></a>
### VEN-08 อนุมัติหรือปฏิเสธคู่ค้า

*Approval workflow*

- **Priority / Phase:** Must · P2 · กลุ่ม: ผลประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** ผู้ประเมินให้ความเห็น และผู้มีอำนาจอนุมัติ / ปฏิเสธ / อนุมัติแบบมีเงื่อนไข

**Backend (Go):** อนุมัติ / ปฏิเสธ / อนุมัติแบบมีเงื่อนไข; เงื่อนไขกลายเป็นแผนแก้ไข

**Frontend (Next.js):** ขั้นตอนอนุมัติคู่ค้า

**Acceptance criteria:** อนุมัติแบบมีเงื่อนไขสร้างแผนแก้ไขอัตโนมัติ

<a id="ven-11"></a>
### VEN-11 ผูกคู่ค้ากับสัญญาและกิจกรรม

*Link to DPA / DSA / RoPA*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-10, ROPA-08
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** แสดงสัญญา DPA/DSA ผลประเมิน และกิจกรรมที่คู่ค้าเกี่ยวข้อง แจ้งเตือนคู่ค้าที่ยังไม่มีสัญญา

**Backend (Go):** หน้าคู่ค้ารวมสัญญา / ผลประเมิน / กิจกรรม + แจ้งเตือนคู่ค้าที่ไม่มีสัญญา

**Frontend (Next.js):** แท็บความเชื่อมโยงของคู่ค้า

**Acceptance criteria:** คู่ค้าที่เป็นผู้ประมวลผลแต่ไม่มี DPA ถูกแจ้งเตือน

<a id="ven-03"></a>
### VEN-03 ผู้ประมวลผลช่วง

*Sub-processor management*

- **Priority / Phase:** Should · P3 · กลุ่ม: ทะเบียน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-01
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** บันทึกผู้ประมวลผลช่วงของคู่ค้า และการอนุมัติก่อนใช้

**Backend (Go):** ทะเบียนผู้ประมวลผลช่วง + ขออนุมัติก่อนใช้ + แจ้งเมื่อคู่ค้าเพิ่มรายใหม่

**Frontend (Next.js):** แท็บผู้ประมวลผลช่วง

**Acceptance criteria:** ผู้ประมวลผลช่วงที่ยังไม่อนุมัติแสดงเตือน

<a id="ven-06"></a>
### VEN-06 ตรวจหลักฐานและใบรับรอง

*Evidence & certificates*

- **Priority / Phase:** Should · P3 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-01
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** เก็บใบรับรอง (ISO 27001, SOC 2) และแจ้งเตือนเมื่อใกล้หมดอายุ

**Backend (Go):** ทะเบียนใบรับรอง + วันหมดอายุ + แจ้งเตือน

**Frontend (Next.js):** แท็บใบรับรอง

**Acceptance criteria:** ใบรับรองใกล้หมดอายุถูกแจ้งเตือนล่วงหน้า

<a id="ven-09"></a>
### VEN-09 แผนแก้ไขข้อบกพร่องของคู่ค้า

*Vendor remediation plan*

- **Priority / Phase:** Should · P3 · กลุ่ม: ผลประเมิน
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-04
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** กำหนดประเด็นที่ต้องแก้ ผู้รับผิดชอบฝั่งคู่ค้า และกำหนดเสร็จ

**Backend (Go):** ประเด็นที่ต้องแก้ + ผู้รับผิดชอบฝั่งคู่ค้า (guest) + กำหนดเสร็จ + ติดตาม

**Frontend (Next.js):** แท็บแผนแก้ไข

**Acceptance criteria:** คู่ค้าอัปเดตความคืบหน้าผ่านลิงก์ได้

<a id="ven-10"></a>
### VEN-10 ประเมินซ้ำตามรอบ

*Periodic re-assessment*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** VEN-02
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** ตั้งรอบประเมินตามระดับความเสี่ยง และส่งแบบประเมินอัตโนมัติ

**Backend (Go):** รอบประเมินตาม tier (River cron) + ส่งแบบประเมินอัตโนมัติ

**Frontend (Next.js):** ตั้งรอบประเมินต่อ tier

**Acceptance criteria:** คู่ค้าถูกส่งแบบประเมินซ้ำตามรอบโดยไม่ต้องสั่งเอง

<a id="ven-12"></a>
### VEN-12 เหตุละเมิดที่เกี่ยวกับคู่ค้า

*Vendor incidents*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40(2)
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** BRE-03
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** บันทึกเหตุที่คู่ค้าแจ้ง และเชื่อมกับโมดูลแจ้งเหตุละเมิด

**Backend (Go):** เหตุที่คู่ค้าแจ้งผูกกับ vendor + สถิติ

**Frontend (Next.js):** แท็บเหตุละเมิดของคู่ค้า

**Acceptance criteria:** ประวัติเหตุของคู่ค้าแสดงครบ

<a id="ven-13"></a>
### VEN-13 แดชบอร์ดความเสี่ยงคู่ค้า

*Vendor risk dashboard*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** จำนวนคู่ค้าตามระดับความเสี่ยง สถานะการประเมิน และสัญญาใกล้หมดอายุ

**Backend (Go):** API สถิติ tier / สถานะประเมิน / สัญญาใกล้หมดอายุ

**Frontend (Next.js):** dashboard ความเสี่ยงคู่ค้า

**Acceptance criteria:** ตัวเลขตรงกับทะเบียนคู่ค้า

<a id="ven-14"></a>
### VEN-14 ยุติการใช้บริการ

*Vendor offboarding*

- **Priority / Phase:** Should · P3 · กลุ่ม: ยุติ
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(3), ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), VENDOR (คู่ค้า / ผู้ประมวลผล (guest))
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-01
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** ขอคืนหรือทำลายข้อมูล เก็บใบยืนยันการทำลาย และปิดสิทธิ์การเข้าถึง

**Backend (Go):** checklist ยุติการใช้บริการ: ขอคืน / ทำลายข้อมูล, ใบยืนยัน, ปิดสิทธิ์ / API client

**Frontend (Next.js):** ขั้นตอน offboarding

**Acceptance criteria:** ยุติคู่ค้าได้เมื่อมีใบยืนยันการทำลายและปิดสิทธิ์ครบ

<a id="ven-15"></a>
### VEN-15 AI ช่วยประเมินคู่ค้า

*AI vendor assessment*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 07_Vendor
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** SEC (ทีม Security / Incident), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-22
- **Process:** [BP-09](../processes/BP-09.md)

**คำอธิบาย:** วิเคราะห์เอกสารของคู่ค้าเพื่อเติมคำตอบและชี้ประเด็นเสี่ยง

**Backend (Go):** AI อ่านเอกสารคู่ค้า (SOC 2, นโยบาย) → เติมคำตอบ + ชี้ประเด็นเสี่ยง

**Frontend (Next.js):** แผง AI ในหน้าการประเมิน

**Acceptance criteria:** ข้อเสนอจาก AI ต้องมีคนยืนยันก่อนบันทึก
