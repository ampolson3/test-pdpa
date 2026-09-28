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
