# DPIA — แบบประเมินผลกระทบ (DPIA)

> ระบบจัดการแบบประเมินผลกระทบด้านการคุ้มครองข้อมูลส่วนบุคคล (DPIA Management Module) · ขอบเขต: คัดกรอง ประเมิน DPIA / LIA / AI เต็มรูปแบบ อนุมัติ และทบทวน  
> 18 features · Must 12 / Should 4 / Nice 2 · phase: P2 (12), P3 (4), P4 (2)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/assess` |
| PostgreSQL schema | [`assess`](../data/assess.md) (8 ตาราง) |
| Admin API prefix | `/admin/v1/assessments` |
| Endpoint ที่ SA กำหนดแล้ว | `GET /public/v1/guest/{token}/assessment` — แบบประเมินผ่าน guest link (BP-08 / BP-09)<br>`POST /admin/v1/assessments` — สร้างแบบประเมิน (BP-08)<br>`POST /admin/v1/assessments/{id}/submit` — ส่งตรวจ (BP-08) |
| หน้าจอ (Next.js) | admin: /assessments/*; portal: /guest/assessment/[token] |
| พึ่งพาบริการ | forms, risk, workflow, docs |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-12 DPIA, BP-08, DFD-1, ERD-10, ST-05 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| GUEST | ผู้ใช้ภายนอก (guest link) | External Guest | Guest link (token หมดอายุ) + OTP |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| AUDIT | ผู้ตรวจสอบ | Auditor | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DPIA-01](#dpia-01) | แบบคัดกรองความจำเป็นในการทำ DPIA | Must | P2 | OWNER DPO | S | S | N | BP-08 |
| [DPIA-02](#dpia-02) | เกณฑ์คะแนนและเงื่อนไขบังคับทำ DPIA | Must | P2 | DPO | S | S | N | BP-08 |
| [DPIA-03](#dpia-03) | คลัง template แบบประเมิน | Must | P2 | DPO | S | S | N | BP-08 |
| [DPIA-04](#dpia-04) | อธิบายกิจกรรมโดยดึงข้อมูลจาก RoPA | Must | P2 | OWNER | M | S | N | BP-08 |
| [DPIA-05](#dpia-05) | ประเมินความจำเป็นและความได้สัดส่วน | Must | P2 | OWNER DPO | S | S | N | BP-08 |
| [DPIA-06](#dpia-06) | ระบุและให้คะแนนความเสี่ยง | Must | P2 | DPO OWNER | M | M | Y | BP-08 |
| [DPIA-07](#dpia-07) | มาตรการลดความเสี่ยงและความเสี่ยงคงเหลือ | Must | P2 | DPO IT | M | M | N | BP-08 |
| [DPIA-09](#dpia-09) | เชิญผู้ร่วมประเมิน | Must | P2 | DPO IT GUEST | M | M | Y | BP-08 |
| [DPIA-10](#dpia-10) | ความเห็น DPO และการอนุมัติ | Must | P2 | DPO EXEC | S | S | N | BP-08 |
| [DPIA-12](#dpia-12) | ทะเบียนและสถานะ DPIA | Must | P2 | DPO EXEC | S | M | Y | BP-08 |
| [DPIA-14](#dpia-14) | ประวัติเวอร์ชันและ audit trail | Must | P2 | DPO AUDIT | XS | S | N | BP-08 |
| [DPIA-15](#dpia-15) | ออกรายงาน DPIA | Must | P2 | DPO AUDIT | S | XS | N | BP-08 |
| [DPIA-08](#dpia-08) | เชื่อมทะเบียนความเสี่ยงและงานแก้ไข | Should | P3 | DPO | S | XS | N | BP-08 |
| [DPIA-11](#dpia-11) | บันทึกการปรึกษาผู้มีส่วนได้เสีย | Should | P3 | DPO | S | S | N | BP-08 |
| [DPIA-13](#dpia-13) | ทบทวนเมื่อกิจกรรมเปลี่ยนหรือครบรอบ | Should | P3 | SCHED DPO | S | XS | N | BP-08 |
| [DPIA-16](#dpia-16) | ประเมินฐานประโยชน์โดยชอบด้วยกฎหมาย (LIA) | Should | P3 | OWNER DPO | S | S | N | BP-08 |
| [DPIA-17](#dpia-17) | ประเมินผลกระทบของระบบ AI | Nice | P4 | DPO IT | S | S | N | BP-08 |
| [DPIA-18](#dpia-18) | AI ช่วยกรอกแบบประเมินจากเอกสาร | Nice | P4 | OWNER LLM | M | S | N | BP-08 |

### ความสัมพันธ์ระหว่าง use case

- DPIA-01 «include» DPIA-02 (ทุกครั้งที่ทำ DPIA-01 ต้องทำ DPIA-02)
- DPIA-13 «include» DPIA-01 (ทุกครั้งที่ทำ DPIA-13 ต้องทำ DPIA-01)
- DPIA-08 «extend» DPIA-07 (DPIA-08 เป็นทางเลือก/ส่วนขยายของ DPIA-07)
- DPIA-18 «extend» DPIA-04 (DPIA-18 เป็นทางเลือก/ส่วนขยายของ DPIA-04)

## กระบวนการ / sequence / state machine

- [BP-08 ประเมินผลกระทบด้านการคุ้มครองข้อมูล (DPIA)](../processes/BP-08.md)
- [ST-05 กิจกรรม RoPA และแบบประเมิน (DPIA / LIA / TIA)](../states/ST-05.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [assess.templates](../data/assess.md#assess-templates) | template แบบประเมิน (DPIA / LIA / TIA / AI / security / maturity ฯลฯ) |
| [assess.screening_rules](../data/assess.md#assess-screening-rules) | เกณฑ์คัดกรอง / บังคับทำ DPIA |
| [assess.assessments](../data/assess.md#assess-assessments) | แบบประเมินแต่ละครั้ง |
| [assess.sections](../data/assess.md#assess-sections) | ส่วนของแบบประเมินที่มอบหมายผู้ตอบ |
| [assess.answers](../data/assess.md#assess-answers) | คำตอบรายข้อ + หลักฐาน |
| [assess.assessment_risks](../data/assess.md#assess-assessment-risks) | ความเสี่ยงที่ระบุในแบบประเมิน |
| [assess.dpo_opinions](../data/assess.md#assess-dpo-opinions) | ความเห็นของ DPO |
| [assess.consultations](../data/assess.md#assess-consultations) | บันทึกการปรึกษาผู้มีส่วนได้เสีย |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `assessment.dpia.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `assessment.dpia` | DPIA / LIA | DPO `CRUDA` · PRIVACY `CRU` · LEGAL `RU` · OWNER `RU` · IT `RU` · SEC `RU` · AUDIT `R` · EXEC `RA` · GUEST `RU` | ผู้ร่วมประเมินแก้ได้เฉพาะส่วนที่ได้รับเชิญ; ผู้บริหารอนุมัติ/ยอมรับความเสี่ยง |
| `assessment.security` | ประเมินมาตรการความปลอดภัย | DPO `RA` · PRIVACY `R` · IT `RU` · SEC `CRUDX` · AUDIT `R` |  |
| `assessment.transfer` | ประเมินการโอนต่างประเทศ (TIA) | DPO `CRUDA` · PRIVACY `CRU` · LEGAL `RU` · AUDIT `R` |  |
| `assessment.template` | Template แบบประเมิน | DPO `CRUDP` · PRIVACY `CRU` · LEGAL `CRU` · SEC `CRU` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `dpia.submitted` | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `dpia.approved` | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DPIA-01, DPIA-02, DPIA-03, DPIA-04, DPIA-05, DPIA-06, DPIA-07, DPIA-09, DPIA-10, DPIA-12, DPIA-14, DPIA-15
- **P3:** DPIA-08, DPIA-11, DPIA-13, DPIA-16
- **P4:** DPIA-17, DPIA-18

