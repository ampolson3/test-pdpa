# PLT — โครงสร้างพื้นฐานแพลตฟอร์ม (Platform Foundation)

> บริการกลางของแพลตฟอร์ม (Platform Services) · ขอบเขต: ภาษา แจ้งเตือน workflow ฟอร์ม เวอร์ชัน import API/webhook เอกสาร portal dashboard connector ค้นหา AI  
> 22 features · Must 18 / Should 3 / Nice 1 · phase: P0 (15), P1 (4), P3 (2), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/platform/<service>` |
| PostgreSQL schema | [`platform`](../data/platform.md) (28 ตาราง) |
| Admin API prefix | `/admin/v1/platform` |
| Endpoint ที่ SA กำหนดแล้ว | `GET /admin/v1/platform/files/{id}/download` — ดาวน์โหลดผ่าน signed URL (SEQ-07) |
| หน้าจอ (Next.js) | admin: /settings/* |
| พึ่งพาบริการ | - |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-03 PLT, DFD-0, DFD-1, ERD-01, ERD-02, SEQ-02, SEQ-04, SEQ-07, ST-07 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | OIDC SSO · Admin app / portal พนักงาน |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| ORGADMIN | ผู้ดูแลระบบขององค์กร | Organization Admin | OIDC (Keycloak) + MFA บังคับ · Admin app |
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | OIDC + MFA + IP allowlist · Provider console |
| APICLIENT | ระบบภายนอก (API client) | API Client | OAuth2 client credentials (token ≤ 15 นาที · scope) |
| LLM | บริการ AI (LLM) | AI Service | ผ่าน AI gateway (mask PII ก่อนส่ง) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [PLT-01](#plt-01) | โครงระบบ Go modular monolith + Next.js monorepo | Must | P0 | ระบบ | L | M | N |  |
| [PLT-02](#plt-02) | Multi-tenant และการแยกข้อมูล | Must | P0 | ระบบ | L | S | N |  |
| [PLT-03](#plt-03) | ระบบหลายภาษา TH/EN | Must | P0 | EMP | S | M | N |  |
| [PLT-04](#plt-04) | บริการแจ้งเตือน Email / SMS / LINE / In-app | Must | P0 | ORGADMIN | L | M | Y |  |
| [PLT-05](#plt-05) | Workflow & SLA engine | Must | P0 | ORGADMIN DPO | XL | M | Y |  |
| [PLT-06](#plt-06) | Form & assessment engine | Must | P0 | DPO | L | XL | Y |  |
| [PLT-07](#plt-07) | ความเห็น ไฟล์แนบ และไทม์ไลน์กิจกรรม | Must | P0 | EMP | M | M | Y |  |
| [PLT-08](#plt-08) | เวอร์ชันและการอนุมัติ | Must | P0 | EMP DPO | M | M | Y |  |
| [PLT-09](#plt-09) | จัดเก็บไฟล์และสแกนไวรัส | Must | P0 | ระบบ | M | S | N |  |
| [PLT-10](#plt-10) | Background jobs และ scheduler | Must | P0 | SUPER ORGADMIN | M | S | N |  |
| [PLT-11](#plt-11) | Domain events และ outbox | Must | P0 | ระบบ | M | - | N |  |
| [PLT-12](#plt-12) | โครงสร้าง Audit log | Must | P0 | ระบบ | M | - | N |  |
| [PLT-13](#plt-13) | เข้ารหัสข้อมูลส่วนบุคคลระดับฟิลด์ | Must | P0 | ระบบ | M | - | N |  |
| [PLT-14](#plt-14) | Import framework (Excel / CSV) | Must | P0 | ORGADMIN DPO | M | M | Y |  |
| [PLT-15](#plt-15) | Public API, webhook และ developer portal | Must | P0 | IT APICLIENT | M | S | N |  |
| [PLT-16](#plt-16) | Document composer (template + clause + merge field) | Must | P1 | DPO | L | L | Y |  |
| [PLT-17](#plt-17) | Public portal framework | Must | P1 | ORGADMIN | M | L | Y |  |
| [PLT-18](#plt-18) | Dashboard & report framework | Must | P1 | EMP | L | L | Y |  |
| [PLT-19](#plt-19) | Observability | Should | P1 | ระบบ | S | S | N |  |
| [PLT-20](#plt-20) | Connector framework | Should | P3 | IT | L | M | Y |  |
| [PLT-21](#plt-21) | ค้นหาข้ามระบบ | Should | P3 | EMP | M | S | N |  |
| [PLT-22](#plt-22) | AI gateway | Nice | P4 | ORGADMIN LLM | M | S | N |  |

### ความสัมพันธ์ระหว่าง use case

- PLT-16 «include» PLT-08 (ทุกครั้งที่ทำ PLT-16 ต้องทำ PLT-08)
- PLT-14 «include» PLT-10 (ทุกครั้งที่ทำ PLT-14 ต้องทำ PLT-10)

## กระบวนการ / sequence / state machine

- [SEQ-02 การตรวจสิทธิ์ต่อ request (x-permission + data scope + RLS + optimistic lock)](../sequences/SEQ-02.md)
- [SEQ-04 Consent API: idempotency + transaction + outbox + webhook](../sequences/SEQ-04.md)
- [SEQ-07 สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg)](../sequences/SEQ-07.md)
- [ST-07 บัญชีผู้ใช้ (iam.users.status) และการส่ง webhook (platform.webhook_deliveries.status)](../states/ST-07.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [platform.tenants](../data/platform.md#platform-tenants) | Tenant (องค์กรลูกค้า) |
| [platform.tenant_modules](../data/platform.md#platform-tenant-modules) | โมดูลที่เปิดใช้ต่อ tenant (license) |
| [platform.public_keys](../data/platform.md#platform-public-keys) | public key สำหรับ /public/v1 และ portal: หา tenant ก่อนเปิด transaction (ทุกคนอ่านได้ · เขียนได้เฉพาะ tenant เจ้าของ) |
| [platform.workflow_definitions](../data/platform.md#platform-workflow-definitions) | นิยาม workflow (state machine) ต่อประเภทงาน |
| [platform.workflow_instances](../data/platform.md#platform-workflow-instances) | workflow ที่กำลังทำงานของแต่ละ record |
| [platform.workflow_tasks](../data/platform.md#platform-workflow-tasks) | งานในแต่ละขั้นของ workflow |
| [platform.sla_timers](../data/platform.md#platform-sla-timers) | ตัวนับเวลา SLA (วันปฏิทิน / วันทำการ / ชั่วโมง) |
| [platform.form_definitions](../data/platform.md#platform-form-definitions) | ฟอร์ม / แบบประเมิน (form engine) |
| [platform.form_versions](../data/platform.md#platform-form-versions) | เวอร์ชันของฟอร์ม (schema JSON) |
| [platform.form_submissions](../data/platform.md#platform-form-submissions) | คำตอบของฟอร์ม |
| [platform.record_versions](../data/platform.md#platform-record-versions) | snapshot และ diff ของ record ที่มีเวอร์ชัน |
| [platform.approvals](../data/platform.md#platform-approvals) | ขั้นตอนอนุมัติ (maker-checker / หลายระดับ) |
| [platform.comments](../data/platform.md#platform-comments) | ความเห็น / @mention ต่อ record |
| [platform.files](../data/platform.md#platform-files) | ไฟล์ใน object storage (เข้ารหัส + สแกนไวรัส) |
| [platform.documents](../data/platform.md#platform-documents) | เอกสารที่สร้างจาก composer (ประกาศ สัญญา หนังสือ แบบแจ้ง) |
| [platform.document_versions](../data/platform.md#platform-document-versions) | เวอร์ชันของเอกสาร (ProseMirror JSON + ไฟล์ที่ render) |
| [platform.templates](../data/platform.md#platform-templates) | template เอกสาร / ข้อความ (กลางและของ tenant) |
| [platform.clause_library](../data/platform.md#platform-clause-library) | คลังข้อความสัญญา / clause มาตรฐาน |
| [platform.notification_templates](../data/platform.md#platform-notification-templates) | template อีเมล / SMS / LINE / in-app |
| [platform.notifications](../data/platform.md#platform-notifications) | ข้อความที่ส่งและสถานะการส่ง |
| [platform.outbox_events](../data/platform.md#platform-outbox-events) | transactional outbox ของ domain event |
| [platform.webhook_subscriptions](../data/platform.md#platform-webhook-subscriptions) | webhook ที่ระบบปลายทางลงทะเบียน |
| [platform.webhook_deliveries](../data/platform.md#platform-webhook-deliveries) | การส่ง webhook แต่ละครั้ง (retry / dead-letter) |
| [platform.audit_log](../data/platform.md#platform-audit-log) | audit log แบบ append-only + hash chain (partition รายเดือน) |
| [platform.import_jobs](../data/platform.md#platform-import-jobs) | งานนำเข้า Excel / CSV |
| [platform.export_jobs](../data/platform.md#platform-export-jobs) | งานส่งออกแบบ async (ศูนย์ดาวน์โหลด) |
| [platform.connectors](../data/platform.md#platform-connectors) | connector ไปยังระบบของลูกค้า (DB / REST / SaaS) |
| [platform.ai_requests](../data/platform.md#platform-ai-requests) | log การเรียก AI (หลังปกปิด PII) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `admin.tenant.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `admin.tenant` | Tenant แพ็กเกจ และโมดูลที่เปิดใช้ | SUPER `CRUD` | เฉพาะผู้ให้บริการแพลตฟอร์ม |
| `admin.apiclient` | API client และ webhook | ORGADMIN `CRUD` · DPO `R` · IT `CRU` · AUDIT `R` | IT จัดการได้เฉพาะ client ของระบบที่ตนดูแล |
| `admin.audit` | Audit log | SUPER `RE` · ORGADMIN `RE` · DPO `RE` · SEC `RE` · AUDIT `RE` | ไม่มีใครแก้ไขหรือลบ log ได้ |
| `admin.notification` | Template แจ้งเตือน | ORGADMIN `CRUD` · DPO `CRU` · PRIVACY `CRU` · MKT `CRU` · AUDIT `R` |  |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `outbox.dispatch` | enqueue ใน tx เดียวกับ outbox (args: tenant_id) + sweeper รายนาที | อ่าน outbox_events ของ tenant (FOR UPDATE SKIP LOCKED) → สร้าง webhook_deliveries | SEQ-04 |
| `webhook.deliver` | ตาม next_retry_at | POST webhook + HMAC · retry 1m, 5m, 30m, 2h, 6h, 24h → dead | ST-07 |
| `partition.maintain` | รายวัน | `SELECT platform.ensure_monthly_partitions(3)` — partition รายเดือนล่วงหน้า 3 เดือนของ audit_log, security_events, consent_transactions, cookie consent_records (migration 00020) | ARC-06 |
| `docs.render` | on demand | render HTML/DOCX → PDF ผ่าน Gotenberg → เก็บ object storage | SEQ-07 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P0:** PLT-01, PLT-02, PLT-03, PLT-04, PLT-05, PLT-06, PLT-07, PLT-08, PLT-09, PLT-10, PLT-11, PLT-12, PLT-13, PLT-14, PLT-15
- **P1:** PLT-16, PLT-17, PLT-18, PLT-19
- **P3:** PLT-20, PLT-21
- **P4:** PLT-22

