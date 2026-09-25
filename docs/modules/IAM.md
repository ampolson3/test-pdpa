# IAM — ระบบจัดการผู้ใช้และสิทธิ์ (User & Permission)

> ผู้ใช้ สิทธิ์ และความปลอดภัยในการเข้าถึง (Identity & Access Management) · ขอบเขต: ผู้ใช้ บทบาท สิทธิ์ ขอบเขตข้อมูล SSO/MFA API client guest และการยืนยันตัวตนเจ้าของข้อมูล  
> 21 features · Must 11 / Should 9 / Nice 1 · phase: P0 (9), P1 (6), P3 (5), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/iam` |
| PostgreSQL schema | [`iam`](../data/iam.md) (19 ตาราง) |
| Admin API prefix | `/admin/v1/iam` · `/admin/v1/me` |
| Endpoint ที่ SA กำหนดแล้ว | `POST /public/v1/otp/send` — ส่ง OTP ให้เจ้าของข้อมูล (BP-02)<br>`POST /public/v1/otp/verify` — ยืนยัน OTP → portal session (BP-02)<br>`POST /scim/v2/Users` — SCIM provisioning (BP-12)<br>`GET /admin/v1/me` — โปรไฟล์ + สิทธิ์ + scope ของผู้ใช้ปัจจุบัน (SEQ-01)<br>`POST /admin/v1/iam/access-requests` — ขอสิทธิ์เพิ่ม (BP-12) |
| หน้าจอ (Next.js) | admin: /admin/users, /admin/roles; portal: /auth/* |
| พึ่งพาบริการ | Keycloak, Valkey, audit |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-01 IAM, BP-12, DFD-1, ERD-03, SEQ-01, SEQ-02, SEQ-05, ST-07 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | OIDC SSO · Admin app / portal พนักงาน |
| MANAGER | หัวหน้าแผนก | Line Manager | OIDC SSO · Admin app |
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| GUEST | ผู้ใช้ภายนอก (guest link) | External Guest | Guest link (token หมดอายุ) + OTP |
| ORGADMIN | ผู้ดูแลระบบขององค์กร | Organization Admin | OIDC (Keycloak) + MFA บังคับ · Admin app |
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | OIDC + MFA + IP allowlist · Provider console |
| SEC | ทีม Security / Incident | Security / Incident Response | OIDC SSO + MFA · Admin app |
| IDP | Keycloak / IdP องค์กร / ThaID | Identity Provider | OIDC / SAML federation |
| DIR | Directory (Entra ID / Okta / HRIS) | Directory / SCIM | SCIM 2.0 bearer token |
| APICLIENT | ระบบภายนอก (API client) | API Client | OAuth2 client credentials (token ≤ 15 นาที · scope) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [IAM-01](#iam-01) | Identity provider และ login flow | Must | P0 | EMP IDP | L | M | Y | BP-12 |
| [IAM-02](#iam-02) | Permission engine (RBAC + data scope) | Must | P0 | EMP APICLIENT | L | M | N | BP-12 |
| [IAM-03](#iam-03) | Console ผู้ให้บริการ: tenant และแพ็กเกจ | Must | P0 | SUPER | M | M | Y | BP-12 |
| [ORG-09](#org-09) | จัดการผู้ใช้ | Must | P0 | ORGADMIN | M | M | Y | BP-12 |
| [ORG-10](#org-10) | Role และ Permission | Must | P0 | ORGADMIN | M | M | Y | BP-12 |
| [ORG-11](#org-11) | ขอบเขตข้อมูลตามหน่วยงาน | Must | P0 | ORGADMIN | L | S | N | BP-12 |
| [ORG-14](#org-14) | Login นโยบายรหัสผ่าน และ MFA | Must | P0 | EMP IDP | M | S | Y | BP-12 |
| [ORG-16](#org-16) | API Client และ Service account | Must | P0 | ORGADMIN APICLIENT | M | S | N | BP-12 |
| [ORG-19](#org-19) | Audit log | Must | P0 | SEC | S | M | Y | BP-12 |
| [IAM-04](#iam-04) | สิทธิ์ผู้ใช้ภายนอก (guest) | Must | P1 | GUEST | M | S | N | BP-12 |
| [IAM-05](#iam-05) | บริการยืนยันตัวตนเจ้าของข้อมูล | Must | P1 | DS | M | S | N | BP-12 |
| [ORG-12](#org-12) | กลุ่มผู้ใช้และทีม | Should | P1 | ORGADMIN | S | S | N | BP-12 |
| [ORG-13](#org-13) | โปรไฟล์และการตั้งค่าส่วนตัว | Should | P1 | EMP | S | S | N | BP-12 |
| [ORG-15](#org-15) | Single Sign-On และ ThaID | Should | P1 | EMP IDP | M | S | N | BP-12 |
| [ORG-17](#org-17) | ปกปิดข้อมูลส่วนบุคคลบนหน้าจอ | Should | P1 | EMP SEC | M | S | N | BP-12 |
| [IAM-06](#iam-06) | Maker-checker สำหรับงานผู้ดูแลระบบ | Should | P3 | ORGADMIN SEC | S | S | N | BP-12 |
| [IAM-07](#iam-07) | Break-glass และสิทธิ์ระดับสูง | Should | P3 | SUPER SEC | S | S | N | BP-12 |
| [IAM-08](#iam-08) | จัดการ session และแจ้งเตือนความปลอดภัย | Should | P3 | ORGADMIN EMP | S | S | N | BP-12 |
| [IAM-09](#iam-09) | SCIM 2.0 provisioning | Should | P3 | DIR | M | - | N | BP-12 |
| [ORG-18](#org-18) | ทบทวนสิทธิ์ตามรอบ | Should | P3 | ORGADMIN MANAGER | M | M | Y | BP-12 |
| [IAM-10](#iam-10) | สิทธิ์ชั่วคราวและมอบอำนาจ | Nice | P4 | EMP ORGADMIN | S | S | N | BP-12 |

### ความสัมพันธ์ระหว่าง use case

- ORG-14 «include» IAM-01 (ทุกครั้งที่ทำ ORG-14 ต้องทำ IAM-01)
- ORG-15 «extend» IAM-01 (ORG-15 เป็นทางเลือก/ส่วนขยายของ IAM-01)
- ORG-10 «include» IAM-02 (ทุกครั้งที่ทำ ORG-10 ต้องทำ IAM-02)
- ORG-11 «include» IAM-02 (ทุกครั้งที่ทำ ORG-11 ต้องทำ IAM-02)
- IAM-09 «extend» ORG-09 (IAM-09 เป็นทางเลือก/ส่วนขยายของ ORG-09)
- IAM-06 «extend» ORG-10 (IAM-06 เป็นทางเลือก/ส่วนขยายของ ORG-10)

## กระบวนการ / sequence / state machine

- [BP-12 บริหารผู้ใช้และสิทธิ์ (Joiner-Mover-Leaver & access review)](../processes/BP-12.md)
- [SEQ-01 Login ผู้ใช้ภายใน (OIDC Authorization Code + PKCE ผ่าน BFF)](../sequences/SEQ-01.md)
- [SEQ-02 การตรวจสิทธิ์ต่อ request (x-permission + data scope + RLS + optimistic lock)](../sequences/SEQ-02.md)
- [SEQ-05 ยื่นคำขอใช้สิทธิ (DSAR) และยืนยันตัวตนด้วย OTP](../sequences/SEQ-05.md)
- [ST-07 บัญชีผู้ใช้ (iam.users.status) และการส่ง webhook (platform.webhook_deliveries.status)](../states/ST-07.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [iam.users](../data/iam.md#iam-users) | ผู้ใช้ระบบ (ผูกกับบัญชี Keycloak) |
| [iam.roles](../data/iam.md#iam-roles) | role มาตรฐาน (tenant_id ว่าง) และ custom role |
| [iam.permissions](../data/iam.md#iam-permissions) | catalog สิทธิ์ module.resource.action (สร้างจาก OpenAPI x-permission) |
| [iam.role_permissions](../data/iam.md#iam-role-permissions) | สิทธิ์ของแต่ละ role |
| [iam.groups](../data/iam.md#iam-groups) | กลุ่มผู้ใช้ / ทีม |
| [iam.group_members](../data/iam.md#iam-group-members) | สมาชิกของกลุ่ม |
| [iam.role_assignments](../data/iam.md#iam-role-assignments) | การมอบ role ให้ผู้ใช้ / กลุ่ม พร้อมขอบเขตข้อมูล |
| [iam.api_clients](../data/iam.md#iam-api-clients) | API client / service account (OAuth2 client credentials) |
| [iam.guest_tokens](../data/iam.md#iam-guest-tokens) | magic link ของผู้ใช้ภายนอก (คู่ค้า ผู้ประมวลผล ผู้ร่วมประเมิน) |
| [iam.idp_configs](../data/iam.md#iam-idp-configs) | การตั้งค่า SSO / IdP ต่อ tenant (สะท้อนค่าใน Keycloak) |
| [iam.security_policies](../data/iam.md#iam-security-policies) | นโยบายรหัสผ่าน MFA session ต่อ tenant |
| [iam.security_events](../data/iam.md#iam-security-events) | เหตุการณ์ความปลอดภัยของบัญชี (login, lockout, อุปกรณ์ใหม่) |
| [iam.access_reviews](../data/iam.md#iam-access-reviews) | รอบทบทวนสิทธิ์ |
| [iam.access_review_items](../data/iam.md#iam-access-review-items) | รายการที่ต้องยืนยันในรอบทบทวน |
| [iam.breakglass_requests](../data/iam.md#iam-breakglass-requests) | การขอใช้สิทธิ์ฉุกเฉิน |
| [iam.delegations](../data/iam.md#iam-delegations) | การมอบอำนาจ / สิทธิ์ชั่วคราว |
| [iam.field_masking_rules](../data/iam.md#iam-field-masking-rules) | กฎการปกปิดข้อมูลบนหน้าจอ |
| [iam.unmask_logs](../data/iam.md#iam-unmask-logs) | การเปิดดูข้อมูลที่ปกปิด (ต้องระบุเหตุผล) |
| [iam.subject_verifications](../data/iam.md#iam-subject-verifications) | การยืนยันตัวตนของเจ้าของข้อมูล (OTP / magic link / IdP) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `admin.user.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `admin.user` | ผู้ใช้ | SUPER `C` · ORGADMIN `CRUD` · DPO `R` · AUDIT `R` | SUPER สร้างเฉพาะผู้ดูแล tenant คนแรก |
| `admin.role` | Role และ Permission | ORGADMIN `CRUDA` · DPO `R` · AUDIT `R` | เปลี่ยน role ระดับ admin ต้องมีผู้อนุมัติ (IAM-06) |
| `admin.scope` | ขอบเขตข้อมูลของผู้ใช้ | ORGADMIN `CRUD` · DPO `R` · AUDIT `R` |  |
| `admin.group` | กลุ่มผู้ใช้ / ทีม | ORGADMIN `CRUD` · DPO `CRU` · PRIVACY `R` · OWNER `R` · IT `R` · SEC `R` · AUDIT `R` |  |
| `admin.idp` | SSO, IdP, นโยบายรหัสผ่านและ MFA | ORGADMIN `CRUD` · SEC `R` · AUDIT `R` | ตั้งค่าใน Keycloak ผ่านหน้าจอของระบบ |
| `admin.apiclient` | API client และ webhook | ORGADMIN `CRUD` · DPO `R` · IT `CRU` · AUDIT `R` | IT จัดการได้เฉพาะ client ของระบบที่ตนดูแล |
| `admin.accessreview` | ทบทวนสิทธิ์ | ORGADMIN `CRUDX` · DPO `R` · OWNER `X` · IT `X` · AUDIT `R` | หัวหน้าแผนกยืนยันสิทธิ์ของทีมตนเอง |
| `admin.breakglass` | ใช้สิทธิ์ฉุกเฉิน | SUPER `X` · ORGADMIN `X` | ต้องระบุเหตุผล มีเวลาจำกัด และแจ้ง DPO / Security ทันที |
| `pii.unmask` | เปิดดูข้อมูลส่วนบุคคลแบบไม่ปกปิด | DPO `X` · PRIVACY `X` · SEC `X` | ต้องระบุเหตุผล ถูกบันทึก log ทุกครั้ง และอาจต้องยืนยัน MFA ซ้ำ |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `user.provisioned` | user_id · role_code · scope | audit · SIEM |
| `user.deprovisioned` | user_id · role_code · scope | audit · SIEM |
| `access.granted` | user_id · role_code · scope | audit · SIEM |
| `access.revoked` | user_id · role_code · scope | audit · SIEM |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `access_review.schedule` | ตาม access_review_months (ค่าเริ่มต้น 6 เดือน) | สร้าง access_reviews + items และ auto-revoke เมื่อครบกำหนด | BP-12 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P0:** IAM-01, IAM-02, IAM-03, ORG-09, ORG-10, ORG-11, ORG-14, ORG-16, ORG-19
- **P1:** IAM-04, IAM-05, ORG-12, ORG-13, ORG-15, ORG-17
- **P3:** IAM-06, IAM-07, IAM-08, IAM-09, ORG-18
- **P4:** IAM-10

