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
