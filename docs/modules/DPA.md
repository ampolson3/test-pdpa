# DPA — ข้อตกลงการประมวลผลข้อมูล (DPA)

> ระบบจัดการเอกสารข้อตกลงการประมวลผลข้อมูลส่วนบุคคล (Data Processing Agreement) · ขอบเขต: ข้อตกลงระหว่างผู้ควบคุมข้อมูลกับผู้ประมวลผลข้อมูลตาม ม.40  
> 14 features · Must 6 / Should 7 / Nice 1 · phase: P2 (6), P3 (7), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/agreement` |
| PostgreSQL schema | [`agreement`](../data/agreement.md) (10 ตาราง) |
| Admin API prefix | `/admin/v1/agreements` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /admin/v1/agreements` — สร้างข้อตกลง DPA / DSA (BP-10)<br>`POST /admin/v1/agreements/{id}/render` — สร้างเอกสาร PDF / DOCX (SEQ-07) |
| หน้าจอ (Next.js) | admin: /agreements/dpa/* |
| พึ่งพาบริการ | docs, workflow, e-signature |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-15 DPA, BP-10, DFD-1, ERD-13, SEQ-07, ST-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| PROC | จัดซื้อ / ผู้ดูแลคู่ค้า | Procurement / Vendor Manager | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | Guest link (token หมดอายุ) + OTP |
| ESIGN | ผู้ให้บริการ e-Signature | e-Signature Provider | API key + callback ลงชื่อ HMAC |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DPA-01](#dpa-01) | Template DPA มาตรฐาน | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-02](#dpa-02) | สร้างแบบกรอกเองและแบบอัตโนมัติ | Must | P2 | LEGAL PROC | L | M | Y | BP-10 |
| [DPA-03](#dpa-03) | ข้อกำหนดที่ต้องมี | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-04](#dpa-04) | ภาคผนวกรายละเอียดการประมวลผล | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-10](#dpa-10) | ทะเบียน DPA และแจ้งเตือนหมดอายุ | Must | P2 | LEGAL SCHED | S | M | N | BP-10 |
| [DPA-11](#dpa-11) | ผูก DPA กับคู่ค้าและกิจกรรม | Must | P2 | PROC DPO | S | S | N | BP-10 |
| [DPA-05](#dpa-05) | ข้อสัญญาการโอนต่างประเทศ | Should | P3 | LEGAL DPO | S | XS | N | BP-10 |
| [DPA-06](#dpa-06) | แก้ไขเอกสารในระบบ | Should | P3 | LEGAL | S | M | N | BP-10 |
| [DPA-07](#dpa-07) | ตรวจทานและอนุมัติ | Should | P3 | DPO LEGAL | XS | S | N | BP-10 |
| [DPA-08](#dpa-08) | เวอร์ชันและประวัติการดาวน์โหลด | Should | P3 | LEGAL | S | S | N | BP-10 |
| [DPA-09](#dpa-09) | ส่งออกและลงนามอิเล็กทรอนิกส์ | Should | P3 | VENDOR ESIGN LEGAL | M | S | N | BP-10 |
| [DPA-12](#dpa-12) | แจ้งเตือนผู้ประมวลผลที่ยังไม่มี DPA | Should | P3 | SCHED PROC | S | XS | N | BP-10 |
| [DPA-13](#dpa-13) | ติดตามการคืน/ทำลายข้อมูลเมื่อสิ้นสุด | Should | P3 | VENDOR PROC | S | S | N | BP-10 |
| [DPA-14](#dpa-14) | ตรวจ DPA ที่ได้รับจากลูกค้า | Nice | P4 | LEGAL | S | S | N | BP-10 |

### ความสัมพันธ์ระหว่าง use case

- DPA-02 «include» DPA-01 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-01)
- DPA-02 «include» DPA-03 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-03)
- DPA-02 «include» DPA-04 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-04)
- DPA-05 «extend» DPA-02 (DPA-05 เป็นทางเลือก/ส่วนขยายของ DPA-02)

## กระบวนการ / sequence / state machine

- [BP-10 จัดทำและบริหารข้อตกลง DPA / DSA (Agreement lifecycle)](../processes/BP-10.md)
- [SEQ-07 สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg)](../sequences/SEQ-07.md)
- [ST-04 ข้อตกลง DPA / DSA และประกาศความเป็นส่วนตัว](../states/ST-04.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [agreement.agreements](../data/agreement.md#agreement-agreements) | ข้อตกลง DPA / DSA / ผู้ควบคุมร่วม / DPA ขาเข้า |
| [agreement.parties](../data/agreement.md#agreement-parties) | คู่สัญญาในข้อตกลง (หลายฝ่าย) |
| [agreement.agreement_activities](../data/agreement.md#agreement-agreement-activities) | กิจกรรม RoPA และเส้นทางข้อมูลที่ข้อตกลงครอบคลุม |
| [agreement.clauses](../data/agreement.md#agreement-clauses) | clause ในข้อตกลง (อ้างอิงคลังกลาง + ข้อความที่ปรับ) |
| [agreement.mandatory_rules](../data/agreement.md#agreement-mandatory-rules) | กฎ clause บังคับ (ม.27, ม.28-29, ม.37(2), ม.40) |
| [agreement.annexes](../data/agreement.md#agreement-annexes) | ภาคผนวก (รายละเอียดการประมวลผล รายการข้อมูล มาตรการ แผนผัง) |
| [agreement.signature_requests](../data/agreement.md#agreement-signature-requests) | การส่งลงนามอิเล็กทรอนิกส์ |
| [agreement.obligations](../data/agreement.md#agreement-obligations) | ภาระผูกพันตามข้อตกลง |
| [agreement.return_confirmations](../data/agreement.md#agreement-return-confirmations) | ใบยืนยันการคืน / ทำลายข้อมูลเมื่อสิ้นสุด |
| [agreement.downloads](../data/agreement.md#agreement-downloads) | ประวัติการดาวน์โหลดเอกสารสัญญา |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `agreement.dpa.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `agreement.dpa` | ข้อตกลงการประมวลผล (DPA) | DPO `RA` · PRIVACY `R` · LEGAL `CRUDAP` · OWNER `R` · PROC `CR` · AUDIT `R` · GUEST `R` | GUEST ดูและลงนามฉบับที่ส่งให้ |
| `agreement.clause` | คลังข้อความสัญญา | DPO `R` · LEGAL `CRUDP` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `agreement.signed` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.expiring` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.terminated` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DPA-01, DPA-02, DPA-03, DPA-04, DPA-10, DPA-11
- **P3:** DPA-05, DPA-06, DPA-07, DPA-08, DPA-09, DPA-12, DPA-13
- **P4:** DPA-14

## รายละเอียด feature

<a id="dpa-01"></a>
### DPA-01 Template DPA มาตรฐาน

*DPA templates*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-16, T34
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** template ข้อตกลงการประมวลผลตาม ม.40 ภาษาไทยและอังกฤษ ทำสำเนาและปรับแก้ได้

**Backend (Go):** template DPA ม.40 TH/EN (เนื้อหา T34) บน document composer, clone / ปรับ

**Frontend (Next.js):** หน้าคลัง template DPA

**Acceptance criteria:** มี template ภาษาไทยและอังกฤษพร้อมใช้

**หมายเหตุ:** OneTrust ไม่มี (จุดต่างหลัก)

**Implementation — done.** No new Go code at all, the same shape VEN-04 just took for its own template
library: PLT-16's document composer already registers the `"dpa"` doc type (`internal/wiring.Docs`,
`agreement.dpa.*` permissions — read/create/update/publish already seeded to LEGAL in the baseline RBAC) and
already has its own generic, doc-type-agnostic template library (`internal/platform/docs/library.go`'s
`ListTemplates`/`GetTemplate`/`CreateTemplate`/`UpdateTemplate`/`PublishTemplate` on `platform.templates` —
the exact same table and mechanism PNG-03's own wizard templates already used, just `language='mul'`: one row
carries both `th`/`en` content together as `render.Content`'s own `{"th": ..., "en": ...}` shape, instead of
PNG-03's separate per-language rows). Migration 00054 (`docs/decisions.md` Q-32) seeds the first, global
(`tenant_id NULL`) published template — code `standard_dpa` — covering every clause topic DPA-01's sibling
features name: processing only on instructions, confidentiality, security measures (ม.37(2)), breach
notification, sub-processors, assistance with data-subject-rights requests, return/destruction on
termination, the controller's audit rights, and cross-border transfer (ม.28-29) — with `org_name_th`/
`org_name_en`/`dpo_name`/`dpo_email` merge fields resolved the same way every other PLT-16 document already
does. Content opens with a `[ร่าง — ...]`/`[DRAFT — ...]` banner paragraph (rule 8 — legal wording stays
flagged until Legal reviews it, the same "seed a draft pending review" move ORG-07/ROPA-09/PNG-03/DPIA-01/
RTG-01/VEN-04 already made). Frontend: the existing `/documents/templates` page (built generically, already
listing every doc type the caller's own permissions make visible) needed no change — a LEGAL user already
sees `"dpa"` as an option there and can view, clone or author further `dpa` templates with it. Tests:
`TestStandardDpaTemplate_SeededReady` (the acceptance criterion directly — a published, global `standard_dpa`
template with both Thai and English content) and `TestStandardDpaTemplate_VisibleToAnyTenant` (a second,
entirely separate tenant sees it too, with nothing of its own) — both run against the real seeded migration
on a real Postgres, using a bare `docs.Service` with only `"dpa"` registered rather than the full
`docstest.Setup` fixture, since `ListTemplates`/`Access` touch neither Files, River nor PDF and that fixture
unconditionally needs S3+clamd this environment doesn't have. Migration verified both directions
(`up`/`down`/`up` against the real local Postgres) before committing.

<a id="dpa-02"></a>
### DPA-02 สร้างแบบกรอกเองและแบบอัตโนมัติ

*Manual & automatic generation*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DPA-01, VEN-01, ROPA-03
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** สร้างจาก template โดยดึงข้อมูลคู่ค้า กิจกรรม และข้อมูลส่วนบุคคลที่ส่งให้ผู้ประมวลผล

**Backend (Go):** agreement engine (ใช้ร่วม DSA): เลือกคู่ค้า + กิจกรรม → merge ข้อมูลคู่ค้า / กิจกรรม / ข้อมูลที่ส่ง → ร่างสัญญา; โหมดกรอกเอง; สถานะสัญญา

**Frontend (Next.js):** wizard สร้างสัญญา + editor

**Acceptance criteria:** สร้างร่าง DPA จากคู่ค้าและกิจกรรมได้ภายใน 10 นาที

**หมายเหตุ:** สร้าง agreement engine ครั้งเดียว ใช้ร่วม DSA

**Implementation — done.** `internal/agreement` is the agreement engine itself — the first, and by design the
only, Go package for it; DSA (not built yet) will be a second `agreement_type` on the same tables and the
same `CreateWizard`, not a new package, per the module's own "สร้าง agreement engine ครั้งเดียว ใช้ร่วม DSA"
note. `agreement.agreements`/`agreement.parties`/`agreement.agreement_activities` (and the other agreement
tables DPA-03/04/05 will use) were already fully specified in the baseline migrations — no new migration.
`agreementservice.CreateWizard` is the acceptance criterion in one call: validates `vendor_id` (via the new
`Vendor` interface's `GetVendor`, then the vendor's own `party_id` through `Org.GetExternalParty` — rule 1,
FKs bypass RLS), `legal_entity_id` (`Org.GetLegalEntity`) and every `activity_ids` entry (`Ropa.GetActivity`)
before anything is written; composes the document through `docs.Service.Create` (rule 9 — agreement never
writes `platform.documents` directly), which already resolves a DPA-01 template's content when `template_id`
is given, or starts blank when it's omitted — that omission *is* "โหมดกรอกเอง" (manual mode), needing no
separate code path; derives the counterparty's own role from ours (controller↔processor, joint_controller↔
joint_controller — the wire schema accepts only `ours`, never asks for the counterparty's); and numbers the
agreement `{TYPE}-{year}-NNNN` with the same per-tenant-per-year advisory-lock pattern breach's own incident
numbering already established (`LockAgreementNumbering` + `CountAgreementsWithPrefix`). Only `agreement_type
= "dpa"` is actually wired to a feature today — `dsa`/`joint_controller`/`inbound_dpa` are real values already
in the table's own CHECK constraint (for when those modules exist) and are refused as "not yet supported" (422)
rather than silently accepted, the same "leave the column, build the real thing later" deferral ROPA-01's own
`discovered_by_finding_id` already used. `agreement.parties`'s own `party_role` CHECK is wider than this
feature's two controller/processor outcomes (it also allows `joint_controller`, used when `our_role` is
`joint_controller`), so no CHECK or migration change was needed either.

API: `GET`/`POST /admin/v1/agreements` (cursor pagination, `agreement_type`/`vendor_id` filters — the same
shape every other module's own list endpoints already use), `GET /admin/v1/agreements/{id}` (includes
`activity_ids`) — all on the already-seeded `agreement.dpa.*` permissions (no new code). UI: a new page,
`/agreements` (the module doc's own "wizard สร้างสัญญา + editor" note) — an inline create form (agreement
type, our role, vendor, legal entity, title, an optional DPA-01 published template, an activity checklist
drawn from ROPA's own activity list, auto-renew/renewal-notice-days) and a type-filtered list linking to each
agreement's detail page, which shows its parties/activities/renewal settings and a link straight into the
composed document's own PLT-16 editor (`/documents/{document_id}`) — DPA-02's own "+ editor" note is PLT-16's
existing editor, not a second one; a link from `/vendors/{id}` points back to `/agreements`. Tests: unit
(the acceptance criterion directly — one call from a vendor + its activities produces a real agreement with
a composed document, the right parties row and every activity linked; the three FK-visibility checks refuse
an unknown id instead of hitting the database's own FK constraint; counterparty-role derivation for all three
`our_role` values; agreement numbering; an unsupported `agreement_type` like `dsa` refused; list filters by
type/vendor; two-tenant isolation), HTTP contract (401/403/400 schema/422 incl. the unsupported-type case/
201/200/404) through the real validator + AuthZ. `pnpm --filter @pdpa/admin build`/`tsc` and the `@pdpa/i18n`
ICU message tests both verified clean. Not done: DPA-03 (mandatory-clause gate before approval), DPA-04
(RoPA-sourced processing-detail annex), DPA-05 (transfer clauses), DPA-10 (registry + expiry alerts), DPA-11
(the vendor-page read-side view of an agreement's own activities — this feature built the write side it will
read from) — all sibling features layered on the same `agreement.agreements` row, not built here.

<a id="dpa-03"></a>
### DPA-03 ข้อกำหนดที่ต้องมี

*Mandatory clauses*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40(1)-(3), ม.28-29
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ประมวลผลตามคำสั่ง มาตรการความปลอดภัย แจ้งเหตุละเมิด จัดทำ RoPA ผู้ประมวลผล ผู้ประมวลผลช่วง ช่วยตอบคำขอใช้สิทธิ คืน/ทำลายข้อมูลเมื่อสิ้นสุด สิทธิตรวจสอบ และการโอนต่างประเทศ

**Backend (Go):** rule ตรวจ clause บังคับ ม.40 ก่อนส่งอนุมัติ

**Frontend (Next.js):** แผงตรวจ clause ที่ขาด

**Acceptance criteria:** สัญญาที่ขาด clause บังคับส่งอนุมัติไม่ได้

**Implementation — done.** The "คลัง control" this feature draws from is PLT-16's own clause library
(`platform.clause_library` — DPA-01's own template already sits on the same document composer, this feature
is the first to use the library's per-clause rows). Migration 00057 seeds nine clauses the module doc's own
ม.40(1)-(3)/37(2)/28-29 topic list names (`dpa.processing_on_instructions`, `confidentiality`,
`security_measures`, `breach_notification`, `sub_processors`, `dsar_assistance`, `return_or_destroy`,
`audit_rights`, `cross_border_transfer`), each `is_mandatory = true`, `applies_to = {dpa}`, published, and
flagged DRAFT (rule 8, the same "seed a draft pending review" move ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01/
VEN-04/DPA-01 already made) — and nine matching `agreement.mandatory_rules` rows (`agreement_type = 'dpa'`),
already fully specified in the baseline migrations and unused until now. `agreement.clauses` is the join: a
new `internal/agreement/service/clauses.go` (`AddClause`/`ListClauses`/`RemoveClause`, same
`agreement.dpa.read`/`.update` permissions DPA-02 already registered — no new code) links a published
library clause to an agreement, only while it's still `draft`.

The acceptance criterion itself ("ส่งอนุมัติไม่ได้") needed a pre-*submit* gate, not a pre-*publish* one —
PLT-08's own `versioning.Policy` only had `OnPublish` and (via `docs.Service.SetValidate`, PNG-02's own
mechanism) a publish-time check; nothing ran before `Submit` moved a draft to `in_review`. `versioning.Policy`
gained a new `Validate` field, called inside `Submit` right after the draft-status check — the submit-time
counterpart of `SetValidate`, and (per `docs.Service`'s own new `SetSubmitValidate` setter, wired the same
lazy-lookup way `SetValidate`/`SetOnPublished` already are) available to every PLT-08 document type, not just
"dpa". `agreement.Service.CheckSubmittable` resolves the document id PLT-08 hands it back to its own
agreement row (`GetAgreementByDocumentID`, new query) and compares `agreement.clauses`' attached codes
against `agreement.mandatory_rules` for that `agreement_type` — missing ones become `ErrMissingMandatoryClauses`
(`Unwrap() -> versioning.ErrInvalidRequest`, the same 422 reporting pattern PNG-02's own `ErrChecklistIncomplete`
established), which `versioning.Submit` now returns straight from the generic `/admin/v1/platform/record-versions/
{id}/submit` endpoint shared by every PLT-08 consumer.

`agreement.mandatory_rules.condition` is a small, deliberately narrow jsonb shape
(`{"requires_transfer": true}`) rather than a general condition language with nothing yet to need one: only
`cross_border_transfer` is conditional — it applies only once a linked RoPA activity actually has a transfer
on record (`Ropa.ListActivityTransfers`, the already-exported ROPA-08 method, rule 9 — agreement never reads
`ropa.activity_transfers` directly), re-checked live on every call, never cached. Every other rule always
applies. `CountAgreementClauses`/`ListAgreementClauseCodes` join `platform.clause_library` directly in SQL for
its own display columns (code/title/legal_ref) — the same "join a global reference table, not another
module's tenant data" exception ROPA-01's own `org.data_categories` join already established; the
cross-border check itself goes through `Ropa`'s own interface precisely because `ropa.activity_transfers` is
real tenant data, not a reference table.

API: `GET`/`POST /admin/v1/agreements/{id}/clauses`, `DELETE /admin/v1/agreements/{id}/clauses/{clauseRowId}`,
`GET /admin/v1/agreements/{id}/missing-clauses` (the module doc's own "แผงตรวจ clause ที่ขาด", computed live,
never persisted). UI: a clause panel on `/agreements/{id}` — the missing-clause list (amber banner while
non-empty, a confirmation once clear), attached clauses with remove, and an add form drawing from the
published `dpa`-applicable clauses in the library (`useClauses`, PLT-16's own hook). Also fixed in passing:
`make gen`'s `oapi-codegen` target had no line for `internal/agreement/http` at all since DPA-02 added the
package — the same gap DSAR-03 already found and fixed for `internal/dsar/http`; added here too. Tests: unit
(the acceptance criterion directly — missing clauses block `CheckSubmittable`/the real `versioning.Submit`
call until every required one is attached; the cross-border rule applies only once a transfer is recorded and
clears once attached; `AddClause` refuses outside `draft`, an unpublished/unknown clause id, and a duplicate
code; two-tenant isolation of both the link and the missing-clause read), HTTP contract (401/403/404/422/201/
200/204) through the real validator + AuthZ. Full backend `go test -count=1 -p 1 ./...` and
`pnpm --filter @pdpa/admin build` both verified clean.

<a id="dpa-04"></a>
### DPA-04 ภาคผนวกรายละเอียดการประมวลผล

*Processing schedule*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ประเภทข้อมูล กลุ่มเจ้าของข้อมูล วัตถุประสงค์ ระยะเวลา และมาตรการความปลอดภัย

**Backend (Go):** ภาคผนวกสร้างจาก RoPA (ประเภทข้อมูล กลุ่มเจ้าของ วัตถุประสงค์ ระยะเวลา มาตรการ)

**Frontend (Next.js):** ส่วนภาคผนวกใน editor

**Acceptance criteria:** ภาคผนวกตรงกับข้อมูล RoPA ของกิจกรรมที่เลือก

**Implementation — done.** `internal/agreement/service/schedule.go`'s `ProcessingSchedule` composes the
annex — the module doc's own topic list (data categories, data subject groups, purposes, retention periods
and security measures) per RoPA activity this agreement covers — computed live on every call, never
persisted (the same reasoning DPIA-04's own `ActivityDescription` already used: there is nothing to go stale,
so "ภาคผนวกตรงกับข้อมูล RoPA" needs no separate sync step). `agreement.annexes` already has an
`annex_type = 'processing_schedule'` CHECK value and a `content` jsonb column for exactly this, but nothing
writes to it here — a live read satisfies the literal acceptance criterion without inventing a freeze/attach
step no screen has asked for yet (the same "leave the column for the sibling feature that needs it" deferral
this codebase uses throughout); a later feature that needs a frozen copy (e.g. for DOCX export) can add that
without touching this read path.

`agreement.Service`'s `Org`/`Ropa` interfaces (rule 9) both grew the exact methods DPIA-04's own description
composer already calls — `Org.GetMaster`/`ListMaster` (data category / subject type / lawful basis names)
and `Ropa.ListActivityPurposes`/`ListActivityData`/`ListRetentionRules`/`ListActivityControls`/`ListControls`
(purposes, data, retention, and — DPA-04's own addition beyond DPIA-04's scope — ROPA-09's security-measure
links, resolved to catalog code/name/category through `Ropa.ListControls`, the same thin pass-through to
`risk/service` ROPA-09 already built). No new migration, permission or endpoint beyond the one read.

API: `GET /admin/v1/agreements/{id}/processing-schedule` (`agreement.dpa.read`, no ETag — nothing here is
ever written back). UI: a read-only "Processing schedule annex" section on `/agreements/{id}`, below the
clause panel (DPA-03) — one block per linked activity with purposes/data/retention/security-measures lists,
an empty-state message when the agreement has no linked activities yet. Tests: unit (the acceptance criterion
directly — the schedule's purposes/data/retention/security-measures for a linked activity match exactly what
was written to that activity's own RoPA rows, resolved through the same master-data/catalog names; an
agreement with no linked activities returns an empty list, not an error; two-tenant isolation), HTTP contract
(200 with an empty list, 404 for an unknown agreement) through the real validator + AuthZ. Full backend test
suite and `pnpm --filter @pdpa/admin build` both verified clean.

<a id="dpa-10"></a>
### DPA-10 ทะเบียน DPA และแจ้งเตือนหมดอายุ

*DPA register & expiry alerts*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** สถานะสัญญา วันเริ่ม/สิ้นสุด และแจ้งเตือนก่อนหมดอายุ

**Backend (Go):** ทะเบียนสัญญา + วันเริ่ม / สิ้นสุด + แจ้งเตือนก่อนหมดอายุ (PLT-05)

**Frontend (Next.js):** หน้าทะเบียน DPA

**Acceptance criteria:** สัญญาใกล้หมดอายุถูกแจ้งเตือนตามเวลาที่ตั้ง

**Implementation — done, scoped to exactly this acceptance criterion.** `agreement.agreements` already had
`status`/`effective_from`/`effective_to`/`auto_renew`/`renewal_notice_days` fully specified in the baseline
migrations (ST-04#1's own 7-state lifecycle), but nothing had ever written `effective_to` or any status past
the DB's own `draft` default — DPA-02's `CreateWizard` only ever set `effective_from`. This pass adds exactly
the registry's own mutable fields and the reminder, not the rest of ST-04#1 (approve/send-for-signature/sign
are DPA-06/07/08/09's own job, Should-priority and not built — building a real e-signature integration to let
`active` ever happen would be far beyond this feature's literal acceptance criterion). `internal/agreement/
service/renewal.go`'s `SetSchedule` (`agreement.dpa.update`, shared with DPA-02 — no new permission) is the
first ever update path on an agreement's core row: ETag-gated like every other module's update endpoint, it
sets `effective_from`/`effective_to`/`auto_renew`/`renewal_notice_days` and reschedules the single reminder
checkpoint in the same call.

The reminder itself follows DSAR-07/PNG-04's own established pattern exactly rather than reaching for the full
PLT-05 workflow engine the module doc's own backend note mentions: `RenewalReminderAt(effectiveTo,
renewalNoticeDays)` is a pure, clock-testable function; `scheduleRenewalReminder` enqueues one River job
(`agreement.renewal_reminder`, unique by args) at that moment, or immediately if it has already passed (a
late-recorded end date still alerts once, at once); `FireRenewalReminder` re-reads the agreement and no-ops if
its own `effective_to`/`renewal_notice_days` no longer match what the job was scheduled for (changing the
dates naturally reschedules — a stale tick from before the change is harmless) or if the agreement was
terminated. It notifies role LEGAL (the module doc's own actor; SCHED is the job itself, not an RBAC role) —
the same "default recipients until real per-record routing exists" fallback BRE-07/PNG-04/DSAR-07 already use,
since there is no per-agreement owner column to route to more precisely. Migration 00059 seeds
`agreement.renewal_reminder` (th/en × in_app/email) — operational text, no DRAFT marker (rule 8 is about legal
wording shown to a counterparty or the PDPC, not an internal reminder). `agreementservice.Service` gained
`Notify`/`River` fields (nil in any wiring that never calls `SetSchedule`/doesn't need the worker, the same
optional-field pattern `dsarservice.Service` already uses) and `cmd/worker` now builds its own minimal
`agreementSvc` (`Audit`/`Notify`/`River` only) registering `agreementservice.ReminderWorker` — the first
agreement wiring in `cmd/worker` at all.

API: `PATCH /admin/v1/agreements/{id}/schedule` (ETag/If-Match), `effective_to` added to the `Agreement` wire
schema (was missing even for reads). UI: a "Registry: start/end dates & renewal" section on `/agreements/{id}`
with an edit form for all four fields, next to the existing read-only metadata block. Tests: unit
(`RenewalReminderAt`'s own boundary math; `SetSchedule`'s round-trip, stale-ETag refusal, negative
`renewal_notice_days` refusal, and that it leaves a real `river_job` row scheduled at the right moment;
`FireRenewalReminder`'s real `platform.notifications` row for a seeded LEGAL-role user, plus its no-op guards
for a stale schedule and an unknown agreement), HTTP contract (401/403/404/412/422/428/200) through the real
validator + AuthZ. `sqlc generate`/`oapi-codegen`/`openapi-typescript` were run for real; `pnpm --filter
@pdpa/admin build`/`tsc` and the `@pdpa/i18n` ICU message tests both verified clean. Not verified against a
real Postgres in this pass (no reachable database in this environment) — the same gap this session's own
earlier VEN-02 note already flagged.

<a id="dpa-11"></a>
### DPA-11 ผูก DPA กับคู่ค้าและกิจกรรม

*Link to vendor & RoPA*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-01
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** แสดง DPA ของแต่ละคู่ค้า ผลประเมิน และกิจกรรมที่ใช้ผู้ประมวลผลนั้น

**Backend (Go):** ความสัมพันธ์สัญญา ↔ คู่ค้า ↔ กิจกรรม

**Frontend (Next.js):** แท็บความเชื่อมโยงในหน้าสัญญา

**Acceptance criteria:** เปิดคู่ค้าแล้วเห็น DPA และกิจกรรมที่เกี่ยวข้อง

**Implementation — done (ผลประเมินยังไม่มีให้แสดง).** The relationship this feature needs
(agreement ↔ vendor ↔ activity) was already fully built by DPA-02 — `agreement.agreements.vendor_id` and
`agreement.agreement_activities` — and DPA-02's own `ListAgreements` already had a `vendor_id` filter with a
doc comment flagging it as "DPA-11's own 'open a vendor and see its agreements' will reuse this filter". What
was missing: the list endpoint never populated each row's `activity_ids` (only `GetAgreement`, one row at a
time, did) — `ListAgreements` left it `nil` to avoid an N+1 query per page. Fixed with one batch query,
`ListActivityIDsForAgreements` (`agreement_id = ANY($1)`), grouped in Go and attached to each row of the
*already-fetched* page — no extra round trip per agreement, same cost as before for a page with no vendor
filter (an empty id slice is a legal, empty-result `ANY()` call). No new migration, permission or endpoint:
`GET /admin/v1/agreements?vendor_id=...` already existed; it just returns complete rows now.

UI: `/vendors/{id}` (VEN-01's own foundation page, built explicitly for exactly this — "sibling features...
add their own sections to this same page") gained a "DPA / DSA agreements" section — one row per linked
agreement (title, number, status, and its own linked RoPA activities as links to `/ropa/activities/{id}`),
replacing the bare "go look at /agreements yourself" link DPA-02 had left there, which stays as a secondary
link to the full list. "ผลประเมิน" (assessment results) from the module doc's own description is deliberately
not shown: `vendor.vendor_assessments.assessment_id` already points at `assess.assessments` (DPIA-10's own
generic review/approval machinery), but no feature populates a vendor assessment yet — VEN-07 ("Automated
scoring") is the sibling feature that would create one; there is nothing to read until it exists, the same
"no consumer yet" deferral this codebase uses throughout (e.g. ROPA-01's own `discovered_by_finding_id`).
Tests: unit (the acceptance criterion directly — listing by `vendor_id` returns each agreement's own linked
activity ids, matching exactly what `GetAgreement` already returns for one of them; an unfiltered/type-filtered
list still works unchanged), full backend test suite and `pnpm --filter @pdpa/admin build` both verified clean.

<a id="dpa-05"></a>
### DPA-05 ข้อสัญญาการโอนต่างประเทศ

*Cross-border clauses*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศ สคส. ตามมาตรา 29 พ.ศ. 2566
- **Actor:** LEGAL (ฝ่ายกฎหมาย), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPX-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ข้อสัญญามาตรฐานสำหรับการโอนข้อมูลไปต่างประเทศตามประกาศ สคส.

**Backend (Go):** clause มาตรฐานตามประกาศ ม.29 พ.ศ. 2566 แทรกเมื่อมีการโอน

**Frontend (Next.js):** ตัวเลือก clause การโอน

**Acceptance criteria:** สัญญาที่มีการโอนต่างประเทศมี clause ครบ

<a id="dpa-06"></a>
### DPA-06 แก้ไขเอกสารในระบบ

*In-app editor*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ปรับข้อความ ฟอนต์ ย่อหน้า สารบัญ และเพิ่มฟิลด์ในเอกสาร

**Backend (Go):** เพิ่มความสามารถ editor: ฟอนต์ ย่อหน้า สารบัญ ฟิลด์ใหม่

**Frontend (Next.js):** เครื่องมือจัดรูปแบบใน editor

**Acceptance criteria:** ปรับรูปแบบแล้วไฟล์ที่ส่งออกตรงกับที่เห็น

<a id="dpa-07"></a>
### DPA-07 ตรวจทานและอนุมัติ

*Review & approval*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งให้ฝ่ายกฎหมายและ DPO ตรวจ และอนุมัติก่อนลงนาม

**Backend (Go):** ใช้ workflow อนุมัติกลาง: กฎหมาย / DPO ตรวจ

**Frontend (Next.js):** ปุ่มส่งตรวจ + ความเห็น

**Acceptance criteria:** ส่งลงนามได้หลังอนุมัติเท่านั้น

<a id="dpa-08"></a>
### DPA-08 เวอร์ชันและประวัติการดาวน์โหลด

*Versions & download history*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** เก็บประวัติเวอร์ชัน ผู้แก้ไข วันที่ และการดาวน์โหลด

**Backend (Go):** เวอร์ชัน + diff + log การดาวน์โหลด

**Frontend (Next.js):** หน้าประวัติเวอร์ชันและการดาวน์โหลด

**Acceptance criteria:** เห็นว่าใครดาวน์โหลดเวอร์ชันใดเมื่อไร

<a id="dpa-09"></a>
### DPA-09 ส่งออกและลงนามอิเล็กทรอนิกส์

*Export & e-signature*

- **Priority / Phase:** Should · P3 · กลุ่ม: ลงนาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** พ.ร.บ.ว่าด้วยธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), ESIGN (ผู้ให้บริการ e-Signature), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T35
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งออก Word/PDF ดูตัวอย่างก่อนส่ง และส่งลงนามอิเล็กทรอนิกส์

**Backend (Go):** export Word / PDF + preview; เชื่อมบริการลงนามอิเล็กทรอนิกส์ผ่าน API + เก็บไฟล์ที่ลงนาม

**Frontend (Next.js):** ปุ่มส่งลงนาม + สถานะการลงนาม

**Acceptance criteria:** ไฟล์ที่ลงนามแล้วถูกเก็บกลับเข้าระบบอัตโนมัติ

**หมายเหตุ:** เลือกผู้ให้บริการ e-signature ใน T35

<a id="dpa-12"></a>
### DPA-12 แจ้งเตือนผู้ประมวลผลที่ยังไม่มี DPA

*Missing DPA alert*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** SCHED (ระบบ: Scheduler / Event), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPA-11
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ตรวจหาคู่ค้าที่เป็นผู้ประมวลผลแต่ยังไม่มีข้อตกลงที่มีผลบังคับ

**Backend (Go):** job ตรวจคู่ค้าที่เป็นผู้ประมวลผลแต่ไม่มี DPA ที่มีผล → แจ้งเตือน / งาน

**Frontend (Next.js):** รายการคู่ค้าที่ขาด DPA

**Acceptance criteria:** คู่ค้าที่ขาด DPA ถูกพบทุกราย

<a id="dpa-13"></a>
### DPA-13 ติดตามการคืน/ทำลายข้อมูลเมื่อสิ้นสุด

*Return / deletion tracking*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(3), ม.40
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-04
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ขอใบยืนยันการทำลายหรือคืนข้อมูลเมื่อสัญญาสิ้นสุด

**Backend (Go):** สัญญาสิ้นสุด → งานขอใบยืนยันการคืน / ทำลาย (guest link ให้คู่ค้าอัปโหลด)

**Frontend (Next.js):** ขั้นตอนปิดสัญญา

**Acceptance criteria:** ปิดสัญญาได้เมื่อได้รับใบยืนยัน

<a id="dpa-14"></a>
### DPA-14 ตรวจ DPA ที่ได้รับจากลูกค้า

*Inbound DPA review*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ผู้ประมวลผล
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** checklist สำหรับองค์กรที่เป็นผู้ประมวลผล ใช้ตรวจ DPA ที่ลูกค้าส่งมา

**Backend (Go):** checklist ตรวจ DPA ที่ลูกค้าส่งมา (องค์กรเป็นผู้ประมวลผล)

**Frontend (Next.js):** หน้าตรวจ DPA ขาเข้า

**Acceptance criteria:** ผลตรวจระบุ clause ที่ขาดหรือเสี่ยง