## รายละเอียด feature

<a id="iam-01"></a>
### IAM-01 Identity provider และ login flow

*Identity provider & login flow*

- **Priority / Phase:** Must · P0 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), IDP (Keycloak / IdP องค์กร / ThaID)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-01, PLT-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** ติดตั้ง Keycloak (HA), realm / Organizations ต่อ tenant, client ของ admin และ portal, Go middleware ตรวจ JWT (JWKS cache) + map ผู้ใช้ Keycloak ↔ ผู้ใช้ในระบบ, sync event เมื่อผู้ใช้ถูกปิด

**Backend (Go):** ติดตั้ง Keycloak (HA), realm / Organizations ต่อ tenant, client ของ admin และ portal, Go middleware ตรวจ JWT (JWKS cache) + map ผู้ใช้ Keycloak ↔ ผู้ใช้ในระบบ, sync event เมื่อผู้ใช้ถูกปิด

**Frontend (Next.js):** Auth.js (OIDC) แบบ BFF: session ใน httpOnly cookie, refresh token rotation, logout ทั้งระบบ (back-channel); theme หน้า login TH/EN ด้วย Keycloakify

**Acceptance criteria:** token ไม่ถูกเปิดเผยใน JavaScript ฝั่ง browser; logout แล้ว session ทุกแท็บสิ้นสุด

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** โมเดล Keycloak ค่าเริ่มต้น: realm เดียว + Organizations ต่อ tenant + mapper ใส่ claim `tid` — ยืนยันใน PoC T13 (decisions Q-18)

