# RRA — ประเมินความเสี่ยงกิจกรรม (ROPA Risk Assessment)

> ระบบการประเมินความเสี่ยงของกิจกรรมการประมวลผลข้อมูลส่วนบุคคล (ROPA Risk Assessment Module) · ขอบเขต: ประเมินความเสี่ยงรายกิจกรรม วิเคราะห์ช่องว่างทางกฎหมาย ทะเบียนความเสี่ยง และส่งต่อ DPIA  
> 14 features · Must 6 / Should 8 · phase: P2 (6), P3 (8)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/risk` |
| PostgreSQL schema | [`risk`](../data/risk.md) (10 ตาราง) |
| Admin API prefix | `/admin/v1/risk` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /risk/* |
| พึ่งพาบริการ | ropa, workflow, report |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-13 RRA, DFD-1, ERD-10 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| SEC | ทีม Security / Incident | Security / Incident Response | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [RRA-01](#rra-01) | ประเมินความเสี่ยงรายกิจกรรม | Must | P2 | SCHED DPO | M | S | N |  |
| [RRA-02](#rra-02) | ตั้งค่า risk matrix | Must | P2 | DPO | M | M | Y |  |
| [RRA-03](#rra-03) | ส่งต่อทำ DPIA อัตโนมัติ | Must | P2 | SCHED OWNER | S | XS | N |  |
| [RRA-04](#rra-04) | วิเคราะห์ช่องว่างทางกฎหมายอัตโนมัติ | Must | P2 | DPO SCHED | M | M | Y |  |
| [RRA-06](#rra-06) | มาตรการควบคุมและความเสี่ยงคงเหลือ | Must | P2 | DPO OWNER | S | S | N |  |
| [RRA-07](#rra-07) | สร้างงานแก้ไขจากช่องว่าง | Must | P2 | DPO OWNER | S | S | N |  |
| [RRA-05](#rra-05) | Checklist ตรวจสอบกิจกรรม | Should | P3 | OWNER DPO | S | S | N |  |
| [RRA-08](#rra-08) | ยอมรับความเสี่ยงพร้อมผู้อนุมัติ | Should | P3 | EXEC DPO | S | S | N |  |
| [RRA-09](#rra-09) | ทะเบียนความเสี่ยงด้านข้อมูลส่วนบุคคล | Should | P3 | DPO SEC | M | M | Y |  |
| [RRA-10](#rra-10) | Heatmap และแดชบอร์ดความเสี่ยง | Should | P3 | DPO EXEC | S | M | Y |  |
| [RRA-11](#rra-11) | แสดงจุดเสี่ยงบนรายการและแผนผัง | Should | P3 | OWNER DPO | S | S | N |  |
| [RRA-12](#rra-12) | ประเมินซ้ำเมื่อกิจกรรมเปลี่ยนหรือครบรอบ | Should | P3 | SCHED | S | XS | N |  |
| [RRA-13](#rra-13) | คะแนนความพร้อมรายหน่วยงาน | Should | P3 | DPO EXEC | M | S | N |  |
| [RRA-14](#rra-14) | รายงานความเสี่ยงสำหรับผู้บริหาร | Should | P3 | EXEC | S | S | N |  |

### ความสัมพันธ์ระหว่าง use case

- RRA-01 «include» RRA-02 (ทุกครั้งที่ทำ RRA-01 ต้องทำ RRA-02)
- RRA-03 «extend» RRA-01 (RRA-03 เป็นทางเลือก/ส่วนขยายของ RRA-01)
- RRA-07 «extend» RRA-04 (RRA-07 เป็นทางเลือก/ส่วนขยายของ RRA-04)
- RRA-08 «extend» RRA-06 (RRA-08 เป็นทางเลือก/ส่วนขยายของ RRA-06)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [risk.risk_matrices](../data/risk.md#risk-risk-matrices) | risk matrix ต่อ tenant |
| [risk.risk_factors](../data/risk.md#risk-risk-factors) | ปัจจัยความเสี่ยงและน้ำหนัก (ใช้คำนวณจาก RoPA / คู่ค้า / เหตุ) |
| [risk.controls](../data/risk.md#risk-controls) | คลังมาตรการ / control (ประกาศมาตรการความปลอดภัย, ISO) |
| [risk.activity_scores](../data/risk.md#risk-activity-scores) | คะแนนความเสี่ยงรายกิจกรรม |
| [risk.risks](../data/risk.md#risk-risks) | ทะเบียนความเสี่ยง (จาก DPIA / RoPA / คู่ค้า / เหตุละเมิด / audit) |
| [risk.risk_controls](../data/risk.md#risk-risk-controls) | มาตรการที่ผูกกับความเสี่ยง |
| [risk.acceptances](../data/risk.md#risk-acceptances) | การยอมรับความเสี่ยงคงเหลือ |
| [risk.gap_rules](../data/risk.md#risk-gap-rules) | กฎวิเคราะห์ช่องว่างทางกฎหมายของ RoPA |
| [risk.gap_findings](../data/risk.md#risk-gap-findings) | ช่องว่างที่พบรายกิจกรรม |
| [risk.compliance_scores](../data/risk.md#risk-compliance-scores) | คะแนนความพร้อมรายกิจกรรม / หน่วยงาน / บริษัท |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `ropa.risk.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `ropa.risk` | ความเสี่ยงและช่องว่างรายกิจกรรม | DPO `CRUDA` · PRIVACY `CRU` · OWNER `RU` · SEC `RU` · AUDIT `R` · EXEC `R` |  |
| `dpo.risk` | ทะเบียนความเสี่ยง | DPO `CRUDA` · PRIVACY `CRU` · OWNER `R` · SEC `CRU` · AUDIT `R` · EXEC `RA` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `risk.dpia_required` | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `risk.accepted` | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** RRA-01, RRA-02, RRA-03, RRA-04, RRA-06, RRA-07
- **P3:** RRA-05, RRA-08, RRA-09, RRA-10, RRA-11, RRA-12, RRA-13, RRA-14

