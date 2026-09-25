# ORG — ข้อมูลหน่วยงาน (Organization)

> ระบบบริหารจัดการข้อมูลหน่วยงาน (Organization Management) · ขอบเขต: นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก ข้อมูลตั้งต้น ปฏิทินวันทำการ และตั้งค่าองค์กร  
> 10 features · Must 4 / Should 5 / Nice 1 · phase: P0 (5), P1 (1), P3 (3), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/org` |
| PostgreSQL schema | [`org`](../data/org.md) (12 ตาราง) |
| Admin API prefix | `/admin/v1/org` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /org/* |
| พึ่งพาบริการ | importer, audit |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-02 ORG, DFD-1, ERD-04 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| ORGADMIN | ผู้ดูแลระบบขององค์กร | Organization Admin | OIDC (Keycloak) + MFA บังคับ · Admin app |
| PROC | จัดซื้อ / ผู้ดูแลคู่ค้า | Procurement / Vendor Manager | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | OIDC + MFA + IP allowlist · Provider console |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [ORG-01](#org-01) | ข้อมูลหน่วยงาน / นิติบุคคล | Must | P0 | ORGADMIN | S | S | Y |  |
| [ORG-02](#org-02) | หลายนิติบุคคลและบริษัทในเครือ | Should | P0 | ORGADMIN | M | S | N |  |
| [ORG-04](#org-04) | โครงสร้างหน่วยงานแบบลำดับชั้น | Must | P0 | ORGADMIN | M | M | Y |  |
| [ORG-07](#org-07) | ข้อมูลตั้งต้นกลาง (Master data) | Must | P0 | DPO SUPER | M | M | N |  |
| [ORG-20](#org-20) | ตั้งค่าองค์กร | Should | P0 | ORGADMIN | S | M | Y |  |
| [ORG-06](#org-06) | ทะเบียนหน่วยงานภายนอก | Must | P1 | DPO PROC | M | M | Y |  |
| [ORG-05](#org-05) | ผู้ประสานงาน PDPA ประจำหน่วยงาน | Should | P3 | ORGADMIN DPO | S | S | N |  |
| [ORG-08](#org-08) | นำเข้าและส่งออกข้อมูลตั้งต้น | Should | P3 | DPO | XS | S | N |  |
| [ORG-21](#org-21) | เลือกที่เก็บข้อมูลและรูปแบบติดตั้ง | Should | P3 | SUPER | M | S | N |  |
| [ORG-03](#org-03) | ผู้แทนในประเทศไทย | Nice | P4 | ORGADMIN | XS | XS | N |  |

### ความสัมพันธ์ระหว่าง use case

- ORG-08 «extend» ORG-07 (ORG-08 เป็นทางเลือก/ส่วนขยายของ ORG-07)
- ORG-05 «extend» ORG-04 (ORG-05 เป็นทางเลือก/ส่วนขยายของ ORG-04)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [org.legal_entities](../data/org.md#org-legal-entities) | นิติบุคคล (บริษัทในกลุ่ม) |
| [org.org_units](../data/org.md#org-org-units) | โครงสร้างหน่วยงาน (ltree) |
| [org.privacy_champions](../data/org.md#org-privacy-champions) | ผู้ประสานงาน PDPA ประจำหน่วยงาน |
| [org.external_parties](../data/org.md#org-external-parties) | ทะเบียนหน่วยงานภายนอก (ผู้ประมวลผล ผู้รับ หน่วยงานรัฐ) |
| [org.data_categories](../data/org.md#org-data-categories) | หมวดข้อมูลส่วนบุคคล (ทั่วไป / อ่อนไหว ม.26) |
| [org.data_subject_types](../data/org.md#org-data-subject-types) | กลุ่มเจ้าของข้อมูล |
| [org.processing_purposes](../data/org.md#org-processing-purposes) | วัตถุประสงค์การประมวลผล (master) |
| [org.lawful_bases](../data/org.md#org-lawful-bases) | ฐานทางกฎหมาย ม.24 / ม.26 / ม.19 |
| [org.countries](../data/org.md#org-countries) | ประเทศและสถานะมาตรฐานการคุ้มครองที่เพียงพอ |
| [org.business_calendars](../data/org.md#org-business-calendars) | ปฏิทินวันทำการ |
| [org.holidays](../data/org.md#org-holidays) | วันหยุดในปฏิทิน |
| [org.org_settings](../data/org.md#org-org-settings) | ค่าตั้งค่าองค์กร (ภาษา แบรนด์ รูปแบบวันที่) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `org.structure.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `org.structure` | นิติบุคคลและโครงสร้างหน่วยงาน | ORGADMIN `CRUD` · DPO `RU` · PRIVACY `R` · LEGAL `R` · OWNER `R` · IT `R` · MKT `R` · FRONT `R` · SEC `R` · PROC `R` · AUDIT `R` · EXEC `R` · EMP `R` |  |
| `org.party` | ทะเบียนหน่วยงานภายนอก | ORGADMIN `CRUD` · DPO `CRUD` · PRIVACY `CRU` · LEGAL `CRU` · OWNER `R` · IT `R` · PROC `CRU` · AUDIT `R` |  |
| `org.masterdata` | ข้อมูลตั้งต้น | SUPER `CRUD` · ORGADMIN `CRUD` · DPO `CRUD` · PRIVACY `CRU` · LEGAL `R` · OWNER `R` · IT `R` · MKT `R` · SEC `R` · AUDIT `R` | SUPER ดูแลค่าตั้งต้นกลางที่ทุก tenant ได้รับ |
| `org.settings` | ตั้งค่าองค์กร แบรนด์ และปฏิทินวันทำการ | SUPER `CRUD` · ORGADMIN `RU` · DPO `R` · AUDIT `R` |  |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P0:** ORG-01, ORG-04, ORG-07, ORG-02, ORG-20
- **P1:** ORG-06
- **P3:** ORG-05, ORG-08, ORG-21
- **P4:** ORG-03