<a id="iam-02"></a>
### IAM-02 Permission engine (RBAC + data scope)

*Authorization engine*

- **Priority / Phase:** Must · P0 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), APICLIENT (ระบบภายนอก (API client))
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** IAM-01
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** permission code module.resource.action, ประกาศสิทธิ์ต่อ endpoint ใน OpenAPI (x-permission) → middleware ตรวจอัตโนมัติ, deny by default, cache permission set ใน Valkey + invalidate ด้วย event, rule ระดับ record (owner / assignee)

**Backend (Go):** permission code module.resource.action, ประกาศสิทธิ์ต่อ endpoint ใน OpenAPI (x-permission) → middleware ตรวจอัตโนมัติ, deny by default, cache permission set ใน Valkey + invalidate ด้วย event, rule ระดับ record (owner / assignee)

**Frontend (Next.js):** hook และ component <Can> ซ่อน/แสดงเมนูและปุ่ม, route guard ใน middleware ของ Next.js

**Acceptance criteria:** เรียก API ที่ไม่มีสิทธิ์ได้ 403 ทุก endpoint (contract test สร้างจาก OpenAPI)

<a id="iam-03"></a>
### IAM-03 Console ผู้ให้บริการ: tenant และแพ็กเกจ

*Tenant & subscription admin*