## รายละเอียด feature

<a id="plt-01"></a>
### PLT-01 โครงระบบ Go modular monolith + Next.js monorepo

*Solution skeleton*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** T04, T05
- **Process:** —

**คำอธิบาย:** โครง backend/internal/<module>, chi router + oapi-codegen (OpenAPI-first), pgx + sqlc, goose migration, config, error model RFC 9457, health/readiness, graceful shutdown; binary api / worker / scanner

**Backend (Go):** โครง backend/internal/<module>, chi router + oapi-codegen (OpenAPI-first), pgx + sqlc, goose migration, config, error model RFC 9457, health/readiness, graceful shutdown; binary api / worker / scanner

**Frontend (Next.js):** Turborepo + pnpm: apps/admin, apps/portal (App Router, TypeScript), packages/ui (shadcn/ui + Tailwind), api-client generate จาก OpenAPI, layout และเมนูตามสิทธิ์

**Acceptance criteria:** สร้างโมดูลใหม่จาก template ได้ภายใน 1 วัน; แก้ OpenAPI แล้ว Go stub และ TS client generate ใน CI

<a id="plt-02"></a>
### PLT-02 Multi-tenant และการแยกข้อมูล

*Multi-tenancy & isolation*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** tenant registry, resolver จาก subdomain / custom domain / token, tenant_id ทุกตาราง + PostgreSQL RLS (SET app.tenant_id ต่อ transaction), provisioning (master data, role มาตรฐาน), โหมด DB แยกต่อ tenant

