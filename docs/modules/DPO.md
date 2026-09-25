# DPO — งานของ DPO (DPO Module)

> ระบบบริหารจัดการสำหรับเจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล (DPO Module) · ขอบเขต: งานประจำของ DPO: การแต่งตั้ง แดชบอร์ด การแจ้งเตือน งาน การให้คำปรึกษา และรายงาน  
> 12 features · Must 3 / Should 8 / Nice 1 · phase: P1 (4), P3 (7), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/dpo` |
| PostgreSQL schema | [`dpo`](../data/dpo.md) (8 ตาราง) |
| Admin API prefix | `/admin/v1/dpo` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /dpo/*, /dashboard |
| พึ่งพาบริการ | workflow, report |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-11 DPO, DFD-1, ERD-14 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | OIDC SSO · Admin app / portal พนักงาน |
| SEC | ทีม Security / Incident | Security / Incident Response | OIDC SSO + MFA · Admin app |
| ORGADMIN | ผู้ดูแลระบบขององค์กร | Organization Admin | OIDC (Keycloak) + MFA บังคับ · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| PDPC | สคส. | PDPC (Regulator) | ไม่ login: รับ/ส่งผ่านช่องทางของ สคส. |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DPO-01](#dpo-01) | ทะเบียนการแต่งตั้งและข้อมูล DPO | Must | P1 | ORGADMIN DPO PDPC | S | S | N |  |
| [DPO-04](#dpo-04) | แดชบอร์ด DPO | Should | P1 | DPO | M | M | Y |  |
| [DPO-05](#dpo-05) | ศูนย์แจ้งเตือนรวม | Must | P1 | DPO SCHED | M | M | Y |  |
| [DPO-09](#dpo-09) | ประเมินมาตรการความปลอดภัย | Must | P1 | SEC DPO | M | S | N |  |
| [DPO-02](#dpo-02) | ประเมินหน้าที่ต้องแต่งตั้ง DPO | Should | P3 | ORGADMIN DPO | S | S | N |  |
| [DPO-06](#dpo-06) | Tasks Management | Should | P3 | DPO EMP | M | M | Y |  |
| [DPO-07](#dpo-07) | รับคำปรึกษาจากหน่วยงาน | Should | P3 | EMP DPO | S | S | N |  |
| [DPO-08](#dpo-08) | ปฏิทินงาน compliance | Should | P3 | DPO | S | M | N |  |
| [DPO-10](#dpo-10) | คะแนนความพร้อมรายหน่วยงาน | Should | P3 | DPO EXEC | M | M | Y |  |
| [DPO-11](#dpo-11) | รายงานการปฏิบัติหน้าที่ DPO และรายงานผู้บริหาร | Should | P3 | DPO EXEC | S | S | N |  |
| [DPO-12](#dpo-12) | คลังเอกสารกฎหมายและ FAQ | Should | P3 | EMP DPO | S | M | N |  |
| [DPO-03](#dpo-03) | ความเป็นอิสระและผลประโยชน์ทับซ้อน | Nice | P4 | DPO | XS | S | N |  |

### ความสัมพันธ์ระหว่าง use case

- DPO-04 «include» DPO-05 (ทุกครั้งที่ทำ DPO-04 ต้องทำ DPO-05)
- DPO-07 «extend» DPO-12 (DPO-07 เป็นทางเลือก/ส่วนขยายของ DPO-12)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [dpo.appointments](../data/dpo.md#dpo-appointments) | การแต่งตั้ง DPO และการแจ้ง สคส. |
| [dpo.requirement_checks](../data/dpo.md#dpo-requirement-checks) | ผลประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41) |
| [dpo.independence_declarations](../data/dpo.md#dpo-independence-declarations) | คำรับรองความเป็นอิสระ / ผลประโยชน์ทับซ้อน |
| [dpo.tasks](../data/dpo.md#dpo-tasks) | งาน / ticket 5 สถานะ (สร้างอัตโนมัติจาก gap / DPIA / audit) |
| [dpo.advisories](../data/dpo.md#dpo-advisories) | คำปรึกษาจากหน่วยงานถึง DPO |
| [dpo.kb_articles](../data/dpo.md#dpo-kb-articles) | คลังเอกสารกฎหมายและ FAQ |
| [dpo.calendar_events](../data/dpo.md#dpo-calendar-events) | ปฏิทินงาน compliance (กิจกรรมที่ไม่ได้มาจากโมดูลอื่น) |
| [dpo.report_schedules](../data/dpo.md#dpo-report-schedules) | ตั้งเวลาส่งรายงาน DPO / ผู้บริหาร |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `dpo.profile.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `dpo.profile` | ทะเบียน DPO | ORGADMIN `RU` · DPO `CRU` · LEGAL `R` · AUDIT `R` · EXEC `R` |  |
| `dpo.task` | Tasks / Ticket | DPO `CRUDX` · PRIVACY `CRUX` · LEGAL `RU` · OWNER `RU` · IT `RU` · SEC `RU` · AUDIT `R` | แก้ได้เฉพาะงานที่ได้รับมอบหมาย |
| `dpo.advisory` | คำปรึกษาถึง DPO | DPO `RUX` · PRIVACY `RUX` · LEGAL `RU` · OWNER `CR` · IT `CR` · MKT `CR` · AUDIT `R` · EMP `CR` | ผู้ถามเห็นเฉพาะเรื่องของตนเอง |
| `dpo.risk` | ทะเบียนความเสี่ยง | DPO `CRUDA` · PRIVACY `CRU` · OWNER `R` · SEC `CRU` · AUDIT `R` · EXEC `RA` |  |
| `dpo.report` | Dashboard และรายงาน | ORGADMIN `R` · DPO `RE` · PRIVACY `RE` · LEGAL `R` · SEC `R` · AUDIT `RE` · EXEC `RE` |  |
| `dpo.kb` | คลังเอกสารกฎหมายและ FAQ | DPO `CRUDP` · PRIVACY `CRU` · LEGAL `CRU` · OWNER `R` · IT `R` · MKT `R` · FRONT `R` · SEC `R` · PROC `R` · AUDIT `R` · EXEC `R` · EMP `R` |  |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** DPO-01, DPO-05, DPO-09, DPO-04
- **P3:** DPO-02, DPO-06, DPO-07, DPO-08, DPO-10, DPO-11, DPO-12
- **P4:** DPO-03

