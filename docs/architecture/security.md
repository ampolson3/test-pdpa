# Security architecture

> ต้นฉบับ: ARC-03 ใน `design/PDPA_System_Analysis.drawio` และชีต Security ของ Development Plan · กฎหมาย: ม.37(1) + ประกาศมาตรการรักษาความมั่นคงปลอดภัย พ.ศ. 2565 · เป้าหมาย OWASP ASVS ระดับ 2

## นโยบายความปลอดภัย (ค่าที่ต้อง implement)

| หัวข้อ | นโยบาย |
|---|---|
| Authentication | Keycloak เป็นผู้ยืนยันตัวตน (OIDC); Next.js ใช้รูปแบบ BFF เก็บ session ใน cookie แบบ httpOnly + Secure + SameSite และไม่เก็บ token ใน localStorage |
| รหัสผ่าน | ยาวอย่างน้อย 12 ตัวอักษร ห้ามซ้ำ 5 ครั้งล่าสุด ตั้งค่าได้ต่อ tenant (password policy ของ Keycloak) |
| Lockout | ผิดครบ 5 ครั้ง ล็อก 30 นาที (brute-force detection ของ Keycloak; ตั้งค่าได้) |
| MFA | TOTP หรือ WebAuthn/passkey; บังคับสำหรับ Super Admin, Org Admin, DPO, Privacy, Security, Auditor; ขอ MFA ซ้ำ (step-up) ก่อน export จำนวนมาก, unmask และเปลี่ยน role |
| Session | idle timeout 30 นาที, อายุสูงสุด 12 ชั่วโมง, refresh token rotation, back-channel logout เมื่อบัญชีถูกปิด |
| SSO | OIDC/SAML brokering (Entra ID, Google, ADFS), LDAP/AD federation, map group → role/scope, JIT provisioning; เมื่อบังคับ SSO บัญชี local ใช้ได้เฉพาะ break-glass |
| API | OAuth2 client credentials, access token อายุไม่เกิน 15 นาที, scope ต่อ client, IP allowlist, rate limit, หมุน secret ทุก 90 วัน; ทางเลือก mTLS สำหรับ on-prem |
| Authorization | deny by default; ทุก endpoint ประกาศ x-permission ใน OpenAPI และมี contract test ตรวจ 403; กรองข้อมูลตาม scope ใน repository + PostgreSQL RLS แยก tenant |
| Segregation of duties | ผู้สร้างกับผู้อนุมัติต้องต่างคน: เผยแพร่ประกาศ/แบนเนอร์/Purpose, ปฏิเสธคำขอ, ส่งแบบแจ้ง สคส., export ความยินยอมจำนวนมาก, เปลี่ยน role ระดับ admin |
| PII protection | เข้ารหัส identifier ระดับฟิลด์ (envelope encryption), ปกปิดบนหน้าจอเป็นค่าเริ่มต้น, unmask ต้องระบุเหตุผล, ห้ามมี PII ใน log (redaction middleware) |
| Guest access | magic link ผูกงานเดียว หมดอายุ 7-30 วัน ยืนยันด้วย OTP ทางอีเมล และเพิกถอนได้ทันที |
| เจ้าของข้อมูล | OTP หมดอายุ 5 นาที จำกัด 5 ครั้ง, rate limit ต่อ IP และ identifier, CAPTCHA บนฟอร์มสาธารณะ |
| Audit | บันทึก login, เปลี่ยนสิทธิ์, export, unmask, ลบ และการอนุมัติ แบบ hash chain; ส่งต่อ SIEM ได้; เก็บตามนโยบายองค์กร (ข้อมูลจราจรทางคอมพิวเตอร์อย่างน้อย 90 วัน) |
| Access review | ทบทวนสิทธิ์ทุก 6 เดือน (ตั้งค่าได้); สิทธิ์ที่ไม่ได้รับการยืนยันถูกถอน |
| Offboarding | ปิดบัญชีภายใน 24 ชั่วโมงหลังพ้นสภาพ (อัตโนมัติผ่าน SCIM / HRIS เมื่อเชื่อมแล้ว) |
| Frontend security | CSP แบบเข้ม, CSRF protection ของ BFF, sanitize HTML ของเนื้อหา rich text (ประกาศ/สัญญา) ก่อนแสดง |

## Control matrix