**Backend (Go):** tenant registry, resolver จาก subdomain / custom domain / token, tenant_id ทุกตาราง + PostgreSQL RLS (SET app.tenant_id ต่อ transaction), provisioning (master data, role มาตรฐาน), โหมด DB แยกต่อ tenant

**Frontend (Next.js):** tenant context ใน middleware ของ Next.js, custom domain ของ portal

**Acceptance criteria:** ผู้ใช้ tenant A เข้าถึงข้อมูล tenant B ไม่ได้แม้เรียก API ตรง (automated test ทุก endpoint)

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** role / privilege / partition ตาม `deploy/db/*.sql` และ security.md (ทดสอบแล้ว) · FK constraint ไม่ผ่าน RLS → service ต้องตรวจว่า id ที่อ้างถึงมองเห็นได้ภายใต้ RLS

<a id="plt-03"></a>
### PLT-03 ระบบหลายภาษา TH/EN

*Localization (i18n)*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** ตาราง translation สำหรับเนื้อหาที่ผู้ใช้สร้าง (ข้อความยินยอม ประกาศ template), Accept-Language, fallback

**Backend (Go):** ตาราง translation สำหรับเนื้อหาที่ผู้ใช้สร้าง (ข้อความยินยอม ประกาศ template), Accept-Language, fallback

