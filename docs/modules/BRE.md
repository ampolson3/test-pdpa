# BRE — แจ้งเหตุละเมิดข้อมูล (Data Breach Notification)

> ระบบจัดการแจ้งเหตุละเมิดข้อมูลส่วนบุคคล (Data Breach Notification) · ขอบเขต: รับแจ้ง ประเมิน แจ้ง สคส. / เจ้าของข้อมูล และติดตามเหตุละเมิด  
> 17 features · Must 12 / Should 3 / Nice 2 · phase: P1 (12), P3 (3), P4 (2)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/breach` |
| PostgreSQL schema | [`breach`](../data/breach.md) (12 ตาราง) |
| Admin API prefix | `/admin/v1/breach` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /public/v1/breach-reports` — ฟอร์มแจ้งเหตุสาธารณะ + CAPTCHA (BP-07)<br>`POST /api/v1/breach/incidents` — SIEM ส่งเหตุเข้า (BP-07) |
| หน้าจอ (Next.js) | admin: /incidents/*; portal: /report-incident |
| พึ่งพาบริการ | workflow, forms, docs, notify |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-10 BRE, BP-07, DFD-1, DFD-2.8, ERD-12, SEQ-06, ST-03 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | OIDC SSO · Admin app / portal พนักงาน |
| PUBLIC | บุคคลภายนอกที่พบเหตุ | External Reporter | ฟอร์มแจ้งเหตุสาธารณะ + CAPTCHA |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | Guest link (token หมดอายุ) + OTP |
| SEC | ทีม Security / Incident | Security / Incident Response | OIDC SSO + MFA · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| PDPC | สคส. | PDPC (Regulator) | ไม่ login: รับ/ส่งผ่านช่องทางของ สคส. |
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [BRE-01](#bre-01) | แบบฟอร์มแจ้งเหตุ | Must | P1 | EMP PUBLIC | S | M | Y | BP-07 |
| [BRE-02](#bre-02) | ทะเบียนเหตุละเมิด | Must | P1 | SEC DPO | M | M | Y | BP-07 |
| [BRE-03](#bre-03) | รับแจ้งเหตุจากผู้ประมวลผล | Must | P1 | VENDOR | S | S | N | BP-07 |
| [BRE-04](#bre-04) | กำหนดผู้รับแจ้งและทีมตอบสนอง | Must | P1 | DPO SCHED | S | S | N | BP-07 |
| [BRE-05](#bre-05) | ประเมินความเสี่ยงของเหตุ | Must | P1 | SEC DPO | M | S | N | BP-07 |
| [BRE-06](#bre-06) | ตัดสินหน้าที่แจ้งพร้อมเหตุผล | Must | P1 | DPO EXEC | S | S | N | BP-07 |
| [BRE-07](#bre-07) | นับเวลา 72 ชั่วโมง | Must | P1 | SCHED | S | S | N | BP-07 |
| [BRE-08](#bre-08) | แจ้งล่าช้าพร้อมเหตุผล | Must | P1 | DPO | S | S | N | BP-07 |
| [BRE-09](#bre-09) | แบบแจ้ง สคส. และแจ้งเพิ่มเติมเป็นระยะ | Must | P1 | DPO PDPC | M | M | Y | BP-07 |
| [BRE-10](#bre-10) | แจ้งเจ้าของข้อมูลเมื่อความเสี่ยงสูง | Must | P1 | DPO DS | M | S | N | BP-07 |
| [BRE-12](#bre-12) | หลักฐานและลำดับเหตุการณ์ | Must | P1 | SEC | S | M | N | BP-07 |
| [BRE-13](#bre-13) | ประวัติและสืบค้นเหตุ | Must | P1 | DPO | XS | S | N | BP-07 |
| [BRE-11](#bre-11) | แผนตอบสนองและงานควบคุมเหตุ | Should | P3 | SEC | M | M | Y | BP-07 |
| [BRE-14](#bre-14) | รายงานและสถิติเหตุละเมิด | Should | P3 | DPO EXEC | S | M | N | BP-07 |
| [BRE-15](#bre-15) | วิเคราะห์สาเหตุและบทเรียน | Should | P3 | SEC | S | S | N | BP-07 |
| [BRE-16](#bre-16) | ซ้อมรับมือเหตุละเมิด | Nice | P4 | SEC DPO | S | S | N | BP-07 |
| [BRE-17](#bre-17) | AI ช่วยประเมินเหตุและร่างรายงาน | Nice | P4 | DPO LLM | M | S | N | BP-07 |

### ความสัมพันธ์ระหว่าง use case

- BRE-01 «include» BRE-02 (ทุกครั้งที่ทำ BRE-01 ต้องทำ BRE-02)
- BRE-03 «extend» BRE-01 (BRE-03 เป็นทางเลือก/ส่วนขยายของ BRE-01)
- BRE-02 «include» BRE-07 (ทุกครั้งที่ทำ BRE-02 ต้องทำ BRE-07)
- BRE-06 «include» BRE-05 (ทุกครั้งที่ทำ BRE-06 ต้องทำ BRE-05)
- BRE-08 «extend» BRE-09 (BRE-08 เป็นทางเลือก/ส่วนขยายของ BRE-09)
- BRE-10 «extend» BRE-06 (BRE-10 เป็นทางเลือก/ส่วนขยายของ BRE-06)

## กระบวนการ / sequence / state machine

- [BP-07 จัดการและแจ้งเหตุละเมิดข้อมูลส่วนบุคคล (Data breach 72 ชม.)](../processes/BP-07.md)
- [SEQ-06 เหตุละเมิดข้อมูล: นับเวลา 72 ชม. และการแจ้ง สคส. / เจ้าของข้อมูล](../sequences/SEQ-06.md)
- [ST-03 เหตุละเมิดข้อมูล (breach.incidents.status)](../states/ST-03.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [breach.incidents](../data/breach.md#breach-incidents) | เหตุละเมิดข้อมูลส่วนบุคคล |
| [breach.incident_assets](../data/breach.md#breach-incident-assets) | ระบบ / กิจกรรมที่เกี่ยวข้องกับเหตุ |
| [breach.assessments](../data/breach.md#breach-assessments) | การประเมินความเสี่ยงของเหตุ (ประกาศแจ้งเหตุ พ.ศ. 2565) |
| [breach.pdpc_notifications](../data/breach.md#breach-pdpc-notifications) | การแจ้ง สคส. (ฉบับเบื้องต้น / เพิ่มเติม / สุดท้าย) |
| [breach.subject_notifications](../data/breach.md#breach-subject-notifications) | การแจ้งเจ้าของข้อมูล (ความเสี่ยงสูง) |
| [breach.notification_recipients](../data/breach.md#breach-notification-recipients) | ผู้รับแจ้งรายคน |
| [breach.playbooks](../data/breach.md#breach-playbooks) | playbook ตามประเภทเหตุ |
| [breach.response_tasks](../data/breach.md#breach-response-tasks) | งานควบคุม / แก้ไข / กู้คืน |
| [breach.evidence](../data/breach.md#breach-evidence) | หลักฐาน (hash) |
| [breach.timeline_events](../data/breach.md#breach-timeline-events) | ลำดับเหตุการณ์และการตัดสินใจ |
| [breach.root_causes](../data/breach.md#breach-root-causes) | สาเหตุและมาตรการป้องกันซ้ำ |
| [breach.routing_rules](../data/breach.md#breach-routing-rules) | กฎผู้รับแจ้งและทีมตอบสนอง |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `breach.incident.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `breach.incident` | เหตุละเมิด | DPO `CRUDAX` · PRIVACY `CRUX` · LEGAL `RU` · OWNER `C` · IT `CRU` · MKT `C` · FRONT `C` · SEC `CRUX` · AUDIT `R` · EXEC `R` · EMP `C` · GUEST `C` | ผู้แจ้งเห็นเฉพาะเหตุที่ตนแจ้ง; GUEST คือผู้ประมวลผลที่แจ้งเหตุ |
| `breach.notification` | แบบแจ้ง สคส. / แจ้งเจ้าของข้อมูล | DPO `CRUAP` · PRIVACY `CRU` · LEGAL `RU` · SEC `R` · AUDIT `R` · EXEC `R` | ส่งแบบแจ้งต้องให้ DPO อนุมัติ |
| `pii.unmask` | เปิดดูข้อมูลส่วนบุคคลแบบไม่ปกปิด | DPO `X` · PRIVACY `X` · SEC `X` | ต้องระบุเหตุผล ถูกบันทึก log ทุกครั้ง และอาจต้องยืนยัน MFA ซ้ำ |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `breach.reported` | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.assessed` | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.pdpc_notified` | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.subjects_notified` | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.closed` | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `breach.sla_timer` | T+24 / 48 / 66 ชม. | เตือนก่อนครบ 72 ชม. นับจาก aware_at | BP-07 / SEQ-06 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** BRE-01, BRE-02, BRE-03, BRE-04, BRE-05, BRE-06, BRE-07, BRE-08, BRE-09, BRE-10, BRE-12, BRE-13
- **P3:** BRE-11, BRE-14, BRE-15
- **P4:** BRE-16, BRE-17

## รายละเอียด feature

<a id="bre-01"></a>
### BRE-01 แบบฟอร์มแจ้งเหตุ

*Incident intake form*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับแจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4)
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), PUBLIC (บุคคลภายนอกที่พบเหตุ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06, PLT-17
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** แบบฟอร์มแจ้งเหตุสำหรับพนักงานและบุคคลภายนอก (ลิงก์/QR) เลือกภาษาได้

**Backend (Go):** ฟอร์มแจ้งเหตุ (PLT-06) สำหรับพนักงาน (login) และบุคคลภายนอก (ลิงก์ / QR + CAPTCHA), TH/EN, สร้าง incident และเริ่มนับเวลา

**Frontend (Next.js):** ฟอร์มใน portal และใน admin

**Acceptance criteria:** แจ้งเหตุแล้วทีมที่กำหนดได้รับแจ้งเตือนทันที

<a id="bre-02"></a>
### BRE-02 ทะเบียนเหตุละเมิด

*Breach register*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับแจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4); ประกาศแจ้งเหตุละเมิด พ.ศ. 2565
- **Actor:** SEC (ทีม Security / Incident), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** บันทึกเหตุ ประเภท (ความลับ / ความถูกต้อง / ความพร้อมใช้) ข้อมูลและจำนวนเจ้าของข้อมูลที่ได้รับผลกระทบ และสถานะ

**Backend (Go):** incident: ประเภท C/I/A, ข้อมูลที่เกี่ยวข้อง (ผูก RoPA / asset), จำนวนเจ้าของข้อมูล, สถานะ; workflow รับเรื่อง → ประเมิน → แจ้ง → ปิด

**Frontend (Next.js):** หน้าทะเบียนเหตุ + หน้ารายละเอียด

**Acceptance criteria:** ทุกเหตุมีสถานะและผู้รับผิดชอบชัดเจน

**Implementation (BRE-02):** `backend/internal/breach` (`service`, `store`, `http`) — เหตุ = เลขที่ `BR-ปีค.ศ.-NNNN` ต่อ tenant (advisory lock), นิติบุคคล, ช่องทาง (`employee_form` / `email` / `phone` / `system` ใน admin), หัวข้อ, รายละเอียด, ลักษณะ C/I/A, เวลาเกิด / ทราบ / ควบคุมได้, จำนวนเจ้าของข้อมูล, หมวดข้อมูล (ORG-07), **ผู้รับผิดชอบ** (`owner_user_id`, migration 00036 — ค่าเริ่มต้นคือผู้บันทึก) · สถานะตาม ST-03 เท่านั้น (`Allowed` = allow-list จาก state-machines.yaml): รับเรื่อง (reported → triage, มอบผู้รับผิดชอบได้), ยืนยันเหตุ (→ assessing), ไม่ใช่เหตุ (triage → closed) / ปิดเหตุ (remediating → closed) ต้องมี `breach.incident.approve` และเหตุผล (`close_reason`), พบข้อเท็จจริงใหม่ (remediating → assessing, เหตุผล) · assessing → notifying / remediating ผ่านการตัดสิน (BRE-06) เท่านั้น · notifying → remediating ต้องมีการแจ้ง สคส. ที่บันทึกแล้ว (BRE-09) — ตอนนี้ตอบ 409 `breach.pdpc_notice_missing` (ดู decisions Q-23) · ทุกการเปลี่ยนสถานะเขียน timeline + audit ใน tx เดียว, event `breach.reported` / `breach.closed` · สิทธิ์เห็นเหตุ: `breach.incident.read` เห็นทั้งหมด, ผู้มีแค่ `create` เห็นเฉพาะที่ตนแจ้ง · API `/admin/v1/breach/incidents` (+ `/{id}`, `/transitions`) · หน้าจอ `/incidents` (ทะเบียน + ค้นหา + ตัวนับถอยหลัง) และ `/incidents/{id}`

<a id="bre-03"></a>
### BRE-03 รับแจ้งเหตุจากผู้ประมวลผล

*Processor breach notification*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับแจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40(2)
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest))
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-04, BRE-01
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** ผู้ประมวลผลแจ้งเหตุต่อผู้ควบคุมข้อมูลผ่านช่องทางที่กำหนด และเริ่มนับเวลาในระบบ

**Backend (Go):** ลิงก์แจ้งเหตุเฉพาะผู้ประมวลผล (guest) ผูกคู่ค้า, เวลาที่ผู้ประมวลผลทราบเหตุ, เริ่มนับเวลา

**Frontend (Next.js):** หน้าแจ้งเหตุของผู้ประมวลผลใน portal

**Acceptance criteria:** เหตุจากผู้ประมวลผลผูกกับคู่ค้าและเริ่มนับเวลาอัตโนมัติ

<a id="bre-04"></a>
### BRE-04 กำหนดผู้รับแจ้งและทีมตอบสนอง

*Response team routing*

- **Priority / Phase:** Must · P1 · กลุ่ม: รับแจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-12
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** ตั้งค่าผู้รับแจ้งเตือนตามประเภทและระดับเหตุ

**Backend (Go):** rule ผู้รับแจ้งตามประเภท / ระดับ / บริษัท → มอบหมายอัตโนมัติ + แจ้งเตือนหลายช่องทาง (LINE / SMS สำหรับเหตุเร่งด่วน)

**Frontend (Next.js):** หน้าตั้งค่าทีมตอบสนอง

**Acceptance criteria:** เหตุระดับสูงถูกมอบหมายและแจ้งทีมภายใน 5 นาที

**หมายเหตุ:** OneTrust ทำได้แค่ส่งอีเมล

<a id="bre-05"></a>
### BRE-05 ประเมินความเสี่ยงของเหตุ

*Breach risk assessment*

- **Priority / Phase:** Must · P1 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศแจ้งเหตุละเมิด พ.ศ. 2565
- **Actor:** SEC (ทีม Security / Incident), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** ประเมินตามปัจจัย เช่น ลักษณะและปริมาณข้อมูล จำนวนและสถานะของเจ้าของข้อมูล ความรุนแรงของผลกระทบ และมาตรการที่มีอยู่

**Backend (Go):** แบบประเมินปัจจัยตามประกาศแจ้งเหตุ พ.ศ. 2565 (PLT-06 + คะแนน) → ระดับความเสี่ยง

**Frontend (Next.js):** หน้าประเมินความเสี่ยงของเหตุ

**Acceptance criteria:** ผลประเมินได้ระดับความเสี่ยงพร้อมเหตุผลตามปัจจัย

**Implementation (BRE-05):** แบบประเมินเป็นฟอร์ม PLT-06 ประเภทใหม่ `breach` (สิทธิ์: สร้าง/แก้/เผยแพร่ = `breach.incident.approve` คือ DPO, ตอบ = `breach.incident.update`) ที่ต้องมีระดับคะแนน (bands) เป็น `none` / `low` / `high` เท่านั้น — ตัวคำถาม/ปัจจัยตามประกาศ พ.ศ. 2565 เป็นเนื้อหาที่ DPO สร้างเอง (ระบบไม่ฝังถ้อยคำกฎหมาย, rule 8) · `Assess` (สถานะ assessing) ส่งคำตอบผ่าน `forms.Service.Record` (ตรวจ + คิดคะแนน + เก็บ `form_submissions`) → ระดับความเสี่ยง = band, **เหตุผลตามปัจจัย** = `forms.Contributions` (คำถาม, คำตอบ + ป้ายตัวเลือก, คะแนนที่ได้) เก็บใน `assessments.factors` · ระดับล่าสุดเป็น `incidents.risk_level`, event `breach.assessed` · API `/incidents/{id}/assessments`

<a id="bre-06"></a>
### BRE-06 ตัดสินหน้าที่แจ้งพร้อมเหตุผล

*Notification decision*

- **Priority / Phase:** Must · P1 · กลุ่ม: ประเมิน
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4)
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** BRE-05
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** สรุปว่า ไม่มีความเสี่ยง / ต้องแจ้ง สคส. / ต้องแจ้ง สคส. และเจ้าของข้อมูล (ความเสี่ยงสูง) พร้อมบันทึกเหตุผล

**Backend (Go):** rule: ไม่มีความเสี่ยง / แจ้ง สคส. / แจ้ง สคส. และเจ้าของข้อมูล; บันทึกเหตุผลและผู้ตัดสิน (DPO อนุมัติ)

**Frontend (Next.js):** ขั้นตอนตัดสินหน้าที่แจ้ง

**Acceptance criteria:** ทุกการตัดสินมีเหตุผลและผู้อนุมัติ

**Implementation (BRE-06):** `Decide` (`breach.incident.approve`, If-Match) ต้องมีผลประเมินแล้วและเหตุผล · กฎ ม.37(4): none → ไม่ต้องแจ้ง, low → แจ้ง สคส., high → แจ้ง สคส. และเจ้าของข้อมูล — DPO เลือกแจ้ง **มากกว่า** ที่ระดับกำหนดได้ แต่น้อยกว่าไม่ได้ (422 `breach.decision_too_weak`) · บันทึก `decision`, `decision_reason`, `decided_by` + timeline + audit · ไม่ต้องแจ้ง → remediating (นาฬิกา 72 ชม. หยุด), นอกนั้น → notifying · หน้าจอ: แผงตัดสิน (ตัวเลือกที่อ่อนกว่ากำหนดถูกปิด)

<a id="bre-07"></a>
### BRE-07 นับเวลา 72 ชั่วโมง

*72-hour countdown*

- **Priority / Phase:** Must · P1 · กลุ่ม: แจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4)
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** นับจากเวลาที่ทราบเหตุ แจ้งเตือนและ escalate ก่อนครบกำหนด

**Backend (Go):** timer 72 ชั่วโมงจริงนับจากเวลาที่ทราบเหตุ, แจ้งเตือนที่ 24 / 48 / 66 ชั่วโมง, escalate ถึงผู้บริหาร

**Frontend (Next.js):** ตัวนับถอยหลังในหน้าเหตุ

**Acceptance criteria:** แจ้งเตือนครบทุกจุดเวลาและ escalate เมื่อใกล้ครบ 72 ชั่วโมง

**Implementation (BRE-07):** ฟังก์ชันล้วน (ทดสอบด้วยเวลาที่กำหนดเอง, rule 7) ใน `breach/service/deadlines.go`: `PDPCDue` = ทราบเหตุ + 72 ชม. (ชั่วโมงจริง ไม่ใช่วันทำการ), `LateDeadline` = + 15 วัน (Q-07), `Checkpoints` 24 / 48 / 66 / 72 ชม., `ToSchedule` (จุดที่ยังไม่ถึง + จุดล่าสุดที่ผ่านไปแล้วให้ยิงทันที — บันทึกเหตุช้าก็ยังเตือน/escalate), `ClockAt` (on_track / due_soon ≥ 66 ชม. / overdue / stopped) · สร้างเหตุ = ตั้ง job `breach.sla_timer` ทุกจุด (River ScheduledAt, unique ต่อ args) · ยิงแล้ว: ถ้านาฬิกายังเดิน (reported…notifying และไม่ได้ตัดสินว่าไม่ต้องแจ้ง) → timeline `deadline:N` + แจ้งเตือน in-app + อีเมล (เร่งด่วน) ถึงผู้รับผิดชอบและผู้มี role DPO, **ตั้งแต่ 66 ชม. เพิ่มผู้มี role EXEC**, 72 ชม. = เกินกำหนด (template `breach.deadline_overdue`) · ผู้รับตาม role นี้เป็นค่าเริ่มต้นจนกว่าจะมีกฎผู้รับแจ้ง BRE-04 · แก้เวลาที่ทราบเหตุต้องมี `approve` + เหตุผล (SEQ-06) แล้วตั้งเวลาใหม่ job เก่าเห็น `aware_at` ไม่ตรงจึงไม่ทำอะไร · แจ้งเหตุใหม่ทันทีด้วย template `breach.reported`

<a id="bre-08"></a>
### BRE-08 แจ้งล่าช้าพร้อมเหตุผล

*Late notification*

- **Priority / Phase:** Must · P1 · กลุ่ม: แจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศแจ้งเหตุละเมิด พ.ศ. 2565 + คำชี้แจง สคส.
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** BRE-07
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** บันทึกเหตุผลความล่าช้า และแจ้งภายในกรอบที่ สคส. กำหนด (ไม่เกิน 15 วัน)

**Backend (Go):** เกิน 72 ชั่วโมง: บังคับบันทึกเหตุผลความล่าช้า, นับกรอบ 15 วัน, แนบในแบบแจ้ง

**Frontend (Next.js):** ส่วนเหตุผลความล่าช้า

**Acceptance criteria:** แบบแจ้งที่ล่าช้าส่งไม่ได้ถ้าไม่มีเหตุผล

<a id="bre-09"></a>
### BRE-09 แบบแจ้ง สคส. และแจ้งเพิ่มเติมเป็นระยะ

*PDPC notification & phased reporting*

- **Priority / Phase:** Must · P1 · กลุ่ม: แจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศแจ้งเหตุละเมิด พ.ศ. 2565
- **Actor:** DPO (DPO / Privacy Team), PDPC (สคส.)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** แบบแจ้งครบหัวข้อตามประกาศ แจ้งข้อมูลเบื้องต้นก่อน แล้วส่งรายละเอียดเพิ่มเมื่อสอบสวนเสร็จ

**Backend (Go):** แบบแจ้ง สคส. ครบหัวข้อตามประกาศ (document composer), ฉบับเบื้องต้น + ฉบับเพิ่มเติมเป็นระยะ, เก็บหลักฐานการยื่น (เลขรับ / วันที่)

**Frontend (Next.js):** หน้าร่างแบบแจ้ง + ประวัติการแจ้งแต่ละรอบ

**Acceptance criteria:** แบบแจ้งมีหัวข้อครบตามประกาศและเก็บหลักฐานการยื่นทุกรอบ

**หมายเหตุ:** ระบบเตรียมเอกสาร ผู้ใช้ยื่นผ่านช่องทางของ สคส.

<a id="bre-10"></a>
### BRE-10 แจ้งเจ้าของข้อมูลเมื่อความเสี่ยงสูง

*Data subject notification*

- **Priority / Phase:** Must · P1 · กลุ่ม: แจ้ง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4)
- **Actor:** DPO (DPO / Privacy Team), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-04
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** template TH/EN ส่งหลายช่องทาง และบันทึกผลการส่ง

**Backend (Go):** template TH/EN, ส่งอีเมล / SMS / LINE แบบทยอยจำนวนมาก, สถานะการส่งรายคน, export รายชื่อ

**Frontend (Next.js):** หน้าส่งแจ้งเจ้าของข้อมูล + สถานะ

**Acceptance criteria:** ส่งแจ้ง 10,000 รายได้และติดตามผลการส่งรายคน

**Implementation (BRE-10):** ใช้ได้เมื่อการตัดสินเป็น `notify_pdpc_and_subjects` · ร่าง (`breach.notification.create`): ช่องทางอีเมล / SMS + ตัวแปรของเหตุ (องค์กร, สรุป, แนวทางเยียวยา, ติดต่อ — `subject_notifications.variables`) ที่เติมลง template `breach.subject_notice` ซึ่ง seed เป็น **ร่างที่ติดป้ายรอฝ่ายกฎหมายอนุมัติ** (rule 8) ให้ tenant แก้ที่หน้าแม่แบบการแจ้งเตือน · รายชื่อ = CSV ที่ผู้ใช้อัปโหลด (ไฟล์ PLT-09 สะอาด) คอลัมน์ email/phone + language — normalize, ตัดซ้ำ, บรรทัดผิดปฏิเสธทั้งไฟล์ (แจ้งเลขบรรทัด+รหัส ไม่แสดงค่า), ที่อยู่เข้ารหัส PLT-13 (`address_enc`), สูงสุด 100,000 · **maker-checker:** ผู้จัดทำอนุมัติเองไม่ได้ (403 `breach.self_approval`), ผู้อนุมัติต้องมี `breach.notification.approve` · job `breach.subject_notice` ส่งต่อให้ PLT-04 ครั้งละ 1,000 รายใน tx ของ job (ล้มกลางทางไม่ส่งซ้ำ) แล้วต่อ job ถัดไป · ติดตามรายคน: ผู้รับเก็บ `notification_id` → สถานะการส่งจริง (queued / sent / failed, จำนวนครั้ง) จาก `notify.Service.Statuses` · จบแล้ว event `breach.subjects_notified` · ทดสอบ 10,000 ราย ~12–17 วินาที · ยังไม่ทำ: LINE, จดหมาย, ประกาศบนเว็บไซต์

**หมายเหตุ:** OneTrust ต้องใช้เครื่องมือส่งภายนอก

<a id="bre-12"></a>
### BRE-12 หลักฐานและลำดับเหตุการณ์

*Evidence & timeline*

- **Priority / Phase:** Must · P1 · กลุ่ม: ตอบสนอง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(4)
- **Actor:** SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-07, PLT-12
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** เก็บหลักฐาน การตัดสินใจ และลำดับเหตุการณ์สำหรับตรวจสอบภายหลัง

**Backend (Go):** ไฟล์หลักฐาน (hash), timeline อัตโนมัติ + บันทึกเอง, การตัดสินใจทุกจุด

**Frontend (Next.js):** แท็บหลักฐานและ timeline

**Acceptance criteria:** timeline แสดงทุกการตัดสินใจพร้อมเวลาและผู้ตัดสิน

**Implementation (BRE-12):** `breach.timeline_events` เพิ่มได้อย่างเดียว (app role ไม่มีสิทธิ์ UPDATE/DELETE, rule 4) · รายการอัตโนมัติเก็บเป็น token คงที่ + ข้อความที่คนให้ (เหตุผล) เช่น `status:triage:assessing`, `assessment:high:8`, `decision:notify_pdpc_and_subjects:high`, `deadline:66`, `evidence:<sha256>`, `notice_sent:email:25:0` — หน้าจอแปลเป็นไทย/อังกฤษ (rule 12) · บันทึกของคนเพิ่มได้ (เวลาย้อนหลังได้, ไม่ใช่อนาคต) · หลักฐาน: ไฟล์ที่ผู้ใช้อัปโหลดและผ่านการตรวจไวรัส ผูกกับเหตุ (ดาวน์โหลดด้วย `breach.incident.read`) + SHA-256 บน timeline · ปิดเหตุแล้วเพิ่มไม่ได้

<a id="bre-13"></a>
### BRE-13 ประวัติและสืบค้นเหตุ

*History & search*

- **Priority / Phase:** Must · P1 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** BRE-02
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** ประวัติการแก้ไขและสถานะของแต่ละเหตุ ค้นหาย้อนหลังได้

**Backend (Go):** ค้นหาย้อนหลังตามช่วงเวลา / ประเภท / สถานะ

**Frontend (Next.js):** ตัวกรองในทะเบียนเหตุ

**Acceptance criteria:** ค้นหาเหตุย้อนหลังได้ครบ

**Implementation (BRE-13):** `GET /admin/v1/breach/incidents` ค้นหาตามสถานะ / ความเสี่ยง / ผู้รับผิดชอบ / ช่วงเวลาที่ทราบเหตุ / เฉพาะที่ยังไม่ปิด / คำค้น (เลขเหตุ หัวข้อ รายละเอียด — escape `%` `_`) เรียงเวลาทราบเหตุใหม่สุด cursor (aware_at, id) · ประวัติ = timeline + audit log

<a id="bre-11"></a>
### BRE-11 แผนตอบสนองและงานควบคุมเหตุ

*Response playbook & tasks*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตอบสนอง
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** playbook ตามประเภทเหตุ มอบหมายงานควบคุม แก้ไข และกู้คืน

**Backend (Go):** playbook ต่อประเภทเหตุ (ransomware, ส่งอีเมลผิดคน, ข้อมูลรั่วจากคู่ค้า) → สร้าง task อัตโนมัติ

**Frontend (Next.js):** หน้าตั้งค่า playbook + checklist ในหน้าเหตุ

**Acceptance criteria:** เลือกประเภทเหตุแล้วได้ชุดงานตาม playbook

<a id="bre-14"></a>
### BRE-14 รายงานและสถิติเหตุละเมิด

*Breach reports*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), EXEC (ผู้บริหาร / ผู้มีอำนาจอนุมัติ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** สรุปจำนวน ประเภท ระยะเวลาตอบสนอง ส่งออก CSV/Excel/PDF

**Backend (Go):** สถิติจำนวน ประเภท เวลาตอบสนอง + export

**Frontend (Next.js):** dashboard เหตุละเมิด

**Acceptance criteria:** รายงานแสดงเวลาตั้งแต่ทราบเหตุถึงแจ้ง สคส. ของทุกเหตุ

<a id="bre-15"></a>
### BRE-15 วิเคราะห์สาเหตุและบทเรียน

*Root cause & lessons learned*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** BRE-02
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** บันทึกสาเหตุ มาตรการป้องกันซ้ำ และติดตามการแก้ไข

**Backend (Go):** บันทึกสาเหตุ (หมวดสาเหตุ), มาตรการป้องกันซ้ำ → สร้าง task / risk

**Frontend (Next.js):** แท็บวิเคราะห์สาเหตุ

**Acceptance criteria:** มาตรการป้องกันซ้ำถูกติดตามจนเสร็จ

<a id="bre-16"></a>
### BRE-16 ซ้อมรับมือเหตุละเมิด

*Breach drill*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ซ้อม
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SEC (ทีม Security / Incident), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** BRE-02
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** สร้างเหตุแบบซ้อมที่ไม่ปนกับเหตุจริง และบันทึกบทเรียน

**Backend (Go):** โหมดซ้อม: flag แยก, ไม่ส่งแจ้งจริง, ไม่นับในสถิติ, บันทึกบทเรียน

**Frontend (Next.js):** ปุ่มสร้างเหตุซ้อม + ป้ายแยก

**Acceptance criteria:** เหตุซ้อมไม่ปนกับเหตุจริงในรายงาน

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="bre-17"></a>
### BRE-17 AI ช่วยประเมินเหตุและร่างรายงาน

*AI breach assistant*

- **Priority / Phase:** Nice · P4 · กลุ่ม: AI
- **ที่มา:** Function List: 09_Breach
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** DPO (DPO / Privacy Team), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-22
- **Process:** [BP-07](../processes/BP-07.md)

**คำอธิบาย:** ช่วยประเมินขอบเขต เขตอำนาจ และร่างรายงานแจ้งเหตุ

**Backend (Go):** AI สรุปขอบเขตเหตุ ประเมินหน้าที่แจ้ง และร่างแบบแจ้ง / หนังสือแจ้ง

**Frontend (Next.js):** แผง AI ในหน้าเหตุ + ปุ่มยืนยัน

**Acceptance criteria:** ร่างจาก AI ต้องมีคนยืนยันก่อนใช้