## รายละเอียด feature

<a id="org-01"></a>
### ORG-01 ข้อมูลหน่วยงาน / นิติบุคคล

*Organization profile*

- **Priority / Phase:** Must · P0 · กลุ่ม: ข้อมูลองค์กร
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23(5), ม.39(3)
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-02
- **Process:** —

**คำอธิบาย:** ชื่อ TH/EN เลขทะเบียนนิติบุคคล/ผู้เสียภาษี ที่อยู่ ช่องทางติดต่อ และโลโก้ ใช้ในประกาศ เอกสาร และแบบแจ้งเหตุ

**Backend (Go):** นิติบุคคล: ชื่อ TH/EN, เลขทะเบียน 13 หลัก (ตรวจ checksum), ที่อยู่, ช่องทางติดต่อ, โลโก้; เป็น merge field ของเอกสาร

**Frontend (Next.js):** ฟอร์มข้อมูลองค์กร + อัปโหลดโลโก้

**Acceptance criteria:** ข้อมูลองค์กรแสดงถูกต้องในประกาศและเอกสารที่สร้าง

**Implementation (ORG-01):** `backend/internal/org/service/structure.go` — CRUD นิติบุคคล (`org.structure.*`): ชื่อ TH/EN, เลขทะเบียน/ผู้เสียภาษี 13 หลักตรวจ check digit (ตัด - และช่องว่าง, ซ้ำในองค์กรไม่ได้ — migration 00030), ที่อยู่ (jsonb: line1, subdistrict, district, province, postal_code 5 หลักเมื่อเป็น TH, country_code), อีเมล/โทรศัพท์ติดต่อ, บริษัทแม่ (กันวน), ผู้ควบคุม/ผู้ประมวลผล, สถานะ · โลโก้ = ไฟล์ PNG/JPEG ของผู้ใช้ที่ผ่านการสแกน ผูกผ่าน PLT-09 (`files` entity `legal_entity`, ดาวน์โหลดด้วย `org.structure.read`) · `MergeFields` ให้ค่าตัวแปรเอกสาร (`org_name_th`, `org_name_en`, `org_registration_no`, `org_tax_id`, `org_address`, `org_email`, `org_phone`) สำหรับ composer ในอนาคต · API `/admin/v1/org/legal-entities` · หน้าจอ `/settings/organization` · ยังไม่ทำ: ผู้แทนในไทย (`representative`, ORG-03)

<a id="org-02"></a>
### ORG-02 หลายนิติบุคคลและบริษัทในเครือ

*Multi-entity / group*

- **Priority / Phase:** Should · P0 · กลุ่ม: ข้อมูลองค์กร
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.41 วรรคสอง
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-04
- **Process:** —

**คำอธิบาย:** จัดการหลายบริษัทในระบบเดียว แยกข้อมูลและสิทธิ์ต่อบริษัท และรองรับ DPO ร่วมของกลุ่ม

