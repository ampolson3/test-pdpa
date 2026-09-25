# RTG — RoPA ฉบับมาตรฐาน (ROPA Template Generator)

> ระบบจัดเตรียมบันทึกรายการกิจกรรมประมวลผลข้อมูลส่วนบุคคลฉบับมาตรฐาน (ROPA Template Generator) · ขอบเขต: คลังกิจกรรมมาตรฐานและตัวช่วยสร้าง RoPA ให้หน่วยงานเริ่มได้เร็ว  
> 13 features · Must 5 / Should 7 / Nice 1 · phase: P1 (2), P2 (3), P3 (7), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/ropa/templates` |
| PostgreSQL schema | [`ropa`](../data/ropa.md) (2 ตาราง) |
| Admin API prefix | `/admin/v1/ropa/templates` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /ropa/templates/* |
| พึ่งพาบริการ | ropa, ai |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-07 RTG, BP-05, DFD-1, ERD-09 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | OIDC + MFA + IP allowlist · Provider console |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [RTG-01](#rtg-01) | คลังกิจกรรมมาตรฐานตามหมวดงาน | Must | P1 | DPO SUPER | M | M | Y |  |
| [RTG-04](#rtg-04) | สร้าง RoPA จาก template ในไม่กี่ขั้นตอน | Must | P1 | OWNER DPO | M | M | Y | BP-05 |
| [RTG-05](#rtg-05) | Wizard ถาม-ตอบภาษาง่าย | Must | P2 | OWNER | M | L | Y | BP-05 |
| [RTG-06](#rtg-06) | ค่าแนะนำพร้อมเหตุผล | Must | P2 | OWNER | M | S | N | BP-05 |
| [RTG-12](#rtg-12) | ส่งออก template และร่าง RoPA | Must | P2 | DPO | S | XS | N |  |
| [RTG-02](#rtg-02) | Template ตามอุตสาหกรรม | Should | P3 | DPO SUPER | XS | S | N |  |
| [RTG-03](#rtg-03) | Template ภาครัฐตามแม่แบบ สพร. | Should | P3 | DPO SUPER | XS | S | N |  |
| [RTG-07](#rtg-07) | สร้าง RoPA ผู้ประมวลผลจาก template | Should | P3 | OWNER DPO | S | S | N | BP-05 |
| [RTG-08](#rtg-08) | แก้ไขและสร้าง template ขององค์กร | Should | P3 | DPO LEGAL | M | M | N |  |
| [RTG-09](#rtg-09) | Template สองภาษา | Should | P3 | LEGAL | XS | XS | N |  |
| [RTG-10](#rtg-10) | เวอร์ชัน template และแจ้งเมื่อกฎหมายเปลี่ยน | Should | P3 | SUPER SCHED DPO | M | S | N |  |
| [RTG-11](#rtg-11) | มอบหมายร่าง RoPA ให้หน่วยงานยืนยัน | Should | P3 | SCHED OWNER | S | S | N |  |
| [RTG-13](#rtg-13) | AI แนะนำกิจกรรมจากคำอธิบายงาน | Nice | P4 | OWNER LLM | M | S | N |  |

### ความสัมพันธ์ระหว่าง use case

- RTG-04 «include» RTG-01 (ทุกครั้งที่ทำ RTG-04 ต้องทำ RTG-01)
- RTG-05 «include» RTG-06 (ทุกครั้งที่ทำ RTG-05 ต้องทำ RTG-06)
- RTG-04 «include» RTG-11 (ทุกครั้งที่ทำ RTG-04 ต้องทำ RTG-11)
- RTG-13 «extend» RTG-05 (RTG-13 เป็นทางเลือก/ส่วนขยายของ RTG-05)

## กระบวนการ / sequence / state machine

- [BP-05 จัดทำและอนุมัติ RoPA (Record of Processing Activities)](../processes/BP-05.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [ropa.template_sets](../data/ropa.md#ropa-template-sets) | ชุด template (มาตรฐาน / อุตสาหกรรม / ภาครัฐ / ขององค์กร) |
| [ropa.activity_templates](../data/ropa.md#ropa-activity-templates) | กิจกรรมมาตรฐานพร้อมค่าตั้งต้นและเหตุผล |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `ropa.template.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `ropa.template` | คลัง RoPA ฉบับมาตรฐาน | SUPER `CRUD` · DPO `CRUDP` · PRIVACY `CRU` · LEGAL `CRU` · OWNER `RX` · AUDIT `R` | SUPER ดูแลคลังกลาง; tenant สร้าง template ของตนเอง |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** RTG-01, RTG-04
- **P2:** RTG-05, RTG-06, RTG-12
- **P3:** RTG-02, RTG-03, RTG-07, RTG-08, RTG-09, RTG-10, RTG-11
- **P4:** RTG-13

## รายละเอียด feature

<a id="rtg-01"></a>
### RTG-01 คลังกิจกรรมมาตรฐานตามหมวดงาน

*Standard activity library*

