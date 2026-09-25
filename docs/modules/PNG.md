# PNG — ประกาศความเป็นส่วนตัว (Privacy Notice Generator)

> ระบบจัดการประกาศความเป็นส่วนตัวแบบอัตโนมัติ (Privacy Notice Generator) · ขอบเขต: สร้าง เผยแพร่ และติดตามประกาศความเป็นส่วนตัวและนโยบายที่เกี่ยวข้อง  
> 16 features · Must 7 / Should 8 / Nice 1 · phase: P1 (9), P3 (6), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/notice` |
| PostgreSQL schema | [`notice`](../data/notice.md) (8 ตาราง) |
| Admin API prefix | `/admin/v1/notices` |
| Endpoint ที่ SA กำหนดแล้ว | `GET /public/v1/notices/{slug}` — ประกาศแบบ hosted / embed (BP-04)<br>`POST /public/v1/notices/{id}/acknowledgements` — บันทึกการรับทราบประกาศ (BP-04) |
| หน้าจอ (Next.js) | admin: /notices/*; portal: /notice/[slug] |
| พึ่งพาบริการ | docs, workflow, events |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-05 PNG, BP-04, DFD-1, ERD-07, SEQ-07, ST-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| WEB | เว็บไซต์ / แอปขององค์กร (SDK) | Website / App with SDK | Public key ของ collection point + CORS allowlist |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [PNG-01](#png-01) | สร้างประกาศแบบถาม-ตอบ | Must | P1 | LEGAL | M | L | Y | BP-04 |
| [PNG-02](#png-02) | ตรวจเนื้อหาครบตาม ม.23 | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-03](#png-03) | แม่แบบตามกลุ่มเจ้าของข้อมูล | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-04](#png-04) | ประกาศกรณีเก็บจากแหล่งอื่น | Must | P1 | OWNER SCHED DS | M | S | N | BP-04 |
| [PNG-05](#png-05) | ประกาศสองภาษา | Must | P1 | LEGAL | S | S | N | BP-04 |
| [PNG-06](#png-06) | จัดการเวอร์ชัน | Must | P1 | LEGAL DPO | S | S | N | BP-04 |
| [PNG-07](#png-07) | แจ้งการเปลี่ยนแปลงและขอความยินยอมใหม่ | Must | P1 | DPO DS | M | S | N | BP-04 |
| [PNG-08](#png-08) | เผยแพร่และฝังในระบบ | Should | P1 | WEB | S | M | Y | BP-04 |
| [PNG-14](#png-14) | อนุมัติก่อนเผยแพร่ | Should | P1 | DPO | XS | S | N | BP-04 |
| [PNG-09](#png-09) | บันทึกการรับทราบ | Should | P3 | DS | S | S | N | BP-04 |
| [PNG-10](#png-10) | ดึงเนื้อหาจาก RoPA | Should | P3 | LEGAL | M | S | N | BP-04 |
| [PNG-11](#png-11) | แม่แบบตามอุตสาหกรรม | Should | P3 | LEGAL | XS | S | N | BP-04 |
| [PNG-12](#png-12) | ประกาศแบบหลายชั้นและป้าย CCTV | Should | P3 | LEGAL | S | M | Y | BP-04 |
| [PNG-15](#png-15) | แจ้งเตือนทบทวนตามรอบ | Should | P3 | SCHED DPO | S | XS | N | BP-04 |
| [PNG-16](#png-16) | สร้างนโยบายความเป็นส่วนตัวและนโยบายคุกกี้ | Should | P3 | LEGAL | M | S | N | BP-04 |
| [PNG-13](#png-13) | ลิงก์เอกสารอ้างอิงในประกาศ | Nice | P4 | LEGAL | XS | S | N | BP-04 |

### ความสัมพันธ์ระหว่าง use case

- PNG-01 «include» PNG-02 (ทุกครั้งที่ทำ PNG-01 ต้องทำ PNG-02)
- PNG-01 «include» PNG-05 (ทุกครั้งที่ทำ PNG-01 ต้องทำ PNG-05)
- PNG-10 «extend» PNG-01 (PNG-10 เป็นทางเลือก/ส่วนขยายของ PNG-01)
- PNG-03 «extend» PNG-01 (PNG-03 เป็นทางเลือก/ส่วนขยายของ PNG-01)
- PNG-08 «include» PNG-14 (ทุกครั้งที่ทำ PNG-08 ต้องทำ PNG-14)

## กระบวนการ / sequence / state machine

- [BP-04 จัดทำและเผยแพร่ประกาศความเป็นส่วนตัว (Privacy notice lifecycle)](../processes/BP-04.md)
- [SEQ-07 สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg)](../sequences/SEQ-07.md)
- [ST-04 ข้อตกลง DPA / DSA และประกาศความเป็นส่วนตัว](../states/ST-04.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [notice.notices](../data/notice.md#notice-notices) | ประกาศความเป็นส่วนตัว / นโยบาย / ป้าย CCTV |
| [notice.notice_versions](../data/notice.md#notice-notice-versions) | เวอร์ชันที่เผยแพร่ + ผล checklist ม.23 |
| [notice.notice_activity_links](../data/notice.md#notice-notice-activity-links) | กิจกรรม RoPA ที่ประกาศครอบคลุม |
| [notice.acknowledgements](../data/notice.md#notice-acknowledgements) | การรับทราบประกาศ |
| [notice.indirect_collections](../data/notice.md#notice-indirect-collections) | การได้ข้อมูลจากแหล่งอื่น ต้องแจ้งภายใน 30 วัน (ม.25) |
| [notice.embeds](../data/notice.md#notice-embeds) | โค้ดฝังและลิงก์ของประกาศ |
| [notice.linked_documents](../data/notice.md#notice-linked-documents) | เอกสารอ้างอิงที่ลิงก์ในประกาศ |
| [notice.wizard_templates](../data/notice.md#notice-wizard-templates) | template wizard ตามกลุ่มเจ้าของข้อมูล / อุตสาหกรรม |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `notice.document.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `notice.document` | ประกาศความเป็นส่วนตัว | DPO `CRUDAP` · PRIVACY `CRU` · LEGAL `CRUA` · OWNER `R` · IT `R` · MKT `R` · AUDIT `R` · EMP `R` · API `R` | ผู้สร้างกับผู้อนุมัติต้องต่างคน |
| `notice.template` | Template ประกาศ | DPO `CRUD` · PRIVACY `R` · LEGAL `CRUD` · AUDIT `R` |  |
| `notice.indirect` | แจ้งกรณีได้ข้อมูลจากแหล่งอื่น (ม.25) | DPO `CRUD` · PRIVACY `CRU` · OWNER `CRU` · MKT `CRU` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `notice.published` | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |
| `notice.material_change` | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `notice.indirect_due` | รายวัน | แจ้งเตือนก่อนครบ 30 วันของการแจ้งตาม ม.25 | BP-04 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** PNG-01, PNG-02, PNG-03, PNG-04, PNG-05, PNG-06, PNG-07, PNG-08, PNG-14
- **P3:** PNG-09, PNG-10, PNG-11, PNG-12, PNG-15, PNG-16
- **P4:** PNG-13