- **Priority / Phase:** Must · P0 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** สร้าง / ระงับ tenant, แพ็กเกจและโมดูลที่เปิดใช้ (feature flag ตาม license), โควตา (ผู้ใช้ โดเมน ธุรกรรม), สร้างผู้ดูแล tenant คนแรก

**Backend (Go):** สร้าง / ระงับ tenant, แพ็กเกจและโมดูลที่เปิดใช้ (feature flag ตาม license), โควตา (ผู้ใช้ โดเมน ธุรกรรม), สร้างผู้ดูแล tenant คนแรก

**Frontend (Next.js):** console สำหรับผู้ให้บริการแพลตฟอร์ม

**Acceptance criteria:** ปิดโมดูลที่ไม่ได้ซื้อแล้วเมนูและ API ของโมดูลนั้นใช้ไม่ได้

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** provider console เป็น surface `/provider/v1` ใช้ DB role `pdpa_platform` เฉพาะ package `internal/platform/provider` · บัญชี SUPER อยู่ใน tenant พิเศษ `platform` (decisions Q-19)

<a id="org-09"></a>
### ORG-09 จัดการผู้ใช้

*User management*

- **Priority / Phase:** Must · P0 · กลุ่ม: ผู้ใช้และสิทธิ์
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-01, PLT-14
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** เชิญทางอีเมล เปิด/ปิดบัญชี ผูกหน่วยงาน และนำเข้าผู้ใช้จาก Excel