## รายละเอียด feature

<a id="dpia-01"></a>
### DPIA-01 แบบคัดกรองความจำเป็นในการทำ DPIA

*DPIA screening*

- **Priority / Phase:** Must · P2 · กลุ่ม: คัดกรอง
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1); TDPG 4.0-P (จุฬาฯ)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** คำถามคัดกรองตามปัจจัยเสี่ยงสูง เช่น ข้อมูลอ่อนไหว (ม.26) ปริมาณมาก การติดตามพฤติกรรม การตัดสินใจอัตโนมัติ เทคโนโลยีใหม่/AI และกลุ่มเปราะบาง (เด็ก ลูกจ้าง) แล้วสรุป ต้องทำ / ควรทำ / ไม่ต้องทำ

**Backend (Go):** แบบคัดกรองตามปัจจัยเสี่ยงสูง (TDPG) บน PLT-06 → สรุป ต้องทำ / ควรทำ / ไม่ต้องทำ

**Frontend (Next.js):** หน้าคัดกรองจากกิจกรรม RoPA

**Acceptance criteria:** ผลคัดกรองตรงตามเกณฑ์ที่ตั้งทุกกรณีทดสอบ

<a id="dpia-02"></a>
### DPIA-02 เกณฑ์คะแนนและเงื่อนไขบังคับทำ DPIA

*Configurable threshold rules*

- **Priority / Phase:** Must · P2 · กลุ่ม: คัดกรอง
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** TDPG 4.0-P
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPIA-01
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** ตั้งเกณฑ์คะแนนหรือจำนวนปัจจัยเสี่ยงที่บังคับให้ทำ DPIA (เช่น คะแนนตั้งแต่ 2 หรือเข้าเกณฑ์ตั้งแต่ 2 ปัจจัย) และบันทึกเหตุผลกรณีตัดสินใจไม่ทำ

**Backend (Go):** ตั้งเกณฑ์คะแนน / จำนวนปัจจัยที่บังคับทำ DPIA, บันทึกเหตุผลกรณีตัดสินใจไม่ทำ

**Frontend (Next.js):** หน้าตั้งค่าเกณฑ์

**Acceptance criteria:** เปลี่ยนเกณฑ์แล้วผลคัดกรองรอบใหม่ใช้เกณฑ์ใหม่

<a id="dpia-03"></a>
### DPIA-03 คลัง template แบบประเมิน

*Assessment template library*

- **Priority / Phase:** Must · P2 · กลุ่ม: แบบประเมิน
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06, T34
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** template DPIA/PIA มาตรฐาน คัดลอกและปรับคำถาม ตัวเลือก และคะแนนได้ มีเวอร์ชัน รองรับ TH/EN

**Backend (Go):** template DPIA / PIA มาตรฐาน TH/EN (เนื้อหา T34), clone / แก้ / เวอร์ชัน

**Frontend (Next.js):** หน้าคลัง template

**Acceptance criteria:** clone template แล้วแก้ได้โดยไม่กระทบต้นฉบับ

