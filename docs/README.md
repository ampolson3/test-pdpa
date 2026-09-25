# docs — ความรู้ของระบบ

ชุดเอกสารนี้แปลงจากชุดวิเคราะห์ระบบ (`design/PDPA_System_Analysis.drawio` 72 หน้า) และ Development Plan (Excel) ให้อยู่ในรูปที่ Claude Code และนักพัฒนาอ่าน / ค้น / อ้างอิงได้ทีละไฟล์ ทุกไฟล์ใช้รหัสเดียวกัน (Function ID, BP-xx, SEQ-xx, ST-xx, ERD-xx, ชื่อตาราง) จึง grep ข้ามไฟล์ได้

## เริ่มอ่านจากไหน

| ต้องการ | เปิด |
|---|---|
| เข้าใจระบบทั้งหมดใน 10 นาที | [overview.md](overview.md) → [architecture/overview.md](architecture/overview.md) |
| ทำ feature หนึ่งตัว (เช่น CON-09) | `backlog/backlog.csv` → `modules/CON.md#con-09` → process / state / sequence ที่ลิงก์ไว้ → `data/<schema>.md` |
| ออกแบบ endpoint | [`api/openapi/README.md`](../api/openapi/README.md) + `security/permissions.md` |
| แก้ / เพิ่มตาราง | `data/<schema>.md` + `backend/db/migrations/` + [deployment.md#database-migration](architecture/deployment.md) |
| เปลี่ยนสถานะของ entity | `states/ST-xx.md` + `states/state-machines.yaml` |
| ตรวจเรื่องกฎหมาย | [legal/pdpa-rules.md](legal/pdpa-rules.md) |
| ความปลอดภัย / tenant / PII | [architecture/security.md](architecture/security.md) |
| event / webhook / job | [architecture/integration.md](architecture/integration.md) + `architecture/events.yaml` |
| เรื่องที่ยังไม่ตัดสิน | [decisions.md](decisions.md) ส่วนที่ 2 |

## โครงสร้าง

| โฟลเดอร์ / ไฟล์ | เนื้อหา |
|---|---|
| [overview.md](overview.md) · [glossary.md](glossary.md) | ขอบเขต epic / phase / actor · ศัพท์ |
| [decisions.md](decisions.md) | สิ่งที่ตัดสินแล้วตอนรวมเอกสาร + คำถามที่ยังเปิด |
| [legal/](legal/pdpa-rules.md) | มาตรา → กฎที่ระบบต้องบังคับ → ที่ implement → กำหนดเวลา |
| [architecture/](architecture/overview.md) | overview · ADR · code structure · security · integration · deployment · events.yaml |
| [modules/](modules/README.md) | 17 epic: technical summary, actors, 275 feature พร้อม BE / FE / acceptance criteria, สิทธิ์, event, job |
| [processes/](processes/README.md) | BP-01 ถึง BP-12 (Mermaid flowchart + ตารางขั้นตอน + กฎ) |
| [sequences/](sequences/README.md) | SEQ-01 ถึง SEQ-07 (Mermaid sequenceDiagram) |
| [states/](states/README.md) | ST-01 ถึง ST-07 (Mermaid stateDiagram) + state-machines.yaml |
| [data/](data/README.md) | data dictionary 16 schema 201 ตาราง (คอลัมน์ key enum RLS index) |
| [security/](security/permissions.md) | RBAC matrix + permissions.yaml |
| [backlog/](backlog/README.md) | backlog.csv (275 feature) + project-tasks.csv (T00–T48) |

## ลำดับความน่าเชื่อถือเมื่อข้อมูลขัดกัน

1. `backend/db/migrations/` (โครงสร้างข้อมูลจริง)  
2. `docs/states/` (ค่าสถานะและ transition)  
3. `docs/architecture/` + `docs/decisions.md`  
4. `docs/modules/` (พฤติกรรม feature)  
5. `docs/processes/`, `docs/sequences/`  
6. ไฟล์ใน `design/` (ต้นฉบับภาพ — ถ้าไม่ตรงกับ docs ให้ถือ docs)

ถ้าสองแหล่งขัดกันในเรื่องที่มีผลต่อพฤติกรรม ให้หยุดและถาม แล้วบันทึกผลใน `decisions.md`

## การดูแลเอกสาร

- เปลี่ยนพฤติกรรม / สถานะ / ตาราง / สิทธิ์ / event → แก้เอกสารที่เกี่ยวข้องใน PR เดียวกับโค้ด
- Mermaid ในไฟล์ถูกตรวจว่า parse ได้ (Mermaid 11) ตอนสร้างชุดนี้ — แก้แล้วตรวจด้วย preview ของ IDE / GitHub
- ไฟล์ใน `design/` เป็นต้นฉบับอ้างอิง ไม่ต้องแก้ตามทุกครั้ง (docs คือฉบับที่ใช้งาน)
