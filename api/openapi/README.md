# API conventions (OpenAPI 3.1)

`openapi.yaml` คือสัญญา API ที่ใช้ generate Go handler (oapi-codegen strict server) และ TypeScript client (openapi-typescript) · ตอนนี้มี components กลางครบ + ตัวอย่าง 3 operation (`GET /admin/v1/me`, `GET /public/v1/collection-points/{key}`, `POST /api/v1/consents`) · เพิ่ม operation ทีละ feature ด้วย `/spec-api <ID>`

เมื่อไฟล์ใหญ่ขึ้นให้แยก `paths/<module>.yaml` และ `components/*.yaml` แล้ว bundle เป็นไฟล์เดียวก่อน codegen (เช่น Redocly CLI) — CI ต้อง validate spec ทุก PR

## Surface

| prefix | ผู้เรียก | security scheme | tenant มาจาก |
|---|---|---|---|
| `/admin/v1` | admin app ผ่าน BFF | `adminJwt` | claim `tid` |
| `/portal/v1` | เจ้าของข้อมูลหลังยืนยันตัวตน | `portalToken` | token |
| `/public/v1` | SDK / ฟอร์มสาธารณะ / portal ก่อนยืนยัน | ไม่มี (`security: []`) + CAPTCHA / rate limit | `platform.public_keys` |
| `/api/v1` | ระบบภายนอก | `clientCredentials` (OAuth2) | api client |
| `/scim/v2` | Directory / HRIS | `scimBearer` | token |
| `/webhooks/*` | ผู้ให้บริการภายนอก (callback) | `webhookSignature` | key ใน path |
| `/provider/v1` | provider console (SUPER, tenant พิเศษ `platform`) | `adminJwt` + MFA + IP allowlist | ไม่มี — เฉพาะตาราง platform / ข้อมูลกลาง (decisions Q-19) |

## Module prefix

path = `<surface>/<module prefix>/<resource>[/{id}[/<sub-resource | action>]]` · prefix ของ admin ตามตาราง · endpoint สาธารณะที่ SA กำหนดแล้วห้ามเปลี่ยนชื่อ

| epic | Go module | admin prefix | public / portal / api ที่กำหนดแล้ว |
|---|---|---|---|
| PLT | `platform/<service>` | `/admin/v1/platform` | — |
| IAM | `iam` | `/admin/v1/iam` · `/admin/v1/me` | `/public/v1/otp/send`<br>`/public/v1/otp/verify`<br>`/scim/v2/Users` |
| ORG | `org` | `/admin/v1/org` | — |
| CON | `consent` | `/admin/v1/consent` · `/admin/v1/cookie` | `/public/v1/collection-points/{key}`<br>`/public/v1/consents`<br>`/public/v1/consents/confirm`<br>`/public/v1/cookie/banners/{domainKey}`<br>`/public/v1/cookie/consents`<br>`/public/v1/preferences`<br>`/api/v1/consents` |
| PNG | `notice` | `/admin/v1/notices` | `/public/v1/notices/{slug}`<br>`/public/v1/notices/{id}/acknowledgements` |
| ROPA | `ropa` | `/admin/v1/ropa` | — |
| RTG | `ropa/templates` | `/admin/v1/ropa/templates` | — |
| DFG | `dataflow` | `/admin/v1/dataflow` | — |
| DSAR | `dsar` | `/admin/v1/dsar` | `/public/v1/dsar-requests`<br>`/public/v1/dsar-requests/{ref}/verify`<br>`/api/v1/dsar/requests` |
| BRE | `breach` | `/admin/v1/breach` | `/public/v1/breach-reports`<br>`/api/v1/breach/incidents` |
| DPO | `dpo` | `/admin/v1/dpo` | — |
| DPIA | `assess` | `/admin/v1/assessments` | `/public/v1/guest/{token}/assessment` |
| RRA | `risk` | `/admin/v1/risk` | — |
| VEN | `vendor` | `/admin/v1/vendors` | — |
| DPA | `agreement` | `/admin/v1/agreements` | — |
| DSA | `agreement` | `/admin/v1/agreements` | — |
| DPX | `gov` | `/admin/v1/governance` | — |

