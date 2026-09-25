# DPA — ข้อตกลงการประมวลผลข้อมูล (DPA)

> ระบบจัดการเอกสารข้อตกลงการประมวลผลข้อมูลส่วนบุคคล (Data Processing Agreement) · ขอบเขต: ข้อตกลงระหว่างผู้ควบคุมข้อมูลกับผู้ประมวลผลข้อมูลตาม ม.40  
> 14 features · Must 6 / Should 7 / Nice 1 · phase: P2 (6), P3 (7), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/agreement` |
| PostgreSQL schema | [`agreement`](../data/agreement.md) (10 ตาราง) |
| Admin API prefix | `/admin/v1/agreements` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /admin/v1/agreements` — สร้างข้อตกลง DPA / DSA (BP-10)<br>`POST /admin/v1/agreements/{id}/render` — สร้างเอกสาร PDF / DOCX (SEQ-07) |
| หน้าจอ (Next.js) | admin: /agreements/dpa/* |
| พึ่งพาบริการ | docs, workflow, e-signature |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-15 DPA, BP-10, DFD-1, ERD-13, SEQ-07, ST-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| LEGAL | ฝ่ายกฎหมาย | Legal | OIDC SSO + MFA · Admin app |
| PROC | จัดซื้อ / ผู้ดูแลคู่ค้า | Procurement / Vendor Manager | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | Guest link (token หมดอายุ) + OTP |
| ESIGN | ผู้ให้บริการ e-Signature | e-Signature Provider | API key + callback ลงชื่อ HMAC |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DPA-01](#dpa-01) | Template DPA มาตรฐาน | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-02](#dpa-02) | สร้างแบบกรอกเองและแบบอัตโนมัติ | Must | P2 | LEGAL PROC | L | M | Y | BP-10 |
| [DPA-03](#dpa-03) | ข้อกำหนดที่ต้องมี | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-04](#dpa-04) | ภาคผนวกรายละเอียดการประมวลผล | Must | P2 | LEGAL | S | S | N | BP-10 |
| [DPA-10](#dpa-10) | ทะเบียน DPA และแจ้งเตือนหมดอายุ | Must | P2 | LEGAL SCHED | S | M | N | BP-10 |
| [DPA-11](#dpa-11) | ผูก DPA กับคู่ค้าและกิจกรรม | Must | P2 | PROC DPO | S | S | N | BP-10 |
| [DPA-05](#dpa-05) | ข้อสัญญาการโอนต่างประเทศ | Should | P3 | LEGAL DPO | S | XS | N | BP-10 |
| [DPA-06](#dpa-06) | แก้ไขเอกสารในระบบ | Should | P3 | LEGAL | S | M | N | BP-10 |
| [DPA-07](#dpa-07) | ตรวจทานและอนุมัติ | Should | P3 | DPO LEGAL | XS | S | N | BP-10 |
| [DPA-08](#dpa-08) | เวอร์ชันและประวัติการดาวน์โหลด | Should | P3 | LEGAL | S | S | N | BP-10 |
| [DPA-09](#dpa-09) | ส่งออกและลงนามอิเล็กทรอนิกส์ | Should | P3 | VENDOR ESIGN LEGAL | M | S | N | BP-10 |
| [DPA-12](#dpa-12) | แจ้งเตือนผู้ประมวลผลที่ยังไม่มี DPA | Should | P3 | SCHED PROC | S | XS | N | BP-10 |
| [DPA-13](#dpa-13) | ติดตามการคืน/ทำลายข้อมูลเมื่อสิ้นสุด | Should | P3 | VENDOR PROC | S | S | N | BP-10 |
| [DPA-14](#dpa-14) | ตรวจ DPA ที่ได้รับจากลูกค้า | Nice | P4 | LEGAL | S | S | N | BP-10 |

### ความสัมพันธ์ระหว่าง use case

- DPA-02 «include» DPA-01 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-01)
- DPA-02 «include» DPA-03 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-03)
- DPA-02 «include» DPA-04 (ทุกครั้งที่ทำ DPA-02 ต้องทำ DPA-04)
- DPA-05 «extend» DPA-02 (DPA-05 เป็นทางเลือก/ส่วนขยายของ DPA-02)

## กระบวนการ / sequence / state machine

- [BP-10 จัดทำและบริหารข้อตกลง DPA / DSA (Agreement lifecycle)](../processes/BP-10.md)
- [SEQ-07 สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg)](../sequences/SEQ-07.md)
- [ST-04 ข้อตกลง DPA / DSA และประกาศความเป็นส่วนตัว](../states/ST-04.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [agreement.agreements](../data/agreement.md#agreement-agreements) | ข้อตกลง DPA / DSA / ผู้ควบคุมร่วม / DPA ขาเข้า |
| [agreement.parties](../data/agreement.md#agreement-parties) | คู่สัญญาในข้อตกลง (หลายฝ่าย) |
| [agreement.agreement_activities](../data/agreement.md#agreement-agreement-activities) | กิจกรรม RoPA และเส้นทางข้อมูลที่ข้อตกลงครอบคลุม |
| [agreement.clauses](../data/agreement.md#agreement-clauses) | clause ในข้อตกลง (อ้างอิงคลังกลาง + ข้อความที่ปรับ) |
| [agreement.mandatory_rules](../data/agreement.md#agreement-mandatory-rules) | กฎ clause บังคับ (ม.27, ม.28-29, ม.37(2), ม.40) |
| [agreement.annexes](../data/agreement.md#agreement-annexes) | ภาคผนวก (รายละเอียดการประมวลผล รายการข้อมูล มาตรการ แผนผัง) |
| [agreement.signature_requests](../data/agreement.md#agreement-signature-requests) | การส่งลงนามอิเล็กทรอนิกส์ |
| [agreement.obligations](../data/agreement.md#agreement-obligations) | ภาระผูกพันตามข้อตกลง |
| [agreement.return_confirmations](../data/agreement.md#agreement-return-confirmations) | ใบยืนยันการคืน / ทำลายข้อมูลเมื่อสิ้นสุด |
| [agreement.downloads](../data/agreement.md#agreement-downloads) | ประวัติการดาวน์โหลดเอกสารสัญญา |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `agreement.dpa.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `agreement.dpa` | ข้อตกลงการประมวลผล (DPA) | DPO `RA` · PRIVACY `R` · LEGAL `CRUDAP` · OWNER `R` · PROC `CR` · AUDIT `R` · GUEST `R` | GUEST ดูและลงนามฉบับที่ส่งให้ |
| `agreement.clause` | คลังข้อความสัญญา | DPO `R` · LEGAL `CRUDP` · AUDIT `R` |  |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `agreement.signed` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.expiring` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.terminated` | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DPA-01, DPA-02, DPA-03, DPA-04, DPA-10, DPA-11
- **P3:** DPA-05, DPA-06, DPA-07, DPA-08, DPA-09, DPA-12, DPA-13
- **P4:** DPA-14

## รายละเอียด feature

<a id="dpa-01"></a>
### DPA-01 Template DPA มาตรฐาน

*DPA templates*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-16, T34
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** template ข้อตกลงการประมวลผลตาม ม.40 ภาษาไทยและอังกฤษ ทำสำเนาและปรับแก้ได้

**Backend (Go):** template DPA ม.40 TH/EN (เนื้อหา T34) บน document composer, clone / ปรับ

**Frontend (Next.js):** หน้าคลัง template DPA

**Acceptance criteria:** มี template ภาษาไทยและอังกฤษพร้อมใช้

**หมายเหตุ:** OneTrust ไม่มี (จุดต่างหลัก)

<a id="dpa-02"></a>
### DPA-02 สร้างแบบกรอกเองและแบบอัตโนมัติ

*Manual & automatic generation*

- **Priority / Phase:** Must · P2 · กลุ่ม: สร้าง
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DPA-01, VEN-01, ROPA-03
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** สร้างจาก template โดยดึงข้อมูลคู่ค้า กิจกรรม และข้อมูลส่วนบุคคลที่ส่งให้ผู้ประมวลผล

**Backend (Go):** agreement engine (ใช้ร่วม DSA): เลือกคู่ค้า + กิจกรรม → merge ข้อมูลคู่ค้า / กิจกรรม / ข้อมูลที่ส่ง → ร่างสัญญา; โหมดกรอกเอง; สถานะสัญญา

**Frontend (Next.js):** wizard สร้างสัญญา + editor

**Acceptance criteria:** สร้างร่าง DPA จากคู่ค้าและกิจกรรมได้ภายใน 10 นาที

**หมายเหตุ:** สร้าง agreement engine ครั้งเดียว ใช้ร่วม DSA

<a id="dpa-03"></a>
### DPA-03 ข้อกำหนดที่ต้องมี

*Mandatory clauses*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40(1)-(3), ม.28-29
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ประมวลผลตามคำสั่ง มาตรการความปลอดภัย แจ้งเหตุละเมิด จัดทำ RoPA ผู้ประมวลผล ผู้ประมวลผลช่วง ช่วยตอบคำขอใช้สิทธิ คืน/ทำลายข้อมูลเมื่อสิ้นสุด สิทธิตรวจสอบ และการโอนต่างประเทศ

**Backend (Go):** rule ตรวจ clause บังคับ ม.40 ก่อนส่งอนุมัติ

**Frontend (Next.js):** แผงตรวจ clause ที่ขาด

**Acceptance criteria:** สัญญาที่ขาด clause บังคับส่งอนุมัติไม่ได้

<a id="dpa-04"></a>
### DPA-04 ภาคผนวกรายละเอียดการประมวลผล

*Processing schedule*

- **Priority / Phase:** Must · P2 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ประเภทข้อมูล กลุ่มเจ้าของข้อมูล วัตถุประสงค์ ระยะเวลา และมาตรการความปลอดภัย

**Backend (Go):** ภาคผนวกสร้างจาก RoPA (ประเภทข้อมูล กลุ่มเจ้าของ วัตถุประสงค์ ระยะเวลา มาตรการ)

**Frontend (Next.js):** ส่วนภาคผนวกใน editor

**Acceptance criteria:** ภาคผนวกตรงกับข้อมูล RoPA ของกิจกรรมที่เลือก

<a id="dpa-10"></a>
### DPA-10 ทะเบียน DPA และแจ้งเตือนหมดอายุ

*DPA register & expiry alerts*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DPA-02
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** สถานะสัญญา วันเริ่ม/สิ้นสุด และแจ้งเตือนก่อนหมดอายุ

**Backend (Go):** ทะเบียนสัญญา + วันเริ่ม / สิ้นสุด + แจ้งเตือนก่อนหมดอายุ (PLT-05)

**Frontend (Next.js):** หน้าทะเบียน DPA

**Acceptance criteria:** สัญญาใกล้หมดอายุถูกแจ้งเตือนตามเวลาที่ตั้ง

<a id="dpa-11"></a>
### DPA-11 ผูก DPA กับคู่ค้าและกิจกรรม

*Link to vendor & RoPA*

- **Priority / Phase:** Must · P2 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** PROC (จัดซื้อ / ผู้ดูแลคู่ค้า), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** VEN-01
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** แสดง DPA ของแต่ละคู่ค้า ผลประเมิน และกิจกรรมที่ใช้ผู้ประมวลผลนั้น

**Backend (Go):** ความสัมพันธ์สัญญา ↔ คู่ค้า ↔ กิจกรรม

**Frontend (Next.js):** แท็บความเชื่อมโยงในหน้าสัญญา

**Acceptance criteria:** เปิดคู่ค้าแล้วเห็น DPA และกิจกรรมที่เกี่ยวข้อง

<a id="dpa-05"></a>
### DPA-05 ข้อสัญญาการโอนต่างประเทศ

*Cross-border clauses*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศ สคส. ตามมาตรา 29 พ.ศ. 2566
- **Actor:** LEGAL (ฝ่ายกฎหมาย), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPX-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ข้อสัญญามาตรฐานสำหรับการโอนข้อมูลไปต่างประเทศตามประกาศ สคส.

**Backend (Go):** clause มาตรฐานตามประกาศ ม.29 พ.ศ. 2566 แทรกเมื่อมีการโอน

**Frontend (Next.js):** ตัวเลือก clause การโอน

**Acceptance criteria:** สัญญาที่มีการโอนต่างประเทศมี clause ครบ

<a id="dpa-06"></a>
### DPA-06 แก้ไขเอกสารในระบบ

*In-app editor*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อกำหนด
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ปรับข้อความ ฟอนต์ ย่อหน้า สารบัญ และเพิ่มฟิลด์ในเอกสาร

**Backend (Go):** เพิ่มความสามารถ editor: ฟอนต์ ย่อหน้า สารบัญ ฟิลด์ใหม่

**Frontend (Next.js):** เครื่องมือจัดรูปแบบใน editor

**Acceptance criteria:** ปรับรูปแบบแล้วไฟล์ที่ส่งออกตรงกับที่เห็น

<a id="dpa-07"></a>
### DPA-07 ตรวจทานและอนุมัติ

*Review & approval*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งให้ฝ่ายกฎหมายและ DPO ตรวจ และอนุมัติก่อนลงนาม

**Backend (Go):** ใช้ workflow อนุมัติกลาง: กฎหมาย / DPO ตรวจ

**Frontend (Next.js):** ปุ่มส่งตรวจ + ความเห็น

**Acceptance criteria:** ส่งลงนามได้หลังอนุมัติเท่านั้น

<a id="dpa-08"></a>
### DPA-08 เวอร์ชันและประวัติการดาวน์โหลด

*Versions & download history*

- **Priority / Phase:** Should · P3 · กลุ่ม: กำกับ
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** เก็บประวัติเวอร์ชัน ผู้แก้ไข วันที่ และการดาวน์โหลด

**Backend (Go):** เวอร์ชัน + diff + log การดาวน์โหลด

**Frontend (Next.js):** หน้าประวัติเวอร์ชันและการดาวน์โหลด

**Acceptance criteria:** เห็นว่าใครดาวน์โหลดเวอร์ชันใดเมื่อไร

<a id="dpa-09"></a>
### DPA-09 ส่งออกและลงนามอิเล็กทรอนิกส์

*Export & e-signature*

- **Priority / Phase:** Should · P3 · กลุ่ม: ลงนาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** พ.ร.บ.ว่าด้วยธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), ESIGN (ผู้ให้บริการ e-Signature), LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T35
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ส่งออก Word/PDF ดูตัวอย่างก่อนส่ง และส่งลงนามอิเล็กทรอนิกส์

**Backend (Go):** export Word / PDF + preview; เชื่อมบริการลงนามอิเล็กทรอนิกส์ผ่าน API + เก็บไฟล์ที่ลงนาม

**Frontend (Next.js):** ปุ่มส่งลงนาม + สถานะการลงนาม

**Acceptance criteria:** ไฟล์ที่ลงนามแล้วถูกเก็บกลับเข้าระบบอัตโนมัติ

**หมายเหตุ:** เลือกผู้ให้บริการ e-signature ใน T35

<a id="dpa-12"></a>
### DPA-12 แจ้งเตือนผู้ประมวลผลที่ยังไม่มี DPA

*Missing DPA alert*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** SCHED (ระบบ: Scheduler / Event), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** DPA-11
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ตรวจหาคู่ค้าที่เป็นผู้ประมวลผลแต่ยังไม่มีข้อตกลงที่มีผลบังคับ

**Backend (Go):** job ตรวจคู่ค้าที่เป็นผู้ประมวลผลแต่ไม่มี DPA ที่มีผล → แจ้งเตือน / งาน

**Frontend (Next.js):** รายการคู่ค้าที่ขาด DPA

**Acceptance criteria:** คู่ค้าที่ขาด DPA ถูกพบทุกราย

<a id="dpa-13"></a>
### DPA-13 ติดตามการคืน/ทำลายข้อมูลเมื่อสิ้นสุด

*Return / deletion tracking*

- **Priority / Phase:** Should · P3 · กลุ่ม: ติดตาม
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(3), ม.40
- **Actor:** VENDOR (คู่ค้า / ผู้ประมวลผล (guest)), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-04
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** ขอใบยืนยันการทำลายหรือคืนข้อมูลเมื่อสัญญาสิ้นสุด

**Backend (Go):** สัญญาสิ้นสุด → งานขอใบยืนยันการคืน / ทำลาย (guest link ให้คู่ค้าอัปโหลด)

**Frontend (Next.js):** ขั้นตอนปิดสัญญา

**Acceptance criteria:** ปิดสัญญาได้เมื่อได้รับใบยืนยัน

<a id="dpa-14"></a>
### DPA-14 ตรวจ DPA ที่ได้รับจากลูกค้า

*Inbound DPA review*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ผู้ประมวลผล
- **ที่มา:** Function List: 10_DPA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** LEGAL (ฝ่ายกฎหมาย)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-06
- **Process:** [BP-10](../processes/BP-10.md)

**คำอธิบาย:** checklist สำหรับองค์กรที่เป็นผู้ประมวลผล ใช้ตรวจ DPA ที่ลูกค้าส่งมา

**Backend (Go):** checklist ตรวจ DPA ที่ลูกค้าส่งมา (องค์กรเป็นผู้ประมวลผล)

**Frontend (Next.js):** หน้าตรวจ DPA ขาเข้า

**Acceptance criteria:** ผลตรวจระบุ clause ที่ขาดหรือเสี่ยง
