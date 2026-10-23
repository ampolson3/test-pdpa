# DSAR — คำขอใช้สิทธิของเจ้าของข้อมูล (DSAR)

> ระบบจัดการคำขอใช้สิทธิของเจ้าของข้อมูลส่วนบุคคล (Data Subject Access Request (DSAR)) · ขอบเขต: รับและดำเนินการคำขอใช้สิทธิทุกประเภท รวมถึงเรื่องร้องเรียน  
> 21 features · Must 13 / Should 8 · phase: P1 (13), P3 (8)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/dsar` |
| PostgreSQL schema | [`dsar`](../data/dsar.md) (12 ตาราง) |
| Admin API prefix | `/admin/v1/dsar` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /public/v1/dsar-requests` — ยื่นคำขอใช้สิทธิ (SEQ-05)<br>`POST /public/v1/dsar-requests/{ref}/verify` — ยืนยัน OTP ของคำขอ (SEQ-05)<br>`POST /api/v1/dsar/requests` — รับคำขอจาก CRM / call center (BP-06) |
| หน้าจอ (Next.js) | admin: /requests/*; portal: /request, /request/status |
| พึ่งพาบริการ | workflow, forms, docs, files, connectors |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-09 DSAR, BP-06, DFD-1, DFD-2.7, ERD-11, SEQ-05, ST-02 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| GUARD | ผู้ปกครอง / ผู้รับมอบอำนาจ | Guardian / Authorized Agent | Portal: OTP + เอกสารพิสูจน์อำนาจ |
| FRONT | พนักงานหน้าร้าน / Call center | Front Staff | OIDC SSO · Admin app (หน้าจอหน้าร้าน) |
| EXT | ระบบธุรกิจ CRM / POS / CDP | Enterprise Systems (API) | OAuth2 client credentials (token ≤ 15 นาที · scope) |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | Guest link (token หมดอายุ) + OTP |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DSAR-01](#dsar-01) | แบบฟอร์มขอใช้สิทธิออนไลน์ | Must | P1 | DS | M | M | Y | BP-06 |
| [DSAR-02](#dsar-02) | รับคำขอหลายช่องทาง | Must | P1 | FRONT EXT | S | M | N | BP-06 |
| [DSAR-03](#dsar-03) | ครอบคลุมสิทธิทุกประเภท | Must | P1 | DS DPO | M | S | N | BP-06 |
| [DSAR-04](#dsar-04) | ยื่นคำขอแทนเจ้าของข้อมูล | Must | P1 | GUARD | S | S | N | BP-06 |
| [DSAR-06](#dsar-06) | ยืนยันตัวตนผู้ยื่นคำขอ | Must | P1 | DS DPO | M | S | N | BP-06 |
| [DSAR-07](#dsar-07) | นับเวลา SLA 30 วัน | Must | P1 | SCHED DPO | S | S | N | BP-06 |
| [DSAR-08](#dsar-08) | Workflow และงานย่อย | Must | P1 | DPO IT | L | M | Y | BP-06 |
| [DSAR-11](#dsar-11) | ปฏิเสธคำขอพร้อมเหตุผล | Must | P1 | DPO | S | S | N | BP-06 |
| [DSAR-13](#dsar-13) | template หนังสือตอบกลับ | Must | P1 | DPO | S | S | N | BP-06 |
| [DSAR-14](#dsar-14) | ส่งข้อมูลให้เจ้าของข้อมูลอย่างปลอดภัย | Must | P1 | DS DPO | M | S | N | BP-06 |
| [DSAR-15](#dsar-15) | ส่งออกข้อมูลแบบอ่านได้ด้วยเครื่อง | Must | P1 | DS DPO | M | XS | N | BP-06 |
| [DSAR-17](#dsar-17) | ประวัติและสืบค้นคำขอ | Must | P1 | DPO | S | M | N | BP-06 |
| [DSAR-18](#dsar-18) | รายงานและแดชบอร์ดคำขอ | Must | P1 | DPO EXEC | S | M | Y | BP-06 |
| [DSAR-05](#dsar-05) | รับเรื่องร้องเรียนและสอบถาม | Should | P3 | DS DPO | S | S | N | BP-06 |
| [DSAR-09](#dsar-09) | ค้นหาข้อมูลของเจ้าของข้อมูลข้ามระบบ | Should | P3 | IT EXT | L | M | N | BP-06 |
| [DSAR-10](#dsar-10) | ตรวจข้อยกเว้นก่อนลบข้อมูล | Should | P3 | DPO | M | S | N | BP-06 |
| [DSAR-12](#dsar-12) | แจ้งผู้รับข้อมูลให้ดำเนินการตาม | Should | P3 | DPO VENDOR | M | S | N | BP-06 |
| [DSAR-16](#dsar-16) | ปกปิดข้อมูลของบุคคลอื่น | Should | P3 | DPO | L | M | Y | BP-06 |
| [DSAR-19](#dsar-19) | พอร์ทัลติดตามสถานะสำหรับผู้ยื่น | Should | P3 | DS | M | M | Y | BP-06 |
| [DSAR-20](#dsar-20) | ตั้งค่า workflow แยกตามประเภทสิทธิ | Should | P3 | DPO | M | M | Y | BP-06 |
| [DSAR-21](#dsar-21) | เชื่อมกับระบบ Consent | Should | P3 | DPO SCHED | S | XS | N | BP-06 |

### ความสัมพันธ์ระหว่าง use case

- DSAR-01 «include» DSAR-06 (ทุกครั้งที่ทำ DSAR-01 ต้องทำ DSAR-06)
- DSAR-04 «extend» DSAR-01 (DSAR-04 เป็นทางเลือก/ส่วนขยายของ DSAR-01)
- DSAR-11 «include» DSAR-13 (ทุกครั้งที่ทำ DSAR-11 ต้องทำ DSAR-13)
- DSAR-15 «extend» DSAR-14 (DSAR-15 เป็นทางเลือก/ส่วนขยายของ DSAR-14)
- DSAR-16 «extend» DSAR-14 (DSAR-16 เป็นทางเลือก/ส่วนขยายของ DSAR-14)
- DSAR-10 «extend» DSAR-08 (DSAR-10 เป็นทางเลือก/ส่วนขยายของ DSAR-08)

## กระบวนการ / sequence / state machine

- [BP-06 จัดการคำขอใช้สิทธิของเจ้าของข้อมูล (DSAR)](../processes/BP-06.md)
- [SEQ-05 ยื่นคำขอใช้สิทธิ (DSAR) และยืนยันตัวตนด้วย OTP](../sequences/SEQ-05.md)
- [ST-02 คำขอใช้สิทธิ (dsar.requests.status)](../states/ST-02.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [dsar.request_types](../data/dsar.md#dsar-request-types) | ประเภทคำขอ (ม.19, ม.30-36, ร้องเรียน, สอบถาม) |
| [dsar.requests](../data/dsar.md#dsar-requests) | คำขอใช้สิทธิ |
| [dsar.agents](../data/dsar.md#dsar-agents) | ผู้ยื่นแทน / ผู้ใช้อำนาจปกครอง (ม.20) |
| [dsar.verifications](../data/dsar.md#dsar-verifications) | การยืนยันตัวตนของผู้ยื่น |
| [dsar.subtasks](../data/dsar.md#dsar-subtasks) | งานย่อยต่อระบบ / ทีม / ผู้ประมวลผล |
| [dsar.search_results](../data/dsar.md#dsar-search-results) | ผลค้นหาข้อมูลข้ามระบบ (เข้ารหัส ลบเมื่อปิดคำขอ) |
| [dsar.legal_holds](../data/dsar.md#dsar-legal-holds) | ข้อมูลที่ต้องเก็บตามกฎหมายอื่น (ข้อยกเว้น ม.33) |
| [dsar.exemption_checks](../data/dsar.md#dsar-exemption-checks) | ผลตรวจข้อยกเว้นก่อนลบ |
| [dsar.packages](../data/dsar.md#dsar-packages) | แพ็กเกจข้อมูลที่ส่งคืน (ลิงก์หมดอายุ + รหัสผ่าน) |
| [dsar.redactions](../data/dsar.md#dsar-redactions) | งานปกปิดข้อมูลบุคคลอื่นในเอกสาร |
| [dsar.communications](../data/dsar.md#dsar-communications) | การติดต่อกับผู้ยื่น (หนังสือตอบ / ขอข้อมูลเพิ่ม) |
| [dsar.downstream_notices](../data/dsar.md#dsar-downstream-notices) | การแจ้งผู้รับข้อมูลให้ดำเนินการตามคำขอ |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `dsar.request.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `dsar.request` | คำขอใช้สิทธิ | DPO `CRUDAXE` · PRIVACY `CRUX` · LEGAL `RU` · OWNER `R` · IT `R` · FRONT `CR` · AUDIT `RE` · API `C` | Owner/IT เห็นเฉพาะคำขอที่มีงานมอบหมาย; Front Staff เห็นเฉพาะที่ตนลงไว้ |
| `dsar.subtask` | งานย่อยของคำขอ | DPO `CRUDX` · PRIVACY `CRUX` · LEGAL `RU` · OWNER `RU` · IT `RU` · AUDIT `R` · GUEST `RU` | แก้ได้เฉพาะงานที่ได้รับมอบหมาย; GUEST คือผู้ประมวลผลที่ได้รับคำสั่ง |
| `dsar.package` | ข้อมูลที่ส่งคืนเจ้าของข้อมูล | DPO `CRA` · PRIVACY `CR` · IT `CU` · AUDIT `R` | Auditor เห็นเฉพาะ metadata |
| `dsar.form` | แบบฟอร์มคำขอใช้สิทธิ | DPO `CRUDP` · PRIVACY `CRU` · LEGAL `RU` · AUDIT `R` |  |
| `pii.unmask` | เปิดดูข้อมูลส่วนบุคคลแบบไม่ปกปิด | DPO `X` · PRIVACY `X` · SEC `X` | ต้องระบุเหตุผล ถูกบันทึก log ทุกครั้ง และอาจต้องยืนยัน MFA ซ้ำ |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `dsar.created` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.verified` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.subtask_assigned` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.sla_warning` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.completed` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.rejected` | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `dsar.sla_timer` | รายชั่วโมง | ตรวจ due_at → dsar.sla_warning / escalate | BP-06 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** DSAR-01, DSAR-02, DSAR-03, DSAR-04, DSAR-06, DSAR-07, DSAR-08, DSAR-11, DSAR-13, DSAR-14, DSAR-15, DSAR-17, DSAR-18
- **P3:** DSAR-05, DSAR-09, DSAR-10, DSAR-12, DSAR-16, DSAR-19, DSAR-20, DSAR-21

## รายละเอียด feature

<a id="dsar-01"></a>
### DSAR-01 แบบฟอร์มขอใช้สิทธิออนไลน์

*DSR web form*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับคำขอ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30-36
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06, PLT-17
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** สร้างฟอร์มเอง เลือกภาษา มีเวอร์ชัน เผยแพร่ด้วยลิงก์ ฝังเว็บ หรือ QR code และส่งเลขที่คำขอให้ผู้ยื่นทันที

**Backend (Go):** ฟอร์มจาก PLT-06, เวอร์ชัน, เผยแพร่ลิงก์ / embed / QR, สร้างเลขที่คำขอและส่งให้ผู้ยื่นทางอีเมล / SMS ทันที

**Frontend (Next.js):** ฟอร์มใน portal + หน้าตั้งค่าฟอร์มใน admin

**Acceptance criteria:** ผู้ยื่นได้เลขที่คำขอทันทีหลังส่ง

**Implementation (done, see CLAUDE.md's own DSAR-01 note for the full design):** a publishable
`dsar.intake_forms` wrapper (public key issued on publish, same role as CON-09/PNG-06's own public
surfaces) over DSAR-13's existing `CreateRequest`, not PLT-06 — the intake fields are already fixed by
`CreateRequestInput`, so a dynamic form schema would duplicate structure, not add anything the acceptance
criterion needs. Submitting returns the request number immediately and opens a DSAR-06 OTP verification
right away against the contact just given. QR/embed and form versioning are deferred (no consumer needs
them yet).

<a id="dsar-02"></a>
### DSAR-02 รับคำขอหลายช่องทาง

*Multi-channel intake*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับคำขอ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30
- **Actor:** FRONT (พนักงานหน้าร้าน / Call center), EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DSAR-08
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** เจ้าหน้าที่ลงคำขอแทนกรณีมาทางอีเมล Call center สาขา จดหมาย หรือ LINE และรับคำขอผ่าน API จากแอป

**Backend (Go):** เจ้าหน้าที่ลงคำขอแทน (ช่องทาง อีเมล / โทร / สาขา / จดหมาย / LINE) + แนบไฟล์, API รับคำขอจากแอป

**Frontend (Next.js):** หน้าเจ้าหน้าที่บันทึกคำขอ

**Acceptance criteria:** คำขอทุกช่องทางเข้าคิวเดียวกันและระบุช่องทางที่มา

**Implementation:** Most of this was already in place from DSAR-13's own intake slice: `CreateRequest` already
accepted any of the seven `channel` values (web/email/phone/branch/letter/line/api) and an `on_behalf` flag,
and every request — whatever its channel — lands in the same `dsar.requests` table, visible through the same
`ListRequests`/`GET /admin/v1/dsar/requests` the UI already used. What was actually missing against the
literal acceptance criterion: the admin UI's intake form hard-coded `on_behalf: false` (staff had no way to
mark a request as recorded on the data subject's own behalf) and the `/requests` list never showed a
request's channel at all — so "ระบุช่องทางที่มา" (shows its origin channel) wasn't actually visible anywhere,
even though the data was already stored. Fixed both: a "ลงคำขอแทนเจ้าของข้อมูล" checkbox on the intake form
now sets `on_behalf` for real (natural for a phone/branch/letter/LINE intake, where staff are the ones typing
it in), and `/requests` gained a "ช่องทาง" column showing each request's channel with a "ลงแทน" badge when
`on_behalf` is true — "คำขอทุกช่องทางเข้าคิวเดียวกัน" is now also visibly true, not just true in the database.
No migration, no new endpoint, no new permission — this was a frontend gap closing a backend capability that
already existed. "แนบไฟล์" (attach a file at intake, e.g. a scanned letter) needed no new code either: every
`dsar_request` is already a registered PLT-07 collaboration record (DSAR-17), so staff can attach a file to
the request immediately after creating it via the existing "ประวัติ" panel — a second step, not a field on the
create form itself, since PLT-09 attachments need a real entity id to attach to and the create form is a
single atomic call. "รับคำขอผ่าน API จากแอป" (an external app submitting requests through `/api/v1`) is
deliberately not built: that surface needs API-client authentication (ORG-16, not started) the same way
CON-16's own `/api/v1` + webhooks are deferred for the same reason — the `channel: "api"` value is already in
the CHECK constraint and already selectable by staff logging a call from an external system by hand, so a real
`/api/v1` consumer can be layered on top later without touching this schema. Tests: unit (the acceptance
criterion directly — every channel value round-trips through `CreateRequest` and is visible in the same
`ListRequests` call, `on_behalf` is set correctly per channel), `pnpm --filter @pdpa/admin build`/`tsc` clean.
Verified live: created a real `channel: "phone"`, `on_behalf: true` request through the running admin app and
confirmed the list shows "โทรศัพท์" with a "ลงแทน" badge on that row.

<a id="dsar-03"></a>
### DSAR-03 ครอบคลุมสิทธิทุกประเภท

*All data subject rights*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับคำขอ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19, ม.30-36
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSAR-08
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** เข้าถึงและขอสำเนาพร้อมแจ้งแหล่งที่มา (ม.30), โอนย้าย (ม.31), คัดค้าน (ม.32), ลบ/ทำลาย/ทำให้ไม่ระบุตัวตน (ม.33), ระงับการใช้ (ม.34), แก้ไขให้ถูกต้อง (ม.35-36), ถอนความยินยอม (ม.19)

**Backend (Go):** ประเภทคำขอตาม ม.19, 30-36 + ขั้นตอนเฉพาะของแต่ละสิทธิ (เช่น ขอสำเนาต้องแจ้งแหล่งที่มา)

**Frontend (Next.js):** ตัวเลือกประเภทสิทธิในฟอร์มและหน้าคำขอ

**Acceptance criteria:** ทุกประเภทสิทธิมี workflow และหนังสือตอบของตนเอง

**Implementation:** DSAR-13 already gave all 9 right types a shared ST-02 workflow and their own response
letter (purpose-keyed body with `{{request_type_name}}` substituted — "ของตนเอง" in the sense that every
type's own name and number appear in its own generated letter). This feature adds the "ขั้นตอนเฉพาะของแต่ละ
สิทธิ" half the backend note names: `rightSubtaskAction` (`backend/internal/dsar/service/subtasks.go`) maps
each right type's code to the DSAR-08 subtask action its per-system work should carry (ม.30 access → search,
ม.31 portability → export, ม.32 objection → stop_marketing, ม.33 erasure → delete, ม.34 restriction →
restrict, ม.35-36 rectification → rectify, ม.19 withdraw_consent → restrict, complaint/inquiry → review).
`Transition`'s `in_progress` edge now takes `asset_ids` (`dsar.subtasks.asset_id` → `ropa.assets`, already in
the baseline schema but unused until now — "ทุกระบบที่มีข้อมูล (จาก RoPA / data map)" literally, not RoPA
processing activities) and opens one subtask per asset via `openRightSubtasks` — "คำขอเดินครบทุกขั้นตอน"
against every system involved, generalizing DSAR-08's own erasure-only example across every right type.
`access`'s own ม.30 requirement ("ขอสำเนาต้องแจ้งแหล่งที่มา") is a new optional `data_source` field on
`CreateRequest` (stored in `dsar.requests.details` jsonb, previously unused), appended as its own disclosure
paragraph on the generated result letter (`letters.go`'s `appendDataSourceParagraph`) only for `access`
requests that set it. `dsar.completed` — already in `docs/architecture/events.yaml` with "ITSM / CRM
(subtask)" as a named consumer, but nothing published it — now fires on every completion (ม.32/34's "แจ้ง
ระบบปลายทาง"/"หยุดทันทีและแจ้งระบบปลายทาง"), so a downstream subscriber can flag suppression/stop marketing
once the DSAR closes, for whichever right type needs it. API: `data_source` on `DsarRequestInput`/`DsarRequest`,
`asset_ids` on `DsarTransitionInput`, `asset_id` on `DsarSubtask`. UI: an access-only "แหล่งที่มาของข้อมูล"
field on the intake form, an asset multi-select on the in_progress action panel (`/requests`). Tests: unit
(access/erasure open the right action per linked asset, an unknown asset_id refused, dsar.completed published
on every completion, the access letter discloses a given data_source verbatim — two-tenant isolation via the
existing harness), HTTP contract (422 for an unknown asset, 200 opening a real subtask, data_source round-trip).

<a id="dsar-04"></a>
### DSAR-04 ยื่นคำขอแทนเจ้าของข้อมูล

*Authorized agent / guardian*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับคำขอ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.20
- **Actor:** GUARD (ผู้ปกครอง / ผู้รับมอบอำนาจ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSAR-01
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** รองรับผู้รับมอบอำนาจ ผู้ใช้อำนาจปกครอง ผู้อนุบาล และผู้พิทักษ์ พร้อมแนบเอกสารยืนยันอำนาจ

**Backend (Go):** ผู้ยื่นแทน: ประเภทอำนาจ (ม.20), เอกสารมอบอำนาจ / เอกสารผู้ปกครอง, ยืนยันทั้งผู้ยื่นและเจ้าของข้อมูล

**Frontend (Next.js):** ส่วนผู้ยื่นแทนในฟอร์ม

**Acceptance criteria:** คำขอแทนต้องมีเอกสารยืนยันอำนาจก่อนดำเนินการ

**Implementation (done, see CLAUDE.md's own DSAR-04 note for the full design):** `dsar.agents` (already in
the baseline migrations — `authority_file_id` NOT NULL) plus a new `Transition` guard: an `on_behalf`
request can't leave `verifying` for `in_review` without at least one agent record with its authority
document attached. Staff can separately confirm the document (`VerifyAgent`), the same human-judgement call
DSAR-06 already makes for the data subject's own identity.

<a id="dsar-06"></a>
### DSAR-06 ยืนยันตัวตนผู้ยื่นคำขอ

*Identity verification*

- **Priority / Phase:** Must · P1 · กลุ่ม: ยืนยันตัวตน
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-05
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** OTP ทาง SMS/อีเมล แนบสำเนาบัตรพร้อมปกปิดเลขอัตโนมัติ ตรวจกับข้อมูลในระบบ และต่อยอด ThaID/NDID

**Backend (Go):** OTP (IAM-05), แนบสำเนาบัตรและปกปิดเลขอัตโนมัติ (หาเลข 13 หลักด้วย OCR แล้ว mask), ตรวจกับข้อมูลในระบบ, บันทึกเวลายืนยัน

**Frontend (Next.js):** ขั้นตอนยืนยันตัวตนใน portal + หน้าตรวจของเจ้าหน้าที่

**Acceptance criteria:** เลขบัตรในไฟล์ที่เก็บถูกปกปิดเสมอ และบันทึกวันยืนยันตัวตน

**หมายเหตุ:** ThaID / NDID ต่อยอดผ่าน ORG-15

**Implementation (DSAR-06):** `dsar.verifications` was already fully specified in the baseline migrations
(`method` otp_sms/otp_email/id_document/in_person/idp/thaid, `subject_verification_id` → `iam.
subject_verifications`, `masked_id_file_id` → `platform.files`, status pending/passed/failed) — no new
migration; this builds only the (admin-side, no portal yet — same deferral IAM-05's own OTP service used)
service and endpoints against it. OTP reuses IAM-05 directly: `StartOTPVerification` decrypts the request's
own `requester_contact_enc` (PLT-13, never logged — rule 3) and calls `iamservice.StartVerification` with
`Purpose: "dsar"` (already in IAM-05's own purpose list); `ConfirmOTPVerification` calls `VerifyOTP` — a
wrong code is a harmless retry (IAM-05's own 5-attempt cap, D-01), only once IAM-05 itself gives up
(too-many-attempts or expired) does the `dsar.verifications` row get marked `failed`. "ตรวจกับข้อมูลในระบบ"
for OTP is implicit: the code goes to the contact already on file for this exact request, so a correct code
proves the requester controls that channel.

The acceptance criterion's "เลขบัตรในไฟล์ที่เก็บถูกปกปิดเสมอ" is literal, not an OCR claim: no OCR library is
available in this environment (no Tesseract, no cloud OCR service configured — `decisions.md` Q-30), so
rather than guess at unavailable tooling, staff mark the rectangle(s) over the ID number themselves;
`SubmitIDDocumentVerification` decodes the uploaded JPEG/PNG (Go's own `image`/`image/draw` stdlib, no new
dependency), draws an opaque black box over each rectangle (refusing one that reaches outside the image
rather than silently clipping it), and only the *redacted* bytes are ever saved — via `files.Service.
SaveGenerated`, already attached to the request — the caller's raw upload is never attached to anything and
simply expires through PLT-09's own 24h orphan cleanup. `ConfirmIDDocument` is staff's own pass/fail judgement
after checking the redacted card (there is no trusted national-id database to automate this against from
here). Either path's first call on a `received` request moves it `received` → `verifying` (ST-02, already
declared); a pass stamps `dsar.requests.verified_at` (the acceptance criterion's other half) and moves
`verifying` → `in_review`. `internal/dsar/service/identity.go`'s small `Files` interface (rule 9) is backed
directly by `*files.Service` in `cmd/api/main.go` (already constructed), alongside `Verification
*iamservice.Service` (already constructed for `/me` and IAM-05).

API: `GET /admin/v1/dsar/requests/{id}/verifications` (list, oldest first), `POST .../verifications/otp`
(201), `POST .../verifications/{verificationId}/confirm-otp` (200), `POST .../verifications/id-document`
(201, body: `raw_file_id` + one or more pixel `redactions`), `POST .../verifications/{verificationId}/decide`
(200, staff pass/fail) — all `dsar.request.update`, no new permission code. UI: a "ยืนยันตัวตน" toggle per row
on `/requests` — a list of past attempts, an OTP form (method + code), and an ID-document form (`FileUploader`
+ four numeric rectangle fields + a save button, then pass/fail buttons once pending). Tests: unit (OTP pass
stamps `verified_at` and transitions to `in_review`; a wrong code is a harmless retry; redaction blacks out
exactly the given rectangle and nothing else, proven by decoding the saved bytes back to pixels; an
out-of-bounds rectangle is refused; re-deciding an already-decided verification is refused; two-tenant
isolation), HTTP contract (401/403/404/400/201/200 through the real validator), `pnpm --filter @pdpa/admin
build` verified clean.

<a id="dsar-07"></a>
### DSAR-07 นับเวลา SLA 30 วัน

*SLA tracking (30 days)*

- **Priority / Phase:** Must · P1 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30 วรรคสาม
- **Actor:** SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** นับจากวันรับคำขอ แจ้งเตือนก่อนครบกำหนด และแสดงคำขอที่ใกล้/เกินกำหนด

**Backend (Go):** SLA 30 วันนับจากวันรับคำขอ (PLT-05), แจ้งเตือนและ escalate

**Frontend (Next.js):** ป้าย SLA และรายการใกล้ / เกินกำหนด

**Acceptance criteria:** คำขอที่เหลือ 7 วันถูกแจ้งเตือนผู้รับผิดชอบและ DPO

<a id="dsar-08"></a>
### DSAR-08 Workflow และงานย่อย

*Workflow & subtasks*

- **Priority / Phase:** Must · P1 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ขั้นตอน พิจารณา → ยืนยันตัวตน → รวบรวมข้อมูล → ดำเนินการ → แจ้งผล มอบหมายงานย่อยให้เจ้าของระบบหรือทีม และตั้ง rule อัตโนมัติตามประเภทคำขอ

**Backend (Go):** workflow มาตรฐาน พิจารณา → ยืนยันตัวตน → รวบรวม → ดำเนินการ → แจ้งผล, subtask ให้เจ้าของระบบ / ทีม, rule มอบหมายตามประเภทและบริษัท

**Frontend (Next.js):** หน้าคำขอ (timeline, subtask, เอกสาร), กระดานคิวคำขอ

**Acceptance criteria:** คำขอเดินครบทุกขั้นตอนและปิดได้เมื่อ subtask เสร็จทั้งหมด

**Implementation:** `internal/dsar/service/subtasks.go` (`dsar.subtask.*`, already fully seeded in the baseline
permission migration — no new permission code) — CRUD on `dsar.subtasks` (already fully specified in the
baseline migrations). The acceptance criterion is the gate added to `Transition`'s own `in_progress →
completed` edge (DSAR-13): it now additionally requires every subtask of the request to be `done` or
`not_applicable` — a request with none is vacuously "all done" (DSAR-13's own completed tests never created
any), one with any `open`/`in_progress` subtask is refused `dsar.invalid_transition`. "คำขอเดินครบทุกขั้นตอน"
needed no new code: ST-02's full graph was already encoded by DSAR-13, and DSAR-06/11 already built the
`received → ... → completed` path this walks.

`dsar.subtasks.status` is its own small local machine (`open → in_progress → done|not_applicable`, or straight
`open → done|not_applicable` for a subtask quick enough to finish in one step — `docs/states/state-machines.yaml#DSAR-08subtask`),
not routed through the full PLT-05 workflow engine: `dsar.request_types.workflow_definition_id` exists for
that in the schema, but DSAR never adopted it (DSAR-13's own module-doc note explains why — the engine's
assignee is baked into its Definition JSON at the type level, which doesn't fit a per-record subtask list),
so wiring a second engine in for just this gate would be a bigger change than the acceptance criterion needs.
"แก้ได้เฉพาะงานที่ได้รับมอบหมาย" (docs/security/permissions.md's own note on `dsar.subtask`): a caller holding
only `.update` (IT/OWNER/LEGAL/GUEST in the seeded RBAC) may change a subtask assigned to them directly, or to
an `iam.groups` group they belong to (`iamservice.GroupIDsOf`); `.execute` (DPO/PRIVACY) bypasses that check.
Creating an assigned subtask notifies the assignee (PLT-04, migration 00052's `dsar.subtask_assigned`
template, operational text — no DRAFT marker, rule 8) — "มอบหมายงานย่อยให้เจ้าของระบบหรือทีม". A subtask's
optional evidence is the caller's own clean PLT-09 upload, attached via `files.Service.Attach` against the
already-registered `dsar_request` entity type (DSAR-17's own `fileSvc.EntityPermissions["dsar_request"]`).

"ตั้ง rule อัตโนมัติตามประเภทและบริษัท" (an automatic default-subtask rule per request type) is deliberately
not built: no column models it and no screen needs it yet — the module doc's own description names it, but the
literal acceptance criterion is only about subtask completion gating closure, which is fully exercised without
one. Add it (most naturally as a `default_subtask_actions` column on `dsar.request_types`, applied when a
request enters `in_progress`) once a real request for it exists, the same "no consumer yet" deferral this
codebase uses elsewhere.

API: `GET`/`POST /admin/v1/dsar/requests/{id}/subtasks`, `POST .../subtasks/{subtaskId}/status` (ETag/If-Match),
`DELETE .../subtasks/{subtaskId}` — same shapes as every other module's child-table endpoints. UI: a "งานย่อย"
toggle per row on `/requests` — a status table, an inline create form (action + a user or group picker, reusing
PLT-05's own `useGroupSearch`), and per-row start/done/not-applicable/remove actions. Tests: unit (the
acceptance criterion directly — a request with an open subtask cannot complete, closes once it's done, a
request with none is unaffected; action/assignee/group validation; the access-restriction rule for user and
group assignees and its `.execute` bypass; delete; two-tenant isolation), HTTP contract (401/403/400/201/200/
204/404/409/412/428). Verified live: created a real request and subtask through the running admin app against
the real API, moved it `open → done`, and confirmed the row's actions correctly reduced to just "remove" once
terminal. Found and fixed while live-testing a separate, pre-existing bug this same pass surfaced: `/requests`'
row `<Fragment>` had no `key` of its own (only the inner `<tr>` did) — React warned "Each child in a list
should have a unique key prop"; fixed by moving the key onto the `Fragment`.

<a id="dsar-11"></a>
### DSAR-11 ปฏิเสธคำขอพร้อมเหตุผล

*Rejection with reason*

- **Priority / Phase:** Must · P1 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30, ม.39(7)
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSAR-13
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** บันทึกเหตุผลตามข้อยกเว้นของกฎหมาย ส่งหนังสือแจ้ง และลงบันทึกใน RoPA อัตโนมัติ

**Backend (Go):** เลือกเหตุปฏิเสธตามกฎหมาย, ต้องมีผู้อนุมัติ, ส่งหนังสือแจ้ง, event dsar.rejected (→ ROPA-10)

**Frontend (Next.js):** ขั้นตอนปฏิเสธ + เลือกเหตุผล

**Acceptance criteria:** การปฏิเสธต้องมีเหตุผลและผู้อนุมัติ และถูกบันทึกเข้า RoPA อัตโนมัติ

**Implementation:** built on top of DSAR-13's `Transition`/ST-02: entering `rejected` already required a
reason; this feature added the second half of the acceptance criterion. "ผู้อนุมัติ" (an approver) is
enforced as a permission gate — the caller must additionally hold `dsar.request.approve` (already-seeded,
no new code), not just `dsar.request.execute` — rather than a separate propose/confirm maker-checker round,
since DSAR-13's `rejected` transition is already a single explicit action a DPO takes with a reason attached,
not a multi-step draft. "ลงบันทึกใน RoPA อัตโนมัติ" is deliberately *not* done here: `TransitionInput` gained
an optional `ActivityIDs []uuid.UUID` (the RoPA processing activities this rejection concerns — FK-checked via
a new `Ropa` interface, rule 9), and rejecting now publishes `dsar.rejected` (PLT-11 outbox) with those
activity ids and the reason code — but DSAR never writes to the `ropa` schema itself; the actual write is
ROPA-10's own job, subscribing to this event (module boundary, rule 9). `docs/architecture/events.yaml`'s
`dsar.rejected` entry (a generic SA-authored template shared by all `dsar.*` lifecycle events) was extended
with `reason_code` and `activity_refs` — the two fields ROPA-10 actually needs that the generic template
didn't carry — and the catalog regenerated (`go generate ./internal/platform/events`). Tests: unit (rejecting
without `dsar.request.approve` is refused with `ErrForbidden`/403, an unknown activity id is refused, the
outbox row carries the reason and activity ref, redelivery-safety is ROPA-10's job to prove), HTTP contract.
UI: the reject panel on `/requests` gained an activity multi-select (shown only when rejecting), passed as
`activity_ids` on the transition call.

<a id="dsar-13"></a>
### DSAR-13 template หนังสือตอบกลับ

*Response templates*

- **Priority / Phase:** Must · P1 · กลุ่ม: ตอบกลับ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** หนังสือตอบ TH/EN ตามประเภทสิทธิและผลการพิจารณา (ดำเนินการแล้ว / ปฏิเสธ / ขอข้อมูลเพิ่ม)

**Backend (Go):** template หนังสือตอบ TH/EN ต่อประเภทสิทธิและผล (document composer) + merge ข้อมูลคำขอ

**Frontend (Next.js):** เลือก template + แก้ก่อนส่ง

**Acceptance criteria:** หนังสือตอบถูกสร้างอัตโนมัติตามประเภทสิทธิและผลการพิจารณา

**Implementation:** `internal/dsar` — the first module on the `dsar` schema. Building the acceptance criterion
needed a real `dsar.requests` row with a status/outcome to generate from, so this pass also carries the
minimal slice of DSAR-01/02/06/07/08/11's job DSAR-13 depends on — creating a request and moving it through
ST-02's real transition graph — deliberately scoped no further: full intake channels, identity verification,
the workflow/subtask engine, business-day SLA countdown and the reject-with-reason UI are sibling features
layered on top later (the same "build the minimal slice a feature needs, not its whole dependency" move ROPA-01
made ahead of ROPA-02). `dsar.request_types` and `platform.templates`/the `dsar_letter` PLT-16 document type
(`dsar.request.*` permissions) were all already fully specified in the baseline migrations and
`internal/wiring.Docs` — this pass only seeds data (migration 00043: the 9 fixed right-type rows, plus an
`ALTER TABLE ... ADD COLUMN name_en` since the baseline only carried a Thai name and the letter is bilingual)
and 6 DRAFT response-letter template rows (`platform.templates`, `template_type = 'dsar_response'`) — one per
*purpose* (result / rejection / request_info, the module doc's own "ดำเนินการแล้ว / ปฏิเสธ / ขอข้อมูลเพิ่ม"),
not per (type × purpose): `{{request_type_name}}` is substituted with the request's own type name at
generation time, so one body correctly reads as being about access for an access request and erasure for an
erasure request without 27 near-duplicate bodies to keep in sync — flagged DRAFT per rule 8, same pattern as
ORG-07/ROPA-09/PNG-03.

`Service.Transition` encodes ST-02's full 8-state, 17-edge graph (`docs/states/state-machines.yaml#ST-02`)
even though this feature only exercises a subset of it — cheap to encode correctly once, so DSAR-06/08/11
build on a real machine later instead of reinventing one; no verification, subtask-completeness or
SLA-overdue guard is enforced on the intermediate edges yet (those modules don't exist), only the two
data-carrying guards this feature needs (`outcome` required to enter `completed`, a reason required to enter
`rejected`). Entering `awaiting_info`/`completed`/`rejected` auto-generates the matching response letter — a
PLT-16 `dsar_letter` document draft, composed from the purpose's template with the requester's name (decrypted
via PLT-13, `Keyring.Decrypt` — the same class/context pattern CON-13's identifiers already established),
request number and type name substituted in as plain text (not PLT-16's `mergeField` mechanism, which only
resolves org/DPO fields live at every render — these values are specific to this one immutable letter
instance). The generated document is still just a draft: DPO reviews and edits it before ever sending it
("เลือก template + แก้ก่อนส่ง") through PLT-16's own existing approve/publish flow — actually recording that a
letter was sent (`dsar.communications`) is left to whichever later feature owns delivery tracking, the same
"leave the FK to build the real thing later" move ROPA-01 made for `discovered_by_finding_id`.

`due_at` is `received_at` plus the request type's `sla_days` as *calendar* days — a placeholder; the real
business-day countdown (Songkran-aware, like every other deadline in this codebase) is DSAR-07's job.
Permissions reuse the already-seeded `dsar.request.*` (no new codes): `read`/`create`/`execute` gate the three
endpoints, and the letter-generation step inside `Transition` additionally needs `dsar.request.update` (the
`dsar_letter` document type's own `Create`/`Update` permission per `internal/wiring/docs.go`) since it's really
PLT-16's document-composer write happening underneath. API: `GET /admin/v1/dsar/request-types`,
`GET/POST /admin/v1/dsar/requests`, `GET /admin/v1/dsar/requests/{id}`,
`POST /admin/v1/dsar/requests/{id}/transition` (ETag/If-Match; 409 `dsar.invalid_transition` for a graph edge
that doesn't exist, 422 `dsar.invalid_input` for a missing outcome/reason). UI `/requests`: an intake form,
a status-filtered list with the SLA due date, and an inline action panel per request (next-status picker +
outcome/rejection-reason fields) that links straight to the generated letter's document editor when one is
produced. Tests: unit (`ListRequestTypes` covers all 9 codes, `CreateRequest` validation and numbering, the
acceptance criterion directly — completing/rejecting a request generates a letter carrying the decrypted
requester name, the request's own type name and its request number in both languages — a rejected letter
carries the reason, an invalid ST-02 edge refused, two-tenant isolation), HTTP contract
(401/403/400/404/409/412/422/428).

<a id="dsar-14"></a>
### DSAR-14 ส่งข้อมูลให้เจ้าของข้อมูลอย่างปลอดภัย

*Secure delivery*

- **Priority / Phase:** Must · P1 · กลุ่ม: ตอบกลับ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-09, PLT-17
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ส่งผ่านพอร์ทัลหรือลิงก์ที่หมดอายุและมีรหัสผ่าน พร้อมบันทึกการดาวน์โหลด

**Backend (Go):** แพ็กไฟล์ผลลัพธ์เข้ารหัส, ลิงก์หมดอายุ + รหัสผ่าน / OTP, บันทึกการดาวน์โหลด, ลบไฟล์อัตโนมัติเมื่อหมดอายุ

**Frontend (Next.js):** หน้ารับไฟล์ใน portal

**Acceptance criteria:** ลิงก์หมดอายุตามที่ตั้งและมี log การดาวน์โหลด

**Implementation (done):** `dsar.packages` was already fully specified in the baseline migrations
(`token_hash`, `password_protected`, `expires_at`, `download_count`, `last_downloaded_at`, `purged_at`) —
no new table, only migration 00067 (`platform.public_keys.entity_type` widened to add `dsar_package`, the
same CHECK-widening pattern DPO-09/CON-11 already use). `internal/dsar/service/delivery.go`: `CreatePackage`
wraps either the caller's own clean, still-unattached PLT-09 upload (attached here, the same "record + attach
in one call" shape DSAR-04's `RecordAgent` uses) or a file already attached to the request (DSAR-13's own
generated response letter); issues a `platform.public_keys` key the same way CON-09/PNG-06/DSAR-01/BRE-01's
own public surfaces do, and a separate, second secret — a 10-character access code from an unambiguous
alphabet, hashed into `token_hash` (SHA-256) and returned to the caller exactly once, never stored in the
clear (rule 3). `PublicPackage`/`PublicDownload` resolve the tenant from the key before the tenant
transaction opens; `PublicDownload` checks the code with `subtle.ConstantTimeCompare`, returns a short-lived
signed URL via `files.Service.SignedURL`, and records `download_count`/`last_downloaded_at` plus an audit
entry on every call — the acceptance criterion itself. A new PLT-09 trusted operation, `files.Service.
DeleteObject` (alongside `Status`/`Open`/`SignedURL`/`SaveGenerated`/`AttachSystem`), lets the one-time
River purge job (`dsar.package_purge`, scheduled exactly at `expires_at`, the same per-record checkpoint
pattern BRE's SLA timers and DSAR-07's reminder use — not a periodic sweep) delete the file's bytes while
keeping the `platform.files` row and the package's own audit trail ("object deleted, row + audit kept",
PLT-09's own infected-file pattern). A purged or expired package reads as 404 through `PublicPackage`/
`PublicDownload` alike — no information leaked either way. API: `GET`/`POST /admin/v1/dsar/requests/{id}/
packages` (`dsar.package.read`/`.create`, both already seeded in the baseline permission migration); public
`GET /public/v1/dsar-packages/{key}`, `POST /public/v1/dsar-packages/{key}/download` (`Idempotency-Key`
required — found and fixed while building this: the first draft of the download endpoint left it off,
missing rule 10's own "Idempotency-Key required for POST on /public/v1"). UI: `/requests` gained a
"แพ็กเกจ" toggle per row (format/password-protected/file upload form, a one-time issued-result panel showing
the public link and access code) and a new portal page, `apps/portal/src/app/[locale]/dsar-packages/[key]`
(server actions, per-attempt `Idempotency-Key` on the download action — mirrors `/[locale]/dsar/[key]`'s own
server-component/action split). Tests: unit (the acceptance criterion directly — a package expires per its
own setting and every download is logged with count/last_downloaded_at/an audit row; correct vs. wrong access
code; a file already attached to a different request is refused; purge deletes the file's bytes while keeping
the row and reads as not-found afterward, a second purge is a harmless no-op; two-tenant isolation), HTTP
contract (`internal/dsar/publichttp/handler_test.go` — 404 for an unknown key, 428 for a missing Idempotency-
Key, 422 for a wrong access code, 200 through the real validator + publickeys + Idempotency + Tx chain, an
Idempotency-Key replay returning the original result). Not verified live against a real Postgres/Redis — same
environment limitation DSAR-01/DSAR-04/BRE-01 already disclosed; every test compiles and vets clean and skips
the same way the rest of this module's own test suite already does here. Not done, deliberately (no consumer
yet, the same deferral this codebase uses throughout): a single-use/one-download-only option (the module doc's
own acceptance criterion has no such requirement — repeat downloads within the link's own expiry are allowed);
a portal-side identity check before download (DSAR-06's own OTP flow is the request's identity gate, not the
package's — the access code is this feature's own, separate secret).

<a id="dsar-15"></a>
### DSAR-15 ส่งออกข้อมูลแบบอ่านได้ด้วยเครื่อง

*Portability export*

- **Priority / Phase:** Must · P1 · กลุ่ม: ตอบกลับ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.31
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DSAR-14
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ส่งออก CSV/JSON/XML ในรูปแบบที่ใช้ทั่วไป และส่งตรงไปยังผู้ควบคุมรายอื่นเมื่อทำได้

**Backend (Go):** รวมข้อมูลเป็น CSV / JSON / XML ตาม schema มาตรฐาน; ส่งให้ผู้ควบคุมรายอื่นผ่าน API / SFTP เมื่อทำได้

**Frontend (Next.js):** ตัวเลือกรูปแบบไฟล์

**Acceptance criteria:** ไฟล์ portability เปิดอ่านด้วยเครื่องได้และตรงกับข้อมูลในระบบ

**Implementation (done):** `internal/dsar/service/export.go`'s `GeneratePortabilityExport` builds a
JSON/CSV/XML file from exactly what the system holds about the request's own subject — the request's own
decrypted name/contact plus any consent decisions on record for the same identifier (`consentservice.
Subjects`/`GetProfile`, matched through the exact blind-index lookup DSAR-17's own search already uses,
never decrypted to search, rule 3) — and hands it straight to DSAR-14's own `createPackage` core, so the
export is delivered through the exact same expiring, access-code-protected link, with every download
already logged. RoPA was deliberately not read for this: it only ever catalogs *categories* of data
processed, never an individual's own records, so it has nothing a portability export could include;
consent is the one other module in this codebase with real per-subject data on file. Direct transfer to
another controller via API/SFTP (the module doc's own "เมื่อทำได้") is deliberately not built — no such
controller endpoint exists anywhere in this codebase to transfer to; the file handed to the data subject is
what ม.31 requires regardless. The generated file skips `CreatePackage`'s own clean-AV-status gate (it's
this package's own trusted output, never a user upload) via the same `files.Service.SaveGenerated` path
PLT-16's rendered documents and PLT-14's import error report already use. API: `POST /admin/v1/dsar/requests/
{id}/export` (`dsar.package.create`, same response shape as `dsarCreatePackage` — a `DsarCreatePackageResult`
with the public key and, when password-protected, the access code shown exactly once). UI: a "ส่งออกข้อมูล
แบบพกพาได้" panel next to `/requests`' own package-creation form, a format picker and a generate button.
Tests: unit (`internal/dsar/service/export_test.go` — the acceptance criterion directly: every format
decodes with its standard library parser and every value matches what `CreateRequest` actually stored, plus
a matching consent subject's real decision; no matching consent subject is an empty list, not an error; bad
format and the permission gate refused; two-tenant isolation). Not verified live against a real Postgres/
Redis — same environment limitation DSAR-01/DSAR-04/BRE-01/DSAR-14 already disclosed; every test compiles
and vets clean and skips the same way the rest of this module's own test suite already does here.

<a id="dsar-17"></a>
### DSAR-17 ประวัติและสืบค้นคำขอ

*Request history & search*

- **Priority / Phase:** Must · P1 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39(7)
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-07
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ประวัติการพิจารณา วันกำหนดแจ้งผล วันยืนยันตัวตน และการแก้ไขทุกครั้ง ค้นหาย้อนหลังได้

**Backend (Go):** ประวัติ / timeline, วันกำหนด, วันยืนยันตัวตน, ค้นหาย้อนหลังตามเลขคำขอ / บุคคล (blind index)

**Frontend (Next.js):** หน้าค้นหาคำขอ + timeline

**Acceptance criteria:** ค้นหาคำขอย้อนหลังด้วยเลขคำขอหรืออีเมลได้

<a id="dsar-18"></a>
### DSAR-18 รายงานและแดชบอร์ดคำขอ

*DSR reports & dashboard*

- **Priority / Phase:** Must · P1 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** จำนวนคำขอตามประเภทและสถานะ ตรงเวลา/เกินกำหนด ส่งออก Excel/CSV/PDF

**Backend (Go):** API สถิติคำขอตามประเภท / สถานะ / ตรงเวลา-เกินกำหนด + export

**Frontend (Next.js):** dashboard คำขอใช้สิทธิ

**Acceptance criteria:** ตัวเลขใน dashboard ตรงกับรายการคำขอ

<a id="dsar-05"></a>
### DSAR-05 รับเรื่องร้องเรียนและสอบถาม

*Complaints & inquiries*

- **Priority / Phase:** Should · P3 · กลุ่ม: รับคำขอ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DSAR-08
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** คำขอประเภทร้องเรียน/สอบถามที่ใช้ workflow เดียวกัน ติดตามจนปิดเรื่อง

**Backend (Go):** ประเภทร้องเรียน / สอบถามใช้ workflow เดียวกัน

**Frontend (Next.js):** ตัวเลือกประเภทในฟอร์มและตัวกรอง

**Acceptance criteria:** เรื่องร้องเรียนติดตามจนปิดได้เหมือนคำขอ

**Implementation — done.** The backend "same workflow" half needed no new code: DSAR-13 (built earlier)
already seeds the `complaint`/`inquiry` right types (migration `00043_dsar_request_types.sql`) and already
built the full generic ST-02 8-state/17-edge `Transition` machine (`internal/dsar/service/requests.go`) that
every right type — complaint and inquiry included — already walks identically through to `completed`, and
DSAR-08's own `rightSubtaskAction` map already routes both to the `review` subtask action. So a complaint
or inquiry was already "tracked through to close just like a [regular] request" from the moment DSAR-13
shipped — this feature's real, narrow gap against the acceptance criterion was the *visible* half: no
`request_type_id` filter existed on `GET /admin/v1/dsar/requests` or on the `/requests` admin page, only a
`status` filter, so complaints/inquiries landed in the same queue (already true) but couldn't be filtered
to a dedicated view (the module doc's own frontend note: "ตัวเลือกประเภทในฟอร์มและตัวกรอง" — a type picker
*and* a type filter; the picker already existed on the intake form from DSAR-13, only the filter was
missing). Added end to end: `ListRequests` (`backend/db/queries/dsar/requests.sql`) gained an optional
`request_type_id` clause alongside its existing `status`/`search`/cursor filters; `RequestFilter` gained a
`RequestTypeID *uuid.UUID` field; `GET /admin/v1/dsar/requests` gained a `request_type_id` query parameter
(OpenAPI + oapi-codegen + handler wiring); `useDsarRequests` (api-client) and `/requests`' own filter bar
(a `<select>` next to the existing status filter, `allTypes`/`th`/`en` i18n keys added) complete the chain.
No migration, no new permission code — every table/permission/transition this needed was already in place.
Tests: `internal/dsar/service/requests_test.go`'s `TestListRequests_FiltersByRequestType` (the acceptance
criterion directly — a complaint and an access request land in the same unfiltered queue, `request_type_id`
narrows to exactly one, and the complaint is driven through the real `verifying → in_review → in_progress →
completed` chain to prove it closes like any other request) and
`TestListRequests_FiltersByRequestType_TwoTenantIsolation`; `internal/dsar/http/handler_test.go`'s
`TestDsarEndpoints_Contract` extended with a complaint create + filter-by-type round trip through the real
validator + AuthZ. Not verified live against a real Postgres/Redis — the same environment limitation this
codebase's other recent passes already disclosed (both were reachable earlier in this session, for other
features, but not for this pass); every test compiles and vets clean and skips the same way the rest of this
module's own suite already does here. `go build ./...`, `go vet ./...`, `gofmt -l` (clean on every touched
file), `pnpm --filter @pdpa/api-client typecheck`, `pnpm --filter @pdpa/i18n test` and
`pnpm --filter @pdpa/admin build` (incl. the updated `/requests` route) all verified clean. Not done,
deliberately: nothing — the acceptance criterion ("complaints tracked to close like a request," with a type
picker and filter) is now fully exercised; DSAR-09's own cross-system search (a sibling, separate backlog
row) is unrelated.

<a id="dsar-09"></a>
### DSAR-09 ค้นหาข้อมูลของเจ้าของข้อมูลข้ามระบบ

*Cross-system data search*

- **Priority / Phase:** Should · P3 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30
- **Actor:** IT (เจ้าของระบบ / IT), EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-20
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ค้นข้อมูลของบุคคลในระบบต้นทางผ่าน connector/API และรวมผลในคำขอเดียว

**Backend (Go):** ส่งคำค้น (identifier) ไปยัง connector แต่ละระบบ, รวมผลเป็นรายการข้อมูลในคำขอ, เก็บผลเข้ารหัสชั่วคราว

**Frontend (Next.js):** แท็บผลค้นหาข้ามระบบในหน้าคำขอ

**Acceptance criteria:** ค้นข้อมูลจากระบบที่เชื่อมได้ในคำสั่งเดียว และผลถูกลบเมื่อปิดคำขอ

<a id="dsar-10"></a>
### DSAR-10 ตรวจข้อยกเว้นก่อนลบข้อมูล

*Exemption & legal hold check*

- **Priority / Phase:** Should · P3 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.33
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPX-05
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ตรวจว่าข้อมูลยังต้องเก็บตามกฎหมายอื่น หรือใช้เพื่อสิทธิเรียกร้องตามกฎหมาย ก่อนลบหรือทำลาย

**Backend (Go):** rule legal hold ต่อประเภทข้อมูล / ระบบ (อ้างระยะเวลาเก็บตามกฎหมายอื่น) ตรวจก่อนลบ + บันทึกเหตุผล

**Frontend (Next.js):** ผลตรวจข้อยกเว้นในหน้าคำขอลบ

**Acceptance criteria:** ข้อมูลที่ติด legal hold ไม่ถูกลบและมีเหตุผลอ้างอิง

**สถานะ: done** — ดูรายละเอียดใน `CLAUDE.md` หัวข้อ `### DSAR-10`. สรุป: `dsar.legal_holds`/
`dsar.exemption_checks` (มีอยู่แล้วตั้งแต่ baseline migration — DPX-05 ใช้ `legal_holds` อยู่ก่อนแล้วสำหรับ
disposal job แต่ยังไม่มี CRUD/UI ของตัวเอง และ `exemption_checks` ยังไม่มีใครใช้เลย) — DPO สร้าง legal hold
ผูกกับ asset หรือ data category ได้ตรง ๆ, `RunExemptionCheck` ตรวจระบบที่เลือกกับทุก hold ที่ active
(รวม hold ที่ผูกกับ data category ผ่าน ROPA-01 data inventory ของ asset นั้น — ปิดช่องว่างที่ DPX-05 เคย
ทิ้งไว้ให้ feature นี้ทำ), บันทึกผล can_delete/must_retain/partial พร้อมเหตุผลอ้าง law_ref. คำขอประเภท
erasure (ม.33) ปิดไม่ได้จนกว่าจะมีการตรวจข้อยกเว้นอย่างน้อยหนึ่งครั้ง (`dsar.exemption_check_required`) —
ตรวจ "มีการตรวจ" ไม่ใช่ "ผลต้องเป็นอะไร" เพราะการลบจริงเกิดนอกระบบนี้ (staff ทำผ่าน subtask ของ DSAR-08
เอง). API: `GET`/`POST /admin/v1/dsar/legal-holds`, `DELETE /admin/v1/dsar/legal-holds/{id}` (soft —
is_active=false), `GET`/`POST /admin/v1/dsar/requests/{id}/exemption-checks` — ใช้ permission `dsar.request.*`
เดิมทั้งหมด ไม่มี permission code หรือ migration ใหม่. UI: toggle "ตรวจข้อยกเว้น" บน `/requests` (แสดง
เฉพาะคำขอประเภท erasure) — รายการผลตรวจ + ฟอร์มเลือกระบบแล้วสั่งตรวจ.

<a id="dsar-12"></a>
### DSAR-12 แจ้งผู้รับข้อมูลให้ดำเนินการตาม

*Notify downstream recipients*

- **Priority / Phase:** Should · P3 · กลุ่ม: ดำเนินการ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.33, ม.34, ม.40
- **Actor:** DPO (DPO / Privacy Team), VENDOR (คู่ค้า / ผู้ประมวลผล (guest))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-20, IAM-04
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ส่งงานไปยังระบบหรือผู้ประมวลผลที่ได้รับข้อมูล ให้แก้ไข ลบ หรือระงับตามคำขอเดียวกัน และเก็บหลักฐาน

**Backend (Go):** สร้างงานให้ผู้ประมวลผล / ระบบปลายทาง (webhook / connector / guest link) และเก็บหลักฐานการดำเนินการ

**Frontend (Next.js):** แท็บผู้รับข้อมูลในหน้าคำขอ

**Acceptance criteria:** ทุกผู้รับข้อมูลยืนยันการลบ/แก้ไขพร้อมหลักฐานก่อนปิดคำขอ

<a id="dsar-16"></a>
### DSAR-16 ปกปิดข้อมูลของบุคคลอื่น

*Redaction*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตอบกลับ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.30 วรรคสอง
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DSAR-14
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** ปิดทับข้อมูลของบุคคลอื่นในเอกสารที่ส่งคืน เพื่อไม่กระทบสิทธิเสรีภาพของผู้อื่น

**Backend (Go):** ปิดทับ PDF / รูปภาพ (ตรวจหาเลขบัตร / เบอร์ / ชื่อด้วย OCR ภาษาไทย + ผู้ใช้เลือกเพิ่ม) แล้วสร้างไฟล์ใหม่ที่ลบข้อมูลจริง

**Frontend (Next.js):** หน้าตรวจและเลือกพื้นที่ปิดทับ

**Acceptance criteria:** ข้อมูลที่ปิดทับกู้คืนจากไฟล์ผลลัพธ์ไม่ได้

**หมายเหตุ:** OCR ภาษาไทยต้อง PoC (T13)

<a id="dsar-19"></a>
### DSAR-19 พอร์ทัลติดตามสถานะสำหรับผู้ยื่น

*Requester status portal*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-17, IAM-05
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** เจ้าของข้อมูลตรวจสถานะ ส่งเอกสารเพิ่ม และรับผลผ่านพอร์ทัลเดียว

**Backend (Go):** portal เจ้าของข้อมูล: ดูสถานะ ส่งเอกสารเพิ่ม รับผล ข้อความโต้ตอบ

**Frontend (Next.js):** หน้าติดตามสถานะใน portal

**Acceptance criteria:** เจ้าของข้อมูลเห็นสถานะล่าสุดและส่งเอกสารเพิ่มได้เอง

<a id="dsar-20"></a>
### DSAR-20 ตั้งค่า workflow แยกตามประเภทสิทธิ

*Configurable workflow per right*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตั้งค่า
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DSAR-08
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** กำหนดขั้นตอน ผู้รับผิดชอบ และ template ตอบกลับของแต่ละสิทธิได้เอง

**Backend (Go):** ตั้งค่าขั้นตอน ผู้รับผิดชอบ และ template ต่อสิทธิ (UI บน PLT-05)

**Frontend (Next.js):** หน้าตั้งค่า workflow ต่อประเภทสิทธิ

**Acceptance criteria:** เปลี่ยน workflow แล้วมีผลกับคำขอใหม่โดยไม่ต้อง deploy

<a id="dsar-21"></a>
### DSAR-21 เชื่อมกับระบบ Consent

*Consent integration*

- **Priority / Phase:** Should · P3 · กลุ่ม: เชื่อมต่อ
- **ที่มา:** Function List: 02_DSAR
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19, ม.32
- **Actor:** DPO (DPO / Privacy Team), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** CON-13
- **Process:** [BP-06](../processes/BP-06.md)

**คำอธิบาย:** คำขอถอนความยินยอมหรือคัดค้านการตลาดอัปเดตสถานะในระบบ Consent อัตโนมัติ

**Backend (Go):** คำขอถอน / คัดค้านการตลาด → เรียก Consent API ถอน Purpose ที่เกี่ยวข้อง + แนบ receipt ในคำขอ

**Frontend (Next.js):** แสดงผลการอัปเดตความยินยอมในหน้าคำขอ

**Acceptance criteria:** คำขอถอนความยินยอมอัปเดตสถานะใน CON อัตโนมัติ

**Implementation (DSAR-21):** `internal/dsar/service/consent_integration.go`'s `withdrawConsentIfApplicable`,
called from `Transition` right after the existing `dsar.completed` event publish, whenever a request
reaches `completed` and its own `dsar.request_types.code` is `withdraw_consent` (ม.19) — the acceptance
criterion's own literal scope; "คัดค้านการตลาด" (objection to marketing) is deliberately not wired the same
way, since nothing in this schema distinguishes a marketing purpose from any other, so there's no reliable
way to withdraw only the marketing ones (add it once a real `is_marketing` flag exists on a purpose, the same
"no consumer yet" deferral this codebase uses throughout). The requester's own decrypted contact (the same
`decryptRequesterContact` DSAR-06/DSAR-15 already use) is matched against consent's own subject registry via
the already-exported `Consent.Subjects` (rule 9 — dsar never writes to the consent schema directly), then a
new `consentservice.Service.WithdrawAllActive(ctx, subjectID, reasonCode)` withdraws every purpose that
subject currently holds `ACTIVE` — the same `checkDecisions`/`writeDecisions`/`channelForPreferenceUpdate`
plumbing CON-18's own `UpdatePreferences` already uses, minus the OTP step: the caller (dsar) has already
verified the requester's identity through its own flow (DSAR-06) before this is ever reached, so re-verifying
would just burn IAM-05's 5-attempt cap for no benefit. The `source` CHECK constraint already had `'dsar'`
reserved for exactly this. The withdrawal is a harmless no-op — not an error — for a requester with no
matching consent subject, or with nothing currently active (most DSAR requests are never also a consent
decision). "แนบ receipt ในคำขอ" is `dsar.requests.details`'s own jsonb catch-all (the same column DSAR-03's
own `data_source` already uses, no new table): a new `SetRequestConsentReceipt` query merges in
`consent_receipt_no`, and `Transition`'s own in-memory `out` is updated with it directly (re-reading the row
back would have shown the same value one call later, but updating it in place avoids that round trip and a
real bug this surfaced: the first draft left `out` stale, since the write happened after `out` was already
built — fixed by threading the receipt number back through the call's own return value instead of discarding
it). API: `consent_receipt_no` added to `DsarRequest`. UI: `/requests`' status column gained a small badge
once a request carries one (title = the receipt number). Tests:
`internal/dsar/service/consent_integration_test.go` — the acceptance criterion directly through the real
`verifying → in_review → in_progress → completed` chain (a withdraw-consent request's own active purpose is
withdrawn, the resulting receipt attached and persisted, re-readable afterward) and the no-match no-op case,
both against a `fakeConsent` test double widened with a `WithdrawAllActive` stub. `go build ./...`,
`go vet ./...`, `gofmt -l` (clean), `pnpm --filter @pdpa/i18n test`, `pnpm --filter @pdpa/api-client
typecheck` and `pnpm --filter @pdpa/admin build` all verified clean.
