# DSA — ข้อตกลงการแบ่งปันข้อมูล (DSA)

> ระบบจัดการเอกสารข้อตกลงการแบ่งปันข้อมูลส่วนบุคคล (Data Sharing Agreement Generator (DSA)) · ขอบเขต: ข้อตกลงการแบ่งปันข้อมูลระหว่างผู้ควบคุมข้อมูล (ไม่ใช่ผู้ประมวลผล)  
> 14 features · Must 6 / Should 6 / Nice 2 · phase: P2 (6), P3 (6), P4 (2)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/agreement` |
| PostgreSQL schema | [`agreement`](../data/agreement.md) (10 ตาราง) |
| Admin API prefix | `/admin/v1/agreements` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /agreements/dsa/* |
| พึ่งพาบริการ | docs, workflow, dataflow |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-16 DSA, BP-10, DFD-1, ERD-13, SEQ-07, ST-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| COUNTER | คู่สัญญา (guest) | Agreement Counterparty | Guest link (token หมดอายุ) + OTP |
| ESIGN | ผู้ให้บริการ e-Signature | e-Signature Provider | API key + callback ลงชื่อ HMAC |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DSA-01](#dsa-01) | ตรวจว่าต้องใช้ DSA หรือ DPA | Must | P2 | OWNER LEGAL | S | S | N | BP-10 |
| [DSA-02](#dsa-02) | ข้อมูลคู่สัญญาและบทบาท | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DSA-03](#dsa-03) | Template DSA มาตรฐาน | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DSA-04](#dsa-04) | สร้างเอกสารอัตโนมัติจาก RoPA | Must | P2 | LEGAL OWNER | S | S | N | BP-10 |
| [DSA-05](#dsa-05) | ข้อกำหนดที่ต้องมี | Must | P2 | LEGAL | S | XS | N | BP-10 |
| [DSA-11](#dsa-11) | ทะเบียนข้อตกลงและแจ้งเตือนหมดอายุ | Must | P2 | LEGAL SCHED | XS | S | N | BP-10 |
| [DSA-06](#dsa-06) | คลังข้อความสัญญา | Should | P3 | LEGAL | S | M | N | BP-10 |
| [DSA-07](#dsa-07) | ภาคผนวกประกอบข้อตกลง | Should | P3 | LEGAL OWNER | S | S | N | BP-10 |
| [DSA-08](#dsa-08) | ตรวจทานและอนุมัติ | Should | P3 | DPO LEGAL | XS | XS | N | BP-10 |
| [DSA-09](#dsa-09) | เวอร์ชันและประวัติ | Should | P3 | LEGAL | XS | XS | N | BP-10 |
| [DSA-10](#dsa-10) | ส่งออกและลงนามอิเล็กทรอนิกส์ | Should | P3 | LEGAL COUNTER ESIGN | XS | XS | N | BP-10 |
| [DSA-12](#dsa-12) | ผูกข้อตกลงกับกิจกรรมและแผนผัง | Should | P3 | DPO | S | S | N | BP-10 |
| [DSA-13](#dsa-13) | ติดตามภาระผูกพันตามข้อตกลง | Nice | P4 | OWNER SCHED | M | S | N | BP-10 |
| [DSA-14](#dsa-14) | รองรับการแบ่งปันข้อมูลระหว่างหน่วยงานรัฐ | Nice | P4 | LEGAL COUNTER | S | S | N | BP-10 |

### ความสัมพันธ์ระหว่าง use case

- DSA-04 «include» DSA-03 (ทุกครั้งที่ทำ DSA-04 ต้องทำ DSA-03)
- DSA-04 «include» DSA-05 (ทุกครั้งที่ทำ DSA-04 ต้องทำ DSA-05)
- DSA-07 «extend» DSA-04 (DSA-07 เป็นทางเลือก/ส่วนขยายของ DSA-04)
- DSA-14 «extend» DSA-03 (DSA-14 เป็นทางเลือก/ส่วนขยายของ DSA-03)

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

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `agreement.dsa.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `agreement.dsa` | ข้อตกลงการแบ่งปันข้อมูล (DSA) | DPO `RA` · PRIVACY `R` · LEGAL `CRUDAP` · OWNER `R` · AUDIT `R` · GUEST `R` |  |
| `agreement.clause` | คลังข้อความสัญญา | DPO `R` · LEGAL `CRUDP` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `agreement.signed` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.expiring` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.terminated` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DSA-01, DSA-02, DSA-03, DSA-04, DSA-05, DSA-11
- **P3:** DSA-06, DSA-07, DSA-08, DSA-09, DSA-10, DSA-12
- **P4:** DSA-13, DSA-14