**Backend (Go):** User API: เชิญทางอีเมล (สร้างผู้ใช้ใน Keycloak ผ่าน Admin API + ลิงก์ตั้งรหัสผ่าน), เปิด/ปิดบัญชี (ปิดใน Keycloak + revoke session), ผูกบริษัท/หน่วยงาน, นำเข้าผู้ใช้จาก Excel

**Frontend (Next.js):** หน้ารายชื่อผู้ใช้ ค้นหา/กรอง, ฟอร์มเพิ่ม/แก้ไข, ส่งคำเชิญซ้ำ, นำเข้า

**Acceptance criteria:** ปิดบัญชีแล้ว session ถูกตัดภายใน 1 นาที และทุกการเปลี่ยนแปลงถูกบันทึก audit

<a id="org-10"></a>
### ORG-10 Role และ Permission

*Roles & permissions*

- **Priority / Phase:** Must · P0 · กลุ่ม: ผู้ใช้และสิทธิ์
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565 (การควบคุมการเข้าถึง)
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** role มาตรฐานและ custom role กำหนดสิทธิ์ราย module และ action (สร้าง/ดู/แก้/ลบ/อนุมัติ/ส่งออก)

**Backend (Go):** role มาตรฐานตามชีต Roles_Permissions (seed ต่อ tenant), custom role, clone role, permission catalog สร้างจาก OpenAPI

**Frontend (Next.js):** หน้า role + permission matrix แบบ checkbox แยกโมดูลและ action (C/R/U/D/A/P/E/X)

**Acceptance criteria:** สร้างหรือแก้ role แล้วสิทธิ์มีผลทันทีโดยไม่ต้อง deploy

<a id="org-11"></a>
### ORG-11 ขอบเขตข้อมูลตามหน่วยงาน

*Data scope*

- **Priority / Phase:** Must · P0 · กลุ่ม: ผู้ใช้และสิทธิ์
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02, ORG-04
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** ผู้ใช้เห็นเฉพาะข้อมูลของบริษัท/แผนก/สาขาที่ได้รับ

**Backend (Go):** ผูก role assignment กับ scope (ทั้งกลุ่ม / บริษัท / ฝ่าย / แผนก / สาขา รวมหน่วยย่อย), resolver สร้าง filter ใน repository ทุก query (ltree ของโครงสร้างองค์กร), หลาย scope ต่อคน

**Frontend (Next.js):** เลือก scope แบบ tree ขณะให้ role

**Acceptance criteria:** ผู้ใช้แผนก A เห็นเฉพาะข้อมูลแผนก A ทุก API (automated test)

<a id="org-14"></a>
### ORG-14 Login นโยบายรหัสผ่าน และ MFA

*Authentication & MFA*

- **Priority / Phase:** Must · P0 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), IDP (Keycloak / IdP องค์กร / ThaID)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-01
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** นโยบายรหัสผ่าน ล็อกบัญชีเมื่อผิดตามจำนวนที่ตั้ง ลืมรหัสผ่านทางอีเมล และ MFA

**Backend (Go):** ตั้ง password policy, brute-force lockout, ลืมรหัสผ่าน, MFA (TOTP, WebAuthn/passkey) ใน Keycloak ต่อ tenant; บังคับ MFA ตาม role (ตรวจ claim ที่ Go middleware) และ step-up MFA

**Frontend (Next.js):** หน้าตั้งค่านโยบายความปลอดภัยของ tenant; theme หน้า login / ลืมรหัส / ตั้ง MFA

**Acceptance criteria:** ผิดครบจำนวนแล้วบัญชีถูกล็อกตามเวลาที่ตั้ง; role ที่บังคับ MFA เข้าไม่ได้ถ้าไม่ผ่าน MFA

<a id="org-16"></a>
### ORG-16 API Client และ Service account

*API credentials*

- **Priority / Phase:** Must · P0 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), APICLIENT (ระบบภายนอก (API client))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** OAuth2 client credentials, scope ต่อระบบ, หมุน secret และ IP allowlist

**Backend (Go):** OAuth2 client credentials (Keycloak service account), scope ต่อ client, ผูก tenant/ระบบ, หมุน secret, IP allowlist ที่ middleware, ปิด client ทันที