ข้อยกเว้นที่ตั้งใจ: `GET /admin/v1/me` (session bootstrap) · `/api/v1/consents` (ชื่อที่ระบบภายนอกใช้ง่าย)

## Resource และ operation

- resource เป็น kebab-case พหูพจน์ (`/ropa/activities`, `/cookie/domains/{id}/scans`) · path parameter `{id}` เป็น UUID · เลขอ้างอิงที่มนุษย์อ่าน (request_no, incident_no) เป็น field แยก
- การเปลี่ยนสถานะเป็น action: `POST /<resource>/{id}/<submit|approve|reject|publish|withdraw|close|…>` body มี `comment` / `reason` · x-permission ใช้ action ที่ตรงกัน (approve, publish, execute)
- `operationId` = `<module><Verb><Resource>` แบบ camelCase เช่น `ropaUpdateActivity`, `dsarApproveRequest` · `tags: [<module>]`
- ทุก operation ต้องมี `x-permission` และ `x-module` · x-permission = code จาก `docs/security/permissions.yaml` รูปแบบ `<area>.<resource>.<action>` (area ตาม RBAC เช่น `admin`, `assessment` ไม่ใช่ชื่อ package) · ค่าพิเศษ: `public`, `authenticated` (ต้อง login แต่ไม่ต้องมี permission เฉพาะ), `scim`, `webhook`
- OAuth2 scope ของ `/api/v1` = permission code เดียวกัน (เช่น `consent.record.create`) — ค่าเริ่มต้นตาม decisions Q-10
- งานใหญ่ / export / import เป็น async: `POST …/exports` → `202` + job id → `GET /admin/v1/platform/jobs/{id}` → ดาวน์โหลดผ่าน signed URL (SEQ-07)
- ไฟล์: `POST /admin/v1/platform/files` (multipart) → สแกน ClamAV → file id ใช้อ้างถึงใน resource อื่น · ดาวน์โหลดด้วย `GET /admin/v1/platform/files/{id}/download` → 302 signed URL อายุ 5 นาที

## Payload

- JSON field เป็น snake_case ตรงกับคอลัมน์ · ค่า enum ตรงกับ CHECK ใน DDL (consent ใช้ตัวใหญ่ตาม FSD V3.2)
- เวลาเป็น RFC 3339 UTC (`2026-09-25T03:15:00Z`) · วันที่อย่างเดียว `YYYY-MM-DD` · UI แปลงเป็น Asia/Bangkok และ พ.ศ.
- ข้อความหลายภาษาใช้คู่ `<field>_th` / `<field>_en` หรือ object `{th, en}` ตามคอลัมน์ต้นทาง · `Accept-Language: th|en` เลือกภาษาของข้อความ error และ field ที่แปลแล้ว
- ข้อมูลส่วนบุคคลใน response ของ admin เป็นแบบ mask (เช่น `email_masked`) · ค่าเต็มผ่าน endpoint unmask เท่านั้น
- field ที่เป็นความลับ / identifier ขาเข้าใช้ `writeOnly: true` และห้ามสะท้อนกลับใน error

## List

- `?limit=` (1–200, ค่าเริ่มต้น 50) + `?cursor=` → response `{ "data": [...], "next_cursor": "…" | null }` (ไม่ใช้ offset)
- `?sort=-created_at,name` · filter เป็น query parameter ชื่อตรงกับ field (`status`, `legal_entity_id`) · ค้นหาข้อความ `?q=` (pg_trgm)

## Concurrency และ idempotency