**สถานะ implementation:** done — see `CLAUDE.md`'s DPIA-03 section for the full implementation note.
Built on the already-existing `assess.templates` table and the generic PLT-06 form engine (no new
migration): a catalog/list of templates by `assessment_type`, `Clone` (copies a source's current form
content into a brand-new, independent `platform.form_definitions` row — editing the clone afterward
through PLT-06's own form builder never touches the source), and a `Publish`/`Retire` lifecycle wrapping
the underlying form's own draft/publish state. API `/admin/v1/dpia/templates` (list, create), `/{id}`
(get), `/{id}/clone`, `/{id}/publish`, `/{id}/retire` — all on the already-seeded `assessment.template.*`
permissions. UI `/settings/dpia-templates` (list + filter by type, clone dialog, create form, publish/
retire actions, links to PLT-06's own `/forms/{id}` builder for editing content), linked from
`/settings/dpia`.

<a id="dpia-04"></a>
### DPIA-04 อธิบายกิจกรรมโดยดึงข้อมูลจาก RoPA

*Processing description from RoPA*

- **Priority / Phase:** Must · P2 · กลุ่ม: แบบประเมิน
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** ดึงวัตถุประสงค์ ประเภทข้อมูล กลุ่มเจ้าของข้อมูล ผู้รับ การโอนต่างประเทศ ระยะเวลาเก็บ และระบบที่ใช้ จาก RoPA มาเป็นส่วนอธิบายของ DPIA

**Backend (Go):** เลือกกิจกรรม RoPA → pre-fill ส่วนอธิบายการประมวลผล, sync เมื่อ RoPA เปลี่ยน

**Frontend (Next.js):** ส่วนอธิบายกิจกรรมที่เติมอัตโนมัติ

**Acceptance criteria:** ข้อมูลที่ดึงจาก RoPA ตรงกับกิจกรรมต้นทาง

**สถานะ implementation:** done — see `CLAUDE.md`'s DPIA-04 section for the full implementation note.
Composed live from the assessment's linked RoPA activity on every read (purposes, data categories, data
subject groups, recipients, cross-border transfers, retention) — never persisted, so "sync เมื่อ RoPA เปลี่ยน"
needs no separate sync step. New `GET /admin/v1/dpia/assessments/{id}/description`
(`assessment.dpia.read`). UI: an auto-filled description panel on `/ropa/activities/{id}`'s own DPIA
screening section, shown once a round is `in_progress`. Not done: `ropa.activity_systems` ("ระบบที่ใช้") —
that link has no CRUD anywhere yet in ROPA-02/03, so there is nothing to surface; add it once a screen
writes to that table.

<a id="dpia-05"></a>
### DPIA-05 ประเมินความจำเป็นและความได้สัดส่วน

*Necessity & proportionality*

- **Priority / Phase:** Must · P2 · กลุ่ม: แบบประเมิน
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.22, ม.24, ม.26
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPIA-03
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** ตรวจฐานกฎหมาย (ม.24/26) การเก็บเท่าที่จำเป็น (ม.22) และทางเลือกที่กระทบสิทธิน้อยกว่า

**Backend (Go):** ส่วนประเมินตาม ม.22 / 24 / 26 (คำถาม + หลักฐาน)

**Frontend (Next.js):** section ความจำเป็นและความได้สัดส่วน

**Acceptance criteria:** ตอบครบแล้วสรุปผลความจำเป็นได้

**สถานะ implementation:** done — see `CLAUDE.md`'s DPIA-05 section for the full implementation note.
4 yes/no questions (ม.22/24/26, `docs/decisions.md` Q-27) on the same PLT-06/assess.templates apparatus
DPIA-01/02/03 already use (migration 00048) — answered via `POST /admin/v1/dpia/assessments/{id}/necessity`,
read via `GET` on the same path (`assessment.dpia.update`/`.read`). "Necessary" only once every question is
"yes"; any other answer is flagged in `missing` and the result reads "needs_review". Answers persist under
`assess.sections`/`assess.answers` with a `necessity` section code, kept separate from DPIA-01's own screening
answers so re-answering never affects `Assessment.Factors`. Re-answering replaces the prior submission (no
round numbering, unlike DPIA-01's own re-screening). UI: a checklist panel on `/ropa/activities/{id}`'s DPIA
section, shown alongside DPIA-04's description panel once a round is `in_progress`.

<a id="dpia-06"></a>
### DPIA-06 ระบุและให้คะแนนความเสี่ยง

*Risk identification & scoring*

- **Priority / Phase:** Must · P2 · กลุ่ม: ความเสี่ยง
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** RRA-02
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** risk matrix โอกาส × ผลกระทบต่อสิทธิเสรีภาพของเจ้าของข้อมูล พร้อมคลังความเสี่ยงสำเร็จรูป

**Backend (Go):** คลังความเสี่ยงสำเร็จรูป, matrix โอกาส × ผลกระทบ (ใช้ risk engine ร่วมกับ RRA)

**Frontend (Next.js):** หน้าระบุความเสี่ยง + matrix

**Acceptance criteria:** คะแนนความเสี่ยงคำนวณตาม matrix ของ tenant

**Implementation — done (likelihood/impact are DPO-entered; DPIA-07's own mitigation/residual columns
deliberately untouched).** The acceptance criterion's own risk matrix is RRA-02's `risk.risk_matrices` engine,
already built and shared verbatim — `Classify(matrix, likelihood, impact)` is called live, uncached, exactly
the way RRA-01's own activity score already does; nothing here duplicates that math. The real new piece is
`risk.risks` itself (already fully specified in the baseline migrations, no new migration): a tenant-wide risk
register row every future risk-scoring feature (vendor, breach, audit) will eventually also write, with
DPIA-06 as its first real writer. `internal/risk/service/risks.go`'s `IdentifyRisk`/`UpdateRisk`/`GetRisk`/
`ListRisksByIDs` own that table; `internal/dpia/service/risks.go`'s `IdentifyRisk`/`UpdateRisk`/
`ListAssessmentRisks`/`RemoveAssessmentRisk` own the link in `assess.assessment_risks` (the schema's own
intended join table for exactly this) — two separate module calls in one request transaction, each writing
only its own schema (rule 9), the same split ROPA-09's own `ropa.activity_controls` ↔ `risk.controls` link
already established. `dpia/service` already imports `risk/service` directly (for `ListControls`'s own
`riskservice.Control` reference, DPIA-15), so `Service.Risk *riskservice.Service` is a plain concrete field,
not a rule-9 interface — there is no cycle to route around, unlike RRA-01/RRA-03's own local-interface dance.

Risks are identified/edited only while the round is `in_progress` or `in_review` — the exact same editable
window DPIA-10's own `RecordOpinion` already uses, so a decided or closed round can't quietly gain a new risk
after the fact. `UpdateRisk`/`RemoveAssessmentRisk` both check the risk id is actually linked to *this*
assessment before touching it (`requireLinkedRisk`), so one round can never edit another's risk by guessing
its id even within the same tenant. Removing a risk only drops the link row — `risk.risks` itself is left
alone, since a later round (or, eventually, another feature) may still reference it. `OwnerUserID`/
`ActivityID` are checked visible under the caller's own RLS before anything is written (rule 1): the activity
id is never taken from the request body at all — it's copied straight from the assessment's own
`activity_id`, so a risk can never be identified against a different activity than the round it belongs to.

"คลังความเสี่ยงสำเร็จรูป" (the ready-made risk catalog) is deliberately a plain Go constant
(`dpiaservice.RiskCatalog()`, 12 common PDPA risk scenarios — re-identification, excessive retention,
unauthorized access, third-party leakage, profiling bias, missing lawful basis, cross-border transfer without
a safeguard, unfulfilled data-subject rights, insecure storage, vendor breach, unreviewed automated decisions,
vulnerable subjects), not a seeded table: picking an entry only prefills the "add risk" form's title/
description, which stays editable, and nothing else references it by id — so there's no tenant-override or
RLS story a table would need. Still flagged draft pending legal review, the same `docs/decisions.md` pattern
ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01 already used for seeded domain content (Q-33). `risk.risks` columns this
feature doesn't touch — `residual_likelihood/impact/score`, `asset_id`, `vendor_id`, and the acceptances table
— are left alone for DPIA-07 ("มาตรการลดความเสี่ยงและความเสี่ยงคงเหลือ") to build on, the same "leave the
column/FK for the sibling feature that actually needs it" deferral this codebase uses throughout.

API: `GET /admin/v1/dpia/risk-catalog` (`assessment.dpia.read`), `GET`/`POST /admin/v1/dpia/assessments/{id}/risks`,
`PUT`/`DELETE /admin/v1/dpia/assessments/{id}/risks/{riskId}` (ETag/If-Match on update, `assessment.dpia.update`
to write) — same shape as DPIA-10's own opinions sub-resource. `DpiaAssessment`'s own wire schema gained
`risk_level`/`owner_user_id` (both previously unused columns, now surfaced — also doubles as RRA-03's own
"which rounds were auto-triggered" signal). UI: a risk panel on `/ropa/activities/{id}`'s DPIA section, shown
for the same in-progress round as DPIA-04/05's own panels — a catalog dropdown, title/description/likelihood/
impact inputs, a level-badged list, and remove. Tests: unit (the acceptance criterion directly — a risk's
score/level come from `Classify` against the tenant's current matrix; changing the matrix changes the next
identified risk's score immediately; refused outside the editable window; a risk not linked to this
assessment is refused with `ErrNotFound` even from a caller who holds the right permission; recomputes the
score on update; remove only unlinks, the `risk.risks` row itself survives; the risk catalog has unique,
complete entries; two-tenant isolation of both the link and the underlying `risk.risks` row), HTTP contract
(401/403/404/412/428/201/200/204) through the real validator + AuthZ. `pnpm --filter @pdpa/admin build` and
the `@pdpa/i18n` ICU message tests both verified clean.

<a id="dpia-07"></a>
### DPIA-07 มาตรการลดความเสี่ยงและความเสี่ยงคงเหลือ

*Mitigation & residual risk*

- **Priority / Phase:** Must · P2 · กลุ่ม: ความเสี่ยง
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1); ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** DPO (DPO / Privacy Team), IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DPIA-06
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** เลือกมาตรการจากคลัง control (เข้ารหัส แฝงข้อมูล จำกัดสิทธิ์ ลดระยะเวลาเก็บ) คำนวณความเสี่ยงคงเหลือ กำหนดผู้รับผิดชอบและวันเสร็จ

**Backend (Go):** คลัง control, ผูกมาตรการ, คำนวณความเสี่ยงคงเหลือ, ผู้รับผิดชอบ / วันเสร็จ → task

**Frontend (Next.js):** หน้ามาตรการและความเสี่ยงคงเหลือ

**Acceptance criteria:** ความเสี่ยงคงเหลือคำนวณใหม่เมื่อเพิ่มมาตรการ

**Implementation — done.** `risk.risk_controls` (risk_id + control_id, already fully specified in the
baseline migrations — owner_user_id, due_at, task_id, status all already there) links a `risk.risks` row
(DPIA-06) to one of ROPA-09's own `risk.controls` catalog entries — the "คลัง control" the module doc names
is that same catalog, not a new one; DPIA-07 reuses it rather than building a second. Migration 00056 adds
only `risk.risks.residual_level` (text, same CHECK as the existing inherent `level` column) for symmetry —
`residual_likelihood`/`residual_impact`/`residual_score` were already on the table from DPIA-06's own
migration, unused until now.

The residual-risk formula itself is not specified anywhere in the module doc or `docs/legal/pdpa-rules.md` —
treated as a configurable business default, not a legal rule needing a `docs/decisions.md` entry (CLAUDE.md's
own distinction: a tunable value gets a documented default, not a blocking question): each **implemented**
control (status = `implemented`; `existing`/`planned`/`not_effective` don't count) reduces the residual
likelihood by one level, floored at 1 — impact is left at the inherent value unchanged, since a control from
this catalog (encryption, pseudonymization, access restriction, retention reduction) makes harm less likely,
not less severe if it still happens. `recomputeResidual` (in `internal/risk/service/risk_controls.go`) is
called after every `AddRiskControl`/`UpdateRiskControlStatus`/`RemoveRiskControl` and reclassifies against the
tenant's current RRA-02 matrix via the same `Classify` function DPIA-06's own `IdentifyRisk`/`UpdateRisk`
already use — never cached, so a later matrix change or control-status edit is always reflected on the next
read, exactly like the acceptance criterion requires.

"ผู้รับผิดชอบและวันเสร็จ → task": giving a control link both an owner and a due date opens one `dpo.tasks`
row (`source_type = "risk"`, numbered `SEC-<year>-NNNN` in the same bucket DPO-09's own remediation tasks
use) via a new `OpenRiskControlTask` on `dpo/service`. This is the third import-cycle-avoidance case in this
codebase's risk/dpo/dsar/ropa neighborhood: `risk/service` cannot import `dpo/service` directly (`dpo` already
imports `dsar`, which imports `ropa`, which imports `risk/service` for ROPA-09's `Control` type — a direct
reverse import would cycle), so `risk/service` declares its own local `DpoTasks` interface (rule 9), satisfied
structurally by `dposervice.Service.OpenRiskControlTask` with a direct field assignment
(`riskSvc.Dpo = dpoSvc` in `cmd/api/main.go`) — no adapter type needed, the same shape RRA-03's own
`DpiaTrigger` already established in the opposite direction. The shared `dpo.tasks` `InsertTask` query gained
optional `assignee_user_id`/`due_at` params (both existing call sites — DPO-09's own remediation task, PNG-07's
consent task — pass neither, so their behaviour is unchanged).

`AddRiskControl` checks the control id is a real catalog row and, when given, that the owner is a real active
user of the tenant (rule 1's FK-visibility pattern, via the already-exported `iamservice.Names`) before
writing; the risk_id+control_id unique constraint is wrapped in `pdb.Savepoint` (ROPA-03/VEN-01's own
established pattern) so re-linking an already-linked control doesn't abort the request transaction.
`dpia.Service` wraps the whole thing (`AddRiskControl`/`ListRiskControls`/`UpdateRiskControlStatus`/
`RemoveRiskControl` in `internal/dpia/service/risks.go`) with the same `canEditRisks`/`requireLinkedRisk`
guards DPIA-06 already built, so a control can only be linked to a risk of an `in_progress`/`in_review`
assessment, and only to a risk actually linked to *this* assessment — dpia never writes `risk.risk_controls`
directly (rule 9).

API: `GET`/`POST /admin/v1/dpia/assessments/{id}/risks/{riskId}/controls`,
`PUT`/`DELETE .../controls/{controlId}` (ETag/If-Match on the status update) — same shape as DPIA-06's own
risk sub-resource. `DpiaRisk`'s wire schema gained `residual_likelihood`/`residual_impact`/`residual_score`/
`residual_level` (all nil until at least one control is linked). UI: each risk row on `/ropa/activities/{id}`'s
DPIA risk panel shows its residual level once computed and expands into a controls panel — catalog dropdown
(reusing the same `useSecurityControls` hook ROPA-09's own picker uses), owner/due-date inputs, a per-control
status `<select>`, and remove. Tests: unit (the acceptance criterion directly — residual recomputes on add,
floors at 1 with multiple implemented controls, recomputes on remove and on status change; unknown control/
owner refused; duplicate link refused without poisoning the transaction; owner+due-date opens a real
`dpo.tasks` row; two-tenant isolation of the link and the underlying risk), integration through
`dpia.Service`'s own wrapper (editable-status gate, cross-assessment risk rejection), HTTP contract
(401/403/201/200/204/428/200) through the real validator. Full backend `go test -count=1 -p 1 ./...` exits 0
(no regressions anywhere in the branch); `pnpm --filter @pdpa/admin build` and the `@pdpa/i18n` ICU message
tests both verified clean. Not done: DPIA-08 (the sibling "extend" feature named in the module's own relation
list) — recommending which controls to add, not just linking a chosen one — is a separate Should feature, not
part of this acceptance criterion.

<a id="dpia-09"></a>
### DPIA-09 เชิญผู้ร่วมประเมิน

*Collaboration & invitations*

- **Priority / Phase:** Must · P2 · กลุ่ม: Workflow
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), IT (เจ้าของระบบ / IT), GUEST (ผู้ใช้ภายนอก (guest link))
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-04, PLT-07
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** เชิญเจ้าของกระบวนการ IT และทีม Security ตอบเฉพาะส่วน แสดงความเห็น และแนบหลักฐาน

**Backend (Go):** มอบหมายรายส่วน, เชิญผู้ใช้ภายใน / ภายนอก (guest), comment + แนบหลักฐาน

**Frontend (Next.js):** หน้าเชิญผู้ร่วมประเมิน + มุมมองของผู้ร่วมประเมิน

**Acceptance criteria:** ผู้ร่วมประเมินแก้ได้เฉพาะส่วนที่ได้รับมอบหมาย

<a id="dpia-10"></a>
### DPIA-10 ความเห็น DPO และการอนุมัติ

*DPO opinion & sign-off*

- **Priority / Phase:** Must · P2 · กลุ่ม: Workflow
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42; TDPG 4.0-P
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** DPO ให้ความเห็น ผู้บริหารอนุมัติหรือยอมรับความเสี่ยง พร้อมบันทึกเหตุผลและวันที่

**Backend (Go):** ส่วนความเห็น DPO, อนุมัติ / ยอมรับความเสี่ยงโดยผู้บริหาร (หลายระดับ)

**Frontend (Next.js):** ขั้นตอนความเห็นและอนุมัติ

**Acceptance criteria:** DPIA ปิดได้เมื่อมีความเห็น DPO และการอนุมัติครบ

**Implementation (DPIA-10) — done:** `assess.dpo_opinions` and the rest of ST-05#2's review/approval states
(`in_review`/`approved`/`rejected`/`needs_review`/`closed`, beyond DPIA-01/02's own `screening`/`not_required`/
`in_progress`) were already fully specified in the baseline migrations and `docs/states/state-machines.yaml`
— no new migration. `internal/dpia/service/opinion.go`'s `Transition` is the whole state machine as one
allow-list (`assessTransitions`, ST-05#2's own edges beyond screening): submit for review, request more info,
decide (approved/rejected/needs_review) and close — the same `db/queries/dpia/opinion.sql`'s `SetAssessmentStatus`
query every edge shares. Deciding or closing needs `assessment.dpia.approve` beyond the endpoint's own
`assessment.dpia.update` permission (checked internally, `ErrForbidden` → 403) — the module doc's own EXEC
actor has no grant on `assessment.dpia` in the baseline RBAC seed, so in this tenant's RBAC only DPO can
decide (the same gap DPIA-02/DPIA-05's own actor lines already document). The acceptance criterion itself is
`Transition`'s own guard on entering `closed`: a round that was actually assessed (`approved`/`rejected`) needs
at least one `RecordOpinion` call already on record — `not_required` never had anything to opine on, so it
closes with none. `RecordOpinion` only accepts a real `recommendation` value (proceed / proceed_with_conditions
/ do_not_proceed / consult_pdpc, `assess.dpo_opinions`' own CHECK) and only while the round is `in_progress` or
`in_review`. `DpiaAssessment`'s wire schema gained `row_version` (an ETag was never needed before this
feature's `If-Match`-gated transition) and its `status` enum widened to the full ST-05#2 set — both additive,
no existing consumer broke.

API: `POST /admin/v1/dpia/assessments/{id}/transition` (ETag/If-Match, `{to, reason}` — `reason` required
entering `rejected`/`needs_review`), `GET`/`POST /admin/v1/dpia/assessments/{id}/opinions`. UI: a decision panel
on `/ropa/activities/{id}`'s existing DPIA section — submit-for-review/resume/close buttons, an opinion form +
list, and approve/needs-review/reject buttons gated on holding `.approve`. Tests: unit (every ST-05#2 edge
allowed/refused, the approve-only gate with a limited-permission caller, reason required entering
rejected/needs_review, close blocked without an opinion unless not_required, opinion validation + status gate,
two-tenant isolation), HTTP contract (401/403/404/409/412/422/428/200) through the real validator + AuthZ,
including a real `iam.users` row for the limited-permission test caller (`assess.dpo_opinions.dpo_user_id` is a
real FK to `iam.users`, not satisfiable by an arbitrary uuid). `pnpm --filter @pdpa/admin build`/`tsc` and the
`@pdpa/i18n` ICU message tests both verified clean.

<a id="dpia-12"></a>
### DPIA-12 ทะเบียนและสถานะ DPIA

*DPIA register & status*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DPIA-01
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** สถานะ คัดกรอง / กำลังประเมิน / รอตรวจ / อนุมัติ / ต้องทบทวน แยกตามกิจกรรมและหน่วยงาน

**Backend (Go):** ทะเบียน DPIA + สถานะ (คัดกรอง / กำลังประเมิน / รอตรวจ / อนุมัติ / ต้องทบทวน) ตามกิจกรรม / หน่วยงาน

**Frontend (Next.js):** หน้าทะเบียน + ตัวกรอง

**Acceptance criteria:** สถานะในทะเบียนตรงกับขั้นตอนจริงของแต่ละ DPIA

**Implementation — done.** `internal/dpia/service/registry.go` (`assessment.dpia.read`, no new migration or
permission): `ListAllAssessments` (new sqlc query, `backend/db/queries/dpia/dpia.sql`) selects `DISTINCT ON
(activity_id) ... ORDER BY activity_id, round_no DESC` over `assess.assessments` where `assessment_type =
'dpia'` — exactly one row per activity, always its *latest* round, computed live on every call (no persisted
copy to drift). `Service.Registry` enriches each round with its RoPA activity (`Ropa.GetActivity`, rule 9)
and that activity's department/legal-entity names (`Org.GetOrgUnit`/`Org.GetLegalEntity`, both already
exported for other modules' own FK-visibility checks — only the `Org` interface here grew wider, no new org
code), then filters by `legal_entity_id`/`org_unit_id`/`status` in Go rather than in SQL: the result set is a
tenant's own activity count, the same "no pagination, report meant to be viewed as one list" precedent
ROPA-04's own processor-RoPA export already set, not a paginated query like DPIA-01/02's own
`ListAssessments`. No pagination, sorted newest-first. `GET /admin/v1/dpia/registry` — also fixed a real gap
while wiring it: `Makefile`'s `gen` target had no `oapi-codegen` line for `internal/dpia/http` at all (the
same class of bug DSAR-03's module note already found for `internal/dsar/http`), so `dpia.gen.go` was never
regenerated by `make gen` — added the missing line. UI: a new page, `/dpia-register` (the module doc's own
UX note: "มีหน้าจอใหม่") — legal-entity-then-department picker (the same two-step pattern
`/settings/organization`/`/ropa/activities` use) + a status filter, a table linking each row to its
activity's own DPIA section on `/ropa/activities/{id}`; linked from that same DPIA screening section's header.
Tests: unit (`TestRegistry_ReflectsLiveStatusAcrossActivitiesAndDepartments` — the acceptance criterion
directly: two activities in two departments under one legal entity screen to different results/statuses, the
registry lists both with the right department/entity names, every filter narrows correctly, and re-screening
an activity to a new round makes the registry's one row for it show that new round's live status, not the
first round's stale one; two-tenant isolation), HTTP contract (401/200 + all three filters) through the real
validator + AuthZ. Verified against a real Postgres + Redis, not just compiled: both the unit tests and the
HTTP contract test ran and passed live.

<a id="dpia-14"></a>
### DPIA-14 ประวัติเวอร์ชันและ audit trail

*Version history & audit trail*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team), AUDIT (ผู้ตรวจสอบ)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08, PLT-12
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** เก็บทุกเวอร์ชันของแบบประเมิน คำตอบ ผู้แก้ไข และวันที่

**Backend (Go):** ใช้เวอร์ชันและ audit กลาง

**Frontend (Next.js):** หน้าเปรียบเทียบคำตอบระหว่างรอบ

**Acceptance criteria:** เห็นความต่างของคำตอบระหว่างรอบได้

**สถานะ implementation:** done — see `CLAUDE.md`'s DPIA-14 section.

<a id="dpia-15"></a>
### DPIA-15 ออกรายงาน DPIA

*DPIA report export*

- **Priority / Phase:** Must · P2 · กลุ่ม: รายงาน
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), AUDIT (ผู้ตรวจสอบ)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** รายงาน PDF/Word ภาษาไทยและอังกฤษ พร้อมผลคะแนน มาตรการ และผู้อนุมัติ

**Backend (Go):** รายงาน DPIA PDF / Word TH/EN (document composer)

**Frontend (Next.js):** ปุ่มส่งออกรายงาน

**Acceptance criteria:** รายงานแสดงคะแนน มาตรการ และผู้อนุมัติครบ

**Implementation — done.** Not built on PLT-16's document composer (no `docs.Service` registration, no new
document type, no draft/approval cycle) — this report is a one-shot, always-live export, not a negotiated
legal document: `internal/platform/docs/render`'s own standalone `Input`/`HTML`/`DOCX`/`PDFRenderer` pieces
(already built for PLT-16, no database access of their own) are reused directly instead, the same "no stored
document, nothing to go stale" discipline DPIA-04/12 already established for this module. `dpiaservice.Report`
(`internal/dpia/service/report.go`) composes a ProseMirror `render.Input` live from three already-built
pieces: the assessment's own score/result/round (DPIA-01/02), its RoPA activity's linked ม.37(1) security
measures — DPIA-15's own "มาตรการ", read via `Ropa.ListActivityControls`/`Ropa.ListControls` (both already
exported by `ropaservice` for ROPA-09's own picker — a thin pass-through to `risk`, so this stays rule-9-clean
without dpia importing `risk` directly) — and every DPO opinion recorded on the round (DPIA-10's
`ListOpinions`), with each opinion's DPO resolved to a display name via `iamservice.AllNames` (a free
function, not a wrapped interface, the same direct-import pattern ROPA-02/DPO-01 already use for it). No
merge fields or clauses (this is an internal operational report, not legal wording — rule 8 doesn't apply),
and never a DRAFT banner. `GET /admin/v1/dpia/assessments/{id}/report?language=th|en&format=pdf|docx`
(`assessment.dpia.read`, same binary/`Content-Disposition` response shape as PLT-16's own
`/admin/v1/platform/documents/{id}/export`, including the same 503 `*.no_renderer` mapping when no
`GOTENBERG_URL`/`CHROMIUM_PATH` is configured for `format=pdf`) — the handler duplicates PLT-16's small
`typedWriter`/`contentType` helpers locally since they're unexported in `platform/docs/http`. `wiring`:
`dpiaservice.Service` gained a `PDF render.PDFRenderer` field, set from `render.FromEnv()` in `cmd/api`
alongside `docsSvc`'s own (`cmd/worker` never renders a DPIA report, so it's left nil there). UI: each
screening round on `/ropa/activities/{id}`'s DPIA section gained four download links (PDF/Word × TH/EN) next
to its existing history toggle. Tests: unit (the acceptance criterion directly — a round with a linked
security measure and a recorded DPO opinion renders both by name/text in the report, in both languages; an
activity with neither still renders with the "none yet" placeholders rather than failing; two-tenant
isolation), HTTP contract (401/200/404, and a real `.docx` download verified as a real OOXML zip
(`bytes.HasPrefix(raw, []byte("PK"))`) with the right `Content-Type`/`Content-Disposition`) — all run against
a real Postgres + Redis, not just compiled.

