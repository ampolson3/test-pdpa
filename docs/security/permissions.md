# สิทธิ์และบทบาท (RBAC + data scope)

ต้นฉบับ: ชีต Roles_Permissions ใน Development Plan · machine-readable: [`permissions.yaml`](permissions.yaml) · seed: `backend/db/migrations/00019_seed_iam_permissions.sql` (282 permission code · 16 system role)

## โมเดล

- **deny by default** — endpoint ที่ไม่มี `x-permission` ใน OpenAPI ต้องทำให้ CI fail (ยกเว้น `/public/v1`, `/scim/v2`, `/webhooks/*` ที่ประกาศ `x-permission: public` / `scim` / `webhook` ชัดเจน)
- **permission code** = `<area>.<resource>.<action>` เช่น `ropa.activity.update`, `admin.user.create`, `assessment.dpia.approve` · area ตามชีต Roles_Permissions (admin, org, cookie, consent, notice, ropa, dsar, breach, assessment, vendor, agreement, dpo, dpx, pii) **ไม่ใช่ชื่อ Go package** · action ∈ create, read, update, delete, approve, publish, export, execute · เก็บใน `iam.permissions.code` (area → คอลัมน์ `module`)
- ใช้ได้เฉพาะ code ที่อยู่ใน `permissions.yaml` — ต้องการ code ใหม่ให้เพิ่มใน yaml + migration ใหม่ + x-permission (ห้ามตั้งเองในโค้ด เพราะ code ที่ไม่มีใน catalogue จะถูกปฏิเสธเสมอ)
- **role** (`iam.roles`) = ชุด permission · system role 16 ตัวเป็นแถวกลาง (tenant_id NULL) · tenant clone ไปปรับได้ (`cloned_from_id`)
- **role assignment** (`iam.role_assignments`) ผูก user/group กับ role พร้อม **data scope**: `tenant` · `legal_entity` · `org_unit` (+ `include_descendants`) · `self` และช่วงเวลา `valid_from` / `valid_to`
- **record rule** — ตรวจใน repository/service เพิ่มจาก scope เช่น Owner เห็นเฉพาะแผนกตน, ผู้แจ้งเหตุเห็นเฉพาะเหตุที่ตนแจ้ง (ดูคอลัมน์หมายเหตุด้านล่าง)
- **RLS** แยก tenant ที่ฐานข้อมูลเป็นชั้นสุดท้าย — authorization ในแอปยังต้องทำครบเสมอ
- cache สิทธิ์ต่อ user ใน Valkey 60 วินาที · ล้าง cache ทันทีเมื่อเปลี่ยน role / ปิดบัญชี
- `pii.unmask` ต้องระบุเหตุผล + MFA step-up + บันทึก `iam.unmask_logs` ทุกครั้ง

## Role