**Frontend (Next.js):** next-intl ข้อความ UI TH/EN, วันที่ พ.ศ. / ค.ศ., เขตเวลา Asia/Bangkok, ฟอนต์ไทย

**Acceptance criteria:** ทุกหน้าสลับ TH/EN ได้ และวันที่แสดง พ.ศ. ตามการตั้งค่า

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** ไม่มีตาราง translation แยก: ข้อความหลายภาษาเก็บเป็นคอลัมน์ `_th` / `_en` หรือ jsonb `{th, en}` ตาม ERD (เช่น purpose_versions.text_th / text_en, cookie.categories.names) · UI ใช้ next-intl

<a id="plt-04"></a>
### PLT-04 บริการแจ้งเตือน Email / SMS / LINE / In-app

*Notification service*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-10
- **Process:** —

**คำอธิบาย:** template มีตัวแปร (TH/EN), adapter SMTP/SES, SMS gateway ไทย, LINE Messaging API, in-app ผ่าน SSE, ส่งผ่าน River + retry, delivery log, quiet hours

**Backend (Go):** template มีตัวแปร (TH/EN), adapter SMTP/SES, SMS gateway ไทย, LINE Messaging API, in-app ผ่าน SSE, ส่งผ่าน River + retry, delivery log, quiet hours

**Frontend (Next.js):** หน้าจัดการ template พร้อม preview, กระดิ่งแจ้งเตือนใน admin, หน้าสถานะการส่ง

**Acceptance criteria:** ส่งล้มเหลวมี retry อัตโนมัติ และตรวจสถานะรายข้อความได้

<a id="plt-05"></a>
### PLT-05 Workflow & SLA engine