## รายละเอียด feature

<a id="dpo-01"></a>
### DPO-01 ทะเบียนการแต่งตั้งและข้อมูล DPO

*DPO appointment & profile*

- **Priority / Phase:** Must · P1 · กลุ่ม: DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.41
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), DPO (DPO / Privacy Team), PDPC (สคส.)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-01
- **Process:** —

**คำอธิบาย:** ข้อมูล DPO ช่องทางติดต่อ คำสั่งแต่งตั้ง และเผยแพร่ข้อมูลติดต่อให้เจ้าของข้อมูลและ สคส.

**Backend (Go):** ทะเบียน DPO (ภายใน / ภายนอก), คำสั่งแต่งตั้ง (ไฟล์), ช่องทางติดต่อ (merge field ในประกาศ), บันทึกการแจ้ง สคส.

**Frontend (Next.js):** หน้าข้อมูล DPO

**Acceptance criteria:** ประกาศทุกฉบับแสดงช่องทางติดต่อ DPO ล่าสุด

<a id="dpo-04"></a>
### DPO-04 แดชบอร์ด DPO

*DPO dashboard*

- **Priority / Phase:** Should · P1 · กลุ่ม: งาน DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-18
- **Process:** —

**คำอธิบาย:** ภาพรวมคำขอใช้สิทธิ เหตุละเมิด DPIA ความเสี่ยง สัญญา และความพร้อมรายหน่วยงาน

**Backend (Go):** API widget ข้ามโมดูล (คำขอ เหตุละเมิด DPIA ความเสี่ยง สัญญา) — P1 แสดงโมดูลที่มี แล้วเพิ่มตาม phase

**Frontend (Next.js):** dashboard DPO

**Acceptance criteria:** ตัวเลขทุก widget ตรงกับข้อมูลในโมดูล