| code | ชื่อ | หน้าที่ | ขอบเขตเริ่มต้น | MFA บังคับ |
|---|---|---|---|---|
| `SUPER` | Platform Super Admin | ผู้ให้บริการแพลตฟอร์ม: tenant แพ็กเกจ และค่าระบบ ไม่เห็นข้อมูลธุรกิจของ tenant เว้นแต่ใช้ break-glass (IAM-07) | ทั้งแพลตฟอร์ม | ✓ |
| `ORGADMIN` | Organization Admin | ผู้ดูแลระบบขององค์กร: ผู้ใช้ role โครงสร้างองค์กร SSO และการตั้งค่า | ทั้ง tenant | ✓ |
| `DPO` | DPO | เจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล ดูแลทุกโมดูลและเป็นผู้อนุมัติหลัก | tenant / บริษัทที่ได้รับมอบหมาย | ✓ |
| `PRIVACY` | Privacy Team | ทีมงาน DPO ปฏิบัติงานประจำ แต่ไม่อนุมัติขั้นสุดท้าย | บริษัทที่ได้รับมอบหมาย | ✓ |
| `LEGAL` | Legal | ฝ่ายกฎหมาย: ประกาศ template สัญญา DPA/DSA | ทั้ง tenant |  |
| `OWNER` | Process Owner / Champion | เจ้าของกระบวนการหรือผู้ประสานงาน PDPA ของแผนก ดูแล RoPA และตอบแบบสอบถาม | แผนกของตนเอง |  |
| `IT` | System Owner / IT | เจ้าของระบบ/IT: asset, data discovery, งานย่อยของคำขอ, ติดตั้ง script, connector | ระบบ/แผนกที่ได้รับมอบหมาย |  |
| `MKT` | Marketing / Business | การตลาด/ธุรกิจ: Purpose, Collection Point และแคมเปญขอความยินยอม | แผนก/แบรนด์ของตนเอง |  |
| `FRONT` | Front Staff | พนักงานหน้าร้าน/Call center: บันทึกความยินยอมแทนลูกค้าและรับคำขอใช้สิทธิ | สาขา/ทีมของตนเอง |  |
| `SEC` | Security / Incident | ทีมความปลอดภัย: เหตุละเมิดและประเมินมาตรการความปลอดภัย | ทั้ง tenant | ✓ |
| `PROC` | Procurement / Vendor Manager | จัดซื้อ/ผู้ดูแลคู่ค้า: ทะเบียนและการประเมินคู่ค้า | คู่ค้าที่ตนดูแล |  |
| `AUDIT` | Auditor | ผู้ตรวจสอบภายใน/ภายนอก ดูและส่งออกได้อย่างเดียว | ทั้ง tenant (อ่านอย่างเดียว) | ✓ |
| `EXEC` | Executive | ผู้บริหาร: dashboard รายงาน และยอมรับความเสี่ยง | ทั้ง tenant (สรุป) |  |
| `EMP` | Employee | พนักงานทั่วไป: แจ้งเหตุ อบรม รับทราบนโยบาย ขอคำปรึกษา | ข้อมูลของตนเอง |  |
| `GUEST` | External Guest | คู่ค้า ผู้ประมวลผล หรือผู้ร่วมประเมินภายนอก เข้าผ่าน magic link (IAM-04) | เฉพาะงานที่ได้รับเชิญ |  |
| `API` | API Client | บัญชีระบบสำหรับเชื่อมต่อ (CRM, POS, แอป) ใช้สิทธิ์ตาม scope | ระบบที่ผูกไว้ |  |

## Actor (use case) ↔ role

| actor ใน UC | role |
|---|---|
| SUPER | SUPER |
| ORGADMIN | ORGADMIN |
| DPO | DPO, PRIVACY |
| LEGAL | LEGAL |
| OWNER | OWNER |
| IT | IT |
| MKT | MKT |
| FRONT | FRONT |
| SEC | SEC |
| PROC | PROC |
| AUDIT | AUDIT |
| EXEC | EXEC |
| EMP / MANAGER | EMP (+ หัวหน้าตามโครงสร้าง org_units) |
| GUEST / VENDOR / COUNTER | GUEST (guest token ผูกงานเดียว) |
| APICLIENT / EXT | API (OAuth2 client credentials) |
| DS / GUARD / PUBLIC | ไม่มี role — portal session หลังยืนยัน OTP / ThaID หรือฟอร์มสาธารณะ + CAPTCHA |
| IDP / DIR / SCHED / LLM ฯลฯ | ระบบ — ไม่ใช่ผู้ใช้ใน RBAC |

## Segregation of duties และนโยบายที่เกี่ยวข้อง