**Backend (Go):** หลายบริษัทใน tenant เดียว, scope ต่อบริษัท, DPO ร่วมของกลุ่ม

**Frontend (Next.js):** สลับบริษัทที่กำลังทำงาน + ตัวกรองบริษัท

**Acceptance criteria:** ผู้ใช้ของบริษัท A ไม่เห็นข้อมูลบริษัท B ถ้าไม่ได้รับ scope

**หมายเหตุ:** ดึงเข้า P0 เพราะกระทบ data model ทุกโมดูล

<a id="org-04"></a>
### ORG-04 โครงสร้างหน่วยงานแบบลำดับชั้น

*Organization hierarchy*

- **Priority / Phase:** Must · P0 · กลุ่ม: โครงสร้าง
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ORG-01
- **Process:** —

**คำอธิบาย:** กลุ่ม → บริษัท → ฝ่าย → แผนก/สาขา แสดงแบบ tree และใช้กำหนดขอบเขตข้อมูล

**Backend (Go):** org tree (ltree), ย้ายหน่วยงาน, ปิดหน่วยงานโดยเก็บประวัติ, event เมื่อโครงสร้างเปลี่ยน

**Frontend (Next.js):** tree view ลากวาง + ค้นหา

**Acceptance criteria:** ย้ายแผนกแล้ว scope ของผู้ใช้และข้อมูลปรับตามทันที

**Implementation (ORG-04):** tree บน ltree (`org_units.path` = ลำดับ label `u<id>`; GiST index, migration 00030) · เพิ่มใต้หน่วยงานของนิติบุคคลเดียวกัน, แก้ชื่อ/รหัส/ประเภท, ย้ายพร้อมหน่วยงานย่อยทั้งหมดในคำสั่งเดียว (กันย้ายไปใต้ตัวเอง), ปิดหน่วยงานได้เมื่อไม่มีหน่วยงานย่อยที่เปิดอยู่ (เก็บไว้เป็นประวัติ) — ทุกการเปลี่ยนแปลงลง audit · `UnitWithin(unit, scope, includeDescendants)` ตรวจกับ tree ปัจจุบัน จึงให้ผลใหม่ทันทีหลังย้าย (acceptance; ใช้โดย data scope ของ IAM-02 / ORG-11 เมื่อสร้าง) · ยังไม่ส่ง event เพราะ `events.yaml` ไม่มี event ของ org — เพิ่มเมื่อมีผู้รับ · หน้าจอ tree ลากวาง + เมนูย้าย + ค้นหา (แสดงผลที่พบพร้อมหน่วยงานแม่)

<a id="org-07"></a>
### ORG-07 ข้อมูลตั้งต้นกลาง (Master data)

*Master data*

- **Priority / Phase:** Must · P0 · กลุ่ม: ข้อมูลตั้งต้น
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.24, ม.26
- **Actor:** DPO (DPO / Privacy Team), SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ORG-01
- **Process:** —

**คำอธิบาย:** ประเภทข้อมูล ข้อมูลอ่อนไหว (ม.26) กลุ่มเจ้าของข้อมูล วัตถุประสงค์ ฐานกฎหมาย (ม.24/26) ระยะเวลาเก็บ และประเทศ

**Backend (Go):** ประเภทข้อมูล, ข้อมูลอ่อนไหว ม.26, กลุ่มเจ้าของข้อมูล, วัตถุประสงค์, ฐานกฎหมาย ม.24/26, ระยะเวลาเก็บ, ประเทศ (สถานะมาตรฐานเพียงพอ); ชุดค่าเริ่มต้นภาษาไทย

**Frontend (Next.js):** หน้าจัดการ master data แยกหมวด

**Acceptance criteria:** แก้ที่เดียวมีผลทุกโมดูล และ tenant ใหม่ได้ชุดค่าเริ่มต้นครบ

