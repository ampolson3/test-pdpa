# Database (PostgreSQL 16+)

| ไฟล์ | เนื้อหา |
|---|---|
| `migrations/00001_init.sql` | ตรวจ extension (citext, ltree, pg_trgm, vector) · 16 schema · function `platform.set_updated_at()` |
| `migrations/00002_platform.sql` … `00017_gov.sql` | 1 ไฟล์ต่อ schema: ตาราง, default partition, index, unique business key, RLS policy (รวม default partition), trigger, comment |
| `migrations/00018_foreign_keys.sql` | foreign key ทั้งหมด (อ้างข้าม schema ได้) |
| `migrations/00019_seed_iam_permissions.sql` | permission catalogue (282 code) + 16 system role + สิทธิ์ของแต่ละ role (แถวกลาง tenant_id NULL) |
| `migrations/00020_partition_maintenance.sql` | `platform.ensure_monthly_partitions(n)` (SECURITY DEFINER) + สร้าง partition เดือนปัจจุบัน + 3 เดือน |
| `schema.sql` | DDL รวมไฟล์เดียวสำหรับอ่าน / อ้างอิง (ไม่ใช้ apply และไม่รวม 00019–00020) |
| `../../deploy/db/00-bootstrap.sql` | role, database, extension — superuser รันครั้งเดียวต่อ environment |
| `../../deploy/db/10-grants.sql` | สิทธิ์ของ role แอป — รันทุกครั้งหลัง migrate (idempotent) |

รายละเอียดตารางทั้งหมด: [`docs/data/`](../../docs/data/README.md) · role และ RLS: [`docs/architecture/security.md`](../../docs/architecture/security.md) · กติกา migration: [`docs/architecture/deployment.md`](../../docs/architecture/deployment.md#database-migration)

## รัน

```bash
# 1) ครั้งแรกต่อ environment (superuser)
psql "$ADMIN_URL" -v ON_ERROR_STOP=1 -v migrator_password=... -v app_password=... \
     -v platform_password=... -v readonly_password=... -f deploy/db/00-bootstrap.sql

# 2) ทุก release: migrate ในฐานะ owner แล้วให้สิทธิ์
MIGRATE_URL="postgresql://pdpa_migrator:...@host/pdpa?options=-c%20role%3Dpdpa_owner"
goose -dir backend/db/migrations postgres "$MIGRATE_URL" up
psql "$MIGRATE_URL" -v ON_ERROR_STOP=1 -f deploy/db/10-grants.sql
```

- `options=-c role=pdpa_owner` ทำให้ทุก object เป็นของ `pdpa_owner` — default privileges ใน `10-grants.sql` จึงใช้ได้กับตารางที่สร้างภายหลัง
- `cmd/migrate` ใน scaffold ทำขั้นที่ 2 ทั้งหมด (goose → River migrations → grants)
- sqlc: วาง `sqlc.yaml` ไว้ที่ `backend/db/` และใช้ `schema: "migrations"` (path สัมพัทธ์กับไฟล์ config) — sqlc ข้ามส่วน `-- +goose Down`
- ไฟล์ใหม่สร้างด้วย `goose -s create <name> sql` (เลขเรียงต่อจาก 00020)

## ผ่านการทดสอบแล้ว (ตอนสร้างชุดนี้)

ทดสอบบน PostgreSQL 16 ด้วย parser ที่ใช้กติกาเดียวกับ goose (Up / Down / StatementBegin–End):

- **ในฐานะ superuser**: up → down → up ผ่าน · 201 ตาราง · FK ครบ · 194 ตารางมี RLS (+ ทุก partition) · seed 282 permission / 16 role / 807 สิทธิ์ · RLS 9 กรณี (แยก tenant, เขียนข้าม tenant ไม่ได้, ไม่ตั้ง tenant = 0 แถว, ข้อมูลกลางอ่านได้แต่แก้ไม่ได้, public_keys, trigger row_version)
- **ในฐานะ role จริง** (bootstrap → migrate ด้วย `pdpa_migrator` + `role=pdpa_owner` → `10-grants.sql` → ทดสอบด้วย `pdpa_app`): 12 กรณีผ่าน — insert / อ่านตาราง partition ผ่านตารางแม่, SELECT partition ตรง ๆ ถูกปฏิเสธ (ทั้ง default และรายเดือน), UPDATE / DELETE ตาราง append-only ถูกปฏิเสธ, แอปแก้ `platform.tenants` และสร้างตารางไม่ได้, worker สร้าง partition ล่วงหน้าผ่าน function และ partition ใหม่ไม่เปิดสิทธิ์ตรง, ตารางจาก migration ถัดไปใช้งานได้ด้วย default privileges, down → up ในฐานะ owner ผ่าน

## เปลี่ยนแปลงโครงสร้าง

ใช้ `/new-migration <คำอธิบาย>` — ไฟล์ใหม่ลำดับถัดไปเสมอ ห้ามแก้ไฟล์ที่ apply แล้ว และอัปเดต `docs/data/<schema>.md` (และ `10-grants.sql` ถ้าเป็นตาราง append-only / global) ใน PR เดียวกัน