- **Segregation of duties:** ผู้สร้างกับผู้อนุมัติต้องต่างคน: เผยแพร่ประกาศ/แบนเนอร์/Purpose, ปฏิเสธคำขอ, ส่งแบบแจ้ง สคส., export ความยินยอมจำนวนมาก, เปลี่ยน role ระดับ admin
- **Authorization:** deny by default; ทุก endpoint ประกาศ x-permission ใน OpenAPI และมี contract test ตรวจ 403; กรองข้อมูลตาม scope ใน repository + PostgreSQL RLS แยก tenant
- **MFA:** TOTP หรือ WebAuthn/passkey; บังคับสำหรับ Super Admin, Org Admin, DPO, Privacy, Security, Auditor; ขอ MFA ซ้ำ (step-up) ก่อน export จำนวนมาก, unmask และเปลี่ยน role
- **PII protection:** เข้ารหัส identifier ระดับฟิลด์ (envelope encryption), ปกปิดบนหน้าจอเป็นค่าเริ่มต้น, unmask ต้องระบุเหตุผล, ห้ามมี PII ใน log (redaction middleware)
- **Guest access:** magic link ผูกงานเดียว หมดอายุ 7-30 วัน ยืนยันด้วย OTP ทางอีเมล และเพิกถอนได้ทันที
- **Access review:** ทบทวนสิทธิ์ทุก 6 เดือน (ตั้งค่าได้); สิทธิ์ที่ไม่ได้รับการยืนยันถูกถอน
- **Offboarding:** ปิดบัญชีภายใน 24 ชั่วโมงหลังพ้นสภาพ (อัตโนมัติผ่าน SCIM / HRIS เมื่อเชื่อมแล้ว)

## Matrix

ตัวอักษร: C = สร้าง · R = ดู · U = แก้ไข · D = ลบ · A = อนุมัติ · P = เผยแพร่ / เปิดใช้งาน · E = ส่งออก · X = ดำเนินการ / มอบหมาย / ทำรายการ · ว่าง = ไม่มีสิทธิ์

### Administration

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `admin.tenant` | Tenant แพ็กเกจ และโมดูลที่เปิดใช้ | CRUD |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  | เฉพาะผู้ให้บริการแพลตฟอร์ม |
| `admin.user` | ผู้ใช้ | C | CRUD | R |  |  |  |  |  |  |  |  | R |  |  |  |  | SUPER สร้างเฉพาะผู้ดูแล tenant คนแรก |
| `admin.role` | Role และ Permission |  | CRUDA | R |  |  |  |  |  |  |  |  | R |  |  |  |  | เปลี่ยน role ระดับ admin ต้องมีผู้อนุมัติ (IAM-06) |
| `admin.scope` | ขอบเขตข้อมูลของผู้ใช้ |  | CRUD | R |  |  |  |  |  |  |  |  | R |  |  |  |  |  |
| `admin.group` | กลุ่มผู้ใช้ / ทีม |  | CRUD | CRU | R |  | R | R |  |  | R |  | R |  |  |  |  |  |
| `admin.idp` | SSO, IdP, นโยบายรหัสผ่านและ MFA |  | CRUD |  |  |  |  |  |  |  | R |  | R |  |  |  |  | ตั้งค่าใน Keycloak ผ่านหน้าจอของระบบ |
| `admin.apiclient` | API client และ webhook |  | CRUD | R |  |  |  | CRU |  |  |  |  | R |  |  |  |  | IT จัดการได้เฉพาะ client ของระบบที่ตนดูแล |
| `admin.audit` | Audit log | RE | RE | RE |  |  |  |  |  |  | RE |  | RE |  |  |  |  | ไม่มีใครแก้ไขหรือลบ log ได้ |
| `admin.job` | งานเบื้องหลัง (background job) | R | R |  |  |  |  |  |  |  |  |  |  |  |  |  |  | เฉพาะ job ของ tenant ตนเอง · SUPER ดูข้าม tenant ผ่าน /provider/v1 (PLT-10) |
| `admin.workflow` | นิยาม workflow และ SLA | CRU | CRU | R |  |  |  |  |  |  |  |  |  |  |  |  |  | บันทึกแล้วเป็นเวอร์ชันใหม่ · instance / task ใช้สิทธิ์ของ record เจ้าของหรือผู้ได้รับมอบหมาย (PLT-05, migration 00028) |
| `admin.accessreview` | ทบทวนสิทธิ์ |  | CRUDX | R |  |  | X | X |  |  |  |  | R |  |  |  |  | หัวหน้าแผนกยืนยันสิทธิ์ของทีมตนเอง |
| `admin.breakglass` | ใช้สิทธิ์ฉุกเฉิน | X | X |  |  |  |  |  |  |  |  |  |  |  |  |  |  | ต้องระบุเหตุผล มีเวลาจำกัด และแจ้ง DPO / Security ทันที |
| `admin.notification` | Template แจ้งเตือน |  | CRUD | CRU | CRU |  |  |  | CRU |  |  |  | R |  |  |  |  |  |