- **Priority / Phase:** Must · P1 · กลุ่ม: คลัง template
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-03, T15
- **Process:** —

**คำอธิบาย:** กิจกรรมสำเร็จรูป เช่น สรรหาพนักงาน เงินเดือน จัดซื้อ การตลาด CCTV ผู้มาติดต่อ IT support พร้อมค่าตั้งต้นครบทุกหัวข้อ ม.39

**Backend (Go):** คลังกิจกรรมมาตรฐาน (reference data ระดับแพลตฟอร์ม มีเวอร์ชัน) + ค่าตั้งต้นครบ ม.39

**Frontend (Next.js):** หน้าเลือกหมวดงาน / กิจกรรม + ดูค่าตั้งต้น

**Acceptance criteria:** มีกิจกรรมมาตรฐานอย่างน้อย 50 รายการใน P1 ครบทุกหัวข้อ ม.39

**หมายเหตุ:** ดึงเข้า P1: จุดขายหลักเทียบ OneTrust; เนื้อหาจาก T15

<a id="rtg-04"></a>
### RTG-04 สร้าง RoPA จาก template ในไม่กี่ขั้นตอน

*One-click generation*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** RTG-01
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** เลือกหมวดงาน → เลือกกิจกรรม → เลือกแผนกเจ้าของ แล้วได้ร่าง RoPA ทันที

**Backend (Go):** เลือกหมวด → กิจกรรม → แผนก → สร้างร่าง RoPA หลายรายการพร้อมกัน (clone ค่าตั้งต้น + ผูก master data ของ tenant)

**Frontend (Next.js):** wizard 3 ขั้นตอน + สรุปก่อนสร้าง

**Acceptance criteria:** สร้างร่าง RoPA 20 กิจกรรมได้ในครั้งเดียว

**หมายเหตุ:** ดึงเข้า P1 คู่กับ RTG-01

<a id="rtg-05"></a>
### RTG-05 Wizard ถาม-ตอบภาษาง่าย

*Question-based wizard*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06, ROPA-03
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** ตอบคำถามภาษาทั่วไป เช่น เก็บข้อมูลอะไร ส่งให้ใคร เก็บนานแค่ไหน แล้วระบบแปลงเป็นฟิลด์ RoPA

**Backend (Go):** คำถามภาษาง่าย → map เป็นฟิลด์ RoPA (rule mapping), สรุปก่อนบันทึก

**Frontend (Next.js):** wizard ถาม-ตอบทีละขั้น + preview RoPA

**Acceptance criteria:** ผู้ใช้ทั่วไปสร้าง RoPA ครบหัวข้อได้โดยไม่ต้องรู้ศัพท์กฎหมาย

<a id="rtg-06"></a>
### RTG-06 ค่าแนะนำพร้อมเหตุผล

*Suggested defaults with rationale*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.24, ม.26, ม.39
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RTG-01
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** แนะนำฐานกฎหมาย ระยะเวลาเก็บ และประเภทข้อมูลตามกิจกรรม พร้อมคำอธิบายให้ผู้ใช้ยืนยัน

**Backend (Go):** rule แนะนำฐานกฎหมาย / ระยะเวลาเก็บ / ประเภทข้อมูลจากกิจกรรม + คำอธิบายอ้างมาตรา; ผู้ใช้ยืนยันทีละข้อ

**Frontend (Next.js):** ป้ายค่าแนะนำ + เหตุผล + ปุ่มยืนยัน

**Acceptance criteria:** ค่าแนะนำทุกข้อมีเหตุผลอ้างมาตรา และไม่ถูกบันทึกจนกว่าผู้ใช้ยืนยัน

<a id="rtg-12"></a>
### RTG-12 ส่งออก template และร่าง RoPA

*Export templates*

- **Priority / Phase:** Must · P2 · กลุ่ม: ส่งออก
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-18
- **Process:** —

**คำอธิบาย:** ส่งออก Excel/Word ตามแบบฟอร์มมาตรฐาน

**Backend (Go):** export template และร่าง RoPA เป็น Excel / Word ตามแบบฟอร์มมาตรฐาน

**Frontend (Next.js):** ปุ่มส่งออก

**Acceptance criteria:** ไฟล์ส่งออกเปิดได้ทั้ง Excel และ Word

<a id="rtg-02"></a>
### RTG-02 Template ตามอุตสาหกรรม

*Industry templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: คลัง template
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RTG-01, T40
- **Process:** —

**คำอธิบาย:** ชุดกิจกรรมสำหรับค้าปลีก โรงพยาบาล การเงิน/ประกัน การศึกษา อสังหาฯ และภาครัฐ

**Backend (Go):** ชุดกิจกรรมตามอุตสาหกรรม (เนื้อหาจาก T40)

**Frontend (Next.js):** ตัวกรองตามอุตสาหกรรม

**Acceptance criteria:** มีชุดกิจกรรมครบ 6 อุตสาหกรรม

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="rtg-03"></a>
### RTG-03 Template ภาครัฐตามแม่แบบ สพร.