| ชั้น | การควบคุม | ตาราง / หลักฐาน |
|---|---|---|
| Identity | Keycloak OIDC + PKCE · MFA (TOTP / WebAuthn) บังคับสำหรับ Super Admin, Org Admin, DPO, Privacy, Security, Auditor + step-up ก่อน unmask / export จำนวนมาก / เปลี่ยน role · SSO SAML/OIDC ขององค์กร · SCIM · guest token (hash, หมดอายุ 7–30 วัน, ผูกงานเดียว) · OTP เจ้าของข้อมูล (hash, 5 นาที, 5 ครั้ง) | iam.users · idp_configs · guest_tokens · subject_verifications |
| Authorization | RBAC + data scope (tenant / legal entity / org unit / self) · x-permission ต่อ endpoint · PostgreSQL RLS ต่อ tenant · field masking + unmask log · maker-checker · break-glass มีวันหมดอายุ | iam.roles · permissions · role_assignments · field_masking_rules · unmask_logs |
| Data protection | TLS 1.2+ ทุกเส้น · AES-256-GCM envelope encryption สำหรับฟิลด์ PII (*_enc) · blind index HMAC-SHA256 · hash IP / UA · backup เข้ารหัส · signed URL หมดอายุ | consent.subject_identifiers · platform.files |
| Application | OWASP ASVS L2 · validate ตาม OpenAPI · CSP / HSTS / X-Frame-Options · CSRF (BFF) · rate limit (Valkey token bucket) · Idempotency-Key · สแกนไฟล์ ClamAV + ตรวจ MIME · SSRF guard สำหรับ scanner / webhook | platform.files · webhook_subscriptions |
| Audit & monitoring | audit_log append-only + hash chain (partition รายเดือน, ห้าม UPDATE/DELETE) · security_events · ส่งออก SIEM (OTLP / syslog) · alert: login ผิดปกติ, unmask / export จำนวนมาก | platform.audit_log ⟨P⟩ · iam.security_events ⟨P⟩ |
| Supply chain & ops | SBOM + cosign · govulncheck / osv-scanner / semgrep ใน CI · secret อยู่ใน OpenBao เท่านั้น · least-privilege service account · NetworkPolicy default-deny | — |

## Principal และการหา tenant ต่อ surface