### Organization

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `org.structure` | นิติบุคคลและโครงสร้างหน่วยงาน |  | CRUD | RU | R | R | R | R | R | R | R | R | R | R | R |  |  |  |
| `org.party` | ทะเบียนหน่วยงานภายนอก |  | CRUD | CRUD | CRU | CRU | R | R |  |  |  | CRU | R |  |  |  |  |  |
| `org.masterdata` | ข้อมูลตั้งต้น | CRUD | CRUD | CRUD | CRU | R | R | R | R |  | R |  | R |  |  |  |  | SUPER ดูแลค่าตั้งต้นกลางที่ทุก tenant ได้รับ |
| `org.settings` | ตั้งค่าองค์กร แบรนด์ และปฏิทินวันทำการ | CRUD | RU | R |  |  |  |  |  |  |  |  | R |  |  |  |  |  |

### Consent

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `cookie.banner` | แบนเนอร์คุกกี้และโดเมน |  |  | CRUDAP | CRU | R |  | RU | R |  |  |  | R |  |  |  |  | IT คัดลอก script และแก้ค่าทางเทคนิค |
| `cookie.scan` | ผลสแกนคุกกี้ |  |  | CRUDX | CRUX |  |  | CRUX | R |  |  |  | R |  |  |  |  |  |
| `consent.purpose` | Purpose / Data Element / Purpose Preference |  |  | CRUDAP | CRU | RU |  |  | CRU |  |  |  | R |  |  |  | R | Marketing ร่างได้ DPO อนุมัติและเปิดใช้ |
| `consent.collectionpoint` | Collection Point / แบบฟอร์ม |  |  | CRUDAP | CRU |  |  | RU | CRU |  |  |  | R |  |  |  | R |  |
| `consent.record` | ความยินยอมรายบุคคล / โปรไฟล์ |  |  | RE | RE |  |  | R | R | R |  |  | RE |  |  |  | CR | identifier แสดงแบบปกปิด ยกเว้นมีสิทธิ์ pii.unmask |
| `consent.onbehalf` | บันทึกความยินยอมแทนลูกค้า |  |  | CR | CR |  |  |  |  | CR |  |  | R |  |  |  |  | Front Staff เห็นเฉพาะรายการที่ตนบันทึกในสาขา |
| `consent.bulk` | นำเข้า / ส่งออกความยินยอม |  |  | XE | XE |  |  | X | E |  |  |  | E |  |  |  |  | export จำนวนมากต้องมีผู้อนุมัติ |
| `consent.campaign` | แคมเปญขอความยินยอม |  |  | CRUDA | CRU |  |  |  | CRUX |  |  |  | R |  |  |  |  |  |

### Notice

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `notice.document` | ประกาศความเป็นส่วนตัว |  |  | CRUDAP | CRU | CRUA | R | R | R |  |  |  | R |  | R |  | R | ผู้สร้างกับผู้อนุมัติต้องต่างคน |
| `notice.template` | Template ประกาศ |  |  | CRUD | R | CRUD |  |  |  |  |  |  | R |  |  |  |  |  |
| `notice.indirect` | แจ้งกรณีได้ข้อมูลจากแหล่งอื่น (ม.25) |  |  | CRUD | CRU |  | CRU |  | CRU |  |  |  | R |  |  |  |  |  |

### RoPA

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `ropa.inventory` | ทะเบียนข้อมูลและ asset |  |  | CRUDA | CRUD | R | CRU | CRU |  |  | R |  | R |  |  |  |  | Process Owner เฉพาะแผนกตนเอง |
| `ropa.activity` | กิจกรรมการประมวลผล (RoPA) |  |  | CRUDAE | CRUE | R | CRU | R | R |  | R |  | RE | R |  |  |  | Process Owner เฉพาะแผนกตนเอง |
| `ropa.template` | คลัง RoPA ฉบับมาตรฐาน | CRUD |  | CRUDP | CRU | CRU | RX |  |  |  |  |  | R |  |  |  |  | SUPER ดูแลคลังกลาง; tenant สร้าง template ของตนเอง |
| `ropa.risk` | ความเสี่ยงและช่องว่างรายกิจกรรม |  |  | CRUDA | CRU |  | RU |  |  |  | RU |  | R | R |  |  |  |  |
| `ropa.dataflow` | แผนผังการไหลของข้อมูล |  |  | RUPE | RUE | R | R | R |  |  | R |  | RE | R |  |  |  |  |
| `ropa.discovery` | Data discovery และ connector |  |  | R | R |  |  | CRUDX |  |  | R |  | R |  |  |  |  |  |

