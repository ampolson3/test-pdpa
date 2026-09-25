# Data dictionary

201 ตาราง ใน 16 schema · PostgreSQL 16+ · ต้นฉบับ DDL: `backend/db/migrations/` (goose — sqlc อ่านจากโฟลเดอร์นี้) และ `backend/db/schema.sql` (ไฟล์รวมสำหรับอ่าน) · role / สิทธิ์: `deploy/db/`

## ข้อตกลงร่วมของทุกตาราง

- `id uuid` — แอปสร้างเป็น **UUIDv7** (เรียงตามเวลา) · `DEFAULT gen_random_uuid()` เป็นค่าสำรองเท่านั้น · ยกเว้น `platform.audit_log.id` เป็น bigint identity
- คอลัมน์มาตรฐาน (ตารางที่ระบุว่า *มีคอลัมน์มาตรฐาน*): `created_at`, `created_by`, `updated_at`, `updated_by`, `row_version` — trigger `platform.set_updated_at()` เพิ่ม `row_version` ทุก UPDATE (ใช้ทำ optimistic lock ผ่าน `If-Match` / `ETag`)
- `tenant_id` + Row-Level Security: แอปต้อง `SET LOCAL app.tenant_id = '<uuid>'` ต้น transaction เสมอ · policy ใช้ `NULLIF(current_setting('app.tenant_id', true), '')::uuid` → ไม่ตั้งค่า = ไม่เห็นข้อมูล (ไม่ error)
- ตาราง *tenant + ข้อมูลกลาง*: แถวที่ `tenant_id IS NULL` คือค่าตั้งต้นของแพลตฟอร์ม ทุก tenant อ่านได้ แต่เขียนได้เฉพาะ migration / role ที่ BYPASSRLS · tenant override ด้วยแถวที่มี tenant_id และ code เดียวกัน (unique `NULLS NOT DISTINCT`)
- ตาราง partition รายเดือน (⟨P⟩): PK รวมคอลัมน์เวลา · มี `<table>_default` partition · job `partition.maintain` เรียก `platform.ensure_monthly_partitions(3)` · ทุก partition มี RLS และแอปเข้าได้ผ่านตารางแม่เท่านั้น · ตารางอื่นอ้างถึงด้วย **LFK** (logical FK ไม่มี constraint) ตรวจใน service
- FK constraint ไม่ผ่าน RLS — service ต้องตรวจว่าแถวที่อ้างถึงอยู่ใน tenant เดียวกัน (มองเห็นได้ภายใต้ RLS) ก่อนเขียน
- `*_enc` = ข้อมูลที่เข้ารหัสระดับฟิลด์ (AES-256-GCM, DEK ต่อ tenant) · `blind_index` = HMAC-SHA256 ด้วย key แยกต่อ tenant สำหรับค้นหาแบบตรงตัว
- ค่าสถานะเป็นตัวพิมพ์เล็ก snake_case ตาม CHECK ยกเว้น consent ใช้ตัวใหญ่ตาม FSD V3.2 · ค่าที่มี state machine ลิงก์ไปที่ `docs/states/`
- เวลาเก็บเป็น `timestamptz` (UTC) · แสดงผล Asia/Bangkok · ปี พ.ศ. ตาม locale ที่ชั้น UI เท่านั้น
- ชื่อ constraint: `pk_<schema>_<table>`, `fk_<table>_<column>`, `uq_<table>_<cols>`, index `ix_<schema>_<table>_<column>`

## Schema

| schema | คำอธิบาย | ตาราง | epic เจ้าของ | migration |
|---|---|---|---|---|
| [platform](platform.md) | บริการกลางของแพลตฟอร์ม: tenant, workflow, form, เอกสาร, ไฟล์, แจ้งเตือน, event, audit | 28 | PLT | `00002_platform.sql` |
| [iam](iam.md) | ผู้ใช้ สิทธิ์ ขอบเขตข้อมูล API client และการยืนยันตัวตน | 19 | IAM | `00003_iam.sql` |
| [org](org.md) | นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก และข้อมูลตั้งต้น | 12 | ORG | `00004_org.sql` |
| [consent](consent.md) | ความยินยอมตาม FSD V3.2: Data Element, Purpose, Purpose Preference, Collection Point, Consent Transaction, Reconcile | 19 | CON | `00005_consent.sql` |
| [cookie](cookie.md) | โดเมน แบนเนอร์ คุกกี้ ผลสแกน และหลักฐานความยินยอมคุกกี้ | 11 | CON | `00006_cookie.sql` |
| [notice](notice.md) | ประกาศความเป็นส่วนตัว เวอร์ชัน การรับทราบ และการแจ้งตาม ม.25 | 8 | PNG | `00007_notice.sql` |
| [ropa](ropa.md) | RoPA ทะเบียนข้อมูล asset การโอน retention และคลังกิจกรรมมาตรฐาน | 17 | ROPA, RTG | `00008_ropa.sql` |
| [dataflow](dataflow.md) | แผนผังการไหลของข้อมูลและ data discovery | 5 | DFG | `00009_dataflow.sql` |
| [risk](risk.md) | risk matrix ทะเบียนความเสี่ยง control และการวิเคราะห์ช่องว่าง | 10 | RRA | `00010_risk.sql` |
| [assess](assess.md) | แบบประเมิน DPIA / LIA / TIA / security / maturity (assessment engine) | 8 | DPIA | `00011_assess.sql` |
| [dsar](dsar.md) | คำขอใช้สิทธิของเจ้าของข้อมูล | 12 | DSAR | `00012_dsar.sql` |
| [breach](breach.md) | เหตุละเมิดข้อมูลและการแจ้ง สคส. / เจ้าของข้อมูล | 12 | BRE | `00013_breach.sql` |
| [vendor](vendor.md) | คู่ค้าและผู้ประมวลผล | 7 | VEN | `00014_vendor.sql` |
| [agreement](agreement.md) | ข้อตกลง DPA / DSA (agreement engine) | 10 | DPA, DSA | `00015_agreement.sql` |
| [dpo](dpo.md) | งานของ DPO: การแต่งตั้ง งาน คำปรึกษา คลังความรู้ | 8 | DPO | `00016_dpo.sql` |
| [gov](gov.md) | อบรม นโยบาย การตรวจประเมิน retention การติดต่อ สคส. และทะเบียน AI | 15 | DPX | `00017_gov.sql` |

เพิ่มเติม: `00001_init.sql` (extension · schema · function), `00018_foreign_keys.sql` (FK ทั้งหมด), `00019_seed_iam_permissions.sql` (permission catalogue + 16 system roles), `00020_partition_maintenance.sql` (function สร้าง partition รายเดือน)
