# Integration

> ต้นฉบับ: ARC-04 ใน `design/PDPA_System_Analysis.drawio` · API conventions: [`api/openapi/README.md`](../../api/openapi/README.md)

## API surfaces

| prefix | ใช้กับ |
|---|---|
| `/public/v1` | SDK / portal แบบไม่ login · tenant จาก platform.public_keys · CORS allowlist · rate limit ต่อ IP |
| `/portal/v1` | เจ้าของข้อมูลหลังยืนยัน OTP / ThaID · session สั้น (idle 30 นาที) |
| `/admin/v1` | Admin app ผ่าน BFF · JWT ผู้ใช้ · x-permission + data scope |
| `/api/v1` | ระบบภายนอก · OAuth2 client credentials (token ≤ 15 นาที · scope) · IP allowlist · quota ต่อ client · หมุน secret 90 วัน |
| `/scim/v2` | Directory provisioning (Users / Groups) · bearer token ต่อ tenant |
| `/webhooks/*` | callback ขาเข้า (e-Sign, สถานะ email / SMS) · ตรวจ HMAC + timestamp ≤ 5 นาที |

## Endpoint ที่ SA กำหนดแล้ว

| method | path | epic | คำอธิบาย |
|---|---|---|---|
| GET | `/public/v1/collection-points/{key}` | CON | โหลด collection point + purpose เวอร์ชันล่าสุด (BP-01) |
| POST | `/public/v1/consents` | CON | บันทึกความยินยอมจากเว็บ / แอป (Idempotency-Key) (BP-01) |
| POST | `/public/v1/consents/confirm` | CON | ยืนยัน double opt-in (BP-01) |
| GET | `/public/v1/cookie/banners/{domainKey}` | CON | config แบนเนอร์ผ่าน CDN (SEQ-03) |
| POST | `/public/v1/cookie/consents` | CON | หลักฐานความยินยอมคุกกี้ (SEQ-03) |
| POST | `/public/v1/otp/send` | IAM | ส่ง OTP ให้เจ้าของข้อมูล (BP-02) |
| POST | `/public/v1/otp/verify` | IAM | ยืนยัน OTP → portal session (BP-02) |
| GET | `/public/v1/preferences` | CON | preference center (session ของ portal) (BP-02) |
| GET | `/public/v1/notices/{slug}` | PNG | ประกาศแบบ hosted / embed (BP-04) |
| POST | `/public/v1/notices/{id}/acknowledgements` | PNG | บันทึกการรับทราบประกาศ (BP-04) |
| POST | `/public/v1/dsar-requests` | DSAR | ยื่นคำขอใช้สิทธิ (SEQ-05) |
| POST | `/public/v1/dsar-requests/{ref}/verify` | DSAR | ยืนยัน OTP ของคำขอ (SEQ-05) |
| POST | `/public/v1/breach-reports` | BRE | ฟอร์มแจ้งเหตุสาธารณะ + CAPTCHA (BP-07) |
| GET | `/public/v1/guest/{token}/assessment` | DPIA | แบบประเมินผ่าน guest link (BP-08 / BP-09) |
| POST | `/api/v1/consents` | CON | ระบบช่องทางส่ง consent (OAuth2 scope consent.record.create) (SEQ-04) |
| POST | `/api/v1/dsar/requests` | DSAR | รับคำขอจาก CRM / call center (BP-06) |
| POST | `/api/v1/breach/incidents` | BRE | SIEM ส่งเหตุเข้า (BP-07) |
| POST | `/scim/v2/Users` | IAM | SCIM provisioning (BP-12) |
| GET | `/admin/v1/me` | IAM | โปรไฟล์ + สิทธิ์ + scope ของผู้ใช้ปัจจุบัน (SEQ-01) |
| POST | `/admin/v1/iam/access-requests` | IAM | ขอสิทธิ์เพิ่ม (BP-12) |
| POST | `/admin/v1/ropa/activities` | ROPA | สร้างกิจกรรม RoPA (BP-05) |
| PATCH | `/admin/v1/ropa/activities/{id}` | ROPA | แก้ไขกิจกรรม (If-Match) (SEQ-02) |
| POST | `/admin/v1/ropa/activities/{id}/submit` | ROPA | ส่งอนุมัติ (BP-05) |
| POST | `/admin/v1/ropa/imports` | ROPA | นำเข้า RoPA จาก Excel (BP-05) |
| POST | `/admin/v1/cookie/domains/{id}/scans` | CON | สั่งสแกนเว็บไซต์ (BP-03) |
| POST | `/admin/v1/assessments` | DPIA | สร้างแบบประเมิน (BP-08) |
| POST | `/admin/v1/assessments/{id}/submit` | DPIA | ส่งตรวจ (BP-08) |
| POST | `/admin/v1/vendors/intakes` | VEN | คำขอรับคู่ค้าใหม่ (BP-09) |
| POST | `/admin/v1/agreements` | DPA | สร้างข้อตกลง DPA / DSA (BP-10) |
| POST | `/admin/v1/agreements/{id}/render` | DPA | สร้างเอกสาร PDF / DOCX (SEQ-07) |
| GET | `/admin/v1/platform/files/{id}/download` | PLT | ดาวน์โหลดผ่าน signed URL (SEQ-07) |