**หมายเหตุ:** ดึงเข้า P1

<a id="dpo-05"></a>
### DPO-05 ศูนย์แจ้งเตือนรวม

*Notification center*

- **Priority / Phase:** Must · P1 · กลุ่ม: งาน DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42
- **Actor:** DPO (DPO / Privacy Team), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05, PLT-04
- **Process:** —

**คำอธิบาย:** รวมกำหนดเวลาสำคัญ: SLA 30 วัน, 72 ชั่วโมง, DPIA ครบรอบ, สัญญาหมดอายุ, ประกาศต้องทบทวน

**Backend (Go):** รวม deadline ทุกโมดูลจาก SLA engine (SLA 30 วัน, 72 ชั่วโมง, ทบทวน, สัญญาหมดอายุ) + ตั้งค่าการแจ้งเตือนส่วนตัว

**Frontend (Next.js):** หน้าศูนย์แจ้งเตือน

**Acceptance criteria:** งานใกล้ครบกำหนดจากทุกโมดูลแสดงในหน้าเดียว

**หมายเหตุ:** OneTrust ไม่มีมุมมองรวม

<a id="dpo-09"></a>
### DPO-09 ประเมินมาตรการความปลอดภัย

*Security measures assessment*

- **Priority / Phase:** Must · P1 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** SEC (ทีม Security / Incident), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** checklist ตามประกาศมาตรการความปลอดภัย (มาตรการเชิงองค์กร เชิงเทคนิค และทางกายภาพ การควบคุมการเข้าถึง) พร้อมหลักฐาน

**Backend (Go):** checklist ตามประกาศมาตรการความปลอดภัย พ.ศ. 2565 (PLT-06) + หลักฐาน + คะแนน + งานแก้ไข

**Frontend (Next.js):** หน้าประเมินและผลคะแนน

**Acceptance criteria:** ข้อที่ไม่ผ่านสร้างงานแก้ไขอัตโนมัติ

**หมายเหตุ:** OneTrust ต้องนำเข้า framework เอง

<a id="dpo-02"></a>
### DPO-02 ประเมินหน้าที่ต้องแต่งตั้ง DPO

*DPO requirement check*

- **Priority / Phase:** Should · P3 · กลุ่ม: DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.41; ประกาศ สคส. เรื่อง DPO ตามมาตรา 41(2) พ.ศ. 2566
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** ประเมินตามประกาศ สคส. พ.ศ. 2566 เช่น ประมวลผลข้อมูลจำนวนมาก (ตั้งแต่ 100,000 รายขึ้นไป) หรือติดตามพฤติกรรมอย่างสม่ำเสมอ

**Backend (Go):** แบบประเมินตาม ม.41 และประกาศ พ.ศ. 2566 (เช่น เกณฑ์ 100,000 ราย) → ผลและเหตุผล

**Frontend (Next.js):** wizard ประเมินหน้าที่แต่งตั้ง DPO

**Acceptance criteria:** ผลประเมินระบุว่าต้องแต่งตั้งหรือไม่พร้อมเหตุผลอ้างประกาศ

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="dpo-06"></a>
### DPO-06 Tasks Management

*DPO task management*

- **Priority / Phase:** Should · P3 · กลุ่ม: งาน DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05
- **Process:** —

**คำอธิบาย:** ticket 5 สถานะ (สร้าง → มอบหมาย → รอตรวจ → เสร็จ → สิ้นสุด) สร้างอัตโนมัติจาก RoPA / ช่องว่าง / DPIA มีผู้ตรวจ วันครบกำหนด ความสำคัญ และความเห็น

**Backend (Go):** ticket 5 สถานะ (สร้าง → มอบหมาย → รอตรวจ → เสร็จ → สิ้นสุด), สร้างอัตโนมัติจาก RoPA / ช่องว่าง / DPIA

**Frontend (Next.js):** kanban + รายการงาน

**Acceptance criteria:** งานที่สร้างอัตโนมัติมีที่มาและลิงก์กลับ

<a id="dpo-07"></a>
### DPO-07 รับคำปรึกษาจากหน่วยงาน

*Advisory requests*

