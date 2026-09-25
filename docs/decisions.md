# Decisions log

เอกสารนี้มีสองส่วน: (1) สิ่งที่ **ตัดสินแล้ว** ตอนรวมชุด SA กับ Development Plan เข้าเป็นชุดความรู้นี้ และ (2) คำถามที่ **ยังเปิดอยู่** ซึ่งต้องให้ลูกค้า / ฝ่ายกฎหมาย / สถาปนิกตอบ

**กติกาสำหรับ Claude Code:** งานที่แตะหัวข้อในส่วนที่ 2 ห้ามเดาค่าเอง — ทำให้เป็น config ที่มีค่าเริ่มต้นตามที่เขียนไว้ และแจ้งผู้ใช้ หรือหยุดถาม · เมื่อผู้ใช้ตอบแล้วให้ย้ายหัวข้อมาไว้ส่วนที่ 1 พร้อมวันที่

## 1. ตัดสินแล้ว (แก้ได้ด้วย ADR ใหม่เท่านั้น)

| # | เรื่อง | ข้อตัดสิน | เหตุผล / ที่มา |
|---|---|---|---|
| D-01 | ค่าความปลอดภัย | ใช้ค่าจากนโยบายใน Development Plan: session idle 30 นาที / สูงสุด 12 ชม. · OTP 5 นาที / 5 ครั้ง · lockout 5 ครั้ง 30 นาที · ทบทวนสิทธิ์ทุก 6 เดือน · ปิดบัญชีภายใน 24 ชม. · API token ≤ 15 นาที · หมุน secret 90 วัน | ชุด SA เดิมใช้ 8 ชม. / OTP 10 นาที / ทบทวนรายไตรมาส — แก้ใน draw.io (SEQ-01, SEQ-05, ARC-03, BP-12) ให้ตรงแล้ว |
| D-02 | โครง repository | monorepo ตาม REPO ของแผน + เพิ่ม `packages/authz` · Go อยู่ใน `backend/` | SA เดิมแยก `web/` กับ `pdpa-platform/` — ARC-05 แก้แล้ว |
| D-03 | API prefix | surface ตาม SA: `/public/v1`, `/portal/v1`, `/admin/v1`, `/api/v1`, `/scim/v2`, `/webhooks/*` แทน `/v1` และ `/p/v1` ในแผน · module prefix ตาม MODMAP (เช่น `/admin/v1/ropa`, `/admin/v1/assessments`) | แยกนโยบาย auth / rate limit / CORS ต่อ surface ได้ชัด |
| D-04 | ชื่อ Go package | ชื่อเดียวกับ schema: `assess` (แผนเขียน assessment), `gov` (แผนเขียน governance) | 1 module = 1 schema = 1 package |
| D-05 | ชื่อตาราง | ยึด ERD / migration · ชื่อในคอลัมน์ “ตาราง” ของแผนเป็นชื่อโดยประมาณ | DDL ผ่านการทดสอบบน PostgreSQL แล้ว |
| D-06 | Permission | `x-permission` = code ใน `docs/security/permissions.yaml` รูปแบบ `<area>.<resource>.<action>` (area ตามชีต Roles_Permissions เช่น admin / assessment / dpx — ไม่ใช่ชื่อ Go package) · catalogue มาจาก yaml → migration (ไม่ sync จาก OpenAPI) · replay webhook ใช้ `admin.apiclient.update` | ไม่มี role ใดมี X ของ `admin.apiclient` ใน matrix · SEQ-02 แก้ note แล้ว |
| D-07 | Event catalog | รวม event จาก BP ทุกหน้าเข้า ARC-04 · ยกเลิก `consent.updated` (ใช้ event ตาม transaction_type) · `reconcile.mismatch_found` → `consent.mismatch_found` · `dpia.required` → `risk.dpia_required` | ชื่อซ้ำ / ไม่สม่ำเสมอระหว่าง BP, SEQ และ ARC |
| D-08 | ระบบ Consent เดิม (.NET, FSD V3.2) | ใช้เป็นต้นแบบ domain model และ test case + ย้ายข้อมูล (T03, T26) · ไม่นำโค้ดมาใช้ | Development Plan |
| D-09 | RLS | `SET LOCAL app.tenant_id` ทุก transaction · policy ใช้ `NULLIF(current_setting('app.tenant_id', true), '')::uuid` · `FORCE ROW LEVEL SECURITY` · role `pdpa_app` ไม่มี BYPASSRLS | ทดสอบแล้ว: connection pool ที่เคยตั้งค่าไม่ error และไม่เห็นข้อมูล (ADR-18) |
| D-10 | tenant ของ request สาธารณะ | ตารางใหม่ `platform.public_keys` (RLS: อ่านได้ทุก request, เขียนได้เฉพาะ tenant) → `SET LOCAL` | SA เดิมไม่ได้ระบุ · ADR-19 |
| D-11 | Business key | เพิ่ม unique constraint 36 ชุด (code / slug / version ต่อ parent) · ตารางที่มีข้อมูลกลางใช้ `UNIQUE NULLS NOT DISTINCT (tenant_id, …)` | พบตอนทำ migration ว่า code / slug ซ้ำได้ |
| D-12 | สิทธิ์เริ่มต้น | seed permission catalogue (282 code) + 16 system role เป็นแถวกลางใน `00019_seed_iam_permissions.sql` | ให้ P0 IAM เริ่มได้ทันที · ปรับหลัง T06 ด้วย migration ใหม่ |
| D-13 | API client auth | OAuth2 client credentials เท่านั้น (ตัด “API key + HMAC” ออกจาก SA) · scope = permission code เช่น `consent.record.create` (แทน `consent:write` ใน SA เดิม) | ตรงกับนโยบาย API ในแผน · Q-10 ยังให้ยืนยันรูปแบบ scope |
| D-14 | Optimistic lock | `ETag: "<row_version>"` + `If-Match` · ไม่ตรง → **412** · ไม่ส่ง → **428** | ตาม RFC 9110 (SA เดิมเขียน 409 — แก้ SEQ-02 / ARC-05 แล้ว) |
| D-15 | JSON | field เป็น snake_case ตรงกับคอลัมน์ · เวลา RFC 3339 UTC · id เป็น UUIDv7 string | ลดการ map ชื่อ · ตรงกับ SEQ และ DDL |
| D-16 | Error | RFC 9457 problem+json + `code` คงที่ (ดู `api/openapi/README.md`) | ADR-22 |
| D-17 | ค่าสถานะ consent | ตัวพิมพ์ใหญ่ตาม FSD V3.2 · module อื่น snake_case | ADR-21 |
| D-18 | Transaction | 1 transaction ต่อ request (middleware Tx ผ่าน `WithTenantTx`) และ 1 ต่อ job ใน worker · service / store ใช้ tx จาก context · AuthZ อ่านสิทธิ์จาก cache หรือ read tx ของตัวเอง | SA เดิมเขียนทั้งแบบ middleware และแบบ service เปิดเอง — เลือกแบบเดียว |
| D-19 | Role และสิทธิ์ของฐานข้อมูล | `deploy/db/00-bootstrap.sql` + migrate ด้วย `pdpa_migrator` (`role=pdpa_owner`) + `deploy/db/10-grants.sql` · partition มี RLS และแอปเข้าได้ผ่านตารางแม่เท่านั้น · partition ใหม่ผ่าน `platform.ensure_monthly_partitions()` (SECURITY DEFINER) | ทดสอบแล้วด้วย role จริง: 12 กรณีผ่าน (ดู `backend/db/README.md`) |
| D-20 | FK ข้าม tenant | FK constraint ไม่ผ่าน RLS → service ต้องตรวจว่าแถวที่อ้างถึงมองเห็นได้ภายใต้ RLS ก่อนเขียน + test | composite FK (tenant_id, id) เป็นทางเลือกภายหลังถ้าต้องการบังคับที่ฐานข้อมูล |
| D-21 | Outbox dispatch | enqueue River job ต่อ tenant ใน transaction เดียวกับ outbox + sweeper วน tenant — worker ไม่ต้องใช้ BYPASSRLS | ลดการใช้ role ที่ข้าม RLS |

