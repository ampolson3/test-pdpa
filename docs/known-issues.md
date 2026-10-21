# known-issues — บั๊กที่พบจากการรันระบบจริง

บันทึกบั๊กที่พบระหว่างการทดสอบระบบจริง (รัน Postgres + Redis + Go API + worker + Next.js admin ทั้งชุด แล้วไล่
ใช้งานผ่านเบราว์เซอร์จริง) ซึ่งต่างจาก `go test`/`pnpm test` ตรงที่ใช้ข้อมูลจริงที่ seed ไว้และเรนเดอร์ผ่าน
เบราว์เซอร์จริง — บั๊กบางตัวจึงไม่ปรากฏใน test suite ที่มีอยู่ (ไม่มี fixture ที่ตรงเงื่อนไข)

สถานะ: `open` (ยังไม่แก้) · `fixed` (แก้แล้ว อ้าง commit/PR) · `wontfix` (ตัดสินใจไม่แก้ พร้อมเหตุผล)

---

## DFG-01 — หน้า `/th/data-flow` ค้างที่ "กำลังโหลด..." ไม่ render ผังเลย

- **สถานะ:** open
- **พบเมื่อ:** 2026-10-21 ระหว่างทดสอบระบบจริงผ่าน Playwright + Chromium จริง (ไม่ใช่ mock)
- **อาการ:** เปิด `/th/data-flow` แล้วหน้าค้างที่ข้อความ "กำลังโหลด..." ตลอดไป ไม่เคย render ผัง React Flow
  เลย แม้ API จะตอบข้อมูลกลับมาแล้ว (ยืนยันจาก response log)
- **สาเหตุ:** browser console แสดง error จาก ELK (Eclipse Layout Kernel, ไลบรารีจัดเลย์เอาต์กราฟที่
  `/th/data-flow` ใช้):

  ```
  org.eclipse.elk.core.UnsupportedGraphException: The source or the target of edge ElkEdge
  "destroy:01a12b66-bd03-769b-9cfe-5e43e417c374" ElkNode "activity:01a12b66-bba6-7a6e-9345-d4dbdb0e3242"
  (0,0 | 0,0) -> ElkNode "disposal:01a12b66-bba6-7a6e-9345-d4dbdb0e3242:delete" (0,0 | 150,36) could not be
  found. This usually happens when an edge connects a node laid out by ELK Layered to a node in another
  level of hierarchy laid out by either another instance of ELK Layered or another layout algorithm
  altogether. The former can be solved by setting the hierarchyHandling option to INCLUDE_CHILDREN.
  ```

  edge ประเภท "ทำลาย/จัดการเมื่อครบกำหนด" (`destroy:...`) เชื่อมจาก node ระดับกิจกรรม (`activity:...`) ไปยัง
  node ย่อยของการจัดการข้อมูล (`disposal:...:delete`) ซึ่งอยู่คนละระดับ hierarchy กัน — ELK ปฏิเสธกราฟทั้งหมด
  แทนที่จะข้าม edge นั้นไปเฉยๆ
- **ทำไม test เดิมไม่จับ:** ข้อมูลทดสอบของ DFG-01/03 เดิมไม่มี fixture ที่มี disposal/destroy edge แบบนี้ —
  เกิดเฉพาะกับกิจกรรมที่มี retention rule จริงที่ระบุ `disposal_method` (ROPA-07)
- **แนวทางแก้ที่เป็นไปได้:** ตั้ง `hierarchyHandling: INCLUDE_CHILDREN` บน ELK layout options ตามที่ error
  บอกไว้ตรงๆ (`apps/admin` ส่วนที่เรียก ELK สำหรับหน้า `/th/data-flow`, DFG-01/03) — ต้องตรวจสอบว่าไม่กระทบ
  เลย์เอาต์ของ edge ประเภทอื่นที่ทำงานถูกต้องอยู่แล้ว (ส่งข้อมูล/โอนไปต่างประเทศ)
- **ผลกระทบ:** ผู้ใช้ไม่สามารถดูผังการไหลของข้อมูลได้เลยเมื่อมี activity ที่มี retention/disposal rule — เป็น
  หนึ่งใน acceptance criteria หลักของ DFG-01/03

---

## แม่แบบสำหรับบั๊กใหม่

```markdown
## <module>-<id-สั้นๆ> — <สรุปสั้นๆ>

- **สถานะ:** open / fixed / wontfix
- **พบเมื่อ:** วันที่ + บริบท (เช่น ทดสอบระบบจริง, code review, รายงานจากผู้ใช้)
- **อาการ:**
- **สาเหตุ:**
- **ทำไม test เดิมไม่จับ:** (ถ้ามี)
- **แนวทางแก้ที่เป็นไปได้:**
- **ผลกระทบ:**
```
