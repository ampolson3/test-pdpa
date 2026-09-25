# PDPA Platform — ชุดความรู้สำหรับ Claude Code

ชุดนี้ย้ายเนื้อหาจากงานวิเคราะห์ระบบ (draw.io 72 หน้า), Development Plan และ Function List มาอยู่ในรูปที่ Claude Code ใช้ได้ทันที: คำสั่งถาวร (`CLAUDE.md`), เอกสารความรู้ที่แยกเป็นไฟล์ตาม module / process / state / ตาราง, migration ที่รันได้จริง, สัญญา API ตั้งต้น และ slash command / skill สำหรับงานประจำ

## ในชุดมีอะไร

| path | เนื้อหา |
|---|---|
| `CLAUDE.md` | stack, โครง repo, ลำดับความน่าเชื่อถือของเอกสาร, กฎที่ห้ามละเมิด 14 ข้อ, ขั้นตอนทำ feature, definition of done |
| `.claude/commands/` | `/scaffold` · `/implement <ID>` · `/spec-api <ID>` · `/new-migration <desc>` · `/review-compliance` · `/trace <ID>` |
| `.claude/skills/pdpa-compliance/` | checklist PDPA + security ที่ Claude ใช้อัตโนมัติเมื่องานแตะข้อมูลส่วนบุคคล |
| `.claude/settings.json` | อนุญาตคำสั่ง build / test ที่ใช้บ่อย · กันการอ่านไฟล์ .env / secrets |
| `docs/` | 77 markdown · 3 YAML · 2 CSV: overview, decisions, legal, architecture, 17 modules (275 features), 12 processes, 7 sequences, 7 state machines, data dictionary 201 ตาราง, RBAC, backlog |
| `backend/db/migrations/` | goose 20 ไฟล์: init · 16 schema · FK · seed สิทธิ์ · partition function (201 ตาราง, 609 FK) |
| `deploy/db/` | bootstrap role / database / extension + ไฟล์ grants หลัง migrate (ทดสอบกับ role จริงแล้ว) |
| `api/openapi/` | OpenAPI 3.1 ตั้งต้น (security scheme, parameter, error, pagination) + ตัวอย่าง 3 operation + conventions |
| `design/` | ไฟล์ต้นฉบับ draw.io / Excel / DDL สำหรับอ้างอิง |

## เริ่มใช้งาน

1. สร้าง repository ใหม่ แล้วคัดลอก **ทุกไฟล์** ในชุดนี้ไปไว้ที่ root (รวมโฟลเดอร์ `.claude` ที่ซ่อนอยู่) แล้ว commit
2. เปิด Claude Code ที่ root ของ repository — `CLAUDE.md` ถูกโหลดอัตโนมัติ และคำสั่งใน `.claude/commands` ใช้ได้ทันที
3. เริ่มด้วย prompt: *“อ่าน docs/README.md, docs/overview.md และ docs/decisions.md แล้วสรุปความเข้าใจ พร้อมคำถามที่ต้องตอบก่อนเริ่ม P0”*
4. สร้างโครงระบบ: `/scaffold backend` → `/scaffold frontend` → `/scaffold infra` → `/scaffold ci`
5. ทำ feature ตามลำดับ dependency ของ P0 ด้วย `/implement <ID>` แล้วปิดงานแต่ละชิ้นด้วย `/review-compliance`

ลำดับ P0 ที่แนะนำ (เรียงตาม dependency ใน backlog):

`PLT-01` → `PLT-02` → `PLT-03` → `PLT-06` → `PLT-09` → `PLT-10` → `PLT-04` → `PLT-11` → `PLT-12` → `PLT-07` → `PLT-13` → `PLT-14` → `IAM-01` → `IAM-02` → `IAM-03` → `ORG-09` → `ORG-10` → `ORG-14` → `ORG-16` → `PLT-15` → `ORG-19` → `ORG-01` → `ORG-04` → `ORG-11` → `ORG-02` → `ORG-07` → `ORG-20` → `PLT-05` → `PLT-08`

หลัง P0 ทำ P1 ตาม `docs/backlog/backlog.csv` (Must ก่อน) — ทุก module ใช้บริการกลาง PLT และสิทธิ์จาก IAM

## ตรวจสอบแล้วตอนสร้างชุดนี้

- migration: parse ตามกติกา goose แล้ว apply บน PostgreSQL 16 แบบ up → down → up ผ่าน · ทดสอบ RLS 9 กรณี · ทดสอบ role จริง (bootstrap → migrate เป็น owner → grants → pdpa_app) 12 กรณี
- Mermaid 29 diagram parse ผ่านด้วย Mermaid 11 · OpenAPI 3.1 valid · YAML 4 ไฟล์ · CSV 275 + 53 แถว
- ลิงก์ภายใน 2084 ลิงก์ชี้ไฟล์และ anchor ที่มีอยู่จริง · feature ทุกตัวมี section · ตารางทุกตัวมี data dictionary
- state machine 10 ชุดตรงกับ CHECK constraint · x-permission ใน OpenAPI อยู่ใน permission catalogue (282 code)

## Local Postgres โดยไม่ใช้ Docker

