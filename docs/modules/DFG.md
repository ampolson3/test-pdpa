# DFG — แผนผังการไหลของข้อมูล (Data Flow Generator)

> ระบบจัดการแผนผังการไหลของการประมวลผลข้อมูลส่วนบุคคลแบบอัตโนมัติ (Data Flow Generator) · ขอบเขต: แผนผังการไหลของข้อมูลที่สร้างจาก RoPA และการค้นหาข้อมูลอัตโนมัติ  
> 11 features · Must 5 / Should 4 / Nice 2 · phase: P2 (5), P3 (4), P4 (2)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/dataflow` |
| PostgreSQL schema | [`dataflow`](../data/dataflow.md) (5 ตาราง) |
| Admin API prefix | `/admin/v1/dataflow` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /data-flow/* (React Flow) |
| พึ่งพาบริการ | events, connectors |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-08 DFG, DFD-1, ERD-09 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| DATASRC | แหล่งข้อมูล (DB / File / Cloud) | Data Sources | Connector credential ใน OpenBao · read-only |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| AUDIT | ผู้ตรวจสอบ | Auditor | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DFG-01](#dfg-01) | สร้างแผนผังอัตโนมัติจาก RoPA | Must | P2 | DPO OWNER | M | XL | Y |  |
| [DFG-02](#dfg-02) | อัปเดตแผนผังเมื่อข้อมูลเปลี่ยน | Must | P2 | SCHED | S | S | N |  |
| [DFG-03](#dfg-03) | แสดงการโอนไปต่างประเทศ | Must | P2 | DPO | S | M | N |  |
| [DFG-04](#dfg-04) | แสดงผู้ประมวลผลและผู้รับข้อมูล | Must | P2 | DPO | S | S | N |  |
| [DFG-08](#dfg-08) | ส่งออกภาพและรายงาน | Must | P2 | DPO AUDIT | M | M | N |  |
| [DFG-05](#dfg-05) | แก้ไขแผนผังแบบลากวาง | Should | P3 | DPO | S | L | Y |  |
| [DFG-06](#dfg-06) | มุมมองหลายระดับ | Should | P3 | OWNER DPO | M | M | N |  |
| [DFG-07](#dfg-07) | เผยแพร่และประวัติเวอร์ชันแผนผัง | Should | P3 | DPO | S | S | N |  |
| [DFG-09](#dfg-09) | ค้นหาข้อมูลส่วนบุคคลอัตโนมัติ (Data Discovery) | Should | P3 | IT DATASRC | XL | M | Y |  |
| [DFG-10](#dfg-10) | แผนผังระดับระบบ (Data lineage) | Nice | P4 | IT | M | M | N |  |
| [DFG-11](#dfg-11) | แผนที่โลกของการโอนข้อมูล | Nice | P4 | DPO | S | M | N |  |

### ความสัมพันธ์ระหว่าง use case

- DFG-01 «include» DFG-03 (ทุกครั้งที่ทำ DFG-01 ต้องทำ DFG-03)
- DFG-01 «include» DFG-04 (ทุกครั้งที่ทำ DFG-01 ต้องทำ DFG-04)
- DFG-02 «extend» DFG-01 (DFG-02 เป็นทางเลือก/ส่วนขยายของ DFG-01)
- DFG-09 «extend» DFG-01 (DFG-09 เป็นทางเลือก/ส่วนขยายของ DFG-01)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [dataflow.layouts](../data/dataflow.md#dataflow-layouts) | ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง |
| [dataflow.snapshots](../data/dataflow.md#dataflow-snapshots) | แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน) |
| [dataflow.classifiers](../data/dataflow.md#dataflow-classifiers) | classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย) |
| [dataflow.discovery_scans](../data/dataflow.md#dataflow-discovery-scans) | รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector |
| [dataflow.discovery_findings](../data/dataflow.md#dataflow-discovery-findings) | ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `ropa.dataflow.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `ropa.dataflow` | แผนผังการไหลของข้อมูล | DPO `RUPE` · PRIVACY `RUE` · LEGAL `R` · OWNER `R` · IT `R` · SEC `R` · AUDIT `RE` · EXEC `R` |  |
| `ropa.discovery` | Data discovery และ connector | DPO `R` · PRIVACY `R` · IT `CRUDX` · SEC `R` · AUDIT `R` |  |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DFG-01, DFG-02, DFG-03, DFG-04, DFG-08
- **P3:** DFG-05, DFG-06, DFG-07, DFG-09
- **P4:** DFG-10, DFG-11