| surface | principal | ยืนยันตัวตน | tenant มาจาก |
|---|---|---|---|
| /admin/v1 | user | JWT (Keycloak) ที่ BFF แนบมา · ตรวจ iss/aud/exp ด้วย JWKS | claim `tid` ใน token |
| /api/v1 | api_client | OAuth2 client credentials (token ≤ 15 นาที) + IP allowlist + rate limit | api_clients ของ client_id (claim `tid`) |
| /portal/v1 | data_subject | portal session หลัง OTP / ThaID (idle 30 นาที) | session (ผูก tenant ตอนยืนยัน) |
| /public/v1 | anonymous | ไม่มี — CAPTCHA บนฟอร์ม, rate limit ต่อ IP + identifier, ตรวจ Origin กับ allowed_origins | `platform.public_keys` (key ใน path / header / Host ของ portal) |
| guest link | guest | token (hash) ผูกงานเดียว + OTP อีเมล · หมดอายุ 7–30 วัน · เพิกถอนได้ | `iam.guest_tokens` ของงานนั้น |
| /scim/v2 | directory | bearer token ต่อ tenant | token |
| /webhooks/* | provider | HMAC + timestamp ≤ 5 นาที ต่อ provider | key ใน path ที่ map กับ tenant |
| /provider/v1 | platform operator (SUPER) | JWT ของ tenant พิเศษ `platform` + MFA + IP allowlist | ไม่มี tenant context — เข้าถึงเฉพาะตาราง platform / ข้อมูลกลาง (decisions Q-19) |

ลำดับใน request: authn → resolve tenant → authz (x-permission + data scope, จาก cache) → idempotency → Tx (`WithTenantTx`) → handler / service → audit + outbox → `COMMIT` (ดู [SEQ-02](../sequences/SEQ-02.md) และ [code-structure.md](code-structure.md#layer-ภายใน-module))

**Keycloak (ค่าเริ่มต้น รอยืนยันใน PoC T13 / decisions Q-18):** realm เดียว `pdpa` + Keycloak Organizations 1 องค์กรต่อ tenant (`platform.tenants.keycloak_org_id`) · protocol mapper ใส่ claim `tid` (tenant id) ใน access token · IdP ของลูกค้า (SAML / OIDC / LDAP) ผูกกับ organization · client credentials ของระบบภายนอกเป็น client ใน realm เดียวกันพร้อม attribute `tid`

## Database roles และ RLS

- ทุกตารางธุรกิจ `ENABLE` + `FORCE ROW LEVEL SECURITY` · policy อ่านค่า `app.tenant_id` ด้วย `NULLIF(current_setting('app.tenant_id', true), '')::uuid`
- แอปต้องตั้ง `app.tenant_id` (และ `app.user_id`) แบบ transaction-local ต้น **ทุก** transaction ด้วย `SELECT set_config('app.tenant_id', $1, true)` (รูปแบบของ SET LOCAL ที่รับ bind parameter ได้) — ใช้ helper กลาง `WithTenantTx` ใน `internal/pkg/db` ห้ามเขียนเอง
- ห้ามใช้ superuser หรือ role ที่ BYPASSRLS นอก package `internal/platform/provider` · job ข้าม tenant ใน worker ให้วน `platform.tenants` (อ่านได้) แล้วทำงานทีละ tenant ด้วย `WithTenantTx` ของ `pdpa_app`
- partition: policy ของตารางแม่ไม่ถูกใช้เมื่อ query partition ตรง ๆ → ทุก partition มี RLS ของตัวเอง และแอปไม่มีสิทธิ์ตรงกับ partition (เข้าผ่านตารางแม่เท่านั้น) · partition ใหม่สร้างด้วย `platform.ensure_monthly_partitions()` (SECURITY DEFINER, migration 00020) ซึ่งตั้ง RLS และ revoke สิทธิ์ให้เอง
- FK constraint ไม่ผ่าน RLS: การอ้าง id ของ tenant อื่นจะผ่าน constraint ได้ → service ต้องตรวจว่าแถวที่อ้างถึงมองเห็นได้ภายใต้ RLS ก่อนเขียน (ดู code-structure)
- ตาราง global ไม่มี RLS: `platform.tenants`, `iam.permissions`, `org.lawful_bases`, `org.countries`, `cookie.cookie_kb`, `agreement.mandatory_rules`, `gov.regulatory_updates` — แอปอ่านอย่างเดียว (ดูแลผ่าน provider console ด้วย `pdpa_platform` หรือ migration)
- `platform.public_keys`: RLS แบบพิเศษ — `public_read` ให้ทุก request อ่านได้เพื่อหา tenant ก่อน `SET LOCAL` · `tenant_write` ให้เขียนเฉพาะ key ของ tenant ตัวเอง · key สุ่ม ≥ 128 bit, หมุน / เพิกถอนได้
- ข้อมูลกลาง (tenant_id NULL) ในตาราง template / master: tenant อ่านได้ เขียนไม่ได้ (policy `tenant_read` / `tenant_write`)
- test ภาคบังคับ: integration test ที่สร้าง 2 tenant และยืนยันว่าอ่าน/เขียนข้ามกันไม่ได้ในทุก repository ใหม่

### Database roles

| role | คุณสมบัติ | ใช้โดย |
|---|---|---|
| `pdpa_owner` | NOLOGIN · เจ้าของ database / schema / ตาราง / function ทั้งหมด | — |
| `pdpa_migrator` | LOGIN · สมาชิกของ pdpa_owner · เชื่อมต่อด้วย `options=-c role=pdpa_owner` เพื่อให้ object ใหม่เป็นของ owner | `cmd/migrate` (goose + River migrations + `10-grants.sql`) |
| `pdpa_app` | LOGIN · NOBYPASSRLS · DML เท่านั้น · ไม่มีสิทธิ์ตรงกับ partition · append-only / global table อ่านอย่างเดียว | `cmd/api` และ job ต่อ tenant ใน `cmd/worker` |
| `pdpa_platform` | LOGIN · BYPASSRLS · ใช้ได้เฉพาะ package `internal/platform/provider` (pool แยก) | provider console `/provider/v1`: tenant, แพ็กเกจ, ข้อมูลกลาง |
| `pdpa_readonly` | LOGIN · NOBYPASSRLS · ยังไม่มีสิทธิ์ตาราง (ให้ผ่าน view ที่ mask แล้วเมื่อทำรายงาน) | BI / รายงาน |

ขั้นตอน (ทดสอบแล้วบน PostgreSQL 16 ด้วย role จริง ไม่ใช่ superuser):

1. `deploy/db/00-bootstrap.sql` — superuser รันครั้งเดียว: สร้าง role, database (owner = pdpa_owner), extension (pgvector ต้องใช้ superuser)
2. migrate — `goose` ด้วย `pdpa_migrator` + `options=-c role=pdpa_owner` แล้ว River migrations
3. `deploy/db/10-grants.sql` — รันทุกครั้งหลัง migrate (idempotent): DML ให้แอป, revoke append-only / global / partition, default privileges สำหรับตารางในอนาคต
4. แอปเชื่อมต่อด้วย `pdpa_app` · provider console ด้วย `pdpa_platform` (pool แยก)

## ลำดับชั้นกุญแจ (envelope encryption)

| ชั้น | รายละเอียด |
|---|---|
| Root key | KMS / HSM ของ cloud หรือ on-prem HSM — ไม่ออกจาก HSM |
| KEK ต่อ tenant | OpenBao transit · หมุนทุก 12 เดือน (rewrap DEK ไม่ต้องเข้ารหัสข้อมูลใหม่) |
| DEK ต่อ tenant + ประเภทข้อมูล | เก็บแบบห่อ (wrapped) ใน DB · cache ใน memory จำกัดเวลา |
| ฟิลด์ *_enc | AES-256-GCM · เช่น value_enc, birth_date_enc, requester_contact_enc |
| Blind index key | HMAC-SHA256 key แยกต่อ tenant · ใช้ค้นหาแบบตรงตัวโดยไม่ถอดรหัส (normalize ก่อน: lower-case อีเมล, E.164 เบอร์โทร) |
| ลบ tenant | ทำลาย KEK = crypto-shredding |

## PII ในโค้ดและ log

- ห้าม log ค่า PII / token / OTP / secret — log ได้เฉพาะ id, subject_ref, request_id · middleware redaction + linter ตรวจ key ต้องห้าม (email, phone, national_id, name ฯลฯ)
- response ของ admin คืนค่า identifier แบบ mask ตาม `iam.field_masking_rules` เป็นค่าเริ่มต้น · ค่าเต็มผ่าน endpoint unmask ที่ต้องมี `pii.unmask.execute` + เหตุผล + step-up MFA
- ไฟล์ export / แพ็กเกจ DSAR เข้ารหัสและหมดอายุ · ดาวน์โหลดผ่าน signed URL อายุสั้น (SEQ-07)
- webhook payload ส่ง `subject_ref` แทน PII — ปลายทางเรียก API กลับเมื่อจำเป็น
- AI gateway ต้อง mask PII ก่อนส่ง LLM และเก็บ log prompt / token ต่อ tenant
- ข้อมูลใน dev / sit เป็นข้อมูลสังเคราะห์ · uat ใช้ข้อมูลที่ผ่าน masking (`gov.masking_jobs`) เท่านั้น

## Audit (สิ่งที่ต้องบันทึก)

- login / logout / login_failed / lockout / MFA (iam.security_events)
- เปลี่ยน role / scope / สิทธิ์ชั่วคราว / break-glass · export · unmask · ลบ · อนุมัติ / ปฏิเสธ / publish
- platform.audit_log: actor_type, actor_id, action, entity_type, entity_id, before/after (mask PII), ip, user_agent, prev_hash → hash
- ห้าม UPDATE / DELETE (สิทธิ์ DB + test) · ส่งต่อ SIEM ได้ · เก็บตามนโยบายองค์กร (ข้อมูลจราจรทางคอมพิวเตอร์อย่างน้อย 90 วัน)
- โค้ด (PLT-12): `audit.Service.Write` (ใช้ `audit.Changes` ลด before/after เหลือเฉพาะฟิลด์ที่เปลี่ยน) · `Verify` + job `audit.verify` รายวัน · รายละเอียด hash ใน `docs/data/platform.md#platform-audit-log` · ยังไม่ทำ: ลบ partition ตามระยะเวลาเก็บ (รอกำหนดนโยบาย) และ IP จริงหลัง load balancer (ต้องกำหนด trusted proxy)

## Threat model

ทำ STRIDE ใน T14 (P0) โดยเริ่มจากจุดเสี่ยง: การรั่วข้าม tenant, public API (enumeration / abuse), guest link, webhook (SSRF / replay), cookie scanner (SSRF, เว็บที่เป็นอันตราย), ไฟล์อัปโหลด, AI (prompt injection / PII leakage), สิทธิ์ผู้ดูแลแพลตฟอร์ม (break-glass)