- **Priority / Phase:** Should · P3 · กลุ่ม: งาน DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42(1)
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** —

**คำอธิบาย:** หน่วยงานส่งคำถามหรือขอความเห็นจาก DPO ติดตามจนตอบ และสร้างคลังคำตอบ

**Backend (Go):** หน่วยงานส่งคำถาม → DPO ตอบ → เก็บเป็นคลังคำตอบ

**Frontend (Next.js):** หน้าส่งคำถามและกล่องงานของ DPO

**Acceptance criteria:** คำถามที่ตอบแล้วค้นได้ในคลังคำตอบ

<a id="dpo-08"></a>
### DPO-08 ปฏิทินงาน compliance

*Compliance calendar*

- **Priority / Phase:** Should · P3 · กลุ่ม: งาน DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42(2)
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** —

**คำอธิบาย:** รอบทบทวน RoPA ประกาศ DPIA การอบรม และการประเมินคู่ค้า

**Backend (Go):** รวมรอบทบทวน / อบรม / ประเมินเป็นปฏิทิน + export iCal

**Frontend (Next.js):** ปฏิทินงาน compliance

**Acceptance criteria:** ปฏิทินแสดงกิจกรรมจากทุกโมดูลตรงกับกำหนดการจริง

<a id="dpo-10"></a>
### DPO-10 คะแนนความพร้อมรายหน่วยงาน

*Compliance scorecard*

- **Priority / Phase:** Should · P3 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** RRA-13
- **Process:** —

**คำอธิบาย:** สรุปความพร้อมตาม PDPA ของแต่ละหน่วยงานจากทุกโมดูล

**Backend (Go):** สูตรคะแนนต่อหน่วยงานจากทุกโมดูล (RoPA ครบ, DPIA, คำขอเกินกำหนด, อบรม) + แนวโน้ม

**Frontend (Next.js):** scorecard รายหน่วยงาน

**Acceptance criteria:** คะแนนคำนวณซ้ำได้และอธิบายที่มาได้

<a id="dpo-11"></a>
### DPO-11 รายงานการปฏิบัติหน้าที่ DPO และรายงานผู้บริหาร

*DPO & executive reports*

- **Priority / Phase:** Should · P3 · กลุ่ม: รายงาน
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-18
- **Process:** —

**คำอธิบาย:** รายงานรายเดือน/ไตรมาส รายงานการปฏิบัติหน้าที่ DPO และสรุปงานรายบุคคล (PDF)

**Backend (Go):** รายงานรายเดือน / ไตรมาส (PDF) + ตั้งเวลาส่ง

**Frontend (Next.js):** หน้าสร้างและตั้งเวลารายงาน

**Acceptance criteria:** รายงานถูกส่งตามเวลาที่ตั้ง

<a id="dpo-12"></a>
### DPO-12 คลังเอกสารกฎหมายและ FAQ

*Knowledge base*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความรู้
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** T40
- **Process:** —

**คำอธิบาย:** รวม พ.ร.บ. ประกาศ สคส. แนวปฏิบัติ นโยบายองค์กร และคำถามที่พบบ่อย

**Backend (Go):** คลังเอกสาร (หมวด ค้นหา เวอร์ชัน) + FAQ; เนื้อหาตั้งต้นจากทีมกฎหมาย

**Frontend (Next.js):** หน้าคลังความรู้

**Acceptance criteria:** ค้นหาเอกสารภาษาไทยได้

<a id="dpo-03"></a>
### DPO-03 ความเป็นอิสระและผลประโยชน์ทับซ้อน

*Independence & conflict of interest*

- **Priority / Phase:** Nice · P4 · กลุ่ม: DPO
- **ที่มา:** Function List: 14_DPO
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.42
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** —

**คำอธิบาย:** บันทึกหน้าที่อื่นของ DPO และคำรับรองว่าไม่ขัดกับหน้าที่ DPO

**Backend (Go):** แบบคำรับรองประจำปี + บันทึกหน้าที่อื่นของ DPO

**Frontend (Next.js):** หน้าคำรับรอง

**Acceptance criteria:** มีคำรับรองที่ลงนามทุกปี