**Frontend (Next.js):** หน้าจัดการ API client แสดง secret ครั้งเดียว

**Acceptance criteria:** client เรียกได้เฉพาะ scope ที่ได้รับ และจาก IP ที่อนุญาตเท่านั้น

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** OAuth scope ของ client = permission code (เช่น `consent.record.create`) ภายใต้ role API (decisions Q-10)

<a id="org-19"></a>
### ORG-19 Audit log

*Audit log*

- **Priority / Phase:** Must · P0 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-12
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** บันทึกทุกการกระทำ ผู้ทำ เวลา และค่าก่อน/หลัง ค้นหาและส่งออกได้

**Backend (Go):** API ค้นหา / กรอง / ส่งออก audit log และตรวจ hash chain

**Frontend (Next.js):** หน้าดู log ตามผู้ใช้ / โมดูล / ช่วงเวลา พร้อมรายละเอียด before/after

**Acceptance criteria:** ตรวจได้ว่าใครเปลี่ยนสิทธิ์ของใคร เมื่อไร และส่งออกได้

<a id="iam-04"></a>
### IAM-04 สิทธิ์ผู้ใช้ภายนอก (guest)

*External guest access*

- **Priority / Phase:** Must · P1 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** GUEST (ผู้ใช้ภายนอก (guest link))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** token แบบ magic link ผูกงานเดียว (แบบประเมิน / เอกสาร / แจ้งเหตุ / งานย่อย) มีวันหมดอายุ, OTP ยืนยันอีเมล, สิทธิ์เฉพาะงานนั้น, เพิกถอนได้

**Backend (Go):** token แบบ magic link ผูกงานเดียว (แบบประเมิน / เอกสาร / แจ้งเหตุ / งานย่อย) มีวันหมดอายุ, OTP ยืนยันอีเมล, สิทธิ์เฉพาะงานนั้น, เพิกถอนได้

**Frontend (Next.js):** หน้า guest ใน portal สำหรับคู่ค้า ผู้ประมวลผล และผู้ร่วมประเมิน

**Acceptance criteria:** ลิงก์ใช้ได้เฉพาะงานที่เชิญ หมดอายุตามที่ตั้ง และเพิกถอนแล้วใช้ไม่ได้ทันที

**หมายเหตุ:** ใช้ร่วม VEN-05, DPIA-09, BRE-03, DSAR-12, DPA/DSA

<a id="iam-05"></a>
### IAM-05 บริการยืนยันตัวตนเจ้าของข้อมูล

*Data subject verification service*

- **Priority / Phase:** Must · P1 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-04
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** OTP ทาง SMS / อีเมล, magic link, ตรวจกับข้อมูลที่มีในระบบ (match score), ระดับความมั่นใจต่อ use case, รองรับ IdP ภายนอก / ThaID ภายหลัง, rate limit กัน brute force

**Backend (Go):** OTP ทาง SMS / อีเมล, magic link, ตรวจกับข้อมูลที่มีในระบบ (match score), ระดับความมั่นใจต่อ use case, รองรับ IdP ภายนอก / ThaID ภายหลัง, rate limit กัน brute force

**Frontend (Next.js):** ขั้นตอนยืนยันตัวตนใน portal ใช้ร่วม preference center, คำขอใช้สิทธิ และ double opt-in

**Acceptance criteria:** OTP หมดอายุใน 5 นาที จำกัดจำนวนครั้ง และบันทึกผลการยืนยันทุกครั้ง

**หมายเหตุ:** ใช้ร่วม CON-18, CON-19, CON-23, DSAR-06, DSAR-19

<a id="org-12"></a>
### ORG-12 กลุ่มผู้ใช้และทีม

*User groups & teams*

- **Priority / Phase:** Should · P1 · กลุ่ม: ผู้ใช้และสิทธิ์
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-09
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** กลุ่มผู้ใช้สำหรับมอบหมายงาน และให้สิทธิ์เป็นกลุ่ม

**Backend (Go):** กลุ่มผู้ใช้สำหรับมอบหมายงานและให้สิทธิ์ทั้งกลุ่ม, ใช้เป็นผู้รับงานใน workflow

**Frontend (Next.js):** หน้าจัดการกลุ่มและสมาชิก

**Acceptance criteria:** มอบหมายงานให้กลุ่มแล้วสมาชิกทุกคนเห็นงาน