<a id="dpia-08"></a>
### DPIA-08 เชื่อมทะเบียนความเสี่ยงและงานแก้ไข

*Link to risk register & tasks*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความเสี่ยง
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** RRA-09
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** ส่งความเสี่ยงที่ยังเหลือเข้าทะเบียนความเสี่ยง และสร้างงานแก้ไขอัตโนมัติ

**Backend (Go):** ส่งความเสี่ยงคงเหลือเข้าทะเบียนความเสี่ยง + สร้างงานแก้ไข

**Frontend (Next.js):** ปุ่มส่งเข้าทะเบียน

**Acceptance criteria:** ความเสี่ยงคงเหลือปรากฏในทะเบียนพร้อมลิงก์กลับ

<a id="dpia-11"></a>
### DPIA-11 บันทึกการปรึกษาผู้มีส่วนได้เสีย

*Stakeholder consultation log*

- **Priority / Phase:** Should · P3 · กลุ่ม: Workflow
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** TDPG 4.0-P
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPIA-03
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** บันทึกการปรึกษาเจ้าของข้อมูล ผู้ประมวลผล หรือผู้เชี่ยวชาญ และข้อโต้แย้งพร้อมคำชี้แจง

**Backend (Go):** ส่วนบันทึกการปรึกษา (ผู้ถูกปรึกษา วันที่ ประเด็น คำชี้แจง)