## Outbox → webhook

- service เขียน `platform.outbox_events` ใน transaction เดียวกับข้อมูล (aggregate_type, aggregate_id, event_type, payload)
- ใน transaction เดียวกัน service enqueue River job `outbox.dispatch` (args: `tenant_id`) — worker เปิด `WithTenantTx(tenant)` แล้ว `SELECT … FOR UPDATE SKIP LOCKED` จาก outbox ของ tenant นั้น → สร้าง `platform.webhook_deliveries` ต่อ subscription ที่สมัคร event · ไม่ต้องใช้ role BYPASSRLS
- sweeper รายนาทีวน `platform.tenants` แล้ว dispatch รายการที่ค้าง (เช่น job ล้มเหลว) · ลำดับรับประกันภายใน aggregate เดียวกัน
- envelope: `{id (UUIDv7), type, tenant_id, occurred_at, subject, data, version}` · payload ไม่ใส่ PII เกินจำเป็น (ใช้ subject_ref)
- header: `X-Event-Id`, `X-Event-Type`, `X-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, t + '.' + body)>` · secret อยู่ใน OpenBao (`webhook_subscriptions.secret_ref`)
- retry: 1 นาที → 5 นาที → 30 นาที → 2 ชม. → 6 ชม. → 24 ชม. แล้ว `dead` + แจ้งเตือน + เข้าคิว reconcile · replay ด้วยมือต้องมี `admin.apiclient.update` และถูก audit (ST-07)
- ผู้รับต้อง idempotent ตาม `X-Event-Id` และปฏิเสธ timestamp เก่ากว่า 5 นาที
- breaking change ของ payload → เพิ่ม `version` ใหม่ และคงเวอร์ชันเดิมอย่างน้อย 12 เดือน

## Event catalog

ชื่อ `<domain>.<past-tense>` · machine-readable: [`events.yaml`](events.yaml)