**Implementation (ORG-07):** `backend/internal/org/service/masterdata.go` — ค่าตั้งต้นกลาง (แถว `tenant_id NULL` / ตาราง global) seed ใน migration 00031: ฐานทางกฎหมาย 13 รายการ (ม.19/24(1)–(6)/26 พร้อม flag ต้องขอความยินยอม / ต้องทำ LIA / สำหรับข้อมูลอ่อนไหว), หมวดข้อมูล 19 (อ่อนไหวตาม ม.26 10 หมวด), กลุ่มเจ้าของข้อมูล 9 (ผู้เยาว์ = กลุ่มเปราะบาง), วัตถุประสงค์ 10, ประเทศ 249 (ISO 3166-1 ชื่อไทย/อังกฤษจาก CLDR, adequacy = `unknown`) — **เป็นร่างรอฝ่ายกฎหมายตรวจ (decisions Q-20)** หน้าจอแสดงป้ายเตือน · tenant ใหม่เห็นชุดนี้ทันทีโดยไม่ต้อง provision · tenant เพิ่ม/แก้/ลบรายการของตนเองได้ (หมวดข้อมูล กลุ่มเจ้าของข้อมูล วัตถุประสงค์; รหัสซ้ำค่าตั้งต้นไม่ได้; ลบได้เมื่อไม่มีการอ้างถึง) ค่าตั้งต้น ฐานกฎหมาย และประเทศแก้ไม่ได้ · module อ้างถึงด้วย id จึงแก้ที่เดียวมีผลทุกที่ · API `/admin/v1/org/master-data/{kind}` (`org.masterdata.*`) · หน้าจอ `/settings/master-data` · ยังไม่ทำ: ระยะเวลาเก็บรักษา (Q-09: กรอกใน RoPA ต่อกิจกรรม ไม่มีค่ากลาง), SUPER แก้ค่ากลางผ่าน provider console (ยังไม่มี `/provider/v1`)

<a id="org-20"></a>
### ORG-20 ตั้งค่าองค์กร

*Organization settings*

- **Priority / Phase:** Should · P0 · กลุ่ม: ตั้งค่า
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-02
- **Process:** —

**คำอธิบาย:** ภาษา TH/EN โลโก้และธีม template แจ้งเตือน และปฏิทินวันทำการ/วันหยุดสำหรับนับ SLA

**Backend (Go):** ตั้งค่าภาษา โลโก้ ธีม template แจ้งเตือน และปฏิทินวันทำการ / วันหยุดราชการไทยต่อปี (ใช้โดย SLA engine)

**Frontend (Next.js):** หน้าตั้งค่าองค์กร + ปฏิทินวันหยุด

**Acceptance criteria:** SLA ของทุกโมดูลนับตามปฏิทินของ tenant

**หมายเหตุ:** ดึงเข้า P0 เพราะ SLA engine ต้องใช้ปฏิทิน

**Implementation (ORG-20 — ส่วนปฏิทิน):** `backend/internal/org` — ปฏิทินวันทำการหลายชุดต่อ tenant (ชื่อ, เขตเวลา IANA, วันทำการ ISO 1–7) ปฏิทินแรกเป็นปฏิทินหลักอัตโนมัติ ย้ายปฏิทินหลักได้ (ซิงก์ `org.org_settings.default_calendar_id`) · วันหยุด: ผู้ดูแลกรอกเองหรือนำเข้า CSV/Excel ผ่าน PLT-14 (import type `org.holiday`: วันที่ YYYY-MM-DD / ว/ด/ปปปป รับปี พ.ศ. / เซลล์วันที่ Excel, ชื่อวันหยุด, ชื่อปฏิทิน — ว่าง = ปฏิทินหลัก) ไม่มีข้อมูลวันหยุดตั้งต้น · module อื่นอ่านผ่าน interface `orgservice.Calendars.BusinessCalendar` แล้วคำนวณด้วย `internal/pkg/bizcal` (ข้ามวันหยุดประจำสัปดาห์ + วันหยุด ตามเขตเวลาของปฏิทิน; tenant ที่ยังไม่มีปฏิทิน = จันทร์–ศุกร์ Asia/Bangkok ไม่มีวันหยุด) · API `/admin/v1/org/calendars` (GET `org.settings.read`, POST/PATCH `org.settings.update` — ORGADMIN ไม่มี `create` จึงใช้ `update` สำหรับการเพิ่มปฏิทิน), `/admin/v1/org/calendars/{id}/holidays[/{date}]` · migration 00027 (ปฏิทินหลักได้หนึ่งเดียว, ชื่อไม่ซ้ำ) · หน้าจอ `/settings/calendar` · ยังไม่ทำ: ภาษา โลโก้ ธีม และ template แจ้งเตือนของ ORG-20 (template อยู่ที่ PLT-04 แล้ว)

<a id="org-06"></a>
### ORG-06 ทะเบียนหน่วยงานภายนอก