## รายละเอียด feature

<a id="png-01"></a>
### PNG-01 สร้างประกาศแบบถาม-ตอบ

*Wizard-based generator*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-16, ORG-07
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ตอบคำถามทีละขั้น หรือเลือกตามกลุ่มเจ้าของข้อมูล / กลุ่มวัตถุประสงค์ / หน่วยงาน แล้วระบบสร้างประกาศ

**Backend (Go):** wizard definition (คำถาม → ส่วนของประกาศ), ประกอบเนื้อหาจากคำตอบ + master data + ข้อมูลองค์กรด้วย document composer

**Frontend (Next.js):** wizard ทีละขั้น + preview คู่ขนาน + แก้ข้อความใน editor

**Acceptance criteria:** ผู้ใช้ที่ไม่มีพื้นฐานกฎหมายสร้างร่างประกาศครบหัวข้อได้ภายใน 30 นาที

**หมายเหตุ:** OneTrust ไม่มี wizard (จุดต่าง)

<a id="png-02"></a>
### PNG-02 ตรวจเนื้อหาครบตาม ม.23

*Mandatory content checklist*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23 (1)-(6)
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ตรวจ 6 หัวข้อ: วัตถุประสงค์และฐานกฎหมาย, กรณีต้องให้ข้อมูลและผลของการไม่ให้, ข้อมูลที่เก็บและระยะเวลา, ผู้รับข้อมูล, ข้อมูลติดต่อผู้ควบคุม/ตัวแทน/DPO, สิทธิของเจ้าของข้อมูล

