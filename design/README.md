# design — ไฟล์ต้นฉบับ (อ้างอิง)

| ไฟล์ | เนื้อหา | ฉบับที่ใช้งานใน repo |
|---|---|---|
| `PDPA_System_Analysis.drawio` | ชุดวิเคราะห์ระบบ 72 หน้า: 00 Index · ARC-01–06 · UC-00–17 · BP-01–12 · DFD-0 / 1 / 2.3 / 2.7 / 2.8 · ERD-00–15 · SEQ-01–07 · ST-01–07 (เปิดด้วย draw.io desktop หรือ app.diagrams.net) | `docs/architecture`, `docs/modules`, `docs/processes`, `docs/sequences`, `docs/states`, `docs/data` |
| `PDPA_Platform_Development_Plan_NextJS_Go.xlsx` | แผนพัฒนา: Dashboard · Assumptions · WBS (275 feature) · Task_List · Project_Tasks · Roadmap · Roles_Permissions · Traceability · Architecture · Risks | `docs/backlog`, `docs/security`, `docs/architecture/adr.md` |
| `PDPA_Thailand_Function_List_15_Modules.xlsx` | Function List 15 โมดูล (243 ฟังก์ชัน) พร้อมคำอธิบาย อ้างอิงกฎหมาย และ priority | `docs/modules` |
| `pdpa_schema.sql` | DDL รวม (ชุดเดียวกับ `backend/db/schema.sql`) | `backend/db/migrations` |

ไฟล์เหล่านี้ใช้ดูภาพรวมและคุยกับลูกค้า — **เอกสารใน `docs/` คือฉบับที่ใช้พัฒนา** ถ้าขัดกันให้ถือ `docs/` (ลำดับความน่าเชื่อถือใน `CLAUDE.md`)

ความต่างที่ตั้งใจระหว่าง Excel กับ docs (reconcile แล้ว): ดู `docs/decisions.md` ส่วนที่ 1 — เช่น API prefix, ชื่อ package `assess` / `gov`, ตาราง `platform.public_keys`, business key และ seed สิทธิ์
