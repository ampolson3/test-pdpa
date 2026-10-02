# PNG — ประกาศความเป็นส่วนตัว (Privacy Notice Generator)

> ระบบจัดการประกาศความเป็นส่วนตัวแบบอัตโนมัติ (Privacy Notice Generator) · ขอบเขต: สร้าง เผยแพร่ และติดตามประกาศความเป็นส่วนตัวและนโยบายที่เกี่ยวข้อง  
> 16 features · Must 7 / Should 8 / Nice 1 · phase: P1 (9), P3 (6), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/notice` |
| PostgreSQL schema | [`notice`](../data/notice.md) (8 ตาราง) |
| Admin API prefix | `/admin/v1/notices` |
| Endpoint ที่ SA กำหนดแล้ว | `GET /public/v1/notices/{slug}` — ประกาศแบบ hosted / embed (BP-04)<br>`POST /public/v1/notices/{id}/acknowledgements` — บันทึกการรับทราบประกาศ (BP-04) |
| หน้าจอ (Next.js) | admin: /notices/*; portal: /notice/[slug] |
| พึ่งพาบริการ | docs, workflow, events |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-05 PNG, BP-04, DFD-1, ERD-07, SEQ-07, ST-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| WEB | เว็บไซต์ / แอปขององค์กร (SDK) | Website / App with SDK | Public key ของ collection point + CORS allowlist |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [PNG-01](#png-01) | สร้างประกาศแบบถาม-ตอบ | Must | P1 | LEGAL | M | L | Y | BP-04 |
| [PNG-02](#png-02) | ตรวจเนื้อหาครบตาม ม.23 | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-03](#png-03) | แม่แบบตามกลุ่มเจ้าของข้อมูล | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-04](#png-04) | ประกาศกรณีเก็บจากแหล่งอื่น | Must | P1 | OWNER SCHED DS | M | S | N | BP-04 |
| [PNG-05](#png-05) | ประกาศสองภาษา | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-06](#png-06) | จัดการเวอร์ชัน | Must | P1 | LEGAL DPO | S | S | N | BP-04 |
| [PNG-07](#png-07) | แจ้งการเปลี่ยนแปลงและขอความยินยอมใหม่ | Must | P1 | DPO DS | M | S | N | BP-04 |
| [PNG-08](#png-08) | เผยแพร่และฝังในระบบ | Should | P1 | WEB | S | M | Y | BP-04 |
| [PNG-14](#png-14) | อนุมัติก่อนเผยแพร่ | Should | P1 | DPO | XS | S | N | BP-04 |
| [PNG-09](#png-09) | บันทึกการรับทราบ | Should | P3 | DS | S | S | N | BP-04 |
| [PNG-10](#png-10) | ดึงเนื้อหาจาก RoPA | Should | P3 | LEGAL | M | S | N | BP-04 |
| [PNG-11](#png-11) | แม่แบบตามอุตสาหกรรม | Should | P3 | LEGAL | XS | S | N | BP-04 |
| [PNG-12](#png-12) | ประกาศแบบหลายชั้นและป้าย CCTV | Should | P3 | LEGAL | S | M | Y | BP-04 |
| [PNG-15](#png-15) | แจ้งเตือนทบทวนตามรอบ | Should | P3 | SCHED DPO | S | XS | N | BP-04 |
| [PNG-16](#png-16) | สร้างนโยบายความเป็นส่วนตัวและนโยบายคุกกี้ | Should | P3 | LEGAL | M | S | N | BP-04 |
| [PNG-13](#png-13) | ลิงก์เอกสารอ้างอิงในประกาศ | Nice | P4 | LEGAL | XS | S | N | BP-04 |

### ความสัมพันธ์ระหว่าง use case

- PNG-01 «include» PNG-02 (ทุกครั้งที่ทำ PNG-01 ต้องทำ PNG-02)
- PNG-01 «include» PNG-05 (ทุกครั้งที่ทำ PNG-01 ต้องทำ PNG-05)
- PNG-10 «extend» PNG-01 (PNG-10 เป็นทางเลือก/ส่วนขยายของ PNG-01)
- PNG-03 «extend» PNG-01 (PNG-03 เป็นทางเลือก/ส่วนขยายของ PNG-01)
- PNG-08 «include» PNG-14 (ทุกครั้งที่ทำ PNG-08 ต้องทำ PNG-14)

## กระบวนการ / sequence / state machine

- [BP-04 จัดทำและเผยแพร่ประกาศความเป็นส่วนตัว (Privacy notice lifecycle)](../processes/BP-04.md)
- [SEQ-07 สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg)](../sequences/SEQ-07.md)
- [ST-04 ข้อตกลง DPA / DSA และประกาศความเป็นส่วนตัว](../states/ST-04.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [notice.notices](../data/notice.md#notice-notices) | ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV |
| [notice.notice_versions](../data/notice.md#notice-notice-versions) | เวอร์ชันที่เผยแพร่ + ผล checklist ม.23 |
| [notice.notice_activity_links](../data/notice.md#notice-notice-activity-links) | กิจกรรม RoPA ที่ประกาศครอบคลุม |
| [notice.acknowledgements](../data/notice.md#notice-acknowledgements) | การรับทราบประกาศ |
| [notice.indirect_collections](../data/notice.md#notice-indirect-collections) | การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25) |
| [notice.embeds](../data/notice.md#notice-embeds) | โค้ดฝังและลิงก์ของประกาศ |
| [notice.linked_documents](../data/notice.md#notice-linked-documents) | เอกสารอ้างอิงที่ลิงก์ในประกาศ |
| [notice.wizard_templates](../data/notice.md#notice-wizard-templates) | template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `notice.document.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `notice.document` | ประกาศความเป็นส่วนตัว | DPO `CRUDAP` · PRIVACY `CRU` · LEGAL `CRUA` · OWNER `R` · IT `R` · MKT `R` · AUDIT `R` · EMP `R` · API `R` | ผู้สร้างกับผู้อนุมัติต้องต่างคน |
| `notice.template` | Template ประกาศ | DPO `CRUD` · PRIVACY `R` · LEGAL `CRUD` · AUDIT `R` |  |
| `notice.indirect` | แจ้งกรณีได้ข้อมูลจากแหล่งอื่น (ม.25) | DPO `CRUD` · PRIVACY `CRU` · OWNER `CRU` · MKT `CRU` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `notice.published` | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |
| `notice.material_change` | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `notice.indirect_due` | รายวัน | แจ้งเตือนก่อนครบ 30 วันของการแจ้งตาม ม.25 | BP-04 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** PNG-01, PNG-02, PNG-03, PNG-04, PNG-05, PNG-06, PNG-07, PNG-08, PNG-14
- **P3:** PNG-09, PNG-10, PNG-11, PNG-12, PNG-15, PNG-16
- **P4:** PNG-13

