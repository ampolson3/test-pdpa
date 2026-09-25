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

## ข้อควรรู้

- สิ่งที่ยังต้องตัดสินใจ (ผู้ให้บริการ SMS / e-Sign, การนับวัน DSAR, การตีความแจ้งเหตุล่าช้า ฯลฯ) อยู่ใน `docs/decisions.md` ส่วนที่ 2 — Claude Code ถูกสั่งให้ถาม ไม่เดา
- ข้อความทางกฎหมายทั้งหมดในระบบต้องผ่านฝ่ายกฎหมาย — `docs/legal/pdpa-rules.md` เป็นสรุปเพื่อออกแบบ ไม่ใช่คำปรึกษาทางกฎหมาย
- เอกสารใน `docs/` เป็นฉบับที่ใช้งาน: เปลี่ยนโค้ดแล้วต้องแก้เอกสารที่เกี่ยวข้องใน PR เดียวกัน
- ชุดนี้ใช้ได้ทั้ง Claude Code บนเครื่อง (CLI / IDE) และบนเว็บ (เมื่อ repository อยู่บน GitHub)