**หมายเหตุ:** ดึงเข้า P1 เพราะ workflow ของ DSAR / Breach มอบงานให้ทีม

<a id="org-13"></a>
### ORG-13 โปรไฟล์และการตั้งค่าส่วนตัว

*User profile*

- **Priority / Phase:** Should · P1 · กลุ่ม: ผู้ใช้และสิทธิ์
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-09
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** แก้ข้อมูลส่วนตัว รูป ภาษา และช่องทางรับแจ้งเตือน

**Backend (Go):** API โปรไฟล์: ข้อมูลส่วนตัว รูป ภาษา ช่องทางรับแจ้งเตือน; ลิงก์ไปตั้งค่า MFA ใน Keycloak account

**Frontend (Next.js):** หน้าโปรไฟล์และการตั้งค่าการแจ้งเตือน

**Acceptance criteria:** ผู้ใช้แก้ข้อมูลของตนได้โดยไม่ต้องขอผู้ดูแล

<a id="org-15"></a>
### ORG-15 Single Sign-On และ ThaID

*SSO & ThaID*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), IDP (Keycloak / IdP องค์กร / ThaID)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-01
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** OIDC/SAML กับ Entra ID/Google, AD/LDAP และเข้าสู่ระบบด้วย ThaID

**Backend (Go):** identity brokering ใน Keycloak: OIDC/SAML (Entra ID, Google Workspace, ADFS), LDAP/AD federation, map group → role/scope, JIT provisioning; ThaID ผ่าน OIDC หลังขึ้นทะเบียน

**Frontend (Next.js):** หน้าตั้งค่า IdP ต่อ tenant + ปุ่ม login ด้วย SSO / ThaID

**Acceptance criteria:** ผู้ใช้ Entra ID เข้าระบบได้และได้ role ตาม group; login ด้วย ThaID ผูกกับบัญชีเดิมได้

**หมายเหตุ:** ดึงเข้า P1: ลูกค้าองค์กรมักบังคับ SSO; ThaID ต้องขึ้นทะเบียน (R09)

<a id="org-17"></a>
### ORG-17 ปกปิดข้อมูลส่วนบุคคลบนหน้าจอ

*PII masking*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02, PLT-13
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** แสดงเลขบัตร/เบอร์/อีเมลแบบปกปิด เปิดดูได้เฉพาะผู้มีสิทธิ์พร้อมระบุเหตุผลและบันทึก log

**Backend (Go):** masking policy ต่อฟิลด์ (เลขบัตร / เบอร์ / อีเมล) ใน response serializer, สิทธิ์ pii.unmask, endpoint unmask ต้องระบุเหตุผลและบันทึก audit

**Frontend (Next.js):** component แสดงค่าปกปิด + ปุ่ม 'แสดง' พร้อมกล่องเหตุผล

**Acceptance criteria:** ผู้ไม่มีสิทธิ์ไม่ได้รับค่าจริงจาก API และการ unmask ทุกครั้งมี log

**หมายเหตุ:** ดึงเข้า P1 ให้ทุกโมดูลใช้ตั้งแต่ต้น; OneTrust ไม่มี (จุดต่าง)

<a id="iam-06"></a>
### IAM-06 Maker-checker สำหรับงานผู้ดูแลระบบ

*Dual control for sensitive admin actions*

- **Priority / Phase:** Should · P3 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** กำหนด action ที่ต้องอนุมัติ: เปลี่ยน role ระดับ admin, export ข้อมูลจำนวนมาก, ลบความยินยอม, ปิด MFA ของผู้อื่น

**Backend (Go):** กำหนด action ที่ต้องอนุมัติ: เปลี่ยน role ระดับ admin, export ข้อมูลจำนวนมาก, ลบความยินยอม, ปิด MFA ของผู้อื่น

**Frontend (Next.js):** กล่องคำขอรออนุมัติ + ประวัติ

**Acceptance criteria:** ผู้ขอกับผู้อนุมัติต้องต่างคน และ action ไม่มีผลจนกว่าจะอนุมัติ

<a id="iam-07"></a>
### IAM-07 Break-glass และสิทธิ์ระดับสูง

*Break-glass & privileged access*

