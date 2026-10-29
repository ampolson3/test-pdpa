# DPX — DPO ส่วนต่อขยาย (DPO Extension)

> ระบบบริหารจัดการสำหรับเจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล ภาคส่วนต่อขยาย (DPO Module Extension) · ขอบเขต: อบรม ตรวจประเมินระดับองค์กร retention การโอนต่างประเทศ กฎหมายอื่น และ AI  
> 13 features · Must 2 / Should 6 / Nice 5 · phase: P2 (2), P3 (6), P4 (5)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/gov` |
| PostgreSQL schema | [`gov`](../data/gov.md) (15 ตาราง) |
| Admin API prefix | `/admin/v1/governance` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /governance/*; portal: /learn |
| พึ่งพาบริการ | forms, workflow, ai, connectors |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-17 DPX, BP-11, DFD-1, ERD-14, ERD-15 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | OIDC SSO · Admin app / portal พนักงาน |
| AUDIT | ผู้ตรวจสอบ | Auditor | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| PDPC | สคส. | PDPC (Regulator) | ไม่ login: รับ/ส่งผ่านช่องทางของ สคส. |
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | OIDC + MFA + IP allowlist · Provider console |
| HRIS | HRIS / ITSM / SIEM | HR, ITSM & SIEM | Connector / webhook / SCIM |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DPX-05](#dpx-05) | บริหารการเก็บรักษาและทำลายข้อมูล | Must | P2 | SCHED IT DPO | L | M | Y | BP-11 |
| [DPX-06](#dpx-06) | ประเมินการโอนข้อมูลไปต่างประเทศ (TIA) | Must | P2 | DPO | M | S | N | BP-11 |
| [DPX-01](#dpx-01) | อบรมและทดสอบพนักงาน | Should | P3 | EMP DPO | L | L | Y |  |
| [DPX-02](#dpx-02) | รับทราบนโยบายภายใน | Should | P3 | EMP DPO | S | S | N |  |
| [DPX-03](#dpx-03) | ตรวจประเมินความพร้อมทั้งองค์กร | Should | P3 | AUDIT DPO | M | M | Y |  |
| [DPX-04](#dpx-04) | ติดตามแผนแก้ไขจากการตรวจ | Should | P3 | DPO EMP | S | S | N |  |
| [DPX-08](#dpx-08) | บันทึกการติดต่อกับ สคส. | Should | P3 | DPO PDPC | S | S | N |  |
| [DPX-13](#dpx-13) | เชื่อมระบบ HR / ITSM / SIEM | Should | P3 | IT HRIS | L | S | N |  |
| [DPX-07](#dpx-07) | เครื่องมือ Masking / Pseudonymization | Nice | P4 | IT | L | S | N |  |
| [DPX-09](#dpx-09) | ติดตามกฎหมายและประกาศใหม่ | Nice | P4 | SUPER DPO | M | S | N |  |
| [DPX-10](#dpx-10) | เชื่อมโยงกฎหมายอื่นที่เกี่ยวข้อง | Nice | P4 | DPO | S | S | N |  |
| [DPX-11](#dpx-11) | ผู้ช่วย AI สำหรับ DPO | Nice | P4 | DPO LLM | L | M | Y |  |
| [DPX-12](#dpx-12) | ทะเบียนระบบ AI | Nice | P4 | IT DPO | S | S | N |  |

### ความสัมพันธ์ระหว่าง use case

- DPX-04 «extend» DPX-03 (DPX-04 เป็นทางเลือก/ส่วนขยายของ DPX-03)

## กระบวนการ / sequence / state machine

- [BP-11 ระยะเวลาเก็บรักษาและการทำลายข้อมูล (Retention & disposal)](../processes/BP-11.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [gov.courses](../data/gov.md#gov-courses) | คอร์สอบรม PDPA |
| [gov.training_assignments](../data/gov.md#gov-training-assignments) | การมอบหมายอบรม |
| [gov.training_attempts](../data/gov.md#gov-training-attempts) | ผลการเรียน / สอบรายบุคคล |
| [gov.policies](../data/gov.md#gov-policies) | นโยบาย / คู่มือภายในที่ต้องรับทราบ |
| [gov.policy_attestations](../data/gov.md#gov-policy-attestations) | การรับทราบนโยบาย |
| [gov.audits](../data/gov.md#gov-audits) | การตรวจประเมินความพร้อม PDPA ระดับองค์กร |
| [gov.audit_findings](../data/gov.md#gov-audit-findings) | ข้อตรวจพบและแผนแก้ไข |
| [gov.retention_schedules](../data/gov.md#gov-retention-schedules) | ตาราง retention ต่อระบบ (สร้างจาก RoPA) |
| [gov.disposal_jobs](../data/gov.md#gov-disposal-jobs) | งานลบ / ทำลาย / ทำให้ไม่ระบุตัวตน |
| [gov.regulator_letters](../data/gov.md#gov-regulator-letters) | ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส. |
| [gov.regulatory_updates](../data/gov.md#gov-regulatory-updates) | ฟีดประกาศ / แนวปฏิบัติใหม่ (ทีมเนื้อหาผู้ให้บริการ) |
| [gov.regulatory_reviews](../data/gov.md#gov-regulatory-reviews) | การประเมินผลกระทบของประกาศใหม่ต่อ tenant |
| [gov.ai_systems](../data/gov.md#gov-ai-systems) | ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล |
| [gov.masking_jobs](../data/gov.md#gov-masking-jobs) | งาน mask / tokenize ข้อมูลสำหรับทดสอบ |
| [gov.kb_chunks](../data/gov.md#gov-kb-chunks) | ชิ้นเอกสารสำหรับ RAG ของผู้ช่วย AI (pgvector) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `assessment.security.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `assessment.security` | ประเมินมาตรการความปลอดภัย | DPO `RA` · PRIVACY `R` · IT `RU` · SEC `CRUDX` · AUDIT `R` |  |
| `assessment.transfer` | ประเมินการโอนต่างประเทศ (TIA) | DPO `CRUDA` · PRIVACY `CRU` · LEGAL `RU` · AUDIT `R` |  |
| `dpx.training` | อบรมและทดสอบ | ORGADMIN `R` · DPO `CRUDP` · PRIVACY `CRU` · LEGAL `X` · OWNER `X` · IT `X` · MKT `X` · FRONT `X` · SEC `X` · PROC `X` · AUDIT `R` · EXEC `X` · EMP `X` | พนักงานทุกคนเรียนและทำแบบทดสอบได้ (X) |
| `dpx.policy` | รับทราบนโยบายภายใน | DPO `CRUDP` · LEGAL `CRU` · OWNER `X` · IT `X` · MKT `X` · FRONT `X` · SEC `X` · PROC `X` · AUDIT `R` · EXEC `X` · EMP `X` |  |
| `dpx.audit` | ตรวจประเมินความพร้อมและแผนแก้ไข | DPO `CRUDA` · PRIVACY `CRU` · OWNER `RU` · IT `RU` · SEC `RU` · AUDIT `RE` · EXEC `R` |  |
| `dpx.retention` | Retention และการทำลายข้อมูล | DPO `CRUDA` · PRIVACY `CRU` · OWNER `R` · IT `RUX` · AUDIT `R` | IT ดำเนินการทำลายและแนบหลักฐาน |
| `dpx.regulator` | ทะเบียนการติดต่อกับ สคส. | DPO `CRUD` · PRIVACY `R` · LEGAL `CRU` · AUDIT `R` · EXEC `R` |  |
| `dpx.ai` | ทะเบียนระบบ AI และผู้ช่วย AI | DPO `CRUDAX` · PRIVACY `CRUX` · LEGAL `X` · IT `CRU` · SEC `R` · AUDIT `R` | ผู้ช่วย AI ใช้ได้ตาม license และการเปิดใช้ของ tenant |
| `dpx.integration` | Connector HR / ITSM / SIEM | ORGADMIN `CRUD` · DPO `R` · IT `CRUD` · SEC `R` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `retention.due` | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |
| `disposal.approved` | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |
| `disposal.completed` | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `retention.sweep` | River cron 02:00 | หา record ที่ครบ retention → retention.due | BP-11 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DPX-05, DPX-06
- **P3:** DPX-01, DPX-02, DPX-03, DPX-04, DPX-08, DPX-13
- **P4:** DPX-07, DPX-09, DPX-10, DPX-11, DPX-12