**Backend (Go):** rule ตรวจ 6 หัวข้อ ม.23 ก่อน publish, บล็อก publish ถ้าขาดหัวข้อบังคับ (ตั้งค่าได้)

**Frontend (Next.js):** แผงตรวจความครบ + ลิงก์ไปส่วนที่ขาด

**Acceptance criteria:** ประกาศที่ขาดหัวข้อบังคับ publish ไม่ได้

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="png-03"></a>
### PNG-03 แม่แบบตามกลุ่มเจ้าของข้อมูล

*Templates by data subject*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01, T15
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ลูกค้า พนักงาน ผู้สมัครงาน คู่ค้า ผู้มาติดต่อ CCTV ผู้ถือหุ้น สมาชิก

**Backend (Go):** template ตั้งต้น 8 กลุ่มเจ้าของข้อมูล TH/EN (เนื้อหาจาก T15)

**Frontend (Next.js):** หน้าเลือก template ตามกลุ่ม

**Acceptance criteria:** เลือก template แล้วได้ร่างประกาศของกลุ่มนั้นทันที

<a id="png-04"></a>
### PNG-04 ประกาศกรณีเก็บจากแหล่งอื่น

*Indirect collection notice*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.25
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), SCHED (ระบบ: Scheduler / Event), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-05, PLT-04
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แจ้งเจ้าของข้อมูลภายใน 30 วันเมื่อได้ข้อมูลจากแหล่งอื่น พร้อมนับเวลาและแจ้งเตือน

**Backend (Go):** บันทึกการได้ข้อมูลจากแหล่งอื่น (แหล่ง วันที่ จำนวน), นับ 30 วัน, แจ้งเตือน, ส่งประกาศทางอีเมล / SMS หรือบันทึกวิธีแจ้ง + หลักฐาน

**Frontend (Next.js):** หน้ารายการที่ต้องแจ้ง + ตัวนับวัน

**Acceptance criteria:** ระบบเตือนก่อนครบ 30 วัน และปิดรายการได้เมื่อมีหลักฐานการแจ้ง

**หมายเหตุ:** OneTrust ไม่มี (จุดต่าง)

<a id="png-05"></a>
### PNG-05 ประกาศสองภาษา

*TH/EN notices*

- **Priority / Phase:** Must · P1 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-03
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ภาษาไทย-อังกฤษคู่ขนาน และรองรับภาษาอื่นสำหรับแรงงานต่างชาติ

**Backend (Go):** เนื้อหาคู่ขนานหลายภาษาใน composer, ตรวจว่าทุกภาษาเผยแพร่เวอร์ชันเดียวกัน, เพิ่มภาษาแรงงานต่างชาติ

**Frontend (Next.js):** สลับภาษาใน editor และ preview

**Acceptance criteria:** publish ไม่ได้ถ้าฉบับแปลยังไม่อัปเดตตามเวอร์ชันล่าสุด (ตั้งค่าได้)

<a id="png-06"></a>
### PNG-06 จัดการเวอร์ชัน

*Versioning*

- **Priority / Phase:** Must · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** เก็บทุกเวอร์ชัน วันมีผล และเปรียบเทียบการเปลี่ยนแปลง

**Backend (Go):** เวอร์ชัน + วันมีผล + diff (PLT-08 / PLT-16)