- **Priority / Phase:** Should · P3 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม), SEC (ทีม Security / Incident)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** บัญชีฉุกเฉินต้องระบุเหตุผล มีเวลาจำกัด แจ้ง DPO / Security ทันที; Super Admin ของแพลตฟอร์มไม่เห็นข้อมูล tenant ถ้าไม่ใช้ break-glass

**Backend (Go):** บัญชีฉุกเฉินต้องระบุเหตุผล มีเวลาจำกัด แจ้ง DPO / Security ทันที; Super Admin ของแพลตฟอร์มไม่เห็นข้อมูล tenant ถ้าไม่ใช้ break-glass

**Frontend (Next.js):** หน้าขอใช้สิทธิ์ฉุกเฉินและประวัติการใช้

**Acceptance criteria:** ใช้ break-glass แล้ว DPO และ Security ได้รับแจ้งทันที และสิทธิ์หมดเมื่อครบเวลา

<a id="iam-08"></a>
### IAM-08 จัดการ session และแจ้งเตือนความปลอดภัย

*Session management & security alerts*

- **Priority / Phase:** Should · P3 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-01
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** ดู / บังคับ logout session (Keycloak Admin API), แจ้งเตือน login จากอุปกรณ์หรือประเทศใหม่, จำกัด session พร้อมกัน

**Backend (Go):** ดู / บังคับ logout session (Keycloak Admin API), แจ้งเตือน login จากอุปกรณ์หรือประเทศใหม่, จำกัด session พร้อมกัน

**Frontend (Next.js):** หน้ารายการ session ของตนเองและของผู้ใช้ (สำหรับผู้ดูแล)

**Acceptance criteria:** ผู้ดูแลบังคับ logout ผู้ใช้ได้ทันที

<a id="iam-09"></a>
### IAM-09 SCIM 2.0 provisioning

*SCIM user & group provisioning*

- **Priority / Phase:** Should · P3 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** DIR (Directory (Entra ID / Okta / HRIS))
- **ขนาดงาน:** BE M (5 วัน) · FE - (0 วัน) · UX —
- **ขึ้นกับ:** ORG-15
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** endpoint SCIM 2.0 (Users / Groups) รับจาก Entra ID / Okta: สร้าง / แก้ / ปิดผู้ใช้และกลุ่ม → sync ไป Keycloak + role mapping

**Backend (Go):** endpoint SCIM 2.0 (Users / Groups) รับจาก Entra ID / Okta: สร้าง / แก้ / ปิดผู้ใช้และกลุ่ม → sync ไป Keycloak + role mapping

**Frontend (Next.js):** -

**Acceptance criteria:** ปิดบัญชีใน directory แล้วบัญชีในระบบถูกปิดอัตโนมัติ

**หมายเหตุ:** OneTrust มี SCIM — ลูกค้าองค์กรใหญ่คาดหวัง

<a id="org-18"></a>
### ORG-18 ทบทวนสิทธิ์ตามรอบ

*Access review*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความปลอดภัย
- **ที่มา:** Function List: 08_Organization
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), MANAGER (หัวหน้าแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05, ORG-10
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** รอบทบทวนสิทธิ์ให้หัวหน้ายืนยันหรือถอน และสรุปผล

**Backend (Go):** สร้างรอบทบทวนสิทธิ์ ส่งให้หัวหน้ายืนยัน/ถอน, สรุปผล, ถอนสิทธิ์อัตโนมัติเมื่อไม่ยืนยันในเวลา

**Frontend (Next.js):** หน้ารอบทบทวน + ปุ่มยืนยัน/ถอนสิทธิ์ราย user

**Acceptance criteria:** สิทธิ์ที่ไม่ได้รับการยืนยันถูกถอนตามนโยบาย และมีรายงานสรุปรอบ

<a id="iam-10"></a>
### IAM-10 สิทธิ์ชั่วคราวและมอบอำนาจ

*Delegation & time-bound access*

- **Priority / Phase:** Nice · P4 · กลุ่ม: IAM
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-10
- **Process:** [BP-12](../processes/BP-12.md)

**คำอธิบาย:** ให้ role แบบมีวันหมดอายุ, มอบงานแทนช่วงลา

**Backend (Go):** ให้ role แบบมีวันหมดอายุ, มอบงานแทนช่วงลา

**Frontend (Next.js):** กำหนดวันเริ่ม / สิ้นสุดสิทธิ์ และผู้รับมอบ

**Acceptance criteria:** สิทธิ์หมดอายุอัตโนมัติตามเวลาที่ตั้ง