| event | producer | ฟิลด์หลักใน data | consumer |
|---|---|---|---|
| `consent.granted` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.denied` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.withdrawn` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.expired` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.preferences_changed` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.mismatch_found` | consent | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `cookie.banner_published` | cookie | domain · banner_version · new_cookies | DPO dashboard · CDN purge |
| `cookie.scan_completed` | cookie | domain · banner_version · new_cookies | DPO dashboard · CDN purge |
| `notice.published` | notice | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |
| `notice.material_change` | notice | notice_id · version · effective_at | acknowledgement job · เว็บไซต์ (embed) |
| `ropa.activity_submitted` | ropa | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |
| `ropa.activity_approved` | ropa | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |
| `ropa.activity_changed` | ropa | activity_id · version · changed_fields | risk scoring · dataflow snapshot · notice links |
| `risk.dpia_required` | risk / assess | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `risk.accepted` | risk / assess | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `dpia.submitted` | risk / assess | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `dpia.approved` | risk / assess | activity_id · score · assessment_id | assess (สร้าง screening) · RoPA |
| `dsar.created` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.verified` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.subtask_assigned` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.sla_warning` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.completed` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `dsar.rejected` | dsar | request_ref · request_type · due_at · status | ITSM / CRM (subtask) · DPO |
| `breach.reported` | breach | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.assessed` | breach | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.pdpc_notified` | breach | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.subjects_notified` | breach | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `breach.closed` | breach | incident_ref · aware_at · risk_level · deadline_at | SIEM / ITSM · ผู้บริหาร |
| `vendor.assessment_completed` | vendor | vendor_id · tier · expires_at | จัดซื้อ · agreement |
| `vendor.approved` | vendor | vendor_id · tier · expires_at | จัดซื้อ · agreement |
| `vendor.certificate_expiring` | vendor | vendor_id · tier · expires_at | จัดซื้อ · agreement |
| `agreement.signed` | agreement | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.expiring` | agreement | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `agreement.terminated` | agreement | agreement_id · type · end_date | ฝ่ายกฎหมาย · vendor |
| `retention.due` | gov | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |
| `disposal.approved` | gov | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |
| `disposal.completed` | gov | rule_id · record_ref · due_at | IT (ทำลายข้อมูล) · DPO |
| `user.provisioned` | iam | user_id · role_code · scope | audit · SIEM |
| `user.deprovisioned` | iam | user_id · role_code · scope | audit · SIEM |
| `access.granted` | iam | user_id · role_code · scope | audit · SIEM |
| `access.revoked` | iam | user_id · role_code · scope | audit · SIEM |

## Background jobs (River)

| job | module | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|---|
| `outbox.dispatch` | platform | enqueue ใน tx เดียวกับ outbox (args: tenant_id) + sweeper รายนาที | อ่าน outbox_events ของ tenant (FOR UPDATE SKIP LOCKED) → สร้าง webhook_deliveries | SEQ-04 |
| `webhook.deliver` | platform | ตาม next_retry_at | POST webhook + HMAC · retry 1m, 5m, 30m, 2h, 6h, 24h → dead | ST-07 |
| `partition.maintain` | platform | รายวัน | `SELECT platform.ensure_monthly_partitions(3)` — partition รายเดือนล่วงหน้า 3 เดือนของ audit_log, security_events, consent_transactions, cookie consent_records (migration 00020) | ARC-06 |
| `consent.expiry_sweep` | consent | River cron รายวัน | ACTIVE ที่ครบ expires_at → EXPIRED + event consent.expired | ST-01 / BP-02 |
| `consent.reconcile` | consent | ตามรอบ / หลัง webhook dead | เทียบสถานะกับระบบปลายทาง → consent.mismatch_found | BP-02 |
| `cookie.scan` | cookie | ตามรอบของโดเมน / สั่งเอง | scanner (chromedp) crawl เว็บ → cookie.scan_completed | BP-03 |
| `notice.indirect_due` | notice | รายวัน | แจ้งเตือนก่อนครบ 30 วันของการแจ้งตาม ม.25 | BP-04 |
| `dsar.sla_timer` | dsar | รายชั่วโมง | ตรวจ due_at → dsar.sla_warning / escalate | BP-06 |
| `breach.sla_timer` | breach | T+24 / 48 / 66 ชม. | เตือนก่อนครบ 72 ชม. นับจาก aware_at | BP-07 / SEQ-06 |
| `retention.sweep` | gov | River cron 02:00 | หา record ที่ครบ retention → retention.due | BP-11 |
| `access_review.schedule` | iam | ตาม access_review_months (ค่าเริ่มต้น 6 เดือน) | สร้าง access_reviews + items และ auto-revoke เมื่อครบกำหนด | BP-12 |
| `docs.render` | platform | on demand | render HTML/DOCX → PDF ผ่าน Gotenberg → เก็บ object storage | SEQ-07 |

## Connector และบริการภายนอก

| ระบบ | ทิศทาง | รายละเอียด |
|---|---|---|
| CRM / POS / CDP | ขาเข้า + ขาออก | ส่ง consent ผ่าน /api/v1 · รับ event consent.* ผ่าน webhook · reconcile รายรอบ |
| ITSM / CRM (DSAR) | ขาออก | สร้าง subtask / ticket ให้ระบบต้นทางค้นหา / ลบข้อมูล (dsar.subtask_assigned) |
| SIEM | ขาเข้า + ขาออก | รับเหตุผ่าน /api/v1/breach/incidents · ส่ง audit_log + security_events (OTLP / syslog) |
| HRIS / Directory | ขาเข้า | SCIM 2.0 (joiner / mover / leaver) · ปิดบัญชีภายใน 24 ชม. |
| Email · SMS · LINE OA | ขาออก | template ต่อ tenant (platform.notification_templates) · สถานะการส่งกลับทาง /webhooks |
| e-Signature | ขาออก + callback | ส่งเอกสาร DPA / DSA · รับ callback เมื่อลงนาม (ตรวจ HMAC) |
| Keycloak / IdP / ThaID | ขาเข้า | OIDC / SAML federation · ThaID ผ่าน identity brokering |
| Data sources (discovery) | ขาออก (read-only) | credential ใน OpenBao · เก็บเฉพาะ metadata + ตัวอย่างที่ mask แล้ว |
| LLM (ผ่าน AI gateway) | ขาออก | mask PII ก่อนส่ง · quota และ log ต่อ tenant · รองรับ LLM on-prem |
| Gotenberg | ภายใน | HTML / DOCX → PDF ฟอนต์ไทย (SEQ-07) |