### DSAR

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `dsar.request` | คำขอใช้สิทธิ |  |  | CRUDAXE | CRUX | RU | R | R |  | CR |  |  | RE |  |  |  | C | Owner/IT เห็นเฉพาะคำขอที่มีงานมอบหมาย; Front Staff เห็นเฉพาะที่ตนลงไว้ |
| `dsar.subtask` | งานย่อยของคำขอ |  |  | CRUDX | CRUX | RU | RU | RU |  |  |  |  | R |  |  | RU |  | แก้ได้เฉพาะงานที่ได้รับมอบหมาย; GUEST คือผู้ประมวลผลที่ได้รับคำสั่ง |
| `dsar.package` | ข้อมูลที่ส่งคืนเจ้าของข้อมูล |  |  | CRA | CR |  |  | CU |  |  |  |  | R |  |  |  |  | Auditor เห็นเฉพาะ metadata |
| `dsar.form` | แบบฟอร์มคำขอใช้สิทธิ |  |  | CRUDP | CRU | RU |  |  |  |  |  |  | R |  |  |  |  |  |

### Breach

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `breach.incident` | เหตุละเมิด |  |  | CRUDAX | CRUX | RU | C | CRU | C | C | CRUX |  | R | R | C | C |  | ผู้แจ้งเห็นเฉพาะเหตุที่ตนแจ้ง; GUEST คือผู้ประมวลผลที่แจ้งเหตุ |
| `breach.notification` | แบบแจ้ง สคส. / แจ้งเจ้าของข้อมูล |  |  | CRUAP | CRU | RU |  |  |  |  | R |  | R | R |  |  |  | ส่งแบบแจ้งต้องให้ DPO อนุมัติ |

### Assessment

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `assessment.dpia` | DPIA / LIA |  |  | CRUDA | CRU | RU | RU | RU |  |  | RU |  | R | RA |  | RU |  | ผู้ร่วมประเมินแก้ได้เฉพาะส่วนที่ได้รับเชิญ; ผู้บริหารอนุมัติ/ยอมรับความเสี่ยง |
| `assessment.security` | ประเมินมาตรการความปลอดภัย |  |  | RA | R |  |  | RU |  |  | CRUDX |  | R |  |  |  |  |  |
| `assessment.transfer` | ประเมินการโอนต่างประเทศ (TIA) |  |  | CRUDA | CRU | RU |  |  |  |  |  |  | R |  |  |  |  |  |
| `assessment.template` | Template แบบประเมิน |  |  | CRUDP | CRU | CRU |  |  |  |  | CRU |  | R |  |  |  |  |  |

### Vendor

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `vendor.vendor` | คู่ค้า / ผู้ประมวลผล |  |  | CRUDA | CRU | R | CR | R |  |  | RU | CRUD | R |  |  |  |  | Owner ขอเพิ่มคู่ค้าใหม่ได้ |
| `vendor.assessment` | แบบประเมินคู่ค้า |  |  | CRUDA | CRUX |  |  |  |  |  | RUX | CRUX | R |  |  | RU |  | GUEST ตอบเฉพาะแบบประเมินของบริษัทตนเอง |

### Agreement

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `agreement.dpa` | ข้อตกลงการประมวลผล (DPA) |  |  | RA | R | CRUDAP | R |  |  |  |  | CR | R |  |  | R |  | GUEST ดูและลงนามฉบับที่ส่งให้ |
| `agreement.dsa` | ข้อตกลงการแบ่งปันข้อมูล (DSA) |  |  | RA | R | CRUDAP | R |  |  |  |  |  | R |  |  | R |  |  |
| `agreement.clause` | คลังข้อความสัญญา |  |  | R |  | CRUDP |  |  |  |  |  |  | R |  |  |  |  |  |