## รายละเอียด feature

<a id="dsa-01"></a>
### DSA-01 ตรวจว่าต้องใช้ DSA หรือ DPA

*Agreement type check*

- **Priority / Phase:** Must · P2 · กลุ่ม: ตั้งต้น
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.27, ม.40
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ถามบทบาทของผู้รับข้อมูล: ผู้ควบคุมข้อมูลรายอื่นใช้ DSA, ผู้ประมวลผลใช้ DPA ตาม ม.40

**Backend (Go):** คำถามบทบาทผู้รับข้อมูล → แนะนำ DSA หรือ DPA พร้อมเหตุผล (ม.27 / ม.40)

**Frontend (Next.js):** wizard ตรวจประเภทข้อตกลง

**Acceptance criteria:** ผลแนะนำถูกต้องตามบทบาททุกกรณีทดสอบ

**หมายเหตุ:** OneTrust ไม่มี (จุดต่างหลัก)

**Implementation — done.** The first feature on the DSA module, built directly on the existing `agreement`
module (migration 00015) rather than a new schema: `agreement.agreements.agreement_type` already has
`dpa`/`dsa`/`joint_controller`/`inbound_dpa` in its own CHECK constraint, and `our_role`/`counterpartyRole`
already express the controller↔processor relationship (DPA-02's own `counterpartyRole` helper) — so this
feature's whole job is a single pure decision function, no migration, no persisted state.

`internal/agreement/service/type_check.go`'s `RecommendAgreementType(counterpartyRole)` is the literal
acceptance criterion: ask one question (the data recipient's role) and return the recommended
`agreement_type` with its PDPA basis — `processor` → `dpa` (ม.40, a processor acting on our instructions),
`controller` → `dsa` (ม.27, another independent controller receiving or sharing data with us),
`joint_controller` → `joint_controller` (also ม.27, but a distinct legal relationship from an ordinary DSA,
so it keeps its own `agreement_type` value rather than being folded into `"dsa"`). Being pure (no
transaction, no database, no tenant context) makes the acceptance criterion itself ("correct recommendation
for every role") a direct table test.

API: `GET /admin/v1/agreements/type-check?counterparty_role=...` → `{agreement_type, legal_ref, reason_code}`.
Gated on `agreement.dsa.read` (already seeded for LEGAL/OWNER/DPO/PRIVACY/AUDIT/GUEST — this feature's own
actors), not `agreement.dpa.read`, since the question is "which agreement do I need" and the answer can be a
DSA — asking under the DPA permission would be backwards. UI: a small "Agreement type check" widget on
`/agreements` (shown only to callers with `agreement.dsa.read`) — pick the counterparty's role, click check,
see the recommended type + legal reference + a plain-language reason (`agreements.typeCheck.reasons.*`,
th/en) immediately; no RoPA/vendor to pick, no modal, consistent with "within one click" being the whole
point of a check this small. Tests: unit (`type_check_test.go` — the acceptance criterion directly, a table
test over all three roles checking both `agreement_type` and `legal_ref`; an unknown/empty role refused as
`ErrInvalid`), HTTP contract (`handler_test.go` — 200 for all three roles through the real validator +
AuthZ, 401 with no principal, 403 for a caller without `agreement.dsa.read`) — the unit tests ran and passed
against this environment's real toolchain; the HTTP contract test compiles clean but is skipped without a
reachable Redis, consistent with every other agreement-module contract test in this pass.
`pnpm --filter @pdpa/admin build`/`tsc` and the `@pdpa/i18n` ICU message tests all verified clean.

<a id="dsa-02"></a>
### DSA-02 ข้อมูลคู่สัญญาและบทบาท

*Parties & roles*

- **Priority / Phase:** Must · P2 · กลุ่ม: ตั้งต้น
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.27
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ดึงข้อมูลจากทะเบียนหน่วยงานภายนอก รองรับหลายฝ่ายและกรณีผู้ควบคุมร่วม

**Backend (Go):** หลายฝ่ายและผู้ควบคุมร่วม ดึงจาก ORG-06

**Frontend (Next.js):** ส่วนคู่สัญญาใน wizard

**Acceptance criteria:** รองรับข้อตกลงที่มีมากกว่าสองฝ่าย

**Implementation — done.** `agreement.parties` (migration 00015) was already fully specified — party_id
(an ORG-06 external party), legal_entity_id (one of our own, for the joint-controller case), party_role,
signatory — and DPA-02's own `CreateWizard` already wrote exactly one row there (the counterparty), but
nothing read, added to, or removed from that table afterward: `ListAgreementPartiesForAgreement` existed
only as a query raw tests called directly. This feature is the CRUD the acceptance criterion needs —
`internal/agreement/service/parties.go`'s `ListParties`/`AddParty`/`RemoveParty` — no new migration, two new
sqlc queries (`InsertAgreementPartyFull`, `DeleteAgreementParty`).

`AddParty` takes exactly one of `party_id`/`legal_entity_id` (refused otherwise as `ErrInvalid`), checked
visible under RLS the same way DPA-02's own counterparty/legal-entity checks already are (rule 1, via the
existing `Org.GetExternalParty`/`Org.GetLegalEntity`), and a `party_role` from the table's own CHECK set
(`disclosing`/`receiving`/`joint_controller`/`controller`/`processor`). Unlike DPA-03's clauses (locked to
`status = "draft"` since they feed a submit-time completeness check), parties carry no such gate — adding or
removing one never blocks anything else, so every status can still manage its own party list.

API: `GET`/`POST /admin/v1/agreements/{id}/parties`, `DELETE /admin/v1/agreements/{id}/parties/{partyRowId}`
— on the already-seeded `agreement.dpa.read`/`.update` (same convention every other agreement sub-resource
endpoint already uses, since `CreateWizard` still only actually creates `agreement_type = "dpa"` rows; a
real per-type permission split is DSA-04's problem, not this one's). UI: a "Parties" panel on
`/agreements/{id}`, right above the clause panel — the counterparty from creation plus an add form (pick an
external party or one of our own legal entities, then a role) and a remove link per row. Tests: unit
(`parties_test.go` — the acceptance criterion directly: an agreement starts with one party and grows past
two with both an external party and our own legal entity as joint controller; both-set/neither-set/unknown
role/unknown party refused; remove then re-remove is `ErrNotFound`; two-tenant isolation, including that
tenant B can't even resolve tenant A's agreement id), HTTP contract (`handler_test.go` — 200/201/204/403/404/422
through the real validator + AuthZ). `go build`/`go vet`/`gofmt` clean; `tsc --noEmit`,
`pnpm --filter @pdpa/admin build` and `pnpm --filter @pdpa/i18n test` all verified clean — DB-backed test
execution wasn't available in this environment (no reachable Postgres/Redis this pass), so these ran
compile-only, consistent with every other agreement-module test in this session.

<a id="dsa-03"></a>
### DSA-03 Template DSA มาตรฐาน

*DSA templates*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.27 วรรคสอง, ม.37(2)
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02, T34
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** แบบส่งข้อมูลทางเดียว แลกเปลี่ยนสองทาง ระหว่างหน่วยงานรัฐ และเพื่อการวิจัย

**Backend (Go):** template 4 แบบ (ส่งทางเดียว สองทาง หน่วยงานรัฐ วิจัย) บน agreement engine

**Frontend (Next.js):** หน้าเลือก template DSA

**Acceptance criteria:** มี template ครบ 4 แบบ TH/EN

**Implementation — done.** No new Go code at all, the exact shape DPA-01's own migration already took for
"dpa": PLT-16's document composer already registers the `"dsa"` doc type (`internal/wiring.Docs`,
`agreement.dsa.*` permissions — read/create/update/publish already seeded to LEGAL in the baseline RBAC)
and already has its own generic, doc-type-agnostic template library (`internal/platform/docs/library.go`'s
`ListTemplates`/`CreateTemplate`/`UpdateTemplate`/`PublishTemplate` on `platform.templates`) that takes
`doc_type` as an ordinary parameter — nothing ties it to "dpa" specifically. Migration 00060
(`docs/decisions.md` Q-34) seeds the module doc's own four named variants as global (`tenant_id NULL`)
published rows, codes `one_way`/`two_way`/`government`/`research`: one-way disclosure, two-way exchange,
a government-agency variant (citing the receiving agency's own statutory authority alongside ม.24/26/27),
and a research variant (citing ม.24(1) and requiring anonymization/pseudonymization before use or
publication) — each covering parties and roles, purpose, legal basis, usage restrictions (ม.27), security
measures (ม.37(2)), breach notification, data subject rights, and return/destruction/retention, with
`org_name_th`/`org_name_en`/`dpo_name`/`dpo_email` merge fields resolved the same way every other PLT-16
document already does. Content opens with a `[ร่าง — ...]`/`[DRAFT — ...]` banner paragraph (rule 8 — legal
wording stays flagged until Legal reviews it, the same "seed a draft pending review" move
ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01/VEN-04/DPA-01 already made). Frontend: the existing
`/documents/templates` page needed no change — a LEGAL user already sees `"dsa"` as an option there and can
view, clone or author further `dsa` templates with it. Tests: `TestStandardDsaTemplates_SeededReady` (the
acceptance criterion directly — all four templates published, global, with both Thai and English content)
and `TestStandardDsaTemplates_VisibleToAnyTenant` (a second, entirely separate tenant sees all four too,
with nothing of its own) — both compile-checked clean; this environment has no reachable Postgres this
pass, so unlike DPA-01's own migration they were not run against a real database (the same limitation
DSA-01/DSA-02 already noted for the agreement module this session) — the JSON content of all four rows was
validated as well-formed with a standalone script before being written into the migration.