**Frontend (Next.js):** section การปรึกษาผู้มีส่วนได้เสีย

**Acceptance criteria:** รายงาน DPIA แสดงบันทึกการปรึกษา

<a id="dpia-13"></a>
### DPIA-13 ทบทวนเมื่อกิจกรรมเปลี่ยนหรือครบรอบ

*Re-assessment triggers*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1); TDPG 4.0-P
- **Actor:** SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-11
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** แจ้งให้ทบทวนเมื่อ RoPA เปลี่ยน ครบรอบ หรือมีเหตุละเมิดที่เกี่ยวข้อง

**Backend (Go):** trigger เมื่อ RoPA เปลี่ยน / ครบรอบ / มีเหตุละเมิดที่เกี่ยวข้อง → เปิดรอบใหม่ (copy คำตอบเดิม)

**Frontend (Next.js):** ป้าย 'ต้องทบทวน' + ปุ่มเริ่มรอบใหม่

**Acceptance criteria:** เหตุละเมิดที่ผูกกิจกรรมทำให้ DPIA ที่เกี่ยวข้องถูกทำเครื่องหมายทบทวน

<a id="dpia-16"></a>
### DPIA-16 ประเมินฐานประโยชน์โดยชอบด้วยกฎหมาย (LIA)

*Legitimate interest assessment*