## รายละเอียด feature

<a id="png-01"></a>
### PNG-01 สร้างประกาศแบบถาม-ตอบ

*Wizard-based generator*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-16, ORG-07
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ตอบคำถามทีละขั้น หรือเลือกตามกลุ่มเจ้าของข้อมูล / กลุ่มวัตถุประสงค์ / หน่วยงาน แล้วระบบสร้างประกาศ

**Backend (Go):** wizard definition (คำถาม → ส่วนของประกาศ), ประกอบเนื้อหาจากคำตอบ + master data + ข้อมูลองค์กรด้วย document composer

**Frontend (Next.js):** wizard ทีละขั้น + preview คู่ขนาน + แก้ข้อความใน editor

**Acceptance criteria:** ผู้ใช้ที่ไม่มีพื้นฐานกฎหมายสร้างร่างประกาศครบหัวข้อได้ภายใน 30 นาที

**หมายเหตุ:** OneTrust ไม่มี wizard (จุดต่าง)

**Implementation (PNG-01):** `backend/internal/notice` — the first module on the `notice` schema
(`notice.document.{read,create,update}`, `notice.template.*`, already seeded in the baseline permission
migration in anticipation of PLT-16's own "notice" document type — no new migration). A notice
(`notice.notices`) is a thin wrapper — legal entity, subject type, slug, ST-04 status — around a PLT-16
document (`document_id`, NOT NULL): `CreateWizard` is the whole wizard in one call — legal entity, notice
type, title, slug and the RoPA processing activities (ROPA-03/06/07/08) it covers — composing the document's
first draft (`docs.Service.Create` + `SaveDraft`) from data the platform already has, rather than a generic
conditional Q&A engine (which would duplicate PLT-06's forms engine for no acceptance-criterion benefit):
for each linked activity, `ListActivityPurposes` (+ ORG-07 lawful basis names), `ListActivityData` (+ data
category names), `ListRetentionRules`, `ListActivityRecipients` and `ListActivityTransfers` (+ ORG-07 country
names) are turned into ม.23 sections (purposes/basis, data collected, retention, recipients/transfers), plus a
fixed rights-of-the-data-subject section and a DPO-contact section built from `mergeField` nodes
(`org_name_th`/`org_email`/`org_phone`/`org_address`) resolved from the legal entity (ORG-01) the same way
every other PLT-16 document resolves them. A topic nothing was linked for becomes a bracketed placeholder
("[โปรดระบุ...]") rather than blocking creation — the acceptance criterion is a *complete* draft within 30
minutes, not a *finished* one; filling in a placeholder, the ม.23 completeness gate (PNG-02), industry/subject
templates (PNG-03/PNG-11), DPO approval (PNG-14) and publish (PNG-08) are all sibling features layered on top
of the same PLT-16 document, not rebuilt here. `notice.notices.status` starts and stays `draft` (ST-04's
`[*] → draft`) — PNG-01 only ever produces that one transition; the rest of ST-04 is built when PNG-02/08/14
are. The slug is typed by the caller, not transliterated from the (often Thai) title — kept simple since
`^[a-z0-9-]+$` is validated and enforced unique per tenant (`uq_notices_slug`) via a `pdb.Savepoint`-wrapped
insert (the established pattern for catching a real unique-constraint violation without aborting the request
transaction). API `/admin/v1/notices` (cursor pagination, same shape as ORG-06/PLT-16/ROPA-02's lists),
`GET /{id}` (includes `activity_ids` via `notice_activity_links`) — editing the composed content itself,
after creation, is PLT-16's own `/admin/v1/platform/documents/{document_id}/draft` (no new endpoint). UI
`/notices`: a wizard form (legal entity, notice type, title, slug, an activity checklist) that on success
routes straight to the PLT-16 document editor page for the freshly composed draft. Tests: unit (the draft
contains every ม.23 topic when an activity is linked — the derived purpose/retention/recipient/transfer text
verified verbatim, not just "non-empty" — and placeholders when none is, validation, duplicate slug, two-tenant
isolation of both the legal entity and the activity FK), HTTP contract (401/403/400/422). Not done, deliberately:
`notice.wizard_templates` (subject-type/industry-driven starter templates and question sets — PNG-03/PNG-11's
job, not PNG-01's, per the module's own «extend» relationships), the ม.23 checklist gate (PNG-02), re-flagging
notices when a linked RoPA activity later changes (PNG-10 — a distinct event-driven feature, not part of the
one-time wizard compose), DPO approval and publish (PNG-14/PNG-08), portal/`/public/v1` hosting and
acknowledgements (no portal feature yet).

<a id="png-02"></a>
### PNG-02 ตรวจเนื้อหาครบตาม ม.23

*Mandatory content checklist*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23 (1)-(6)
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ตรวจ 6 หัวข้อ: วัตถุประสงค์และฐานกฎหมาย, กรณีต้องให้ข้อมูลและผลของการไม่ให้, ข้อมูลที่เก็บและระยะเวลา, ผู้รับข้อมูล, ข้อมูลติดต่อผู้ควบคุม/ตัวแทน/DPO, สิทธิของเจ้าของข้อมูล

**Backend (Go):** rule ตรวจ 6 หัวข้อ ม.23 ก่อน publish, บล็อก publish ถ้าขาดหัวข้อบังคับ (ตั้งค่าได้)

**Frontend (Next.js):** แผงตรวจความครบ + ลิงก์ไปส่วนที่ขาด

**Acceptance criteria:** ประกาศที่ขาดหัวข้อบังคับ publish ไม่ได้

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

**Implementation (PNG-02):** No new endpoint or migration — the gate lives inside the existing publish flow.
PLT-16's document publish (`docs.Service.publish`, called from PLT-08's generic
`POST /admin/v1/platform/record-versions/{id}/publish`) already ran one completeness check (merge fields /
clauses) before this feature; PNG-02 adds a second, notice-specific one at the same point via a small new
platform hook (`docs.Policy` — actually `docs.Service` itself — gained `SetValidate(docType, fn)`, called once
both `docs.Service` and the owning module's own service exist — wired for `"notice"` in `cmd/api/main.go` right
after `noticeSvc` is constructed) rather than baking notice's own business rule into the generic PLT-16/PLT-08
packages. `compose.go`'s headings (PNG-01) now carry a stable topic code (`attrs.topic`) matched against six
checklist items — `purpose_basis`, `consequence`, `data_retention` (needs both the data *and* retention
sections, ม.23 states them as one item), `recipients`, `contact`, `rights` — each complete when its heading
exists and the text under it isn't empty or one of the wizard's own bracketed placeholders (`Checklist`, a pure
function over the document's Thai content — BP-04 rule 5: Thai is the minimum required language). A document
with no topic-coded headings at all (created directly through PLT-16's generic document endpoints, bypassing
the wizard) reads as every topic missing; this checklist only recognizes what PNG-01 itself composes, not
free-form authoring — a known, documented limit rather than a fuzzy text-matching guess. `notice.Service`'s
`CheckPublishable` (the registered validator) looks up the notice by `document_id` (`GetNoticeByDocumentID`,
new sqlc query — still no migration) and blocks with `ErrChecklistIncomplete` (422 `versioning.invalid_request`,
following `docs.IncompleteError`'s own pattern for reporting through PLT-08) whenever a topic is missing.
Configurable per the description's "(ตั้งค่าได้)": `Service.EnforceChecklist` (default true; `NOTICE_CHECKLIST_ENFORCE=false`
turns it off), no `docs/decisions.md` entry needed since it's a tunable, not legally-relevant behaviour.
`GET /admin/v1/notices/{id}/checklist` (`notice.document.read`) reads the same `Checklist` function live off
the current draft, for the UI panel — no separate stored checklist result (unlike `notice_versions.checklist_result`,
which is PNG-08's job at actual publish time, once that table gets a writer). UI: an expandable "ตรวจความครบถ้วน"
row per notice in `/notices`' list showing all six items with ✓/✗. Tests: unit (`Checklist` directly — complete,
one placeholder blocking only its own item, `data_retention` needing both halves, no topic codes at all →
everything missing), integration through the real PLT-08 submit → DPO approve → publish chain (a notice left
with placeholders is blocked at publish with the itemized list; a notice completed *before* submission —
matching the real BP-04 order, content edited ahead of the ม.23 gate — publishes normally; `EnforceChecklist =
false` skips the gate), HTTP contract (401/200/404). Not done: PNG-08's own publish/versioning
(`notice_versions`, hosting), PNG-14's dedicated approval UI (PLT-08's generic `/approvals` inbox already
works), and — deliberately — recovering an *already-approved* version whose publish was blocked: PLT-08 has no
"unapprove" action, only a DPO return-to-draft during review, so a blocked notice must go through a fresh
review round once edited; that is existing PLT-08 behaviour, not something to work around from here.

<a id="png-03"></a>
### PNG-03 แม่แบบตามกลุ่มเจ้าของข้อมูล

*Templates by data subject*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01, T15
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ลูกค้า พนักงาน ผู้สมัครงาน คู่ค้า ผู้มาติดต่อ CCTV ผู้ถือหุ้น สมาชิก

**Backend (Go):** template ตั้งต้น 8 กลุ่มเจ้าของข้อมูล TH/EN (เนื้อหาจาก T15)

**Frontend (Next.js):** หน้าเลือก template ตามกลุ่ม

**Acceptance criteria:** เลือก template แล้วได้ร่างประกาศของกลุ่มนั้นทันที

**Implementation:** T15 resolves via `docs/decisions.md` Q-14 ("ระบบให้กลไก + ข้อความตัวอย่างที่ติดป้าย DRAFT") — the
same "seed DRAFT sample content, flag for legal review" move ORG-07 (Q-20) and ROPA-09 (Q-25) already made.
`platform.templates` and `notice.wizard_templates` were both already fully specified in the baseline migrations
(00002/00007) with the same global (`tenant_id NULL`) + tenant-override RLS pattern as ORG-07's master data — no
schema migration needed, only seed data (migration 00042): 8 groups (customer, employee, job_applicant, vendor,
visitor, cctv, shareholder, member) × th/en = 16 `platform.templates` rows (`template_type = 'notice_wizard'`),
each a full ม.23-topic-coded ProseMirror document (same shape PNG-01's `compose()` produces — every heading
carries the `topic` attr PNG-02's checklist keys off, so a template-sourced draft is checklist-compatible from
the start) with bracketed placeholders for anything group-specific and an opening `[ร่าง — ...]`/`[DRAFT — ...]`
paragraph (CLAUDE.md rule 8), plus a linking `notice.wizard_templates` row per (group, language). `WizardInput`
gained `TemplateGroup string`, mutually exclusive with `ActivityIDs` (refused with `ErrInvalid` if both are set —
the two content sources don't merge); `notice.Service.templateContent` (`internal/notice/service/templates.go`)
loads and JSON-decodes both languages' stored `render.Node` trees directly (no conversion needed, since the seed
data already matches `compose.go`'s own output shape) and `CreateWizard` uses it in place of `compose()` when a
group is picked — everything downstream (document creation, PLT-16 draft save, notice row, activity linking —
skipped when there's no activity) is identical to PNG-01's existing path. `ListTemplateGroups` (backed by
`notice.wizard_templates`, not a hardcoded list) drives the picker; `TemplateGroups` in Go is only the fixed
8-code list the UI's i18n keys are built against. API: `GET /admin/v1/notices/template-groups`
(`notice.document.read`) and `template_group` added to `NoticeWizardInput`. UI: a group `<select>` on the same
wizard form (`/notices`) that, when chosen, disables the activity picker (client-side mirror of the
mutual-exclusivity rule) — picking a group and submitting routes straight to the composed draft exactly like the
RoPA-activity path already did. Tests: unit (`ListTemplateGroups` covers all 8, the acceptance criterion directly
— picking a group produces an immediate draft carrying that group's own sample text and DRAFT marker in both
languages, unknown group refused, group+activity_ids together refused), HTTP contract (200 list, 201 create,
422 unknown group).

<a id="png-04"></a>
### PNG-04 ประกาศกรณีเก็บจากแหล่งอื่น

*Indirect collection notice*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.25
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), SCHED (ระบบ: Scheduler / Event), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05, PLT-04
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แจ้งเจ้าของข้อมูลภายใน 30 วันเมื่อได้ข้อมูลจากแหล่งอื่น พร้อมนับเวลาและแจ้งเตือน

**Backend (Go):** บันทึกการได้ข้อมูลจากแหล่งอื่น (แหล่ง วันที่ จำนวน), นับ 30 วัน, แจ้งเตือน, ส่งประกาศทางอีเมล / SMS หรือบันทึกวิธีแจ้ง + หลักฐาน

**Frontend (Next.js):** หน้ารายการที่ต้องแจ้ง + ตัวนับวัน

**Acceptance criteria:** ระบบเตือนก่อนครบ 30 วัน และปิดรายการได้เมื่อมีหลักฐานการแจ้ง

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

**Implementation (PNG-04):** `notice.indirect_collections` (already fully specified in the baseline
migrations — `notify_due_at date NOT NULL`, `status` pending/notified/overdue/exempted, `method`,
`notified_at`, `evidence_file_id` — no new migration) plus the already-seeded `notice.indirect.*`
permissions (no new permission code). `internal/notice/service/indirect.go` follows BRE-07's exact
deadline pattern rather than instantiating the generic PLT-05 workflow engine: `Checkpoints`/`ToSchedule`
are pure, clock-testable functions (reminders at 20 and 25 days elapsed, overdue at 30 — matching
PLT-05's own worked example for a 30-day SLA) and `notice.indirect_due` River jobs (the exact job name
BP-04's own sequence already names) fire them. This was a deliberate choice, not a literal use of PLT-05
itself: the engine's task assignee is baked into its Definition JSON at the *type* level, not resolvable
per record, and `notice.indirect_collections` has no owner column to resolve one from — so alerts go to
role DPO (`iamservice.UsersWithRole`, migration 00041's two new global notification templates,
`notice.indirect_reminder`/`notice.indirect_overdue`), the same "default recipients until real routing
exists" fallback BRE-07 used before BRE-04. `RegisterCollection` validates the source party (and,
optionally, a linked RoPA activity) and computes `notify_due_at = obtained_at + 30 calendar days` (ม.25
counts calendar days, not business days). `RecordNotice` is the acceptance criterion's other half — method
+ evidence (a PLT-09 file, `Files.Get` + `AttachSystem`) close a `pending` or `overdue` record as
`notified`; a stale `notice.indirect_due` tick after that is a harmless no-op. `exempted` is in the schema
and the new `docs/states/state-machines.yaml#PNG-04` machine but has no transition into it in this pass —
deliberately deferred (ม.25's exemption grounds aren't modeled by any column yet; add the transition when
a screen actually needs it, rather than guessing the UI now). API
`/admin/v1/notices/indirect-collections` (cursor pagination, list + create) and `/{id}` (get),
`/{id}/notify` (ETag/If-Match). UI `/notices/indirect-collections` (linked from `/notices`): a register
form, a status-filtered list with the due date and a colored status badge, and an inline "record notice"
panel (method + `FileUploader` evidence). Tests: unit (`Checkpoints`/`ToSchedule` incl. a late-recorded
event still alerting at once, validation, the acceptance criterion directly — overdue after the 30-day
checkpoint, still closable afterwards with evidence, a stale tick is harmless — two-tenant isolation),
HTTP contract (401/403/201/200/404/412/428/422).

<a id="png-05"></a>
### PNG-05 ประกาศสองภาษา

*TH/EN notices*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-03
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ภาษาไทย-อังกฤษคู่ขนาน และรองรับภาษาอื่นสำหรับแรงงานต่างชาติ

**Backend (Go):** เนื้อหาคู่ขนานหลายภาษาใน composer, ตรวจว่าทุกภาษาเผยแพร่เวอร์ชันเดียวกัน, เพิ่มภาษาแรงงานต่างชาติ

**Frontend (Next.js):** สลับภาษาใน editor และ preview

**Acceptance criteria:** publish ไม่ได้ถ้าฉบับแปลยังไม่อัปเดตตามเวอร์ชันล่าสุด (ตั้งค่าได้)

**Implementation (PNG-05):** "เนื้อหาคู่ขนานหลายภาษาใน composer" and "สลับภาษาใน editor และ preview" were already
built generically by PLT-01/PLT-16 (`render.Content{"th","en"}`; the document editor already switches between
th/en for both editing and preview) — PNG-01's wizard already composes both languages side by side. This
feature's actual new work is only the acceptance criterion itself: a second publish-time gate, reusing the
exact `docs.Service.SetValidate` hook PNG-02 added (no new hook mechanism), extended with one thing PNG-02
didn't need — the *previous* published version's content, so the check has something to diff against. `docs.Service`
gained `previousPublished` (fetches the version being superseded, if any, via the existing `s.Versioning.List`)
and the `Validate` hook signature grew a `previous *Draft` parameter (nil on a document's first publish);
`docsSvc.SetValidate`'s only caller (notice) was updated, so this was a safe, non-breaking-in-practice signature
change. `notice.Service.CheckPublishable` (already PNG-02's gate) now also runs `StaleTranslation(current, previous)`:
stale only when both versions carry English content, the Thai section changed, and the English section did not —
adding English for the first time or removing it entirely is never itself flagged, since neither is "an update
the translation missed". Blocks with `ErrTranslationStale` (422 `versioning.invalid_request`, same reporting
pattern as `ErrChecklistIncomplete`). Configurable per "(ตั้งค่าได้)": `Service.EnforceTranslationSync` (default
true, `NOTICE_TRANSLATION_SYNC_ENFORCE=false` to turn off) — a tunable, no `docs/decisions.md` entry needed.
Read-side: `docs.Service.PublishedContent` (new, generic — reads a document's currently published frozen
content) backs `GET /admin/v1/notices/{id}/translation-status`, so the UI can show the same staleness signal
before anyone actually attempts to submit/publish. Not built, deliberately: "เพิ่มภาษาแรงงานต่างชาติ" (a third,
migrant-worker language) — `render.Content`'s language validation is hard-coded to exactly `th`/`en` throughout
PLT-16 (`Validate()`, `FormatDate`, the DOCX/PDF renderers, the editor's language tabs); adding a third language
is a cross-cutting PLT-16 change with no other feature asking for it yet, well beyond this feature's literal
acceptance criterion, which only mentions the TH/EN pair. UI: the same expandable checklist panel from PNG-02
(`/notices`) now also shows a translation-stale warning when applicable. Tests: unit (`StaleTranslation`
directly — stale only on Thai-changed/English-unchanged, every other combination not stale), integration
through the real PLT-08 submit → DPO approve → publish chain (a second version with an untouched translation is
blocked; one with both languages updated together publishes), the read-side status check before/after a
publish, HTTP contract.

<a id="png-06"></a>
### PNG-06 จัดการเวอร์ชัน

*Versioning*

- **Priority / Phase:** Must · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** เก็บทุกเวอร์ชัน วันมีผล และเปรียบเทียบการเปลี่ยนแปลง

**Backend (Go):** เวอร์ชัน + วันมีผล + diff (PLT-08 / PLT-16)

**Frontend (Next.js):** หน้าประวัติเวอร์ชันและเปรียบเทียบ

**Acceptance criteria:** หน้า public แสดงเวอร์ชันปัจจุบันและดูประวัติย้อนหลังได้

**Implementation (PNG-06):** `docs.Service` gained a second extension point symmetric with PNG-02's own
`SetValidate` — `SetOnPublished(docType, fn)`, called inside the same publish transaction right after PLT-08
freezes a new `platform.document_versions` row, so a module can react to its own document's publish without
`docs` ever reaching back into that module (rule 9 stays intact). `notice.Service.OnDocumentPublished`
(registered in `cmd/api/main.go` next to the existing `SetValidate("notice", ...)` call) inserts one
`notice.notice_versions` row per publish (version_no, document_version_id, languages, effective_from — default
`now()` since no input wires `is_material_change`/`changes_purpose` yet, that is PNG-07's job) and snapshots
PNG-02's own `Checklist(content)` result into `checklist_result` so a later change to the checklist logic can
never rewrite history. A public key (`notice.notices.public_key`, migration 00050) is issued on the *first*
publish only — `SetNoticePublished`'s sqlc query uses `COALESCE(sqlc.narg(public_key), public_key)` so every
later publish leaves an already-bookmarked public URL alone. `publickeys.Middleware`'s single-entity regex was
generalized (`EntityNotice` alongside the existing `EntityCollectionPoint`) rather than adding a second
mechanism, so `/public/v1/notices/{key}` resolves tenant the exact way CON-09's own collection-point links do.
A new, deliberately permission-free `docs.Service.PublicVersionHTML` (reuses the existing `buildInputs`/
`render.HTML` the authenticated `Export` path already uses) serves fully merge-field-resolved HTML to an
anonymous caller — the module's own prior lookup of the version through a notice it owns is what already
proves access, so no fake "public" grant is threaded through the normal `authz`-gated read path.
API: `GET /admin/v1/notices/{id}/versions` (`notice.document.read`); public
`GET /public/v1/notices/{key}`, `/public/v1/notices/{key}/versions`,
`/public/v1/notices/{key}/versions/{versionNo}` (`x-permission: public`). UI: `/notices` gained a
version-history toggle per row (public link or "not published yet", a version_no/effective_from/published_at
table) next to the existing checklist toggle; the portal gained a new public page,
`/[locale]/n/[key]` (mirrors the existing `/[locale]/c/[key]` consent-form pattern exactly — a server
component fetching `/public/v1/notices/...` directly, `?v={no}` for a past version, a history list linking
back to the current one). Tests: unit (`OnDocumentPublished` records the version and issues a key on first
publish only, re-publish reuses it; `PublicNotice` serves the current version and history by number; unknown
key/version is `ErrNotFound`), HTTP contract (401/200/404 on the admin versions list; 404/200 with correct
rendered HTML on all three public endpoints), `pnpm --filter @pdpa/admin build` and
`pnpm --filter @pdpa/portal build` both verified clean including the new routes.

<a id="png-07"></a>
### PNG-07 แจ้งการเปลี่ยนแปลงและขอความยินยอมใหม่

*Change notification*

- **Priority / Phase:** Must · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.21
- **Actor:** DPO (DPO / Privacy Team), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-06, CON-12
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แจ้งเจ้าของข้อมูลเมื่อประกาศเปลี่ยน และขอความยินยอมใหม่หากเปลี่ยนวัตถุประสงค์

**Backend (Go):** เมื่อ publish เวอร์ชันใหม่: ระบุว่าเปลี่ยนสาระสำคัญหรือไม่, ส่งแจ้งเจ้าของข้อมูล, ถ้าเปลี่ยนวัตถุประสงค์ → สร้างคำขอความยินยอมใหม่ใน CON

**Frontend (Next.js):** ขั้นตอนยืนยันการแจ้งเปลี่ยนแปลงตอน publish

**Acceptance criteria:** การเปลี่ยนวัตถุประสงค์สร้างงานขอความยินยอมใหม่อัตโนมัติ

<a id="png-08"></a>
### PNG-08 เผยแพร่และฝังในระบบ

*Hosting & embed*

- **Priority / Phase:** Should · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** WEB (เว็บไซต์ / แอปขององค์กร (SDK))
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-17
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** หน้าเว็บประกาศ ลิงก์ script/iframe ส่งออก Word/PDF และใช้ในแอปหรือ LINE OA

**Backend (Go):** URL ถาวรและต่อเวอร์ชัน, embed script / iframe, ส่งออก Word / PDF, ลิงก์สำหรับ LINE OA และแอป

**Frontend (Next.js):** หน้า public ของประกาศใน portal + ตัวสร้างโค้ดฝัง

**Acceptance criteria:** ประกาศแสดงบนเว็บลูกค้าผ่าน embed และอัปเดตเองเมื่อ publish

**หมายเหตุ:** ดึงเข้า P1 เพราะต้องมีช่องทางเผยแพร่

<a id="png-14"></a>
### PNG-14 อนุมัติก่อนเผยแพร่

*Review & approval*

- **Priority / Phase:** Should · P1 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ส่งตรวจ แสดงความเห็น และอนุมัติโดย DPO/ฝ่ายกฎหมาย

**Backend (Go):** ใช้ workflow อนุมัติกลาง: DPO / กฎหมายตรวจ แสดงความเห็น อนุมัติก่อน publish

**Frontend (Next.js):** ปุ่มส่งตรวจ + กล่องความเห็น

**Acceptance criteria:** ประกาศเผยแพร่ได้หลังอนุมัติเท่านั้น

**หมายเหตุ:** ดึงเข้า P1 (ใช้ PLT-08 จึงใช้แรงน้อย)

<a id="png-09"></a>
### PNG-09 บันทึกการรับทราบ

*Acknowledgement log*

- **Priority / Phase:** Should · P3 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-17
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** บันทึกว่าใครรับทราบประกาศเวอร์ชันใด ผ่านช่องทางใด

**Backend (Go):** บันทึกการรับทราบ (ใคร เวอร์ชัน ช่องทาง เวลา) ผ่าน API / SDK / หน้า public

**Frontend (Next.js):** รายงานการรับทราบต่อเวอร์ชัน

**Acceptance criteria:** ตรวจได้ว่าบุคคลรับทราบประกาศเวอร์ชันใด

<a id="png-10"></a>
### PNG-10 ดึงเนื้อหาจาก RoPA

*Auto-populate from RoPA*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23, ม.39
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03, PLT-11
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ประกอบเนื้อหาจากข้อมูลกิจกรรม และแจ้งเมื่อ RoPA เปลี่ยน

**Backend (Go):** เลือกกิจกรรม RoPA → ประกอบหัวข้อวัตถุประสงค์ / ฐาน / ข้อมูล / ระยะเวลา / ผู้รับ; event ropa.updated → แจ้งให้ทบทวนประกาศ

**Frontend (Next.js):** ปุ่มดึงข้อมูลจาก RoPA ใน wizard

**Acceptance criteria:** RoPA เปลี่ยนแล้วประกาศที่เกี่ยวข้องถูกทำเครื่องหมายให้ทบทวน

<a id="png-11"></a>
### PNG-11 แม่แบบตามอุตสาหกรรม

*Industry templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T40
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ค้าปลีก โรงพยาบาล การเงิน/ประกัน โรงแรม การศึกษา ภาครัฐ

**Backend (Go):** ชุด template 6 อุตสาหกรรม (เนื้อหาจาก T40)

**Frontend (Next.js):** ตัวกรอง template ตามอุตสาหกรรม

**Acceptance criteria:** มี template ครบ 6 อุตสาหกรรมทั้ง TH/EN

<a id="png-12"></a>
### PNG-12 ประกาศแบบหลายชั้นและป้าย CCTV

*Layered notice & CCTV signage*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PNG-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ฉบับย่อ + ฉบับเต็ม และป้าย CCTV พร้อม QR code

**Backend (Go):** ฉบับย่อ + ฉบับเต็ม, สร้างป้าย CCTV พร้อม QR (PDF A4 / A3)

**Frontend (Next.js):** หน้าออกแบบประกาศหลายชั้นและป้าย CCTV

**Acceptance criteria:** ป้าย CCTV พิมพ์ได้และ QR เปิดฉบับเต็มได้

<a id="png-15"></a>
### PNG-15 แจ้งเตือนทบทวนตามรอบ

*Periodic review reminder*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-10
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** เตือนให้ทบทวนตามรอบ หรือเมื่อ RoPA เปลี่ยน

**Backend (Go):** รอบทบทวนต่อประกาศ + trigger เมื่อ RoPA เปลี่ยน

**Frontend (Next.js):** ตั้งรอบทบทวนในหน้าประกาศ

**Acceptance criteria:** เจ้าของประกาศได้รับงานทบทวนตามรอบ

<a id="png-16"></a>
### PNG-16 สร้างนโยบายความเป็นส่วนตัวและนโยบายคุกกี้

*Privacy & cookie policy generator*

- **Priority / Phase:** Should · P3 · กลุ่ม: เอกสารอื่น
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01, CON-05
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** สร้าง Privacy Policy ระดับองค์กรและ Cookie Policy จากข้อมูลในระบบ

**Backend (Go):** template นโยบายระดับองค์กร ประกอบจากข้อมูลองค์กร ประกาศทุกกลุ่ม และตารางคุกกี้

**Frontend (Next.js):** wizard นโยบายองค์กร

**Acceptance criteria:** นโยบายที่สร้างอ้างถึงประกาศและตารางคุกกี้เวอร์ชันล่าสุด

<a id="png-13"></a>
### PNG-13 ลิงก์เอกสารอ้างอิงในประกาศ

*Linked documents*

- **Priority / Phase:** Nice · P4 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แนบลิงก์ตารางระยะเวลาเก็บรักษา นโยบายความปลอดภัย และเอกสารอื่น พร้อมกำหนดการแสดงผล

**Backend (Go):** ลิงก์เอกสารอ้างอิง + รูปแบบการแสดง (inline / popup)

**Frontend (Next.js):** ส่วนจัดการลิงก์ใน editor

**Acceptance criteria:** ลิงก์แสดงตามรูปแบบที่ตั้ง