- GET resource ที่แก้ไขได้คืน `ETag: "<row_version>"` · PATCH / PUT / DELETE / action ต้องส่ง `If-Match` → ไม่ตรง 412 `conflict.version_mismatch` · ไม่ส่ง 428
- POST ใน `/public/v1` และ `/api/v1` ต้องมี `Idempotency-Key` (แนะนำให้ใช้กับ POST ของ admin ที่สร้างข้อมูลด้วย)
- เก็บใน Valkey 24 ชม. ที่ key `idem:{tenant}:{principal}:{key}` = สถานะ + hash ของ body + response: ทำงานอยู่ → 409 `idempotency.in_progress` · body ต่าง → 422 `idempotency.key_reused` · ซ้ำ → คืน **status และ body เดิม** (เช่น 201) + header `Idempotent-Replayed: true`
- header ที่ operation บังคับ (Idempotency-Key, If-Match) ประกาศ `required: true` ใน spec — error handler ของ request validator ต้องแปลง “missing required header” เป็น 428 `precondition.required`

## Error (RFC 9457 problem+json)

`Content-Type: application/problem+json` · field: `type`, `title`, `status`, `detail`, `instance`, **`code`** (คงที่ ใช้ตัดสินใจในโค้ด), `request_id`, `errors[]`

| status | code | ใช้เมื่อ |
|---|---|---|
| 400 | `request.invalid` | JSON / schema / format ไม่ถูกต้อง (มี `errors[]` ราย field) |
| 401 | `authn.required` | ไม่มีหรือ token ไม่ถูกต้อง / หมดอายุ |
| 403 | `authz.denied` | ไม่มี x-permission หรืออยู่นอก data scope (บันทึก security_events) |
| 404 | `not_found` | ไม่พบ หรือมองไม่เห็นใน tenant / scope นี้ (ห้ามบอกว่ามีอยู่แต่ไม่มีสิทธิ์) |
| 409 | `<module>.invalid_transition` | เปลี่ยนสถานะที่ state machine ไม่อนุญาต |
| 409 | `idempotency.in_progress` | request เดิมที่ใช้ Idempotency-Key นี้ยังทำงานอยู่ |
| 412 | `conflict.version_mismatch` | If-Match ไม่ตรง row_version ปัจจุบัน |
| 422 | `<module>.<rule>` | ผิดกฎธุรกิจ เช่น consent.purpose_inactive, dsar.identity_not_verified, breach.late_reason_required |
| 422 | `idempotency.key_reused` | ใช้ Idempotency-Key เดิมกับ body ที่ต่างไป |
| 423 | `<module>.verification_locked` | ยืนยัน OTP ผิดครบ 5 ครั้ง — ต้องยืนยันด้วยวิธีอื่น (SEQ-05) |
| 428 | `precondition.required` | ไม่ส่ง Idempotency-Key / If-Match ที่ operation บังคับ (request validator ต้อง map header ที่หายเป็น 428 ไม่ใช่ 400) |
| 429 | `rate_limited` | เกิน rate limit (มี Retry-After) |
| 500 | `internal` | ข้อผิดพลาดภายใน — ห้ามมีรายละเอียดภายในหรือ PII ใน detail |

## Rate limit

token bucket ใน Valkey ต่อ IP (public) · ต่อ client (api) · ต่อ user (admin) · ตอบ 429 + `Retry-After` และส่ง `RateLimit-Remaining`

## Versioning

เวอร์ชันใน path (`/v1`) · breaking change → `/v2` และคง `/v1` อย่างน้อย 12 เดือนพร้อม header `Deprecation` / `Sunset`

## Checklist ก่อน merge endpoint ใหม่

- มี `x-permission` ที่อยู่ใน `docs/security/permissions.yaml` (หรือเพิ่ม permission ใหม่พร้อม migration)
- มี error response ครบตามที่ operation คืนได้ + ตัวอย่าง request / response
- list มี pagination · update มี If-Match · POST สาธารณะ / api มี Idempotency-Key
- contract test: 401, 403, 400 / 422 และ happy path · spec validate ผ่าน · `make gen` ไม่มี diff