<a id="dsa-04"></a>
### DSA-04 สร้างเอกสารอัตโนมัติจาก RoPA

*Auto-generate from RoPA*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** LEGAL (ฝ่ายกฎหมาย), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ดึงวัตถุประสงค์ ประเภทข้อมูล ฐานกฎหมาย ระยะเวลาเก็บ และผู้รับจากกิจกรรมใน RoPA มาเติมในข้อตกลง

**Backend (Go):** merge จากกิจกรรม RoPA (agreement engine)

**Frontend (Next.js):** ขั้นตอนเลือกกิจกรรมใน wizard

**Acceptance criteria:** ข้อตกลงดึงวัตถุประสงค์ ประเภทข้อมูล ฐาน และระยะเวลาจาก RoPA ถูกต้อง

**Implementation — done.** Two halves, one already built and one genuinely new. The "pull from RoPA" half
is DPA-04's own `ProcessingSchedule` (`internal/agreement/service/schedule.go`) — already
agreement_type-agnostic (reads an agreement's own `ActivityIDs`, never checks `agreement_type`), so it
already satisfies this feature's literal acceptance criterion for a "dsa" agreement with zero new code: the
same live annex (purposes + lawful basis, data categories, retention, security measures) that DPA-04 proved
correct against real RoPA data now works unchanged for DSA.

The genuinely new half is actually *creating* a "dsa" agreement at all: DPA-02's `CreateWizard` only ever
supported `agreement_type = "dpa"`, and its whole counterparty model is a VEN-01 vendor — wrong fit for
DSA, since a DSA counterparty (another controller, a government agency, a researcher) need not be a tracked
vendor relationship. `CreateInput` gained `CounterpartyPartyID`/`CounterpartyRole` alongside the existing
`VendorID`: for `"dpa"` the counterparty is still resolved through `Vendor.GetVendor` with its role derived
from `OurRole` (unchanged); for `"dsa"` the counterparty is any `Org.GetExternalParty` (rule 1 — visible
under RLS) with an *explicit* `party_role` (disclosing/receiving/joint_controller/controller/processor —
DSA-02's own set), since there is no controller↔processor flip to derive it from. `supportedAgreementTypes`
now includes `"dsa"`. API: `vendor_id` is no longer required on `POST /admin/v1/agreements` — give it for
`dpa`, or `counterparty_party_id`+`counterparty_role` for `dsa`. UI: the `/agreements` wizard's vendor picker
is replaced with a counterparty-party + counterparty-role picker whenever `dsa` is selected, and the
template dropdown now filters by the chosen type (previously hard-coded to `"dpa"`). Tests: unit
(`dsa_wizard_test.go` — the acceptance criterion directly: a dsa agreement created from an ORG-06
counterparty (not a vendor) and a RoPA activity has that counterparty on `ListParties` with the given role,
and its `ProcessingSchedule` resolves without error; missing/invalid `counterparty_role` refused;
`agreements_test.go`'s own FK-validation table updated for the new dsa shape), HTTP contract
(`handler_test.go` — a real 201 `dsa` create through `counterparty_party_id`+`counterparty_role`, replacing
the old "dsa not yet supported" 422 case with a real validation 422 for a dsa request missing those fields).
`go build`/`go vet`/`gofmt` clean; `tsc --noEmit`, `pnpm --filter @pdpa/admin build` and
`pnpm --filter @pdpa/i18n test` all verified clean — no reachable Postgres/Redis this pass, so the Go tests
ran compile-only, consistent with every other agreement-module test this session.

<a id="dsa-05"></a>
### DSA-05 ข้อกำหนดที่ต้องมี

*Mandatory clauses*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.27, ม.28-29, ม.37(2)
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DSA-03
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** วัตถุประสงค์ที่ใช้ได้, ห้ามใช้หรือเปิดเผยเกินวัตถุประสงค์, มาตรการความปลอดภัย, การแจ้งเหตุละเมิด, การรองรับคำขอใช้สิทธิ, ระยะเวลาเก็บและการทำลาย, การส่งต่อ, การโอนต่างประเทศ, การสิ้นสุดข้อตกลง

**Backend (Go):** rule clause บังคับ (ม.27, ม.28-29, ม.37(2))

**Frontend (Next.js):** แผงตรวจ clause

**Acceptance criteria:** ข้อตกลงที่ขาด clause บังคับส่งอนุมัติไม่ได้

<a id="dsa-11"></a>
### DSA-11 ทะเบียนข้อตกลงและแจ้งเตือนหมดอายุ

*Agreement register & expiry alerts*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(2)
- **Actor:** LEGAL (ฝ่ายกฎหมาย), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-10
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** รายการข้อตกลง สถานะ วันเริ่ม/สิ้นสุด และแจ้งเตือนก่อนหมดอายุ

**Backend (Go):** ทะเบียน DSA (ใช้ร่วมกับ DPA-10)

**Frontend (Next.js):** หน้าทะเบียน DSA

**Acceptance criteria:** ข้อตกลงใกล้หมดอายุถูกแจ้งเตือน

<a id="dsa-06"></a>
### DSA-06 คลังข้อความสัญญา

*Clause library*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ข้อความมาตรฐาน TH/EN ที่เลือกใช้และปรับได้ มีเวอร์ชัน

**Backend (Go):** คลัง clause TH/EN มีเวอร์ชัน (ใช้ร่วม DPA)

**Frontend (Next.js):** หน้าคลัง clause

**Acceptance criteria:** แก้ clause แล้วเอกสารใหม่ใช้เวอร์ชันล่าสุด

<a id="dsa-07"></a>
### DSA-07 ภาคผนวกประกอบข้อตกลง

*Annexes*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSA-04, DFG-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** รายการข้อมูลที่แบ่งปัน มาตรการความปลอดภัย แบบคำขอข้อมูล และแผนผังการส่งข้อมูล

**Backend (Go):** ภาคผนวก: รายการข้อมูล มาตรการ แบบคำขอข้อมูล ภาพแผนผังจาก DFG

**Frontend (Next.js):** ส่วนภาคผนวกใน editor

**Acceptance criteria:** ภาคผนวกแนบภาพแผนผังล่าสุดได้

<a id="dsa-08"></a>
### DSA-08 ตรวจทานและอนุมัติ

*Review & approval*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPA-07
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งให้ฝ่ายกฎหมายและ DPO ตรวจ แสดงความเห็น และอนุมัติก่อนลงนาม

**Backend (Go):** ใช้ workflow อนุมัติเดียวกับ DPA

**Frontend (Next.js):** ปุ่มส่งตรวจ

**Acceptance criteria:** ส่งลงนามได้หลังอนุมัติเท่านั้น

<a id="dsa-09"></a>
### DSA-09 เวอร์ชันและประวัติ

*Versions & history*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPA-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** เก็บทุกเวอร์ชัน เปรียบเทียบการแก้ไข และบันทึกการดาวน์โหลด

**Backend (Go):** ใช้เวอร์ชันและ log การดาวน์โหลดเดียวกับ DPA

**Frontend (Next.js):** หน้าประวัติเวอร์ชัน

**Acceptance criteria:** เห็นประวัติการแก้ไขและดาวน์โหลด

<a id="dsa-10"></a>
### DSA-10 ส่งออกและลงนามอิเล็กทรอนิกส์

*Export & e-signature*

- **Priority / Phase:** Should · P3 · กลุ่ม: ลงนาม
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** พ.ร.บ.ว่าด้วยธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544
- **Actor:** LEGAL (ฝ่ายกฎหมาย), COUNTER (คู่สัญญา (guest)), ESIGN (ผู้ให้บริการ e-Signature)
- **ขนาดงาน:** BE XS (1 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPA-09
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งออก Word/PDF และส่งลงนามอิเล็กทรอนิกส์

**Backend (Go):** ใช้ connector e-signature ของ DPA-09

**Frontend (Next.js):** ปุ่มส่งลงนาม

**Acceptance criteria:** ไฟล์ที่ลงนามถูกเก็บกลับอัตโนมัติ

<a id="dsa-12"></a>
### DSA-12 ผูกข้อตกลงกับกิจกรรมและแผนผัง

*Link to RoPA & data flow*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DFG-04
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** แสดงข้อตกลงที่รองรับการส่งข้อมูลแต่ละเส้นทาง และแจ้งเตือนเส้นทางที่ยังไม่มีข้อตกลง

**Backend (Go):** ผูก DSA กับเส้นทางข้อมูล + เตือนเส้นทางที่ไม่มีข้อตกลง

**Frontend (Next.js):** แสดงข้อตกลงบนแผนผัง

**Acceptance criteria:** เส้นทางส่งข้อมูลให้ผู้ควบคุมรายอื่นที่ไม่มี DSA ถูกเตือน

<a id="dsa-13"></a>
### DSA-13 ติดตามภาระผูกพันตามข้อตกลง

*Obligation tracking*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(2), ม.37(3)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSA-11
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ติดตามการลบข้อมูลเมื่อสิ้นสุดข้อตกลง การรายงานการใช้ และการทบทวนตามรอบ

**Backend (Go):** ภาระผูกพันราย clause → งาน / รอบรายงาน / ลบข้อมูลเมื่อสิ้นสุด

**Frontend (Next.js):** แท็บภาระผูกพัน

**Acceptance criteria:** ภาระผูกพันที่ถึงกำหนดสร้างงานอัตโนมัติ

<a id="dsa-14"></a>
### DSA-14 รองรับการแบ่งปันข้อมูลระหว่างหน่วยงานรัฐ

*Inter-agency data sharing*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ภาครัฐ
- **ที่มา:** Function List: 06_DSA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** พ.ร.บ.การบริหารงานและการให้บริการภาครัฐผ่านระบบดิจิทัล พ.ศ. 2562
- **Actor:** LEGAL (ฝ่ายกฎหมาย), COUNTER (คู่สัญญา (guest))
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSA-03
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** template และขั้นตอนสำหรับการแลกเปลี่ยนข้อมูลระหว่างหน่วยงานของรัฐ

**Backend (Go):** template และขั้นตอนสำหรับการแลกเปลี่ยนข้อมูลระหว่างหน่วยงานรัฐ

**Frontend (Next.js):** ตัวเลือก template ภาครัฐ

**Acceptance criteria:** สร้างข้อตกลงระหว่างหน่วยงานรัฐได้ครบหัวข้อ