เครื่องที่สร้างชุดนี้ไม่มี Docker และมี PostgreSQL 18 ตัวอื่นใช้ port 5432 อยู่แล้ว (ของโปรเจกต์อื่น ไม่แตะ) จึงตั้ง
PostgreSQL 17 + pgvector ผ่าน Homebrew แยกต่างหากที่ **port 5433** แทน `deploy/compose/docker-compose.yml`
(รันจริงแล้ว: bootstrap → migrate ผ่านทั้งหมด ดู `CLAUDE.md` § Scaffold status) หากเครื่องคุณมี Docker
ใช้ `make dev` ตามปกติแทนได้เลย (ไม่ต้องทำตามหัวข้อนี้) — ค่า `DATABASE_URL` ใน `.env.example` จะต้องเปลี่ยน
port กลับเป็น 5432

ทำซ้ำ setup นี้บนเครื่องอื่น (หรือหลัง `brew uninstall`):

```bash
brew install postgresql@17 pgvector redis
# pgvector build ไว้เฉพาะ postgresql@17/@18 (ไม่มี @16) — คัดลอกเข้า postgresql@17 เอง:
cp "$(brew --prefix pgvector)/../../Cellar/pgvector"/*/lib/postgresql@17/vector.dylib \
   "$(brew --prefix postgresql@17)/lib/postgresql/"
cp "$(brew --prefix pgvector)/../../Cellar/pgvector"/*/share/postgresql@17/extension/vector* \
   "$(brew --prefix postgresql@17)/share/postgresql/extension/"

# เปลี่ยน port เป็น 5433 (5432 ชนกับ Postgres อื่นที่มีอยู่แล้ว) — ข้ามถ้าเครื่องคุณว่าง 5432
sed -i '' "s/^#port = 5432/port = 5433/" "$(brew --prefix)/var/postgresql@17/postgresql.conf"

brew services start postgresql@17
brew services start redis  # Valkey-compatible สำหรับ dev

psql -h localhost -p 5433 -d postgres -v ON_ERROR_STOP=1 \
  -v migrator_password=pdpa_migrator -v app_password=pdpa_app \
  -v platform_password=pdpa_platform -v readonly_password=pdpa_readonly \
  -f deploy/db/00-bootstrap.sql

(cd backend && MIGRATOR_DATABASE_URL="postgres://pdpa_migrator:pdpa_migrator@localhost:5433/pdpa?sslmode=disable" \
  go run ./cmd/migrate -grants ../deploy/db/10-grants.sql)
```

บน Linux (Debian/Ubuntu, เช่น container ของ Claude Code on the web) ใช้ PostgreSQL 16 จาก apt แทน:
`apt-get install -y postgresql-16-pgvector` → ตั้ง `port = 5433` ใน `/etc/postgresql/16/main/postgresql.conf` →
`pg_ctlcluster 16 main start` → รัน `00-bootstrap.sql` ด้วย `su postgres -c "psql -p 5433 ..."` และ `cmd/migrate` ตามด้านบน
(`redis-server --daemonize yes` แทน Valkey)

OpenBao (KEK ของ PLT-13) สำหรับ test: ดาวน์โหลด binary จาก github.com/openbao/openbao/releases แล้ว
`bao server -dev -dev-root-token-id=dev-root -dev-listen-address=127.0.0.1:8200` + `BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=dev-root bao secrets enable transit`
(test ของ `internal/platform/crypto` ใช้ `TEST_OPENBAO_ADDR` / `TEST_OPENBAO_TOKEN`, ถ้าไม่มีจะทดสอบกับ LocalKEK อย่างเดียว)

ไฟล์ (PLT-09) สำหรับ test: S3 ที่ `127.0.0.1:8333` key `pdpa-dev` / `pdpa-dev-secret-key` (`TEST_S3_*`) — MinIO หรือ SeaweedFS
(`weed server -s3 -s3.port=8333 -s3.config=<identities json> -master.volumeSizeLimitMB=64 -volume.max=50`, build จาก
source ได้ถ้าดาวน์โหลด MinIO ไม่ได้) · clamd ที่ `127.0.0.1:3310` (`TEST_CLAMD_ADDR`, `apt install clamav-daemon` + `TCPSocket 3310`;
ถ้า freshclam ดาวน์โหลด signature ไม่ได้ ให้ใส่ signature ของ EICAR ใน `/var/lib/clamav/eicar.hdb`) — ไม่มีจะ skip

Keycloak ยังไม่ได้ตั้งทางนี้ (Organizations / `tid` claim ต้องรอ PoC T13 ตาม `docs/decisions.md` Q-18) — ทดสอบ
endpoint ที่ต้อง login ได้ด้วย JWT ที่เซ็นเองชั่วคราวเท่านั้น (ดูวิธีใน git log ของ commit ที่ verify reference
slice)

## ข้อควรรู้

- สิ่งที่ยังต้องตัดสินใจ (ผู้ให้บริการ SMS / e-Sign, การนับวัน DSAR, การตีความแจ้งเหตุล่าช้า ฯลฯ) อยู่ใน `docs/decisions.md` ส่วนที่ 2 — Claude Code ถูกสั่งให้ถาม ไม่เดา
- ข้อความทางกฎหมายทั้งหมดในระบบต้องผ่านฝ่ายกฎหมาย — `docs/legal/pdpa-rules.md` เป็นสรุปเพื่อออกแบบ ไม่ใช่คำปรึกษาทางกฎหมาย
- เอกสารใน `docs/` เป็นฉบับที่ใช้งาน: เปลี่ยนโค้ดแล้วต้องแก้เอกสารที่เกี่ยวข้องใน PR เดียวกัน
- ชุดนี้ใช้ได้ทั้ง Claude Code บนเครื่อง (CLI / IDE) และบนเว็บ (เมื่อ repository อยู่บน GitHub)