**Frontend (Next.js):** หน้าประวัติเวอร์ชันและเปรียบเทียบ

**Acceptance criteria:** หน้า public แสดงเวอร์ชันปัจจุบันและดูประวัติย้อนหลังได้

<a id="png-07"></a>
### PNG-07 แจ้งการเปลี่ยนแปลงและขอความยินยอมใหม่

*Change notification*

- **Priority / Phase:** Must · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.21
- **Actor:** DPO (DPO / Privacy Team), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-06, CON-12
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แจ้งเจ้าของข้อมูลเมื่อประกาศเปลี่ยน และขอความยินยอมใหม่หากเปลี่ยนวัตถุประสงค์

**Backend (Go):** เมื่อ publish เวอร์ชันใหม่: ระบุว่าเปลี่ยนสาระสำคัญหรือไม่, ส่งแจ้งเจ้าของข้อมูล, ถ้าเปลี่ยนวัตถุประสงค์ → สร้างคำขอความยินยอมใหม่ใน CON

**Frontend (Next.js):** ขั้นตอนยืนยันการแจ้งเปลี่ยนแปลงตอน publish

**Acceptance criteria:** การเปลี่ยนวัตถุประสงค์สร้างงานขอความยินยอมใหม่อัตโนมัติ

<a id="png-08"></a>
### PNG-08 เผยแพร่และฝังในระบบ

*Hosting & embed*

- **Priority / Phase:** Should · P1 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** WEB (เว็บไซต์ / แอปขององค์กร (SDK))
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-17
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** หน้าเว็บประกาศ ลิงก์ script/iframe ส่งออก Word/PDF และใช้ในแอปหรือ LINE OA

**Backend (Go):** URL ถาวรและต่อเวอร์ชัน, embed script / iframe, ส่งออก Word / PDF, ลิงก์สำหรับ LINE OA และแอป

**Frontend (Next.js):** หน้า public ของประกาศใน portal + ตัวสร้างโค้ดฝัง

**Acceptance criteria:** ประกาศแสดงบนเว็บลูกค้าผ่าน embed และอัปเดตเองเมื่อ publish

**หมายเหตุ:** ดึงเข้า P1 เพราะต้องมีช่องทางเผยแพร่

<a id="png-14"></a>
### PNG-14 อนุมัติก่อนเผยแพร่

*Review & approval*

- **Priority / Phase:** Should · P1 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ส่งตรวจ แสดงความเห็น และอนุมัติโดย DPO/ฝ่ายกฎหมาย

**Backend (Go):** ใช้ workflow อนุมัติกลาง: DPO / กฎหมายตรวจ แสดงความเห็น อนุมัติก่อน publish

**Frontend (Next.js):** ปุ่มส่งตรวจ + กล่องความเห็น

**Acceptance criteria:** ประกาศเผยแพร่ได้หลังอนุมัติเท่านั้น

**หมายเหตุ:** ดึงเข้า P1 (ใช้ PLT-08 จึงใช้แรงน้อย)

<a id="png-09"></a>
### PNG-09 บันทึกการรับทราบ

*Acknowledgement log*

- **Priority / Phase:** Should · P3 · กลุ่ม: เผยแพร่
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-17
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** บันทึกว่าใครรับทราบประกาศเวอร์ชันใด ผ่านช่องทางใด

**Backend (Go):** บันทึกการรับทราบ (ใคร เวอร์ชัน ช่องทาง เวลา) ผ่าน API / SDK / หน้า public

**Frontend (Next.js):** รายงานการรับทราบต่อเวอร์ชัน

**Acceptance criteria:** ตรวจได้ว่าบุคคลรับทราบประกาศเวอร์ชันใด

<a id="png-10"></a>
### PNG-10 ดึงเนื้อหาจาก RoPA

*Auto-populate from RoPA*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23, ม.39
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ROPA-03, PLT-11
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ประกอบเนื้อหาจากข้อมูลกิจกรรม และแจ้งเมื่อ RoPA เปลี่ยน