*External parties directory*

- **Priority / Phase:** Must · P1 · กลุ่ม: โครงสร้าง
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39, ม.40
- **Actor:** DPO (DPO / Privacy Team), PROC (จัดซื้อ / ผู้ดูแลคู่ค้า)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ORG-01
- **Process:** —

**คำอธิบาย:** ผู้ประมวลผล ผู้รับข้อมูล และหน่วยงานรัฐ ใช้ร่วมกับ RoPA, DSA, DPA และ Vendor

**Backend (Go):** directory หน่วยงานภายนอก: ประเภท (ผู้ประมวลผล / ผู้รับข้อมูล / หน่วยงานรัฐ), ประเทศ, ผู้ติดต่อ, ตรวจรายการซ้ำ; ใช้ร่วม RoPA, DSA, DPA, Vendor

**Frontend (Next.js):** หน้ารายการ + ฟอร์ม + รวมรายการซ้ำ

**Acceptance criteria:** หน่วยงานภายนอกหนึ่งรายมี record เดียวที่ทุกโมดูลอ้างถึง

<a id="org-05"></a>
### ORG-05 ผู้ประสานงาน PDPA ประจำหน่วยงาน

*Privacy champions*

- **Priority / Phase:** Should · P3 · กลุ่ม: โครงสร้าง
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-04, ORG-12
- **Process:** —

**คำอธิบาย:** ระบุผู้ประสานงานของแต่ละแผนก เพื่อรับงาน RoPA คำขอใช้สิทธิ และการแจ้งเตือน

**Backend (Go):** ผู้ประสานงานต่อแผนก ใช้เป็นผู้รับงานเริ่มต้นของ RoPA / คำขอ / การแจ้งเตือน

**Frontend (Next.js):** กำหนดผู้ประสานงานในหน้าหน่วยงาน

**Acceptance criteria:** งานของแผนกถูกส่งถึงผู้ประสานงานอัตโนมัติ

<a id="org-08"></a>
### ORG-08 นำเข้าและส่งออกข้อมูลตั้งต้น

*Master data import/export*

- **Priority / Phase:** Should · P3 · กลุ่ม: ข้อมูลตั้งต้น
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XS (1 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-14, ORG-07
- **Process:** —

**คำอธิบาย:** นำเข้าและส่งออกข้อมูลตั้งต้นด้วย Excel

**Backend (Go):** template นำเข้า / ส่งออก master data (ใช้ PLT-14)

**Frontend (Next.js):** ปุ่มนำเข้า / ส่งออกในหน้า master data

**Acceptance criteria:** นำเข้าแล้วรายการซ้ำถูกตรวจพบก่อนบันทึก

<a id="org-21"></a>
### ORG-21 เลือกที่เก็บข้อมูลและรูปแบบติดตั้ง

*Data residency & deployment*

- **Priority / Phase:** Should · P3 · กลุ่ม: ตั้งค่า
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** T41
- **Process:** —

**คำอธิบาย:** ติดตั้งแบบ on-premise หรือ cloud ในประเทศ เพื่อให้ข้อมูลอยู่ในไทย

**Backend (Go):** โหมดติดตั้ง on-prem: license key แบบ offline, ตั้งค่า region / data residency, ตรวจความพร้อมของ environment

**Frontend (Next.js):** หน้าแสดงสถานะการติดตั้งและ license

**Acceptance criteria:** ติดตั้ง on-prem ด้วย Helm ได้ตามคู่มือ และข้อมูลทั้งหมดอยู่ใน environment ของลูกค้า

**หมายเหตุ:** SaaS บน cloud ในไทยทำใน T17 ตั้งแต่ P1

<a id="org-03"></a>
### ORG-03 ผู้แทนในประเทศไทย

*Local representative*

- **Priority / Phase:** Nice · P4 · กลุ่ม: ข้อมูลองค์กร
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(5)
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE XS (1 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** ORG-01
- **Process:** —

**คำอธิบาย:** บันทึกผู้แทนกรณีผู้ควบคุมข้อมูลอยู่นอกประเทศ และใช้ในประกาศ

**Backend (Go):** ฟิลด์ผู้แทนในประเทศไทย + merge field ในประกาศ

**Frontend (Next.js):** ส่วนผู้แทนในหน้าข้อมูลองค์กร

**Acceptance criteria:** ประกาศแสดงข้อมูลผู้แทนเมื่อกำหนดไว้