## รายละเอียด feature

<a id="dpx-05"></a>
### DPX-05 บริหารการเก็บรักษาและทำลายข้อมูล

*Retention & disposal*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อมูล
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.33, ม.37(3)
- **Actor:** SCHED (ระบบ: Scheduler / Event), IT (เจ้าของระบบ / IT), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-07, PLT-05
- **Process:** [BP-11](../processes/BP-11.md)

**คำอธิบาย:** ตาราง retention แจ้งเตือนครบกำหนด workflow ลบ/ทำลาย/ทำให้ไม่ระบุตัวตน พร้อมหลักฐาน

**Backend (Go):** ตาราง retention (จาก RoPA), job ตรวจครบกำหนด, workflow ลบ / ทำลาย / ทำให้ไม่ระบุตัวตน มอบหมายเจ้าของระบบ, หลักฐานการทำลาย, legal hold

**Frontend (Next.js):** หน้าตาราง retention + งานทำลายที่ครบกำหนด

**Acceptance criteria:** ข้อมูลครบกำหนดสร้างงานทำลายอัตโนมัติและปิดได้เมื่อแนบหลักฐาน

**Implementation — done.** `internal/gov` — the first feature on the `gov` schema: `gov.retention_schedules`
and `gov.disposal_jobs` were both already fully specified in the baseline migrations (00017), and
`dpx.retention.{read,create,update,delete,approve,execute}` was already seeded (migration 00019) — no new
migration or permission code. `internal/ropa/service`'s `GetRetentionRule` (a new by-id query, mirroring
`GetAsset`'s own role) and the already-exported `GetAsset` give `gov` its two FK-visibility checks (rule 1);
a new `internal/dsar/service/legal_holds.go` (`ActiveHoldForAsset`) gives it the active-legal-hold check on
`dsar.legal_holds` — no import cycle either direction (neither `dsar` nor `ropa` imports `gov`), so both are
plain concrete-type fields on `Service`, not a `wiring`-adapter situation like DPX-06's own ropa↔dpia fix.

Scope deliberately trimmed against BP-11's full diagram, each a plain engineering simplification rather
than a `docs/decisions.md` question (none change the data model, a contract, or legally-relevant behaviour):
BP-11's three human steps (data-owner confirm, DPO approve, IT execute) collapse into the one real
`approved_by` column the schema actually has — `ApproveJob` is both confirm and approve at once, gated on
`dpx.retention.approve`; a legal hold is matched by `asset_id` only, not also `data_category_id` (the
broader match is DSAR-11's own exemption-check concern, not this feature's literal acceptance criterion);
`next_due_at` is entered by a human when creating a schedule, with no automatic trigger-event-date
computation (the system has no way to know when a real-world trigger event — "สิ้นสุดการจ้าง" — actually
happened); and PLT-05's generic workflow/task engine is not used at all, the same reasoning PNG-04's own
indirect-collection deadline already recorded — nothing here needs a per-record task assignee resolved from
a Definition JSON.