## 2. ยังเปิดอยู่ (ต้องถาม — ห้ามเดา)

| # | คำถาม | ค่าเริ่มต้นที่ใช้ระหว่างรอ | ผู้ตอบ / งานในแผน |
|---|---|---|---|
| Q-01 | ยืนยัน matrix สิทธิ์ (Roles_Permissions) และ data scope ของแต่ละ role | ตาม `docs/security/permissions.yaml` | ลูกค้า · T06 |
| Q-02 | Cloud provider / region ในไทย และ managed PostgreSQL รองรับ citext, ltree, pg_trgm, pgvector หรือไม่ | Kubernetes + CloudNativePG | สถาปนิก / DevOps · T09 |
| Q-03 | ผู้ให้บริการ SMS / Email / LINE OA | interface + adapter แบบ mock | ลูกค้า · T16 |
| Q-04 | ThaID: ใช้ได้จริงไหม เงื่อนไขการเชื่อมต่อ | OTP อีเมล / SMS | ลูกค้า · T16 |
| Q-05 | ผู้ให้บริการ e-Signature | interface + mock | ลูกค้า · T35 |
| Q-06 | DSAR: นับวันตามปฏิทินหรือวันทำการ · หยุดนับ SLA ระหว่าง `awaiting_info` หรือไม่ ต่อประเภทคำขอ | วันตามปฏิทิน · หยุดนับเฉพาะ `awaiting_info` (config ต่อ request type) | ฝ่ายกฎหมาย |
| Q-07 | เหตุละเมิด: การตีความ “แจ้งล่าช้าไม่เกิน 15 วัน” · ช่องทางยื่นแบบแจ้ง สคส. (ระบบบันทึก `submission_ref` ของการยื่นด้วยมือ) | บังคับกรอก late_reason เมื่อเกิน 72 ชม. | ฝ่ายกฎหมาย |
| Q-08 | อายุความยินยอม (lifespan) และนโยบายขอใหม่ต่อ purpose | ไม่มีวันหมดอายุ ถ้าไม่ตั้ง `lifespan_days` | ลูกค้า / DPO |
| Q-09 | ระยะเวลาเก็บรักษาต่อกิจกรรม / ประเภทข้อมูล และวิธีทำลาย | กรอกใน RoPA ต่อกิจกรรม (ไม่มีค่ากลาง) | ลูกค้า / ฝ่ายกฎหมาย |
| Q-10 | รูปแบบ OAuth scope ของ API client | scope = รายการ permission code (subset ของ role API) | สถาปนิก |
| Q-11 | NFR เชิงตัวเลข (latency p95, throughput ของ Consent API, จำนวน tenant / เจ้าของข้อมูล) | วัดใน T22 แล้วกำหนด | ลูกค้า / สถาปนิก |
| Q-12 | Mobile SDK (CON-07): เทคโนโลยีและผู้พัฒนา | ไม่อยู่ในขอบเขตทีม Next.js / Go | ลูกค้า |
| Q-13 | ผู้ให้บริการ LLM / LLM on-prem และขอบเขตข้อมูลที่ส่งได้ (P4) | ปิดฟีเจอร์ AI จนกว่าจะเลือก | ลูกค้า / DPO |
| Q-14 | เนื้อหากฎหมาย: template ประกาศ ข้อความยินยอม หนังสือตอบ แบบแจ้ง สคส. clause สัญญา | ระบบให้กลไก + ข้อความตัวอย่างที่ติดป้าย DRAFT | ฝ่ายกฎหมาย · T15, T34, T40 |
| Q-15 | ลูกค้าที่ต้องแยกฐานข้อมูล (DB ต่อ tenant / on-prem) | shared DB + RLS | ฝ่ายขาย / สถาปนิก |
| Q-16 | mapping ข้อมูลจากระบบ Consent เดิม | รอผล T03 | SA · T03 / T26 |
| Q-17 | ภาษาเพิ่มจาก TH / EN | TH (ค่าเริ่มต้น) + EN | ลูกค้า |
| Q-18 | โมเดล multi-tenant ของ Keycloak | realm เดียว `pdpa` + Organizations 1 องค์กรต่อ tenant (`platform.tenants.keycloak_org_id`) + mapper ใส่ claim `tid` · ทางเลือก: realm ต่อ tenant | สถาปนิก · PoC T13 (บล็อก IAM-01) |
| Q-19 | Provider console ของผู้ให้บริการ (SUPER) | surface `/provider/v1` ใน admin app · บัญชี SUPER อยู่ใน tenant พิเศษ `platform` (iam.users.tenant_id NOT NULL) · DB role `pdpa_platform` เฉพาะ package `internal/platform/provider` · เข้า tenant ลูกค้าได้ผ่าน break-glass เท่านั้น | สถาปนิก / ผู้ให้บริการ (บล็อก IAM-03) |
| Q-20 | ชุดค่าตั้งต้น master data (ORG-07): ชื่อและขอบเขตฐานทางกฎหมาย ม.19/24/26 · รายการหมวดข้อมูลอ่อนไหว · กลุ่มเจ้าของข้อมูล · วัตถุประสงค์ · สถานะ adequacy ของประเทศ | seed เป็นร่างใน migration 00031 (ถ้อยคำสรุปจากตัวบท, adequacy = `unknown` ทุกประเทศ) หน้าจอแสดงป้าย "รอฝ่ายกฎหมายตรวจ" · แก้ด้วย migration ใหม่เมื่อได้คำตอบ | ฝ่ายกฎหมาย / DPO |
