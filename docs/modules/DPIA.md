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