The acceptance criterion's first half ("ข้อมูลครบกำหนดสร้างงานทำลายอัตโนมัติ") is a daily sweep mirroring
`internal/platform/audit/jobs/verify.go`'s own `VerifySweeper`/`Verifier` pair exactly: `gov.retention_sweep`
(global, `jobs.GlobalKinds`, daily `river.PeriodicJob`, reusing `platform/audit/store`'s own
`ListLiveTenants`) enqueues one `gov.retention_check` per tenant, which runs inside that tenant's own
transaction (`TenantTxMiddleware`) and calls `Service.RunRetentionCheck` — every `active` schedule whose
`next_due_at` has passed with **no disposal job still open** (`pending`/`approved`/`in_progress`/`on_hold`)
gets a fresh one, using the schedule's own retention rule's `disposal_method`. This "no open job" check is
what keeps the sweep idempotent day over day without needing to advance `next_due_at` itself. A schedule
whose asset is under an active legal hold gets its job created `on_hold` directly (not `pending`), so it
never shows up for approval until the hold lifts — `ApproveJob` re-checks the same hold and refuses with
`ErrOnLegalHold` even if a hold appeared after the job was created `pending`.

The second half ("ปิดได้เมื่อแนบหลักฐาน") is `CompleteJob`'s own gate: `evidence_file_id` is required
(refused with `ErrEvidenceRequired` before ever touching the database), and the file must be the caller's
own clean, still-unattached PLT-09 upload (`Files.Get` + `AttachSystem`, the exact rule every other module's
evidence requirement already uses) — the sqlc query itself takes `evidence_file_id` as a non-nullable
`sqlc.arg`, never `sqlc.narg`, so there is no code path that can write a `done` row without one.

API: `GET`/`POST /admin/v1/governance/retention-schedules`, `PATCH .../retention-schedules/{id}` (pause/
resume/retire, ETag/If-Match), `GET /admin/v1/governance/disposal-jobs` (+ `/{id}`), `POST .../approve`
(refused on an active hold), `/start`, `/complete` (evidence required), `/release-hold` — cursor pagination,
same shape as every other module's list endpoints. UI: a new page, `/governance` — a retention-schedule
list + create form (asset picker from ROPA-02's own `useAssets`, retention rule entered by id since no
cross-activity retention-rule picker exists yet) and a disposal-job list with inline approve/start/
complete-with-evidence (`FileUploader`)/release-hold actions, i18n th/en. Tests: unit (the acceptance
criterion directly — a due schedule gets exactly one job and re-running the sweep doesn't duplicate it; a
not-yet-due schedule is left alone; a paused schedule is never picked up; a held asset's job is created
`on_hold` and blocked from approval until the hold is released; completion refused without evidence, succeeds
and attaches the file once given; two-tenant isolation), HTTP contract (401/403/201/200/428/422) through the
real validator + AuthZ chain. `go build`/`go vet`/`gofmt` clean across the whole backend; `tsc --noEmit` and
`pnpm --filter @pdpa/admin build` both verified clean — no Postgres/Redis reachable in this environment this
round, so tests are compile-verified and skip-cleanly-verified only, the same caveat DPX-06/VEN-07/VEN-08
already recorded. Not done: `ropa.activity_systems`-based asset linking beyond the direct `asset_id` FK
(ROPA-02 never built CRUD on that table either — nothing to link to yet), and a standalone cross-activity
retention-rule picker on the schedule-create form (entering the rule's own id is a deliberate stand-in until
a screen actually needs one).

<a id="dpx-06"></a>
### DPX-06 ประเมินการโอนข้อมูลไปต่างประเทศ (TIA)

*Transfer impact assessment*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อมูล
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.28-29; ประกาศ สคส. ตามมาตรา 28 และ 29 พ.ศ. 2566
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06, ORG-07
- **Process:** [BP-11](../processes/BP-11.md)

**คำอธิบาย:** ประเมินประเทศปลายทางและกลไกการโอน (BCR, ข้อสัญญามาตรฐาน, ใบรับรอง) ตามประกาศ สคส.

**Backend (Go):** แบบประเมินการโอนตามประกาศ ม.28/29 พ.ศ. 2566 (ประเทศปลายทาง, BCR / ข้อสัญญา / ใบรับรอง) ผูกกิจกรรมและคู่ค้า

**Frontend (Next.js):** หน้า TIA

**Acceptance criteria:** การโอนต่างประเทศทุกเส้นทางมีผล TIA อ้างอิง