## รายละเอียด feature

<a id="dfg-01"></a>
### DFG-01 สร้างแผนผังอัตโนมัติจาก RoPA

*Auto data flow diagram*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE XL (20 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-03
- **Process:** —

**คำอธิบาย:** สร้างแผนผังแยกตามหน่วยงาน แสดงการเก็บ บันทึก ใช้ ส่ง/โอน และลบ/ทำลาย

**Backend (Go):** graph API: node (แหล่ง / ระบบ / หน่วยงาน / ผู้รับ / ประเทศ) และ edge (เก็บ / ใช้ / ส่ง / โอน / ทำลาย) สร้างจาก RoPA

**Frontend (Next.js):** React Flow + ELK auto-layout แยก swimlane ตามหน่วยงาน, legend, zoom / filter

**Acceptance criteria:** แผนผังของ 50 กิจกรรมแสดงภายใน 3 วินาทีและตรงกับข้อมูล RoPA

<a id="dfg-02"></a>
### DFG-02 อัปเดตแผนผังเมื่อข้อมูลเปลี่ยน

*Auto refresh*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DFG-01, PLT-11
- **Process:** —

**คำอธิบาย:** ปรับแผนผังตามการแก้ไข RoPA หรือข้อมูลตั้งต้น โดยไม่ต้องสร้างใหม่

**Backend (Go):** ฟัง event ropa.updated / masterdata.updated → rebuild graph เฉพาะส่วน, คงตำแหน่งที่ผู้ใช้จัดไว้

**Frontend (Next.js):** ป้ายแจ้งว่าแผนผังอัปเดตแล้ว

**Acceptance criteria:** แก้ RoPA แล้วแผนผังเปลี่ยนตามโดยไม่ต้องสร้างใหม่

<a id="dfg-03"></a>
### DFG-03 แสดงการโอนไปต่างประเทศ

*Cross-border mapping*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.28-29; ประกาศ สคส. ตามมาตรา 28 และ 29 พ.ศ. 2566
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ROPA-08
- **Process:** —

**คำอธิบาย:** ระบุประเทศปลายทางและฐานการโอน (ประเทศที่มีมาตรฐานเพียงพอ, BCR, ข้อสัญญามาตรฐาน, ข้อยกเว้น)

**Backend (Go):** node ประเทศปลายทาง + ป้ายฐานการโอน, เตือนเส้นทางที่ไม่มีฐาน

**Frontend (Next.js):** สัญลักษณ์การโอนต่างประเทศบนแผนผัง

**Acceptance criteria:** เส้นทางโอนที่ไม่มีฐานการโอนแสดงเป็นสีเตือน

<a id="dfg-04"></a>
### DFG-04 แสดงผู้ประมวลผลและผู้รับข้อมูล

*Processor & recipient mapping*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-11, DSA-11
- **Process:** —

**คำอธิบาย:** node ผู้รับข้อมูลบนแผนผัง พร้อมลิงก์ไปยังสัญญา DPA/DSA

**Backend (Go):** node ผู้รับ + ลิงก์ DPA / DSA, เตือนผู้ประมวลผลที่ไม่มี DPA

**Frontend (Next.js):** คลิก node เพื่อดูสัญญา

**Acceptance criteria:** ผู้ประมวลผลที่ไม่มี DPA แสดงเตือนบนแผนผัง

<a id="dfg-08"></a>
### DFG-08 ส่งออกภาพและรายงาน

*Export image & report*

- **Priority / Phase:** Must · P2 · กลุ่ม: ส่งออก
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), AUDIT (ผู้ตรวจสอบ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** —

**คำอธิบาย:** ส่งออก PNG/PDF กำหนดขนาดภาพ และออกรายงานประกอบ

**Backend (Go):** export PNG / SVG / PDF กำหนดขนาดได้ (render ฝั่ง server ด้วย headless Chromium) + รายงานประกอบ

**Frontend (Next.js):** ตัวเลือกขนาดและรูปแบบไฟล์

**Acceptance criteria:** ภาพที่ส่งออกอ่านได้ชัดในขนาด A3

<a id="dfg-05"></a>
### DFG-05 แก้ไขแผนผังแบบลากวาง

*Drag & drop editor*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DFG-01
- **Process:** —

**คำอธิบาย:** ขยับกล่องข้อมูล เพิ่มคำอธิบาย และคืนค่าเดิม

**Backend (Go):** บันทึก layout overlay (ตำแหน่ง / คำอธิบาย) ต่อเวอร์ชัน

**Frontend (Next.js):** ลากวาง, เพิ่มคำอธิบาย, undo / redo, คืนค่าเดิม

**Acceptance criteria:** ตำแหน่งที่จัดเองยังอยู่หลังข้อมูลอัปเดต

<a id="dfg-06"></a>
### DFG-06 มุมมองหลายระดับ

*Multi-view*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DFG-01
- **Process:** —

**คำอธิบาย:** มุมมองรายแผนก รายระบบ รายบริษัท และรายประเภทข้อมูล

**Backend (Go):** query graph หลายมุม: รายแผนก / ระบบ / บริษัท / ประเภทข้อมูล

**Frontend (Next.js):** ตัวเลือกมุมมอง

**Acceptance criteria:** สลับมุมมองได้โดยข้อมูลตรงกัน

<a id="dfg-07"></a>
### DFG-07 เผยแพร่และประวัติเวอร์ชันแผนผัง

*Publish & history*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** —

**คำอธิบาย:** เผยแพร่แผนผังที่ตรวจแล้ว และดูประวัติการแก้ไข

**Backend (Go):** snapshot แผนผังที่ publish + ประวัติ

**Frontend (Next.js):** ปุ่ม publish + ดูเวอร์ชันเก่า

**Acceptance criteria:** ดูแผนผังที่ publish ย้อนหลังได้

<a id="dfg-09"></a>
### DFG-09 ค้นหาข้อมูลส่วนบุคคลอัตโนมัติ (Data Discovery)

*Data discovery*

- **Priority / Phase:** Should · P3 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** IT (เจ้าของระบบ / IT), DATASRC (แหล่งข้อมูล (DB / File / Cloud))
- **ขนาดงาน:** BE XL (20 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-20
- **Process:** —

**คำอธิบาย:** สแกนฐานข้อมูล ไฟล์ และคลาวด์ ตรวจจับข้อมูลแบบไทย (เลขบัตร 13 หลัก เบอร์โทรไทย ชื่อภาษาไทย) แล้วเสนอเพิ่มในแผนผัง

**Backend (Go):** discovery ผ่าน connector: sampling จาก DB / ไฟล์ / S3, classifier ข้อมูลไทย (เลขบัตร 13 หลัก + checksum, เบอร์ไทย, ชื่อ-สกุลไทยด้วยพจนานุกรม / ML), เก็บเฉพาะ metadata ไม่เก็บค่าจริง, เสนอเพิ่มในทะเบียน

**Frontend (Next.js):** หน้าผลสแกน + ยืนยัน / ปฏิเสธข้อเสนอ

**Acceptance criteria:** ตรวจจับเลขบัตรประชาชนในชุดทดสอบได้อย่างน้อย 95% และไม่มีค่าจริงถูกเก็บในระบบ

**หมายเหตุ:** OneTrust ไม่มี classifier ไทย (จุดต่าง)

<a id="dfg-10"></a>
### DFG-10 แผนผังระดับระบบ (Data lineage)

*System-level lineage*

- **Priority / Phase:** Nice · P4 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ROPA-02
- **Process:** —

**คำอธิบาย:** แสดงการไหลของข้อมูลระหว่างแอปพลิเคชันและฐานข้อมูลจากทะเบียนระบบ

**Backend (Go):** แผนผังระดับระบบจากทะเบียน asset และการเชื่อมต่อ

**Frontend (Next.js):** มุมมอง lineage ระดับระบบ

**Acceptance criteria:** แสดงการไหลระหว่างระบบตรงกับทะเบียน asset

<a id="dfg-11"></a>
### DFG-11 แผนที่โลกของการโอนข้อมูล

*Transfer world map*

- **Priority / Phase:** Nice · P4 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DFG-03
- **Process:** —

**คำอธิบาย:** แสดงเส้นทางการโอนข้อมูลบนแผนที่โลก

**Backend (Go):** ข้อมูลเส้นทางการโอนรายประเทศ

**Frontend (Next.js):** แผนที่โลก (ECharts geo) แสดงเส้นทางการโอน

**Acceptance criteria:** คลิกประเทศแล้วเห็นกิจกรรมที่โอนไปประเทศนั้น