*Government templates (DGA)*

- **Priority / Phase:** Should · P3 · กลุ่ม: คลัง template
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** เอกสารแม่แบบผู้ควบคุมข้อมูลภาครัฐ สพร. v1.0 (2564)
- **Actor:** DPO (DPO / Privacy Team), SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RTG-01, T40
- **Process:** —

**คำอธิบาย:** ใช้โครง RoPA ตามเอกสารแม่แบบสำหรับผู้ควบคุมข้อมูลภาครัฐของ สพร.

**Backend (Go):** ชุด template ตามแม่แบบผู้ควบคุมข้อมูลภาครัฐของ สพร.

**Frontend (Next.js):** ตัวกรองภาครัฐ

**Acceptance criteria:** RoPA ที่สร้างมีหัวข้อตามแม่แบบ สพร.

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="rtg-07"></a>
### RTG-07 สร้าง RoPA ผู้ประมวลผลจาก template

*Processor RoPA templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศ สคส. RoPA ผู้ประมวลผล พ.ศ. 2565
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-04
- **Process:** [BP-05](../processes/BP-05.md)

**คำอธิบาย:** template สำหรับองค์กรที่เป็นผู้ประมวลผลข้อมูล ตามประกาศ RoPA ผู้ประมวลผล

**Backend (Go):** template สำหรับองค์กรที่เป็นผู้ประมวลผล

**Frontend (Next.js):** ตัวกรอง template ผู้ประมวลผล

**Acceptance criteria:** สร้าง RoPA ผู้ประมวลผลจาก template ได้

<a id="rtg-08"></a>
### RTG-08 แก้ไขและสร้าง template ขององค์กร

*Custom templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: ปรับแต่ง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** RTG-01
- **Process:** —

**คำอธิบาย:** คัดลอก template กลางมาปรับ และกำหนด template มาตรฐานของกลุ่มบริษัท

**Backend (Go):** clone template กลาง → template ของกลุ่มบริษัท, publish ให้บริษัทในเครือ

**Frontend (Next.js):** หน้าแก้ไข template ขององค์กร

**Acceptance criteria:** บริษัทในเครือใช้ template มาตรฐานของกลุ่มได้

<a id="rtg-09"></a>
### RTG-09 Template สองภาษา

*Bilingual templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: ปรับแต่ง
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-03
- **Process:** —

**คำอธิบาย:** ทุก template มีภาษาไทยและอังกฤษ

**Backend (Go):** เนื้อหา template TH/EN ครบทุกรายการ

**Frontend (Next.js):** สลับภาษาในหน้า template

**Acceptance criteria:** ทุก template มีทั้ง TH และ EN

<a id="rtg-10"></a>
### RTG-10 เวอร์ชัน template และแจ้งเมื่อกฎหมายเปลี่ยน

*Template versioning*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม), SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** RTG-08
- **Process:** —

**คำอธิบาย:** ปรับ template เมื่อมีประกาศ สคส. ใหม่ และแจ้งกิจกรรมที่สร้างจาก template เดิมให้ทบทวน

**Backend (Go):** เวอร์ชัน template; ออกเวอร์ชันใหม่แล้วหา RoPA ที่สร้างจากเวอร์ชันเก่าและสร้างงานทบทวน

**Frontend (Next.js):** หน้าประวัติ template + รายการที่ต้องทบทวน

**Acceptance criteria:** ประกาศ สคส. ใหม่ → กิจกรรมที่ได้รับผลกระทบได้รับงานทบทวน

<a id="rtg-11"></a>
### RTG-11 มอบหมายร่าง RoPA ให้หน่วยงานยืนยัน

*Auto task to department*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SCHED (ระบบ: Scheduler / Event), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** —

**คำอธิบาย:** สร้างงานให้แผนกเจ้าของตรวจและยืนยันร่างที่สร้างจาก template โดยอัตโนมัติ

**Backend (Go):** สร้างงานให้แผนกเจ้าของยืนยันร่างอัตโนมัติ + SLA

**Frontend (Next.js):** กล่องงานยืนยันร่างของแผนก

**Acceptance criteria:** ร่างที่ยังไม่ยืนยันถูกเตือนตาม SLA

<a id="rtg-13"></a>
### RTG-13 AI แนะนำกิจกรรมจากคำอธิบายงาน

*AI activity suggestion*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 04_ROPA_Template
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-22
- **Process:** —

**คำอธิบาย:** วิเคราะห์คำอธิบายงานหรือเอกสารของแผนก แล้วแนะนำกิจกรรมที่ควรมีใน RoPA

**Backend (Go):** AI วิเคราะห์คำอธิบายงาน / เอกสาร → แนะนำกิจกรรมจากคลังและฟิลด์ที่ควรมี

**Frontend (Next.js):** ช่องใส่คำอธิบายงาน + รายการคำแนะนำให้ยืนยัน

**Acceptance criteria:** คำแนะนำทุกข้อต้องมีคนยืนยันก่อนสร้างกิจกรรม