**Implementation — done.** The module doc's own Go package (`internal/gov`) owns training/policy/audit/
retention/disposal/regulator-letter/AI-registry content (`gov` schema) — none of it related to this
feature. The real table this feature writes to, `ropa.activity_transfers.tia_assessment_id` (FK →
`assess.assessments.id`), was already fully specified in the baseline migrations as part of ROPA-08's own
schema, unused until now — per rule 1 (a module reads/writes only its own schema), this feature's code
lives in `internal/ropa`, not `internal/gov`, following the migration/schema reality over the module doc's
own package assignment (CLAUDE.md's own source-priority order: migrations > docs/states > docs/modules).

No new migration for the linking mechanism itself: `assess.templates.assessment_type`'s own CHECK
constraint already listed `'tia'` since the baseline (migration 00011), unused — the exact same "reserved
but unused" pattern VEN-04 (`docs/decisions.md` Q-31) found for `assessment_type='vendor'`. Migration 00063
(`docs/decisions.md` Q-36) seeds one global (`tenant_id NULL`), published TIA questionnaire — `transfer_tia`,
6 yes_no questions on destination-country adequacy, binding safeguards (BCR/SCC), certification, government
access risk, onward-transfer risk, and enforceability of data subject rights — on the exact same PLT-06
`"assessment"` form type DPIA-03's own generic template library (`CRUD`/`clone`/`publish`/`retire` on
`assess.templates`) already serves without any new code, since it was never hardcoded to one
`assessment_type`. The questionnaire's own wording is a draft pending legal review (name carries
"(ร่าง — รอฝ่ายกฎหมายตรวจ)" directly in the data, rule 8), summarized from this feature's own description and
ม.28-29's transfer mechanisms, not copied from an official PDPC form.

`internal/ropa/service/transfers.go` gained `RecordTransferAssessment(ctx, activityID, transferID,
templateCode, answers)` — VEN-07's own `RecordAssessment` pattern, transplanted: resolve the published
template by code among `assessment_type='tia'` templates, run the answers through dpia's generic
`RecordSubjectAssessment` (`subject_type='transfer'`, already in `assess.assessments`' own CHECK constraint
alongside `'vendor'`/`'activity'`/etc., unused until now), then link the resulting assessment id back onto
the transfer row (`SetTransferTIA`, a plain `UPDATE ... WHERE id = $1 AND activity_id = $2`) — no scoring/
classification step like VEN-07's own `ClassifyScore`, since this feature's acceptance criterion is only
"has a referenced TIA," not a risk-level verdict.

