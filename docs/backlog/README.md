# Backlog

- [`backlog.csv`](backlog.csv) — 1 แถวต่อ feature (275 แถว) · id = Function ID · มีขนาดงาน ประมาณการวัน (BE/FE จากขนาด XS=1 S=3 M=5 L=10 XL=20; SA 12% · UX 40% ของ FE เมื่อมีหน้าจอใหม่ · QA 35%) · acceptance criteria · dependency · ลิงก์ไปเอกสาร module
- [`project-tasks.csv`](project-tasks.csv) — งานระดับโครงการ T00–T48 (วิเคราะห์ สถาปัตยกรรม DevOps ทดสอบ กฎหมาย go-live)
- ฉบับเต็มพร้อมสูตรคำนวณ roadmap / งบ: `design/PDPA_Platform_Development_Plan_NextJS_Go.xlsx`

## Phase

| phase | ชื่อ | ขอบเขต | features | BE days | FE days |
|---|---|---|---|---|---|
| P0 | Foundation + User & Permission | โครง Go / Next.js, multi-tenant, บริการกลาง, ผู้ใช้ สิทธิ์ และข้อมูลหน่วยงาน | 29 | 187 | 127 |
| P1 | MVP กฎหมายหลัก | Consent & Cookie, Privacy Notice, RoPA, DSAR, Breach และ DPO พื้นฐาน พร้อมใช้งานจริง | 82 | 369 | 355 |
| P2 | Governance & คู่ค้า | DPIA, ความเสี่ยง RoPA, RoPA Template, Data Flow, Vendor, DPA, DSA, Retention และ TIA | 47 | 181 | 183 |
| P3 | Should | ฟังก์ชันมาตรฐานตลาดและความต้องการของลูกค้าองค์กร | 94 | 364 | 318 |
| P4 | Nice-to-have + AI | จุดขายเพิ่มเติม ฟีเจอร์ AI และแนวโน้มตลาด | 23 | 95 | 75 |

## วิธีใช้กับ Claude Code

- เลือกงานตาม phase และ dependency: `/implement CON-09` จะอ่านแถวใน backlog + เอกสาร module + process ที่เกี่ยวข้อง
- สถานะ (`status`) แก้ใน CSV ได้ถ้าไม่ใช้ issue tracker (todo → in_progress → review → done) หรือ import เข้า GitHub Issues / Jira
- P0 ต้องเสร็จก่อน: T-scaffold (monorepo, CI, migrations), PLT (บริการกลาง), IAM (login, RBAC, RLS) — ทุก module ที่เหลือพึ่งพา