### DPO

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `dpo.profile` | ทะเบียน DPO |  | RU | CRU |  | R |  |  |  |  |  |  | R | R |  |  |  |  |
| `dpo.task` | Tasks / Ticket |  |  | CRUDX | CRUX | RU | RU | RU |  |  | RU |  | R |  |  |  |  | แก้ได้เฉพาะงานที่ได้รับมอบหมาย |
| `dpo.advisory` | คำปรึกษาถึง DPO |  |  | RUX | RUX | RU | CR | CR | CR |  |  |  | R |  | CR |  |  | ผู้ถามเห็นเฉพาะเรื่องของตนเอง |
| `dpo.risk` | ทะเบียนความเสี่ยง |  |  | CRUDA | CRU |  | R |  |  |  | CRU |  | R | RA |  |  |  |  |
| `dpo.report` | Dashboard และรายงาน |  | R | RE | RE | R |  |  |  |  | R |  | RE | RE |  |  |  |  |
| `dpo.kb` | คลังเอกสารกฎหมายและ FAQ |  |  | CRUDP | CRU | CRU | R | R | R | R | R | R | R | R | R |  |  |  |

### DPO Extension

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `dpx.training` | อบรมและทดสอบ |  | R | CRUDP | CRU | X | X | X | X | X | X | X | R | X | X |  |  | พนักงานทุกคนเรียนและทำแบบทดสอบได้ (X) |
| `dpx.policy` | รับทราบนโยบายภายใน |  |  | CRUDP |  | CRU | X | X | X | X | X | X | R | X | X |  |  |  |
| `dpx.audit` | ตรวจประเมินความพร้อมและแผนแก้ไข |  |  | CRUDA | CRU |  | RU | RU |  |  | RU |  | RE | R |  |  |  |  |
| `dpx.retention` | Retention และการทำลายข้อมูล |  |  | CRUDA | CRU |  | R | RUX |  |  |  |  | R |  |  |  |  | IT ดำเนินการทำลายและแนบหลักฐาน |
| `dpx.regulator` | ทะเบียนการติดต่อกับ สคส. |  |  | CRUD | R | CRU |  |  |  |  |  |  | R | R |  |  |  |  |
| `dpx.ai` | ทะเบียนระบบ AI และผู้ช่วย AI |  |  | CRUDAX | CRUX | X |  | CRU |  |  | R |  | R |  |  |  |  | ผู้ช่วย AI ใช้ได้ตาม license และการเปิดใช้ของ tenant |
| `dpx.integration` | Connector HR / ITSM / SIEM |  | CRUD | R |  |  |  | CRUD |  |  | R |  | R |  |  |  |  |  |

### Special

| code | ความหมาย | SUPER | ORGADMIN | DPO | PRIVACY | LEGAL | OWNER | IT | MKT | FRONT | SEC | PROC | AUDIT | EXEC | EMP | GUEST | API | หมายเหตุ |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `pii.unmask` | เปิดดูข้อมูลส่วนบุคคลแบบไม่ปกปิด |  |  | X | X |  |  |  |  |  | X |  |  |  |  |  |  | ต้องระบุเหตุผล ถูกบันทึก log ทุกครั้ง และอาจต้องยืนยัน MFA ซ้ำ |

## เพิ่ม / แก้สิทธิ์

- แก้ `permissions.yaml` + สร้าง migration ใหม่ (INSERT permission / role_permissions) — ห้ามแก้ `00019` หลัง deploy
- ใส่ `x-permission` ใน OpenAPI ของ endpoint ใหม่ และเพิ่ม contract test: role ที่ไม่มีสิทธิ์ต้องได้ 403 `authz.denied`
- UI ใช้ `packages/authz` (`usePermission('ropa.activity.update')`, `<Can permission=…>`) ซ่อนเมนู/ปุ่ม — แต่ backend ต้องตรวจเสมอ