**Backend (Go):** เลือกกิจกรรม RoPA → ประกอบหัวข้อวัตถุประสงค์ / ฐาน / ข้อมูล / ระยะเวลา / ผู้รับ; event ropa.updated → แจ้งให้ทบทวนประกาศ

**Frontend (Next.js):** ปุ่มดึงข้อมูลจาก RoPA ใน wizard

**Acceptance criteria:** RoPA เปลี่ยนแล้วประกาศที่เกี่ยวข้องถูกทำเครื่องหมายให้ทบทวน

<a id="png-11"></a>
### PNG-11 แม่แบบตามอุตสาหกรรม

*Industry templates*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T40
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ค้าปลีก โรงพยาบาล การเงิน/ประกัน โรงแรม การศึกษา ภาครัฐ

**Backend (Go):** ชุด template 6 อุตสาหกรรม (เนื้อหาจาก T40)

**Frontend (Next.js):** ตัวกรอง template ตามอุตสาหกรรม

**Acceptance criteria:** มี template ครบ 6 อุตสาหกรรมทั้ง TH/EN

<a id="png-12"></a>
### PNG-12 ประกาศแบบหลายชั้นและป้าย CCTV

*Layered notice & CCTV signage*

- **Priority / Phase:** Should · P3 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PNG-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** ฉบับย่อ + ฉบับเต็ม และป้าย CCTV พร้อม QR code

**Backend (Go):** ฉบับย่อ + ฉบับเต็ม, สร้างป้าย CCTV พร้อม QR (PDF A4 / A3)

**Frontend (Next.js):** หน้าออกแบบประกาศหลายชั้นและป้าย CCTV

**Acceptance criteria:** ป้าย CCTV พิมพ์ได้และ QR เปิดฉบับเต็มได้

<a id="png-15"></a>
### PNG-15 แจ้งเตือนทบทวนตามรอบ

*Periodic review reminder*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** SCHED (ระบบ: Scheduler / Event), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-10
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** เตือนให้ทบทวนตามรอบ หรือเมื่อ RoPA เปลี่ยน

**Backend (Go):** รอบทบทวนต่อประกาศ + trigger เมื่อ RoPA เปลี่ยน

**Frontend (Next.js):** ตั้งรอบทบทวนในหน้าประกาศ

**Acceptance criteria:** เจ้าของประกาศได้รับงานทบทวนตามรอบ

<a id="png-16"></a>
### PNG-16 สร้างนโยบายความเป็นส่วนตัวและนโยบายคุกกี้

*Privacy & cookie policy generator*

- **Priority / Phase:** Should · P3 · กลุ่ม: เอกสารอื่น
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-01, CON-05
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** สร้าง Privacy Policy ระดับองค์กรและ Cookie Policy จากข้อมูลในระบบ

**Backend (Go):** template นโยบายระดับองค์กร ประกอบจากข้อมูลองค์กร ประกาศทุกกลุ่ม และตารางคุกกี้

**Frontend (Next.js):** wizard นโยบายองค์กร

**Acceptance criteria:** นโยบายที่สร้างอ้างถึงประกาศและตารางคุกกี้เวอร์ชันล่าสุด

<a id="png-13"></a>
### PNG-13 ลิงก์เอกสารอ้างอิงในประกาศ

*Linked documents*

- **Priority / Phase:** Nice · P4 · กลุ่ม: เนื้อหา
- **ที่มา:** Function List: 13_Notice
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PNG-08
- **Process:** [BP-04](../processes/BP-04.md)

**คำอธิบาย:** แนบลิงก์ตารางระยะเวลาเก็บรักษา นโยบายความปลอดภัย และเอกสารอื่น พร้อมกำหนดการแสดงผล

**Backend (Go):** ลิงก์เอกสารอ้างอิง + รูปแบบการแสดง (inline / popup)

**Frontend (Next.js):** ส่วนจัดการลิงก์ใน editor

**Acceptance criteria:** ลิงก์แสดงตามรูปแบบที่ตั้ง