*Workflow / SLA engine*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE XL (20 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-10, ORG-20
- **Process:** —

**คำอธิบาย:** state machine กำหนดได้ต่อ tenant (JSON definition), มอบหมายคน/กลุ่ม, SLA timer แบบวันปฏิทิน / วันทำการ / ชั่วโมง พร้อมวันหยุดไทย, reminder และ escalation ด้วย River scheduled job, comment, history

**Backend (Go):** state machine กำหนดได้ต่อ tenant (JSON definition), มอบหมายคน/กลุ่ม, SLA timer แบบวันปฏิทิน / วันทำการ / ชั่วโมง พร้อมวันหยุดไทย, reminder และ escalation ด้วย River scheduled job, comment, history

**Frontend (Next.js):** component กระดานงาน, timeline, ป้าย SLA, หน้าตั้งค่า workflow แบบฟอร์ม

**Acceptance criteria:** นับ SLA ถูกต้องเมื่อข้ามวันหยุด และแจ้งเตือนก่อนครบกำหนดตามที่ตั้ง (unit test ครอบคลุมกรณีข้ามวันหยุด)

**หมายเหตุ:** บริการกลางที่ DSAR, Breach, DPIA, Vendor และ Tasks ใช้

<a id="plt-06"></a>
### PLT-06 Form & assessment engine

*Dynamic form & assessment engine*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE XL (20 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** form schema มีเวอร์ชัน (JSON), ประเภทคำถาม, conditional logic, คะแนน/น้ำหนัก, validation ฝั่ง server, คำตอบ JSONB, มอบหมายผู้ตอบรายส่วน

**Backend (Go):** form schema มีเวอร์ชัน (JSON), ประเภทคำถาม, conditional logic, คะแนน/น้ำหนัก, validation ฝั่ง server, คำตอบ JSONB, มอบหมายผู้ตอบรายส่วน

**Frontend (Next.js):** Form builder ลากวาง (dnd-kit) + renderer ใช้ร่วม admin และ portal (react-hook-form + zod), preview หลายภาษา

**Acceptance criteria:** สร้างแบบประเมินที่มีเงื่อนไขซ่อน/แสดงและคิดคะแนนได้โดยไม่เขียนโค้ด; ทุกโมดูลใช้ renderer เดียวกัน

**หมายเหตุ:** ใช้ร่วม: ฟอร์มความยินยอม, คำขอใช้สิทธิ, แจ้งเหตุ, DPIA, คู่ค้า, RoPA, checklist

<a id="plt-07"></a>
### PLT-07 ความเห็น ไฟล์แนบ และไทม์ไลน์กิจกรรม

*Collaboration components*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-09, PLT-12
- **Process:** —

**คำอธิบาย:** comment + @mention, ไฟล์แนบ, activity feed ต่อ record แบบ polymorphic, แจ้งเตือนผู้เกี่ยวข้อง

**Backend (Go):** comment + @mention, ไฟล์แนบ, activity feed ต่อ record แบบ polymorphic, แจ้งเตือนผู้เกี่ยวข้อง

**Frontend (Next.js):** component ความเห็น / ไฟล์แนบ / ไทม์ไลน์ ใช้ร่วมทุกโมดูล

**Acceptance criteria:** ทุกโมดูลใช้ component เดียวกัน และการ mention ส่งแจ้งเตือน

<a id="plt-08"></a>
### PLT-08 เวอร์ชันและการอนุมัติ

*Versioning & approval framework*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-05, PLT-07
- **Process:** —

**คำอธิบาย:** snapshot + diff ของ record, สถานะ ร่าง → รอตรวจ → อนุมัติ → เผยแพร่, ผู้อนุมัติหลายระดับ, maker-checker

**Backend (Go):** snapshot + diff ของ record, สถานะ ร่าง → รอตรวจ → อนุมัติ → เผยแพร่, ผู้อนุมัติหลายระดับ, maker-checker

**Frontend (Next.js):** แถบสถานะเวอร์ชัน, หน้าเปรียบเทียบเวอร์ชัน, กล่องงานรออนุมัติ

**Acceptance criteria:** record ที่เผยแพร่แล้วแก้ไม่ได้ต้องสร้างเวอร์ชันใหม่; ผู้สร้างอนุมัติงานของตนเองไม่ได้

<a id="plt-09"></a>
### PLT-09 จัดเก็บไฟล์และสแกนไวรัส

*File storage & AV scan*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** S3 API (S3 / SeaweedFS / MinIO), เข้ารหัส, ClamAV, จำกัดชนิดและขนาด, pre-signed URL หมดอายุ, อายุไฟล์

**Backend (Go):** S3 API (S3 / SeaweedFS / MinIO), เข้ารหัส, ClamAV, จำกัดชนิดและขนาด, pre-signed URL หมดอายุ, อายุไฟล์

**Frontend (Next.js):** component อัปโหลด / ดาวน์โหลด (ลากวาง, progress)

**Acceptance criteria:** ไฟล์ติดไวรัสถูกปฏิเสธ และลิงก์ดาวน์โหลดหมดอายุตามเวลาที่ตั้ง

<a id="plt-10"></a>
### PLT-10 Background jobs และ scheduler

*Job queue & scheduler*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** SUPER (ผู้ให้บริการแพลตฟอร์ม), ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** River (job queue บน PostgreSQL): enqueue ใน transaction เดียวกับข้อมูล, retry/backoff, periodic job (cron), unique job, graceful shutdown ใน worker

**Backend (Go):** River (job queue บน PostgreSQL): enqueue ใน transaction เดียวกับข้อมูล, retry/backoff, periodic job (cron), unique job, graceful shutdown ใน worker

**Frontend (Next.js):** หน้าดูสถานะ job สำหรับผู้ดูแลระบบ

**Acceptance criteria:** job ที่ล้มเหลวมี retry และแจ้งเตือน; ไม่มี job ซ้ำเมื่อรันหลาย instance

**Implementation (PLT-10):** `backend/internal/platform/jobs` (กฎดู `docs/architecture/code-structure.md`) · หน้าจอ admin `/settings/jobs` ← `GET /admin/v1/platform/jobs` (`platformListJobs`, `x-permission: admin.job.read` — ORGADMIN, SUPER; migration 00021) แสดงเฉพาะ job ของ tenant ตนเอง (กรองด้วย `args.tenant_id` เพราะ `river_job` ไม่มี RLS) · SUPER ดูข้าม tenant ผ่าน `/provider/v1` ภายหลัง · การแจ้งเตือนเมื่อ job ล้มเหลว = log `alert=job_discarded` + metric `pdpa.jobs.discarded` (ส่งถึงคนผ่าน PLT-04 เมื่อสร้างแล้ว)

<a id="plt-11"></a>
### PLT-11 Domain events และ outbox

*Domain events & outbox*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE M (5 วัน) · FE - (0 วัน) · UX —
- **ขึ้นกับ:** PLT-10
- **Process:** —

**คำอธิบาย:** transactional outbox, event catalog (consent.changed, ropa.updated, dsar.closed ฯลฯ), subscriber ภายใน, ส่งต่อ webhook / NATS

**Backend (Go):** transactional outbox, event catalog (consent.changed, ropa.updated, dsar.closed ฯลฯ), subscriber ภายใน, ส่งต่อ webhook / NATS

**Frontend (Next.js):** -

**Acceptance criteria:** event ไม่สูญหายเมื่อระบบล่มกลางคัน (at-least-once + idempotent consumer)

**หมายเหตุ:** ใช้เชื่อมโมดูลกัน เช่น ROPA-19, DSAR-21, DFG-02

**หมายเหตุจาก SA (ใช้แทนข้อความในแผนเมื่อขัดกัน):** dispatch แบบต่อ tenant: service enqueue River job `outbox.dispatch` (args: tenant_id) ใน tx เดียวกับ outbox · sweeper วน tenant เก็บรายการค้าง — ไม่ต้องใช้ BYPASSRLS (ดู integration.md)

<a id="plt-12"></a>
### PLT-12 โครงสร้าง Audit log

*Tamper-evident audit log*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE M (5 วัน) · FE - (0 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** event แบบ append-only + hash chain ต่อ tenant, before/after (JSON diff), ผู้ทำ, IP, user agent, partition รายเดือน, นโยบายระยะเวลาเก็บ

**Backend (Go):** event แบบ append-only + hash chain ต่อ tenant, before/after (JSON diff), ผู้ทำ, IP, user agent, partition รายเดือน, นโยบายระยะเวลาเก็บ

**Frontend (Next.js):** -

**Acceptance criteria:** แก้ log ย้อนหลังแล้วการตรวจ hash chain ล้มเหลว

**หมายเหตุ:** หน้าดู log คือ ORG-19

<a id="plt-13"></a>
### PLT-13 เข้ารหัสข้อมูลส่วนบุคคลระดับฟิลด์

*PII encryption & key management*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE M (5 วัน) · FE - (0 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** envelope encryption (DEK ต่อ tenant, KEK ใน KMS / OpenBao Transit), blind index (HMAC) สำหรับค้นหาอีเมล / เบอร์ / เลขบัตร, key rotation

**Backend (Go):** envelope encryption (DEK ต่อ tenant, KEK ใน KMS / OpenBao Transit), blind index (HMAC) สำหรับค้นหาอีเมล / เบอร์ / เลขบัตร, key rotation

**Frontend (Next.js):** -

**Acceptance criteria:** identifier ในฐานข้อมูลอ่านไม่ออกถ้าไม่มี key แต่ยังค้นหาแบบตรงตัวได้

<a id="plt-14"></a>
### PLT-14 Import framework (Excel / CSV)

*Bulk import framework*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-10
- **Process:** —

**คำอธิบาย:** import แบบ async: อ่าน Excel/CSV (excelize), map คอลัมน์, validate ทีละแถว, dry-run, รายงานข้อผิดพลาด, rollback

**Backend (Go):** import แบบ async: อ่าน Excel/CSV (excelize), map คอลัมน์, validate ทีละแถว, dry-run, รายงานข้อผิดพลาด, rollback

**Frontend (Next.js):** wizard นำเข้า: อัปโหลด → map คอลัมน์ → ตรวจ → ยืนยัน

**Acceptance criteria:** นำเข้า 50,000 แถวได้ และได้ไฟล์รายงานแถวที่ผิดพร้อมเหตุผล

**หมายเหตุ:** ใช้ร่วม ORG-08, ORG-09, ROPA-18, CON-22

<a id="plt-15"></a>
### PLT-15 Public API, webhook และ developer portal

*Public API & webhooks*

- **Priority / Phase:** Must · P0 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** IT (เจ้าของระบบ / IT), APICLIENT (ระบบภายนอก (API client))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** ORG-16, PLT-11
- **Process:** —

**คำอธิบาย:** versioning /v1, OpenAPI 3.1 + เอกสาร, rate limit ต่อ client (Valkey), idempotency key, webhook subscription + HMAC signature + retry / dead-letter

**Backend (Go):** versioning /v1, OpenAPI 3.1 + เอกสาร, rate limit ต่อ client (Valkey), idempotency key, webhook subscription + HMAC signature + retry / dead-letter

**Frontend (Next.js):** หน้า developer: เอกสาร API, จัดการ webhook และดู delivery log

**Acceptance criteria:** client เกิน rate limit ได้ 429; webhook ที่ล้มเหลวถูกส่งซ้ำตามนโยบาย

<a id="plt-16"></a>
### PLT-16 Document composer (template + clause + merge field)

*Document composer*

- **Priority / Phase:** Must · P1 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06, PLT-08
- **Process:** —

**คำอธิบาย:** เก็บเอกสารเป็นโครงสร้าง (ProseMirror JSON) + merge field จากองค์กร / RoPA / คู่ค้า, คลัง clause มีเวอร์ชัน, render HTML → PDF ผ่าน Gotenberg (ฟอนต์ไทย) และสร้าง DOCX, diff ระหว่างเวอร์ชัน

**Backend (Go):** เก็บเอกสารเป็นโครงสร้าง (ProseMirror JSON) + merge field จากองค์กร / RoPA / คู่ค้า, คลัง clause มีเวอร์ชัน, render HTML → PDF ผ่าน Gotenberg (ฟอนต์ไทย) และสร้าง DOCX, diff ระหว่างเวอร์ชัน

**Frontend (Next.js):** editor แบบ TipTap: บล็อก clause, merge field, สารบัญ, preview TH/EN, เปรียบเทียบเวอร์ชัน

**Acceptance criteria:** เอกสารภาษาไทยส่งออก PDF / Word ได้ตัวอักษรถูกต้อง และเปรียบเทียบสองเวอร์ชันได้

**หมายเหตุ:** ต้องเสร็จใน 2 sprint แรกของ P1; ใช้ร่วมประกาศ, หนังสือตอบคำขอ, แบบแจ้ง สคส., DPA, DSA

<a id="plt-17"></a>
### PLT-17 Public portal framework

*Public portal framework*

- **Priority / Phase:** Must · P1 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร)
- **ขนาดงาน:** BE M (5 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-02, IAM-05
- **Process:** —

**คำอธิบาย:** route สาธารณะแยก, CAPTCHA, session ของเจ้าของข้อมูล (OTP / magic link), CSP และ rate limit

**Backend (Go):** route สาธารณะแยก, CAPTCHA, session ของเจ้าของข้อมูล (OTP / magic link), CSP และ rate limit

**Frontend (Next.js):** apps/portal: theme ตามแบรนด์ tenant, custom domain, ISR/caching, WCAG 2.1 AA, TH/EN

**Acceptance criteria:** หน้า portal หลักผ่านการตรวจ accessibility ระดับ AA และโหลดเสร็จภายใน 2 วินาทีบน 4G

<a id="plt-18"></a>
### PLT-18 Dashboard & report framework

*Reporting framework*

- **Priority / Phase:** Must · P1 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE L (10 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-10, IAM-02
- **Process:** —

**คำอธิบาย:** query / aggregate API กรองตามสิทธิ์, export CSV / Excel (excelize) / PDF แบบ async + ศูนย์ดาวน์โหลด, ตั้งเวลาส่งรายงาน

**Backend (Go):** query / aggregate API กรองตามสิทธิ์, export CSV / Excel (excelize) / PDF แบบ async + ศูนย์ดาวน์โหลด, ตั้งเวลาส่งรายงาน

**Frontend (Next.js):** widget dashboard ปรับได้ (ECharts), ตัวกรองร่วม, data table (TanStack Table) + ปุ่ม export

**Acceptance criteria:** export 100,000 แถวแบบ async และแจ้งเมื่อไฟล์พร้อม; ตัวเลขตรงกับข้อมูลที่ผู้ใช้มีสิทธิ์เห็น

<a id="plt-19"></a>
### PLT-19 Observability

*Observability*

- **Priority / Phase:** Should · P1 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ระบบ / งานเทคนิค (ไม่มี actor โดยตรง)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-01
- **Process:** —

**คำอธิบาย:** OpenTelemetry (trace / metric / log) ใน Go, Prometheus + Grafana + Loki, alert rules

**Backend (Go):** OpenTelemetry (trace / metric / log) ใน Go, Prometheus + Grafana + Loki, alert rules

**Frontend (Next.js):** instrument Next.js (server + web vitals)

**Acceptance criteria:** มี dashboard สุขภาพระบบ และแจ้งเตือนเมื่อ error rate หรือ latency เกินเกณฑ์

**หมายเหตุ:** ดึงเข้า P1 เพื่อพร้อม go-live

<a id="plt-20"></a>
### PLT-20 Connector framework

*Connector framework*

- **Priority / Phase:** Should · P3 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-11
- **Process:** —

**คำอธิบาย:** Connector SDK ใน Go (test, discover, search subject, update / delete), credential เข้ารหัส, ตั้งเวลา; ชุดแรก PostgreSQL, MySQL, SQL Server, REST ทั่วไป

**Backend (Go):** Connector SDK ใน Go (test, discover, search subject, update / delete), credential เข้ารหัส, ตั้งเวลา; ชุดแรก PostgreSQL, MySQL, SQL Server, REST ทั่วไป

**Frontend (Next.js):** หน้าจัดการ connector + ปุ่มทดสอบการเชื่อมต่อ

**Acceptance criteria:** เพิ่ม connector ใหม่ได้โดยไม่แก้โมดูลหลัก และทดสอบการเชื่อมต่อจากหน้าจอได้

**หมายเหตุ:** ใช้ร่วม DSAR-09, DSAR-12, DFG-09, DPX-13

<a id="plt-21"></a>
### PLT-21 ค้นหาข้ามระบบ

*Global search*

- **Priority / Phase:** Should · P3 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** EMP (พนักงาน / ผู้ใช้งานทุกคน)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-02
- **Process:** —

**คำอธิบาย:** ค้นหาข้าม entity ด้วย PostgreSQL pg_trgm (รองรับภาษาไทย) กรองตามสิทธิ์; ทางเลือก OpenSearch + Thai analyzer เมื่อข้อมูลโต

**Backend (Go):** ค้นหาข้าม entity ด้วย PostgreSQL pg_trgm (รองรับภาษาไทย) กรองตามสิทธิ์; ทางเลือก OpenSearch + Thai analyzer เมื่อข้อมูลโต

**Frontend (Next.js):** ช่องค้นหากลาง (Cmd+K) + ผลลัพธ์แยกประเภท

**Acceptance criteria:** ผลค้นหาไม่แสดงข้อมูลที่ผู้ใช้ไม่มีสิทธิ์

<a id="plt-22"></a>
### PLT-22 AI gateway

*AI gateway*

- **Priority / Phase:** Nice · P4 · กลุ่ม: PLT
- **ที่มา:** เพิ่มโดย PM/SA
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** —
- **Actor:** ORGADMIN (ผู้ดูแลระบบขององค์กร), LLM (บริการ AI (LLM))
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-13
- **Process:** —

**คำอธิบาย:** abstraction ผู้ให้บริการ LLM (API หรือ on-prem ผ่าน vLLM), ปกปิด PII ก่อนส่ง, prompt template มีเวอร์ชัน, โควตา / ค่าใช้จ่ายต่อ tenant, log เพื่อ audit, RAG ด้วย pgvector

**Backend (Go):** abstraction ผู้ให้บริการ LLM (API หรือ on-prem ผ่าน vLLM), ปกปิด PII ก่อนส่ง, prompt template มีเวอร์ชัน, โควตา / ค่าใช้จ่ายต่อ tenant, log เพื่อ audit, RAG ด้วย pgvector

**Frontend (Next.js):** หน้าตั้งค่า AI ต่อ tenant (เปิด/ปิด, เลือก provider), ป้าย 'AI แนะนำ' + ปุ่มยืนยัน

**Acceptance criteria:** เมื่อเปิดโหมดปกปิด ไม่มีข้อมูลระบุตัวตนถูกส่งออกนอกระบบ; ผลจาก AI ต้องมีคนยืนยันก่อนบันทึก

**หมายเหตุ:** ใช้ร่วม DPIA-18, RTG-13, VEN-15, BRE-17, DPX-11