- **Priority / Phase:** Should · P3 · กลุ่ม: ประเภทอื่น
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.24(5)
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPIA-03
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** template LIA: วัตถุประสงค์ ความจำเป็น และการชั่งน้ำหนักกับสิทธิของเจ้าของข้อมูล ผูกกับกิจกรรมที่ใช้ฐาน ม.24(5)

**Backend (Go):** template LIA 3 ขั้น (วัตถุประสงค์ / ความจำเป็น / ชั่งน้ำหนัก) ผูกกิจกรรมที่ใช้ฐาน ม.24(5)

**Frontend (Next.js):** หน้า LIA

**Acceptance criteria:** กิจกรรมที่ใช้ฐาน ม.24(5) ต้องมี LIA ที่อนุมัติ

<a id="dpia-17"></a>
### DPIA-17 ประเมินผลกระทบของระบบ AI

*AI impact assessment*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ประเภทอื่น
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** DPO (DPO / Privacy Team), IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPX-12
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** แบบประเมินความเสี่ยงของระบบ AI ที่ใช้ข้อมูลส่วนบุคคล เช่น การตัดสินใจอัตโนมัติและความลำเอียง

**Backend (Go):** template ประเมินระบบ AI (การตัดสินใจอัตโนมัติ ความลำเอียง ความโปร่งใส) ผูกทะเบียน AI

**Frontend (Next.js):** หน้าประเมินระบบ AI

**Acceptance criteria:** ระบบ AI ในทะเบียนมีผลประเมินล่าสุด

<a id="dpia-18"></a>
### DPIA-18 AI ช่วยกรอกแบบประเมินจากเอกสาร

*AI-assisted drafting*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 01_DPIA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-22
- **Process:** [BP-08](../processes/BP-08.md)

**คำอธิบาย:** ดึงข้อมูลจากเอกสารโครงการหรือ RoPA มาเติมคำตอบให้ผู้ประเมินตรวจ

**Backend (Go):** AI ดึงข้อมูลจากเอกสาร / RoPA เติมคำตอบเป็นข้อเสนอ

**Frontend (Next.js):** ป้ายคำตอบที่ AI เสนอ + ยืนยันทีละข้อ

**Acceptance criteria:** คำตอบจาก AI ไม่ถูกบันทึกจนกว่าผู้ประเมินยืนยัน