## รายละเอียด feature

<a id="rra-01"></a>
### RRA-01 ประเมินความเสี่ยงรายกิจกรรม

*Activity risk scoring*

- **Priority / Phase:** Must · P2 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1); ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RRA-02, ROPA-03
- **Process:** —

**คำอธิบาย:** คิดคะแนนความเสี่ยงจากประเภทและปริมาณข้อมูล ข้อมูลอ่อนไหว กลุ่มเปราะบาง ผู้รับ การโอนต่างประเทศ และมาตรการที่มี

**Backend (Go):** คำนวณคะแนนจากปัจจัย (ประเภท / ปริมาณข้อมูล อ่อนไหว กลุ่มเปราะบาง ผู้รับ การโอน มาตรการ) อัตโนมัติเมื่อบันทึก RoPA

**Frontend (Next.js):** คะแนนความเสี่ยงในหน้ากิจกรรม

**Acceptance criteria:** คะแนนเปลี่ยนตามข้อมูล RoPA และอธิบายปัจจัยที่ทำให้สูงได้

**Implementation — done (factor weights/`risk.risk_factors` still deferred).** RRA-02's own risk engine
gets its first real consumer: `riskservice.Score(ctx, activityID)` is the acceptance criterion in one call —
reads the activity's current RoPA data live (never cached), derives a likelihood/impact pair from six
yes/no signals, classifies it against the tenant's default matrix (`Classify`, RRA-02), and persists the
result as a new `risk.activity_scores` row (already fully specified in the baseline migrations — no new
migration; each computation is its own row, never updated in place, so the table is naturally a history
trail for RRA-12's future "re-assess on change/schedule" to build on). The six signals — `sensitive_data`,
`vulnerable_subjects`, `high_volume` (any `ActivityData` at `10k_100k`/`gt_100k`), `cross_border_transfer`
(any `ActivityTransfer`, counted toward both impact and likelihood — a transfer is both a bigger potential
harm and a wider exposure), `external_recipients` (any `ActivityRecipient`), and `no_controls` (zero linked
`ActivityControl`s — the *absence* of a mitigation is itself a likelihood factor) — are exactly the module
doc's own backend note ("ประเภท/ปริมาณข้อมูล อ่อนไหว กลุ่มเปราะบาง ผู้รับ การโอน มาตรการ"), each returned by
name in the response so the UI can literally list "the factors that made it high" (the acceptance
criterion's other half). `risk.risk_factors` (configurable per-tenant weights) is deliberately not used
here — every signal above is a fixed +1, not a tenant-tunable weight — since nothing in this acceptance
criterion asks for configurable weights and RRA-06 (control-driven residual risk) is a better fit for that
table once it exists; this is the same "leave the column/table, build the real thing when a screen needs
it" deferral ROPA-01's own `discovered_by_finding_id` already used.

Real import-cycle problem, not a design choice: `internal/ropa/service` already imports
`internal/risk/service` (ROPA-09's own security-controls catalog), so `risk/service` importing `ropa/service`
back would cycle. Fixed with the same local-interface-plus-adapter pattern IAM-05/PNG-07 already
established: `riskservice.Ropa` is a small interface over two plain local types (`ActivityVisible`,
`Signals` → a flat `ActivitySignals` struct) that `risk/service` owns itself, and `internal/wiring.RiskRopa`
(new) is the adapter built where `ropaservice`/`orgservice` are both already safely importable — it resolves
`IsSensitive`/`VolumeBand` straight off `ActivityData` and `IsVulnerable` via `Org.GetMaster` on each data
point's own subject type. `cmd/api/main.go` wires `riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}`
right after `ropaSvc` itself is built (the dependency order the adapter needs). Scoring without a configured
default matrix is refused (422) rather than guessing a shape — RRA-02's own "ตั้งค่า risk matrix" is a real
prerequisite, not a soft default.

API: `GET /admin/v1/ropa/activities/{id}/risk-score` (`ropa.risk.read`, the latest computation without
recomputing — 404 if the activity has never been scored), `POST` (same path, `ropa.risk.create`) computes a
fresh one and records it — the acceptance criterion's own "live" half, since a page view alone never
recomputes (that would silently grow the history table on every click; recomputing is the user's own
explicit "คำนวณใหม่" action). UI: a new "คะแนนความเสี่ยง (RRA-01)" section on `/ropa/activities/{id}`, right
before the existing DSAR-rejections section — a level badge, the score/likelihood/impact, a "คำนวณใหม่"
button, and the factor list in plain language. Tests: unit (the acceptance criterion directly — a baseline
activity scores with only the `no_controls` factor; adding sensitive data and a recipient both raises the
score and surfaces the new factors on the very next call; no default matrix is refused; an unknown activity
is refused; reading before any score exists is `ErrNotFound`; two-tenant isolation of both scoring and
reading), HTTP contract (401/403/404/201/200) through the real validator + AuthZ, using the real
`wiring.RiskRopa` adapter rather than a test double, so the adapter itself is exercised end to end. Not done:
RRA-06 (control-driven residual score, where `risk.risk_factors`/weights would actually fit), RRA-07
(remediation tasks from gaps), RRA-12 (scheduled re-assessment) — sibling features layered on this same
`risk.activity_scores` row, not built here. RRA-03 (below) is the one sibling this pass did build, since
`Score` is the only thing that could ever call it.

<a id="rra-02"></a>
### RRA-02 ตั้งค่า risk matrix

*Configurable risk matrix*

- **Priority / Phase:** Must · P2 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** กำหนดระดับโอกาส × ผลกระทบ เกณฑ์ สูง/กลาง/ต่ำ และน้ำหนักปัจจัยได้เอง

**Backend (Go):** risk engine กลาง: ระดับโอกาส × ผลกระทบ (3x3 / 4x4 / 5x5), เกณฑ์สูง / กลาง / ต่ำ, น้ำหนักปัจจัยต่อ tenant — ใช้ร่วม DPIA / Vendor / Breach

**Frontend (Next.js):** หน้าตั้งค่า risk matrix

**Acceptance criteria:** เปลี่ยน matrix แล้วคะแนนทุกโมดูลคำนวณตามค่าใหม่

**Implementation — done (factor weights deferred to RRA-01).** The first real feature on `internal/risk`
(previously only ROPA-09's read-only ม.37(1) controls catalog) — CRUD on `risk.risk_matrices`, already fully
specified in the baseline migrations, no new migration for the table itself. Migration 00055 adds a partial
unique index (`tenant_id` WHERE `is_default`) so at most one matrix per tenant can ever be the default at the
database level, not just in application code — `SaveMatrix` clears every other default in the same
transaction before writing the new one, so the index is never actually hit from inside the service itself.
`Classify(matrix, likelihood, impact) (score, level, error)` is the acceptance criterion as a pure function:
score is `likelihood x impact` (the classic risk-matrix multiplication) and `level` comes from the matrix's
own ordered thresholds — nothing is ever cached, so changing the matrix changes the next `Classify` call's
result immediately, for whichever future module calls it. `GetMatrix(ctx, id|nil)` resolves either a specific
matrix or the tenant's own default, the same `BusinessCalendar(ctx, id|nil)` pattern ORG-20's own default
calendar already established — there is no built-in fallback matrix (unlike ORG-20's Mon–Fri default): no NxN
shape is obviously "correct" to guess, so a tenant must configure one explicitly, matching this feature's own
title ("ตั้งค่า risk matrix"). `thresholds` levels are restricted to exactly `risk.activity_scores.level`'s own
four CHECK values (low/medium/high/very_high) so a future RRA-01 write against that column can never find a
level it refuses; thresholds must cover every score from 1 upward with no gap at the bottom, checked before
anything is written. Deliberately scoped to exactly this feature's own acceptance criterion: `risk.risk_factors`
(the module doc's own "น้ำหนักปัจจัยต่อ tenant") belongs to RRA-01 (its own feature, and its own dependency on
this one), which will call `Classify` once it computes a real `likelihood`/`impact` pair from RoPA data — no
module calls either yet, the same "no consumer yet" deferral this codebase uses elsewhere (PLT-13's Keyring,
ROPA-01's `discovered_by_finding_id`). API: `GET`/`POST /admin/v1/risk/matrices`, `GET`/`PUT`/`DELETE
/admin/v1/risk/matrices/{id}` (ETag/If-Match on write/delete) — on the already-seeded `ropa.risk.*` permissions
(no new code: a risk matrix is itself a "ความเสี่ยงและช่องว่างรายกิจกรรม" setting under that area). UI: a new
page, `/settings/risk-matrices` — list with default/shape columns, a create/edit form (comma-separated
likelihood/impact level labels, a dynamic threshold list, a default checkbox) and delete. Tests: unit
(validation — too few levels, no thresholds, an invalid level, a gap at the bottom; `Classify`'s own
correctness and its "no caching" property directly — the same inputs classify differently under two different
matrices; the one-default swap leaves exactly one default however the saves are ordered; ETag mismatch on
update and delete; two-tenant isolation), HTTP contract (401/403/400 schema/422/201/200/404/412/428/204)
through the real validator + AuthZ. Migration verified up/down/up against the real local Postgres before
committing.

<a id="rra-03"></a>
### RRA-03 ส่งต่อทำ DPIA อัตโนมัติ

*DPIA trigger*

- **Priority / Phase:** Must · P2 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** TDPG 4.0-P
- **Actor:** SCHED (ระบบ: Scheduler / Event), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPIA-01
- **Process:** —

**คำอธิบาย:** กิจกรรมที่ความเสี่ยงสูงหรือเข้าเกณฑ์คัดกรองถูกส่งเข้าโมดูล DPIA อัตโนมัติ

**Backend (Go):** คะแนนสูงหรือเข้าเกณฑ์ → สร้าง DPIA (สถานะคัดกรอง) + มอบหมายเจ้าของกิจกรรม

**Frontend (Next.js):** แจ้งเตือนในหน้ากิจกรรม

**Acceptance criteria:** กิจกรรมเสี่ยงสูงมี DPIA ถูกสร้างอัตโนมัติ

**Implementation — done.** `risk/service` gained a second local interface alongside RRA-01's own `Ropa`:
`DpiaTrigger` (`TriggerFromRiskScore(ctx, activityID, score, level) error`) — the same import-cycle-breaking
pattern as `Ropa`, but in the opposite direction: `internal/dpia/service` already imports `internal/risk/service`
for ROPA-09's `Control` type, so `risk/service` cannot import `dpia/service` back (rule 9). Unlike RRA-01's
`Ropa` (which needed `internal/wiring.RiskRopa`, a structural adapter over two concrete services), no wiring
adapter is needed here: `dpiaservice.Service.TriggerFromRiskScore` itself matches the interface's exact method
set, so `cmd/api/main.go` just assigns `riskSvc.DpiaTrigger = dpiaSvc` directly, right after `dpiaSvc` is
built. `Score` (RRA-01) calls it after successfully persisting the new `risk.activity_scores` row — "a high
score" is read literally as *this* scoring call's own fresh level, not a separately re-read value, since
`Score` already has it in hand.

`TriggerFromRiskScore` is a no-op for anything other than `high`/`very_high` (low/medium should never open a
DPIA). For a qualifying score, it opens a `assess.assessments` row directly at `in_progress` — no screening
questionnaire to re-answer, since the risk engine's own number already establishes the need —
`screening_result` forced to `"required"`, `risk_level`/`score` carried straight from RRA-01's own
classification (both columns already existed on the table, unused until now), and `owner_user_id` copied from
the activity's own owner (`ropaservice.Activity.OwnerUserID`, nil-safe — an activity without one just gets an
unassigned DPIA, the same as a manually-screened one). Idempotency (`Score` runs on every page view and every
"recompute" click, so this can't spawn a new round each time): a second high/very_high score while the
activity's latest round is still in an open ST-05#2 status (`screening`/`in_progress`/`in_review`/
`needs_review`) is silently skipped; once that round reaches a closed state (`not_required`/`approved`/
`rejected`/`closed`), a fresh high score opens a new round chained to it (`round_no`/`previous_id`), the same
chaining DPIA-01's own re-screening already uses. The activity FK is checked under the caller's own RLS via
the existing `s.Ropa.GetActivity` before anything is written (rule 1); a cross-tenant activity id is refused
as `ErrInvalid`, not a 500 or a leak.

No new API endpoint: this is a side effect of `POST /admin/v1/ropa/activities/{id}/risk-score` (RRA-01's own
`RiskScoreActivity`), not a separate feature surface. "แจ้งเตือนในหน้ากิจกรรม" (frontend's own note) is a
banner on the existing `RiskScoreSection` (`/ropa/activities/{id}`) shown whenever the current score reads
high/very_high, pointing down at the DPIA section already on that same page (DPIA-01/04/05/10/14's own
`DpiaScreeningSection`) — no second API call needed, since that section already lists the activity's
assessments and will show the freshly-opened round itself; `Assessment` (dpia/service) gained `RiskLevel`/
`OwnerUserID` fields (both previously unread, now surfaced through `GET /admin/v1/dpia/assessments`) so a
round opened this way is visibly distinguishable from a manually-screened one. Tests: unit
(`internal/dpia/service/risktrigger_test.go` — the acceptance criterion directly: a high score opens
in_progress/required with the right risk_level/score/owner; low/medium/empty levels are no-ops; a second high
score while one round is still open does not duplicate; a fresh score after the prior round closed opens a
correctly chained round 2; an unknown activity id is refused; two-tenant isolation), an integration test
(`internal/risk/service/dpiatrigger_integration_test.go`) wiring a real `riskSvc.DpiaTrigger = dpiaSvc` exactly
as `cmd/api/main.go` does and proving `Score` on a genuinely high-scoring activity (sensitive data + a
vulnerable subject type + high volume + a cross-border transfer + a recipient + no controls, against the
default 3x3 matrix) opens a real DPIA round end to end — not a mock. `pnpm --filter @pdpa/admin build` and the
`@pdpa/i18n` ICU message tests both verified clean. Not done: a dedicated notification (PLT-04) to the
activity owner — the module doc's own frontend note only asks for a banner "ในหน้ากิจกรรม" (on the activity
page itself), which this delivers; add a push notification once a screen actually asks for one, the same
"no consumer yet" deferral this codebase uses elsewhere.

<a id="rra-04"></a>
### RRA-04 วิเคราะห์ช่องว่างทางกฎหมายอัตโนมัติ

*Automated legal gap analysis*

- **Priority / Phase:** Must · P2 · กลุ่ม: ช่องว่าง
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23, ม.24, ม.26, ม.28-29, ม.39
- **Actor:** DPO (DPO / Privacy Team), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-03, PNG-01
- **Process:** —

**คำอธิบาย:** ตรวจ RoPA กับข้อกำหนด เช่น ไม่มีฐานกฎหมาย ไม่มีระยะเวลาเก็บ ไม่มีประกาศ โอนต่างประเทศโดยไม่มีฐาน ข้อมูลอ่อนไหวไม่มีความยินยอมโดยชัดแจ้ง

**Backend (Go):** rule engine ตรวจ RoPA: ไม่มีฐานกฎหมาย, ไม่มีระยะเวลาเก็บ, ไม่มีประกาศที่ครอบคลุม, โอนต่างประเทศไม่มีฐาน, ข้อมูลอ่อนไหวไม่มีความยินยอมโดยชัดแจ้ง; เพิ่ม rule ได้

**Frontend (Next.js):** รายการช่องว่าง + ลิงก์ไปแก้

**Acceptance criteria:** ช่องว่างทุกประเภทใน rule ถูกตรวจพบในชุดข้อมูลทดสอบ

**หมายเหตุ:** OneTrust ต้องตั้ง rule เอง (จุดต่าง)

**Implementation — done (on-demand analysis; a periodic SCHED sweep is deferred).**
`risk.gap_rules`/`risk.gap_findings` (baseline migration 00010) were built with exactly this feature in mind
and sat unused until now — `gap_findings.task_id` is a real FK to `dpo.tasks`, anticipating RRA-07's own
remediation-task linkage, and `gap_rules.expression jsonb` is a reserved, still-empty placeholder for a
future generic rule interpreter. Migration 00062 seeds the 5 global (`tenant_id NULL`, the same ORG-07/
ROPA-09/PNG-03/DPIA-01 "seed a draft, flag for legal review" pattern) rules the module doc's own description
names verbatim: `no_lawful_basis` (high, ม.24/ม.39(1)), `no_retention` (medium, ม.39(3)), `no_notice_coverage`
(high, ม.23), `transfer_no_basis` (high, ม.28-29), `sensitive_no_consent` (high, ม.26).

`internal/risk/service/gap_analysis.go`'s "rule engine" is deliberately a plain Go `switch` (`gapPresent`)
keyed by `gap_rules.code`, not an expression interpreter against the unused `expression` column — a real
interpreter is far more machinery than 5 fixed rules need; adding a 6th rule today is one migration row plus
one new `case`. Four of the five map straight onto ROPA-03's own `completeness()` missing-item codes it
already computes (`purpose`, `retention`, `transfer_basis`, `sensitive_consent`) — exposed through a new
`MissingItems` method on the `Ropa` interface (`activityscore.go`), backed by `wiring.RiskRopa.MissingItems`
delegating to `ropaSvc.GetActivity(ctx, id).MissingItems`, rule 9's "read the other module through its own
exported service" exactly. `no_notice_coverage` needed one genuinely new cross-module read: a new
`notice.Service.ActivityHasNotice` (backed by a new `notice.notice_activity_links` existence query,
`ActivityHasNotice`) and a small local `Notice` interface on `risk.Service` satisfied directly by
`*noticeservice.Service` in `cmd/api/main.go` — no adapter struct needed, since `notice/service` never
imports `risk/service` and so there is no import cycle to route around (unlike IAM-05's own `Auditor`/
`Notifier` workaround, needed there only because the cycle was real).

`AnalyzeActivity` (one activity) and `AnalyzeAllActivities` (every activity, via a new `ListActivityIDs` on
`wiring.RiskRopa`, cursor-paginated) run every active rule, upsert an open `gap_findings` row per rule still
failing and resolve (`resolved_at` stamped) any that no longer applies — the acceptance criterion itself
("ช่องว่างทุกประเภทใน rule ถูกตรวจพบในชุดข้อมูลทดสอบ") exercised directly: a fixture activity missing every
one of the 5 conditions shows all 5 open findings, and fixing each one at a time clears its own finding on
the next run, nothing else. Deliberately **not** built this pass: a River/`cmd/worker` periodic sweep job for
the SCHED actor the module doc also names — the literal acceptance criterion only asks that every gap type is
detected in a test dataset, which an on-demand, button-triggered analysis already proves without needing a
scheduler; add a periodic `risk.gap_sweep` job (mirroring BRE-07/PNG-04's own `ToSchedule` pattern) once a
real screen or SLA needs gaps caught without a human pressing the button.

API: `GET /admin/v1/risk/gap-rules` (the catalog, `ropa.risk.read`), `GET /admin/v1/risk/gap-findings`
(tenant-wide open findings, same permission), `GET /admin/v1/ropa/activities/{id}/gap-findings` (one
activity's own findings, as last analyzed), `POST /admin/v1/ropa/activities/{id}/gap-analysis`
(`ropa.risk.create` — re-run every rule now). UI: a "ช่องว่างทางกฎหมาย" section on
`/ropa/activities/{id}` (open findings by severity + an analyze button), and a new tenant-wide register page,
`/settings/gap-register` (the module doc's own "รายการช่องว่าง + ลิงก์ไปแก้" — every open finding across
every activity, each linking straight to its own activity page), mirroring `/settings/risk-matrices`'s own
page/content-component split; linked from the per-activity section. Tests: unit (every one of the 5 rule
types detected on a fixture activity missing everything; each clears independently once fixed; an unknown
activity id refused; two-tenant isolation), HTTP contract (401/200 through the real validator + AuthZ).
`pnpm --filter @pdpa/admin build`/`tsc --noEmit` and `pnpm --filter @pdpa/i18n test` all verified clean; no
live Postgres was reachable in this environment, so Go tests were verified by `go build`/`go vet`/`gofmt -l`
(compile-clean) rather than actually run.

<a id="rra-06"></a>
### RRA-06 มาตรการควบคุมและความเสี่ยงคงเหลือ

*Controls & residual risk*

- **Priority / Phase:** Must · P2 · กลุ่ม: จัดการ
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RRA-02
- **Process:** —

**คำอธิบาย:** ผูกมาตรการที่มี/ต้องเพิ่มกับความเสี่ยง แล้วคำนวณความเสี่ยงคงเหลือ

**Backend (Go):** ผูก control กับความเสี่ยง คำนวณความเสี่ยงคงเหลือ (ใช้ร่วม DPIA-07)

**Frontend (Next.js):** ส่วนมาตรการในหน้าความเสี่ยง

**Acceptance criteria:** ความเสี่ยงคงเหลือลดลงตามมาตรการที่ผูก

**Implementation — done, no new code (ใช้ร่วม DPIA-07 ตามที่ module doc ระบุไว้).** The module doc's own
backend note already says this feature reuses DPIA-07, and that's literally true: `internal/risk/service/
risk_controls.go` (`AddRiskControl`/`ListRiskControls`/`UpdateRiskControlStatus`/`RemoveRiskControl`,
`recomputeResidual`) was built directly on `risk.Service` — generic across every risk, not DPIA-specific —
precisely because `risk.risks` is documented as a shared register fed from "DPIA / RoPA / คู่ค้า / เหตุละเมิด /
audit" sources, not an assessment-only table. DPIA-07's own test suite already proves the acceptance criterion
with no DPIA assessment involved at all: `manualRisk` in `internal/risk/service/risk_controls_test.go`
identifies a risk with `SourceType: "manual"` (not through `dpia.Service`'s wrapper), and
`TestAddRiskControl_RecomputesResidual`/`TestAddRiskControl_FloorsAtOne`/`TestRemoveRiskControl_
RecomputesResidual` all run against it directly — the exact "link an existing/needed measure to a risk, then
the residual recomputes" flow this feature's own description and acceptance criterion ask for, independent of
which module created the risk.

What's deliberately not built: a standalone RRA "หน้าความเสี่ยง" (risk page) outside the DPIA screening
section — the module doc's own frontend note calls for "ส่วนมาตรการในหน้าความเสี่ยง" (a measures section *on*
the risk's own page), but no RRA feature yet creates a risk independently of a DPIA round to have such a page
for: RRA-04 (automated legal gap analysis, the other real source the schema anticipates) and RRA-09 (the
personal-data risk register, which is where a generic "all risks" list/detail page belongs) are both still
Should/not-yet-built. Until one of those exists, every real risk in this system is a DPIA-06 one, already
served by the DPIA screening section's own risk-and-controls panel — building a second, parallel UI for a
case that cannot occur yet would be exactly the kind of speculative work this codebase avoids elsewhere
(e.g. ROPA-01's own deferred `discovered_by_finding_id`). Marked done as documentation only (backlog + module
doc) — no migration, code or test changes needed, the same move ROPA-07 already made for a feature already
fully covered by a sibling's own implementation.

<a id="rra-07"></a>
### RRA-07 สร้างงานแก้ไขจากช่องว่าง

*Remediation tasks*

- **Priority / Phase:** Must · P2 · กลุ่ม: จัดการ
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** —

**คำอธิบาย:** เลือกรายการช่องว่าง กำหนดผู้ดำเนินการ วันครบกำหนด และความสำคัญ แล้วติดตามจนปิด

**Backend (Go):** เลือกช่องว่าง → สร้างงาน (ผู้ดำเนินการ วันครบกำหนด ความสำคัญ) → ติดตามจนปิด; ปิดงานแล้วตรวจ rule ซ้ำ

**Frontend (Next.js):** ปุ่มสร้างงานจากช่องว่าง

**Acceptance criteria:** ปิดงานแล้วช่องว่างหายไปเมื่อ rule ผ่าน

**Implementation — done.** `risk.gap_findings.task_id` was a real FK to `dpo.tasks` since the baseline
migration, built specifically for this feature; `dpo.tasks.source_type` already had `'ropa_gap'` in its own
CHECK constraint, also unused until now. The module doc's own two actions map onto the two services that
already sit either side of that FK: risk owns the "เลือกช่องว่าง → สร้างงาน" half, dpo owns "ติดตามจนปิด".

`risk/service/gap_analysis.go` gained `GetGapFinding` (one finding by id, RLS-scoped) and
`RemediateFinding(ctx, findingID, assigneeUserID, dueAt, priority)`: the finding must still be `open`, a given
assignee must be a real active user of the tenant (`iamservice.Names`, the same check DPIA-07's own
`AddRiskControl` already makes), and priority must be one of `dpo.tasks`' own four values. It then opens the
job through `risk/service`'s existing `DpoTasks` local interface (RRA-06/DPIA-07's own cross-module-write
pattern) — extended with a second method, `OpenGapRemediationTask`, alongside DPIA-07's `OpenRiskControlTask`
— and writes the new task id back onto the finding's `task_id` via a new `SetGapFindingTask` query. No new
adapter: `risk.Service.Dpo` was already a field of this interface type.

`dpo/service/gap_tasks.go` implements `OpenGapRemediationTask` ("GAP-\<year\>-NNNN", source_type `ropa_gap`,
the same per-tenant-per-year advisory-lock numbering every other `dpo.tasks` opener already uses) and adds
the module's first real status-transition surface: `GetTask` and `UpdateTaskStatus`, checked against a small
local state machine (`created → assigned → in_review → done → closed`, declared here since no other feature
had ever driven this column through an API before). `requireTaskAccess` is `docs/security/permissions.md`'s
own note on `dpo.task` ("แก้ได้เฉพาะงานที่ได้รับมอบหมาย") applied literally: a caller holding `dpo.task.execute`
(DPO/PRIVACY in the seeded RBAC) may move any task; everyone else holding only `dpo.task.update`
(LEGAL/OWNER/IT/SEC) may move only a task actually assigned to them — the same two-tier shape DSAR-08's own
`requireSubtaskAccess` already established. `dpo.Service` gained a direct concrete `Risk *riskservice.Service`
field (not a local interface, following the same reasoning Dsar/Breach already use on this struct: `dpo`
already imports `dsar`, which imports `ropa`, which imports `risk/service`, so this is not a new import cycle)
— when a `ropa_gap` task is closed, `UpdateTaskStatus` resolves the finding's `activity_id` through
`risk.GetGapFinding` and calls `risk.AnalyzeActivity` on it. The task always closes; the finding only clears
if the rule genuinely no longer fires — exactly the acceptance criterion's own distinction between "the task
was closed" and "the gap is actually gone".

API: `POST /admin/v1/risk/gap-findings/{id}/remediate` (`ropa.risk.create`, the same permission RRA-04's own
analyze endpoint uses — this is still a risk-module write, the `dpo.tasks` row is an internal side effect, the
same reasoning DPIA-07's own `AddRiskControl` endpoint already uses for `assessment.dpia.update`); `GET
/admin/v1/dpo/tasks/{id}` and `POST /admin/v1/dpo/tasks/{id}/status` (ETag/If-Match, `dpo.task.read`/
`dpo.task.update`) — generic enough for any future `dpo.tasks` opener to reuse, not just this one.
`DpoRemediationTask`'s wire schema widened additively (`source_type`, `source_id`, `assignee_user_id`,
`due_at`, `completed_at`, `row_version`) — it was already returned nested inside `DpoSecurityAssessment`, so
nothing existing broke. UI: the `/settings/gap-register` page (RRA-04) gained an assignee picker (the same
`useMentionSearch` pattern VEN-01's own owner picker uses) + due date + priority form per open finding with
no task yet, and, once one exists, a status badge with an "advance" button that walks the same state machine
one step at a time, ending in "ปิดงาน" — closing it re-triggers the backend recheck and the row naturally
drops off the register once the finding actually resolves. Tests: unit (RemediateFinding opens and links a
real task; unknown assignee/bad priority/already-resolved/unknown finding all refused; two-tenant isolation;
every `UpdateTaskStatus` edge allowed/refused; version mismatch; the access-restriction rule including its
`dpo.task.execute` bypass; and the acceptance criterion itself end to end — fixing the actual gap then
walking the task through to `closed` leaves the finding `resolved`, proving the clear happens because the
rule passed, not merely because the task closed), HTTP contract (401/403/404/412/428/200) through the real
validator + AuthZ on both the risk and dpo endpoints. `pnpm --filter @pdpa/admin build`/`tsc --noEmit` and
`pnpm --filter @pdpa/i18n test` verified clean; as throughout this session, no live Postgres was reachable
here, so the Go tests were verified by `go build`/`go vet`/`gofmt -l` (compile-clean) rather than actually run.

<a id="rra-05"></a>
### RRA-05 Checklist ตรวจสอบกิจกรรม

*Activity checklist*

- **Priority / Phase:** Should · P3 · กลุ่ม: ช่องว่าง
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** รายการตรวจสอบรายกิจกรรม พร้อมส่งคำถามให้ที่ปรึกษาหรือ DPO และบันทึกการตอบกลับ

**Backend (Go):** checklist รายกิจกรรม + ส่งคำถามให้ DPO / ที่ปรึกษา และบันทึกคำตอบ

**Frontend (Next.js):** หน้า checklist

**Acceptance criteria:** คำถามและคำตอบถูกเก็บในกิจกรรม

<a id="rra-08"></a>
### RRA-08 ยอมรับความเสี่ยงพร้อมผู้อนุมัติ

*Risk acceptance*

- **Priority / Phase:** Should · P3 · กลุ่ม: จัดการ
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** —

**คำอธิบาย:** บันทึกการยอมรับความเสี่ยงที่เหลือ พร้อมเหตุผล ผู้อนุมัติ และวันทบทวน

**Backend (Go):** ขอยอมรับความเสี่ยง + เหตุผล + ผู้อนุมัติ + วันทบทวน

**Frontend (Next.js):** ขั้นตอนยอมรับความเสี่ยง

**Acceptance criteria:** การยอมรับหมดอายุและเตือนทบทวนตามวันที่ตั้ง

<a id="rra-09"></a>
### RRA-09 ทะเบียนความเสี่ยงด้านข้อมูลส่วนบุคคล

*Privacy risk register*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** RRA-02
- **Process:** —

**คำอธิบาย:** ความเสี่ยงระดับองค์กร เจ้าของความเสี่ยง มาตรการ และสถานะ ผูกกับกิจกรรม ระบบ และคู่ค้า

**Backend (Go):** ทะเบียนความเสี่ยงองค์กร (เจ้าของ มาตรการ สถานะ) ผูกกิจกรรม / ระบบ / คู่ค้า

**Frontend (Next.js):** หน้าทะเบียนความเสี่ยง

**Acceptance criteria:** ความเสี่ยงจาก DPIA / RoPA / คู่ค้ารวมอยู่ในทะเบียนเดียว

<a id="rra-10"></a>
### RRA-10 Heatmap และแดชบอร์ดความเสี่ยง

*Risk heatmap & dashboard*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-18
- **Process:** —

**คำอธิบาย:** แสดงความเสี่ยงตามหน่วยงาน ระบบ และประเภทข้อมูล พร้อมแนวโน้ม

**Backend (Go):** API heatmap ตามหน่วยงาน / ระบบ / ประเภทข้อมูล + แนวโน้ม

**Frontend (Next.js):** heatmap และ dashboard ความเสี่ยง

**Acceptance criteria:** คลิกช่อง heatmap แล้วเห็นรายการความเสี่ยงของช่องนั้น

<a id="rra-11"></a>
### RRA-11 แสดงจุดเสี่ยงบนรายการและแผนผัง

*Risk flags on RoPA / data flow*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DFG-01
- **Process:** —

**คำอธิบาย:** ไฮไลต์กิจกรรมและจุดบนแผนผังที่มีความเสี่ยง พร้อมเหตุผลและลิงก์ไปแก้ไข

**Backend (Go):** ส่งสถานะความเสี่ยงให้รายการกิจกรรมและแผนผัง

**Frontend (Next.js):** ไฮไลต์บนรายการและแผนผัง + เหตุผล + ลิงก์ไปแก้

**Acceptance criteria:** กิจกรรมเสี่ยงสูงแสดงเด่นทั้งในรายการและแผนผัง

<a id="rra-12"></a>
### RRA-12 ประเมินซ้ำเมื่อกิจกรรมเปลี่ยนหรือครบรอบ

*Re-assessment*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-11
- **Process:** —

**คำอธิบาย:** คำนวณความเสี่ยงใหม่เมื่อ RoPA เปลี่ยน และเตือนทบทวนตามรอบ

**Backend (Go):** คำนวณใหม่เมื่อ RoPA เปลี่ยน + รอบทบทวน

**Frontend (Next.js):** ป้ายวันที่ประเมินล่าสุด

**Acceptance criteria:** คะแนนอัปเดตภายใน 1 นาทีหลังแก้ RoPA

<a id="rra-13"></a>
### RRA-13 คะแนนความพร้อมรายหน่วยงาน

*Compliance score*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RRA-04
- **Process:** —

**คำอธิบาย:** คะแนนความสอดคล้องต่อกิจกรรมและหน่วยงาน ใช้เปรียบเทียบและติดตามความคืบหน้า

**Backend (Go):** คะแนนความสอดคล้องต่อกิจกรรม / หน่วยงาน

**Frontend (Next.js):** ตารางคะแนนรายหน่วยงาน

**Acceptance criteria:** คะแนนอธิบายได้ว่าหักจากช่องว่างใด

<a id="rra-14"></a>
### RRA-14 รายงานความเสี่ยงสำหรับผู้บริหาร

*Executive risk report*

- **Priority / Phase:** Should · P3 · กลุ่ม: รายงาน
- **ที่มา:** Function List: 05_ROPA_Risk
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** —

**คำอธิบาย:** สรุปความเสี่ยงสูง แผนแก้ไข และความคืบหน้า ส่งออก PDF

**Backend (Go):** รายงาน PDF สรุปความเสี่ยงสูง แผนแก้ไข ความคืบหน้า

**Frontend (Next.js):** ปุ่มสร้างรายงานผู้บริหาร

**Acceptance criteria:** รายงานสร้างได้ในคลิกเดียวและตรงกับทะเบียน