Real import-direction constraint, not a design choice: `dpia/service` already imports `ropa/service`
directly (for its own `Ropa` interface's return types — `ropaservice.Activity`, `ropaservice.
ActivityTransfer`, etc.), so `ropa/service` importing `dpia/service` back would cycle — the identical
situation `internal/wiring/risk.go`'s own `RiskRopa` adapter already documents for `risk/service` ↔
`ropa/service`. Fixed the same way: `ropa/service` declares its own small local `Dpia` interface
(`ListTemplates`/`RecordSubjectAssessment`) plus two mirror structs (`DpiaTemplate`, `TiaResult` — just the
fields this feature needs, not a full copy of dpia's own types), and a new `internal/wiring.RopaDpia`
adapter (built where both concrete packages are already safely importable) converts between them —
`ropaSvc.Dpia = wiring.RopaDpia{Dpia: dpiaSvc}` wired once in `cmd/api/main.go`, nil in `cmd/worker` (no job
there ever records a TIA).

The acceptance criterion itself ("ทุกเส้นทางมีผล TIA อ้างอิง") is a new conditional item in ROPA-03's own
`completeness()`, right next to ROPA-08's own `transfer_basis` item: any `ropa.activity_transfers` row with
no `tia_assessment_id` yet flags `tia_missing` and blocks `/submit` (ROPA-03's own
`len(a.MissingItems) > 0` guard already covers every conditional item, no new code there) — unconditional
on whether a recipient is foreign-coded, since every *logged* transfer (by definition, to another country)
needs its own TIA, not just the ones ROPA-08's own foreign-recipient-coverage check happens to flag.

API: `POST /admin/v1/ropa/activities/{id}/transfers/{transferId}/tia` (`ropa.activity.update`, same
permission as logging the transfer itself; 422 for an unknown/unpublished template_code, 404 for an
unknown transfer) → the updated `ActivityTransfer` (now carrying `tia_assessment_id`). UI: `/ropa/activities/
{id}`'s existing transfers section (`activity-detail-content.tsx`) renders a new `TiaPanel`
(`tia-panel.tsx`, the same `FormRenderer` + `useDpiaTemplates("tia")` + `useForm` pattern VEN-07/VEN-08's
own vendor-assessment panels already use) under each transfer row — a "TIA recorded" badge once set, an
inline answer form otherwise. Tests: unit (the acceptance criterion directly — a logged transfer flags
`tia_missing` until a TIA is recorded, linking clears it and `ListActivityTransfers` reports the id back;
answers scored either direction; an unknown template_code or transfer id refused; two-tenant isolation),
HTTP contract (401/403/200/404/422) through the real validator + AuthZ chain. `go build`/`go vet`/`gofmt`
clean; `tsc --noEmit` and `pnpm --filter @pdpa/admin build` both verified clean — no Postgres/Redis
reachable in this environment this round, so tests are compile-verified only, the same caveat VEN-07/VEN-08
already recorded. Not done: a standalone "/governance" TIA registry page listing every TIA across
activities (the module doc's own frontend note just says "หน้า TIA," read here as the inline panel on each
transfer route it actually names in the acceptance criterion — a cross-activity register has no acceptance
criterion of its own yet) and classifying the TIA's own score into a risk verdict (ม.28/29 doesn't define
scoring bands the way RRA-02's matrix does, and the acceptance criterion only asks that a TIA exist, not
that it pass a threshold).

<a id="dpx-01"></a>
### DPX-01 อบรมและทดสอบพนักงาน

*Awareness training & quiz*

- **Priority / Phase:** Should · P3 · กลุ่ม: บุคลากร
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1), ม.42
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** คอร์ส e-Learning PDPA ภาษาไทย แบบทดสอบ ใบประกาศ และติดตามผลรายบุคคล หรือเชื่อม LMS

**Backend (Go):** LMS ขนาดเล็ก: คอร์ส (วิดีโอ / สไลด์), แบบทดสอบ, เกณฑ์ผ่าน, ใบประกาศ PDF, มอบหมายตามแผนก, รายงาน; หรือเชื่อม LMS ผ่าน SCORM / xAPI

**Frontend (Next.js):** หน้าเรียนใน portal + หน้าจัดการคอร์ส

**Acceptance criteria:** พนักงานเรียนและทำแบบทดสอบได้ และรายงานแสดงผลรายบุคคล

**หมายเหตุ:** OneTrust ขายส่วนนี้ให้ EQS แล้ว (จุดต่าง)

**Implementation — done, scoped to the literal acceptance criterion.** `gov.courses`/`gov.training_
assignments`/`gov.training_attempts` were already fully specified in the baseline migrations (00017_gov.sql,
FK'd in 00018: `content_file_id`/`certificate_file_id` → `platform.files`, `quiz_form_id` →
`platform.form_definitions`, `assignment_id` → `gov.training_assignments`) and `dpx.training.{create,read,
update,delete,publish,execute}` was already seeded (DPO holds create/read/update/publish/delete, every
other seeded role holds `.execute`) — no new migration or permission code, only
`internal/gov/service/training.go` against both. A small built-in LMS, not the module doc's own alternative
("หรือเชื่อม LMS ผ่าน SCORM / xAPI" — `scorm`/`xapi` stay in the schema's own `content_type` CHECK for a
future external-LMS connector this pass doesn't build). A course's own quiz is an ordinary `"quiz"`-type
PLT-06 form the DPO designs and publishes — `'quiz'` has been reserved in `platform.form_definitions.
form_type`'s own CHECK since the very first baseline migration, unused until now (`internal/wiring/
forms.go`'s new `s.Register("quiz", ...)`).

`CreateCourse`/`UpdateCourse`/`SetCourseStatus` are plain CRUD + a `draft -> published -> retired` move
(publishing needs `dpx.training.publish`, retiring reuses `dpx.training.delete` — a permanent status
change, the same reasoning DPIA-03's own template retire and ORG-06's party merge already established for
reusing a "permanent decision" permission rather than minting a new code); a course's content file (video/
slides) is the caller's own clean, still-unattached PLT-09 upload. `AssignTraining` is "มอบหมายตามแผนก":
names who must take a published course and by when — `target_type=all` carries no target id at all;
`org_unit`/`group`/`user` are each checked real/visible before the write (rule 1), a new `Org` interface on
`gov.Service` for the org-unit case (rule 9, no import cycle), `iamservice.GroupNames`/`Names` for the other
two. `MyAssignments` resolves what currently applies to the calling employee — every `"all"` assignment,
plus any naming their own primary org unit (a new `iamservice.PrimaryOrgUnit`, backed by a new
`iam.users.primary_org_unit_id` lookup), one of their own groups (`iamservice.GroupIDsOf`), or them
directly.

`StartAttempt`/`SubmitAttempt` are the acceptance criterion's own first half ("พนักงานเรียนและทำแบบทดสอบ
ได้"): an employee begins a published course (optionally against a real assignment), then completes it —
a course with a quiz is scored through the form engine (`forms.Service.Record`, the same one-step-record
path DPO-09's own security checklist already uses) against the course's own `passing_score` (a percentage
of `max_score`); a quiz-less course is simply marked done with no score, since studying it is enough.
`TrainingReport` is the other half ("รายงานแสดงผลรายบุคคล"): every attempt against a course with the
attempting user's own name resolved (`iamservice.AllNames`) — the per-individual report, read live, never a
stale copy. `MyAttempts` is the employee's own symmetric view.

API: `GET`/`POST /admin/v1/governance/courses`, `GET`/`PUT /admin/v1/governance/courses/{id}` (ETag/
If-Match on PUT), `PATCH /admin/v1/governance/courses/{id}/status` (ETag/If-Match — declares
`dpx.training.update` as its floor permission, the service's own internal check additionally requires
`.publish`/`.delete` per transition, the same "declared floor, stricter internal check" pattern DSAR-11's
own approve-gated reject already uses), `GET /admin/v1/governance/courses/{id}/report`, `POST
/admin/v1/governance/courses/{id}/attempts` (start), `POST /admin/v1/governance/training-attempts/{id}/
submit` (ETag/If-Match), `GET /admin/v1/governance/my-training-attempts`, `GET`/`POST
/admin/v1/governance/training-assignments`, `GET /admin/v1/governance/my-training-assignments` — all added
to `internal/gov/http/oapi-codegen.yaml`'s own `include-operation-ids` allowlist, the recurring gotcha
several prior features already hit. UI: a new "Awareness training & quiz" section on the existing
`/governance` page (DPX-02/DPX-05/DPX-08's own page) — course CRUD + publish/retire, an assign-to form
(everyone/department/group/person + due date), a per-course results-by-individual report, and a "My
training" panel (assigned-to-me list with a start button, past attempts with a submit/mark-complete
action).

Tests: unit (`internal/gov/service/dpx01_test.go` — the acceptance criterion directly: an employee starts
a published course and submits its quiz, scored against `passing_score`, and the resulting report shows
that individual's own result with their name resolved; a quiz-less course completes with no score; an
"all" assignment reaches every employee with no FK target; an org-unit assignment is checked visible first
and reaches only that unit's own member, not an unrelated admin; two-tenant isolation), HTTP contract
(`internal/gov/http/handler_test.go`'s existing `TestGovHandler_Contract`, extended with the full create →
publish → assign → my-assignments → start → submit (428 without If-Match) → report → my-attempts chain
through the real validator + AuthZ). Not verified live against a real Postgres/Redis in this pass — neither
was reachable in this environment for this feature; every test compiles and vets clean and skips the same
way the rest of this module's own suite already does without one. `go build ./...`, `go vet ./...`,
`gofmt -l` (clean on every touched file), `go test ./...` (no regressions — only the pre-existing,
unrelated `internal/pkg/validate` failure), `make gen` re-run with no drift, `pnpm --filter @pdpa/api-client
typecheck`, `pnpm --filter @pdpa/i18n test` and `pnpm --filter @pdpa/admin build` (incl. the updated
`/governance` route) all verified clean. Not done, deliberately (no consumer yet, the same deferral this
codebase uses throughout): a certificate PDF (`certificate_file_id` stays unwritten — the acceptance
criterion is about completing the quiz and the report, not the artifact; render one via
`platform/docs/render` once a screen actually asks for it), SCORM/xAPI external-LMS content, and a
course-validity-expiry reminder (`validity_months` is stored but nothing yet re-assigns an expired
certification — a `gov.course_validity_*`-style sweep, the same VEN-06/VEN-10 pattern, once a real need
surfaces).

<a id="dpx-02"></a>
### DPX-02 รับทราบนโยบายภายใน

*Policy attestation*

- **Priority / Phase:** Should · P3 · กลุ่ม: บุคลากร
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** —

**คำอธิบาย:** พนักงานรับทราบนโยบาย/คู่มือ PDPA พร้อมบันทึกหลักฐาน

**Backend (Go):** เผยแพร่นโยบาย → พนักงานกดรับทราบ + หลักฐาน + ติดตามผู้ยังไม่รับทราบ

**Frontend (Next.js):** หน้ารับทราบนโยบาย

**Acceptance criteria:** รายงานแสดงผู้ที่ยังไม่รับทราบ

**สถานะ:** done — `internal/gov/service/policies.go` (+ `gov/http`), บน `gov.policies`/`gov.policy_attestations` ที่มีอยู่แล้วในฐานข้อมูลตั้งต้น
(ไม่ต้อง migration ใหม่). `PublishPolicy` ติดตามเวอร์ชันที่เผยแพร่ล่าสุดของเอกสาร PLT-16 (ผ่าน `Docs.Get`/`PublishedVersionOf`, รูปแบบ
เดียวกับที่ breach ใช้สำหรับ PDPC filing) — เผยแพร่เอกสารเดิมซ้ำ (ไม่มีเวอร์ชันใหม่) เป็น no-op บนแถวเดิม ไม่สร้างซ้ำ. `AttestPolicy`
บันทึกการรับทราบของพนักงานต่อเวอร์ชันปัจจุบัน (รับทราบซ้ำเป็น no-op) — ไม่มี IP/evidence แยก เพราะ attested_at + user_id ก็เพียงพอ
ตามเกณฑ์. `MissingAttestations` (เกณฑ์การรับรู้ผลลัพธ์โดยตรง) คือ roster ผู้ใช้ active ทั้งหมดของ tenant (`iamservice.AllActiveUsers`,
ใหม่) ลบด้วยผู้ที่รับทราบเวอร์ชันปัจจุบันแล้ว — การรับทราบเวอร์ชันเก่าที่ถูกแทนที่ไม่ถูกนับ. สิทธิ์ใช้โค้ดที่ seed ไว้แล้ว
`dpx.policy.{read,create,update,delete,publish,execute}` (DPO ครบทุกสิทธิ์, LEGAL create/read/update, ทุก role อื่นมีแค่ `.execute`
สำหรับรับทราบ) — ไม่มีโค้ดสิทธิ์ใหม่. API: `GET`/`POST /admin/v1/governance/policies`, `GET /admin/v1/governance/policies/{id}`,
`POST .../attest`, `GET .../attestations`, `GET .../missing-attestations`. UI: ส่วน "นโยบายภายใน" บนหน้า `/governance` เดิม (ฟอร์ม
publish ด้วย document_id, ปุ่มรับทราบ, และแผงรายละเอียดแสดงรายชื่อผู้ยังไม่รับทราบ/ประวัติการรับทราบ). ทดสอบ: unit (เกณฑ์การรับรู้ผลลัพธ์
ตรง — รับทราบแล้วหายจาก missing list, รับทราบซ้ำเป็น no-op, publish ซ้ำเวอร์ชันเดียวกันเป็น no-op, เอกสารที่ยังไม่เผยแพร่ถูกปฏิเสธ,
สิทธิ์ถูกบังคับ, two-tenant isolation — ขับผ่าน PLT-08 submit→DPO approve→publish จริง), HTTP contract ผ่าน validator/AuthZ จริง. ไม่ได้
ยืนยันแบบ live กับ Postgres/Redis จริงในรอบนี้ (ไม่ reachable ในสภาพแวดล้อมนี้) — คอมไพล์/vet ผ่านและ skip อย่างสุภาพเหมือน feature อื่น
ที่เพิ่งผ่านมาในช่วงนี้.

<a id="dpx-03"></a>
### DPX-03 ตรวจประเมินความพร้อมทั้งองค์กร

*Organization-wide audit & maturity*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตรวจสอบ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** AUDIT (ผู้ตรวจสอบ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** แบบประเมินความสอดคล้อง PDPA ระดับองค์กร/บริษัทในเครือ คะแนน และแผนแก้ไข

**Backend (Go):** แบบประเมินความพร้อมระดับองค์กร / บริษัทในเครือ + คะแนนรายด้าน + แผนแก้ไข

**Frontend (Next.js):** หน้าประเมินและผลเปรียบเทียบรายบริษัท

**Acceptance criteria:** ผลประเมินแสดงคะแนนรายด้านและแผนแก้ไข

<a id="dpx-04"></a>
### DPX-04 ติดตามแผนแก้ไขจากการตรวจ

*Audit remediation tracking*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตรวจสอบ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPO-06
- **Process:** —

**คำอธิบาย:** มอบหมายงานตามข้อตรวจพบ ติดตามกำหนดเสร็จ และแนบหลักฐาน

**Backend (Go):** ข้อตรวจพบ → งาน + หลักฐาน + ติดตาม

**Frontend (Next.js):** แท็บแผนแก้ไขในหน้าผลตรวจ

**Acceptance criteria:** ข้อตรวจพบปิดได้เมื่อแนบหลักฐาน

<a id="dpx-08"></a>
### DPX-08 บันทึกการติดต่อกับ สคส.

*Regulator correspondence*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42(3)
- **Actor:** DPO (DPO / Privacy Team), PDPC (สคส.)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-07
- **Process:** —

**คำอธิบาย:** ทะเบียนหนังสือ คำสั่ง การตรวจสอบจาก สคส. และกำหนดส่ง

**Backend (Go):** ทะเบียนหนังสือ / คำสั่ง / การตรวจสอบจาก สคส. + กำหนดส่ง + ไฟล์

**Frontend (Next.js):** หน้าทะเบียนการติดต่อกับ สคส.

**Acceptance criteria:** กำหนดส่งถูกแจ้งเตือนก่อนครบ

**Implementation — done.** `gov.regulator_letters` was already fully specified in the baseline migrations
(00017_gov.sql, FK'd in 00018: `file_id` → `platform.files`, `incident_id` → `breach.incidents`) and
`dpx.regulator.{create,read,update,delete}` was already seeded (DPO holds all four, LEGAL holds
create/read/update, AUDIT holds read) — no new migration or permission code, only
`internal/gov/service/regulator_letters.go` against both: plain CRUD (`SaveRegulatorLetter`/
`GetRegulatorLetter`/`ListRegulatorLetters`/`UpdateRegulatorLetter`/`DeleteRegulatorLetter`), an
`open -> responded -> closed` status move (`SetRegulatorLetterStatus`, stamping `responded_at`
automatically), and the acceptance criterion itself — a single reminder checkpoint before `due_at`, the
exact one-time-per-record pattern VEN-06's own certificate-expiry reminder already established (not a
recurring sweep, since a due date doesn't move on its own the way a review cycle does).

A new `Breach` interface on `gov.Service` (rule 9: `breach/service` never imports `gov`, so no cycle) gives
`incident_id`'s own FK-visibility check (rule 1) without `gov` touching the `breach` schema directly — the
first cross-module read this package has needed beyond `Ropa`/`Dsar`/`Docs`. The reminder itself follows
VEN-06's `scheduleCertificateExpiryReminder`/`FireCertificateExpiryReminder` shape exactly:
`scheduleRegulatorLetterDueReminder` enqueues `gov.regulator_letter_due` (unique per letter)
`RegulatorLetterDueNoticeDays` (7, a plain tunable default per CLAUDE.md's "implement the listed default as
config") ahead of `due_at`, or at once if that moment has already passed; `FireRegulatorLetterDueReminder`
re-reads the letter and no-ops if `due_at` no longer matches what the job was scheduled for or the letter
is no longer `open` (responded/closed/deleted since) — then notifies every DPO and LEGAL user (this record
has no owner column of its own, so there's no single-person fallback the way VEN-06's `relationship_owner_id`
gives one). Migration 00092 seeds the `gov.regulator_letter_due` template (th/en × in_app/email, operational
text, no DRAFT marker — rule 8 is about legal/notice wording, not an internal ops reminder). A regulator
letter's own evidence file (the letter itself, an order, a filed response) is the caller's own clean,
still-unattached PLT-09 upload, attached via `Files.AttachSystem` the same way every other evidence
attachment in this codebase already works.

API: `GET`/`POST /admin/v1/governance/regulator-letters`, `GET`/`PUT`/`DELETE
/admin/v1/governance/regulator-letters/{id}` (ETag/If-Match on PUT), `PATCH
/admin/v1/governance/regulator-letters/{id}/status` (ETag/If-Match). UI: a "Regulator correspondence"
section added to the existing `/governance` page (DPX-02/DPX-05's own page) — a register form
(direction/letter number/subject/received/due date/optional linked incident id), a status/direction-filtered
list, and per-row respond/close/delete actions. Tests: unit
(`internal/gov/service/dpx08_test.go` — the acceptance criterion directly: a real `due_at` schedules exactly
one checkpoint; firing it notifies DPO/LEGAL while the letter is still open and is a harmless no-op once
responded; an unknown `incident_id` is refused as `ErrInvalid`, a known one round-trips; two-tenant
isolation), HTTP contract (`internal/gov/http/handler_test.go`'s existing `TestGovHandler_Contract`,
extended with the full create → list → get → update (428 without If-Match) → respond → delete → 404 chain
through the real validator + AuthZ). Not verified live against a real Postgres/Redis in this pass — neither
was reachable in this environment for this feature; every test compiles and vets clean and skips the same
way the rest of this module's own suite already does without one. `go build ./...`, `go vet ./...`,
`gofmt -l` (clean on every touched file), `go test ./...` (no regressions — only the pre-existing, unrelated
`internal/pkg/validate` failure), `make gen` re-run with no drift, `pnpm --filter @pdpa/api-client typecheck`,
`pnpm --filter @pdpa/i18n test` and `pnpm --filter @pdpa/admin build` (incl. the updated `/governance` route)
all verified clean.

<a id="dpx-13"></a>
### DPX-13 เชื่อมระบบ HR / ITSM / SIEM

*HR, ITSM & SIEM connectors*

- **Priority / Phase:** Should · P3 · กลุ่ม: เชื่อมต่อ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** IT (เจ้าของระบบ / IT), HRIS (HRIS / ITSM / SIEM)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-20
- **Process:** —

**คำอธิบาย:** HRIS (พนักงานเข้า/ออก), ServiceNow/Jira (งาน), SIEM (ส่ง audit log)

**Backend (Go):** HRIS (พนักงานเข้า/ออก → ผู้ใช้ / อบรม), ServiceNow / Jira (สร้างงาน), SIEM (ส่ง audit log แบบ syslog / webhook)

**Frontend (Next.js):** หน้าตั้งค่า connector

**Acceptance criteria:** พนักงานลาออกใน HRIS แล้วบัญชีถูกปิดอัตโนมัติ

<a id="dpx-07"></a>
### DPX-07 เครื่องมือ Masking / Pseudonymization

*Data masking tool*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ข้อมูล
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.33, ม.37(1)
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-13
- **Process:** —

**คำอธิบาย:** mask/tokenize ข้อมูลสำหรับสภาพแวดล้อมทดสอบหรือรายงาน

**Backend (Go):** เครื่องมือ mask / tokenize ไฟล์ CSV และฐานข้อมูลตัวอย่าง (format-preserving สำหรับเลขบัตร / เบอร์)

**Frontend (Next.js):** หน้าสร้างงาน masking

**Acceptance criteria:** ไฟล์ที่ mask แล้วไม่มีค่าจริงหลงเหลือแต่รูปแบบยังถูกต้อง

<a id="dpx-09"></a>
### DPX-09 ติดตามกฎหมายและประกาศใหม่

*Regulatory update feed*

- **Priority / Phase:** Nice · P4 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T40
- **Process:** —

**คำอธิบาย:** แจ้งเตือนประกาศ/แนวปฏิบัติ สคส. ใหม่ และประเมินผลกระทบต่อองค์กร

**Backend (Go):** ฟีดประกาศ สคส. (ทีมเนื้อหาของเราดูแล) → แจ้ง tenant + แบบประเมินผลกระทบ

**Frontend (Next.js):** หน้าฟีดกฎหมาย

**Acceptance criteria:** ประกาศใหม่ถูกแจ้งทุก tenant ภายใน 3 วันทำการหลังประกาศ

<a id="dpx-10"></a>
### DPX-10 เชื่อมโยงกฎหมายอื่นที่เกี่ยวข้อง

*Related laws mapping*

- **Priority / Phase:** Nice · P4 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** พ.ร.บ.การรักษาความมั่นคงปลอดภัยไซเบอร์ พ.ศ. 2562, พ.ร.บ.คอมพิวเตอร์ (เก็บข้อมูลจราจร 90 วัน), ประกาศ ธปท./คปภ./ก.ล.ต.

**Backend (Go):** framework กฎหมายอื่น (พ.ร.บ.ไซเบอร์, พ.ร.บ.คอมพิวเตอร์ – เก็บข้อมูลจราจร 90 วัน, ธปท. / คปภ. / ก.ล.ต.) เป็น checklist

**Frontend (Next.js):** หน้าเลือก framework

**Acceptance criteria:** เลือก framework แล้วได้ checklist พร้อมใช้

<a id="dpx-11"></a>
### DPX-11 ผู้ช่วย AI สำหรับ DPO

*AI assistant*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** DPO (DPO / Privacy Team), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-22
- **Process:** —

**คำอธิบาย:** ร่างคำตอบคำขอใช้สิทธิ สรุป DPIA ตรวจสัญญา และถาม-ตอบกฎหมาย PDPA ภาษาไทย

**Backend (Go):** ผู้ช่วยแชต: ถาม-ตอบ PDPA ภาษาไทย (RAG จากคลังกฎหมายและเอกสารองค์กร), ร่างคำตอบคำขอ, สรุป DPIA, ตรวจสัญญา — ทุกคำตอบอ้างแหล่ง

**Frontend (Next.js):** แผงแชตใน admin

**Acceptance criteria:** ทุกคำตอบอ้างแหล่งและผ่านชุดทดสอบคุณภาพ (T45)

<a id="dpx-12"></a>
### DPX-12 ทะเบียนระบบ AI

*AI governance register*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 15_DPO_Ext
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** IT (เจ้าของระบบ / IT), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** ทะเบียนระบบ AI ที่ประมวลผลข้อมูลส่วนบุคคล พร้อมผลประเมินความเสี่ยง

**Backend (Go):** ทะเบียนระบบ AI + ผลประเมิน (DPIA-17)

**Frontend (Next.js):** หน้าทะเบียน AI

**Acceptance criteria:** ระบบ AI ทุกตัวผูกกับผลประเมิน
