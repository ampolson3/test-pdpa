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

## TEST-01 — รัน `go test ./...` เต็มรูปแบบครั้งแรกกับ Postgres/Redis จริง พบว่า 20+ แพ็กเกจล้มเหลวจริง

- **สถานะ:** open — ต้องการ triage แยกต่างหาก ไม่ใช่งานที่ทำพร้อม DSA-08
- **พบเมื่อ:** 2026-10-21 ระหว่างทำ DSA-08 — รัน `go test -count=1 ./...` แบบเต็มเป็นครั้งแรกในเซสชันนี้ที่
  Postgres/Redis reachable ตลอดทั้งรัน (รันนานพอที่จะไม่ถูก skip กลางคัน)
- **อาการ:** 43 บรรทัด FAIL กระจายอยู่ใน 20 แพ็กเกจ: `agreement/{http,service}` (ดู AGR-01 ด้านล่าง — ส่วนที่
  เกี่ยวกับ DSA-08 เองแก้แล้ว), `breach/publichttp`, `consent/{http,service}`, `cookie/service`,
  `dsar/{http,publichttp,service}`, `gov/{http,service}`, `notice/service`, `pkg/modulegate`,
  `platform/{forms,forms/http,jobs}`, `risk/{http,service}`, `ropa/service`, `vendormgmt/{http,service}`
- **ทำไม test เดิมไม่จับ:** แทบทุกฟีเจอร์ที่สร้างฟีเจอร์เหล่านี้ขึ้นมาระบุไว้ใน CLAUDE.md ของตัวเองว่า
  "not verified live against a real Postgres/Redis... skips the same way" — Postgres/Redis ไม่ reachable
  ตอนฟีเจอร์ส่วนใหญ่ถูกสร้าง เทสต์จึง skip มาตลอดจนถึงตอนนี้ นี่คือครั้งแรกที่ full suite ถูกรันจริงแบบ
  ไม่ถูกตัดตอนกลางคันตั้งแต่ Postgres/Redis กลับมา reachable ในเซสชันนี้
- **ยังไม่ได้ diagnose อาการแต่ละตัว:** ปริมาณงาน (20 แพ็กเกจ) เกินขอบเขตของ DSA-08 — ตัวอย่างอาการที่เห็นขณะ
  สแกนผ่านๆ: `vendormgmt/http`'s `record certificate: 500`, `vendormgmt/service`'s
  `TestOffboarding_FullLifecycle` (FK violation บน `destruction_certificate_file_id`), `ropa/service` และ
  `risk/service` ล้มเหลวเป็นชุดใหญ่ (25s/22s แต่ละแพ็กเกจ) — ต้องดู log เต็มทีละแพ็กเกจเพื่อแยกบั๊กจริงออกจาก
  ปัญหาเฉพาะ fixture ของเทสต์เอง (ตามที่ AGR-01 เจอว่าเป็นกรณีนี้ไปแล้วหลายจุด)
- **แนวทางแก้ที่เป็นไปได้:** ทำ pass แยกต่างหาก ไล่ทีละแพ็กเกจจาก log เต็ม (`/tmp/full_test_run.log` ในเซสชันนี้
  — ไม่ persist ข้ามเซสชัน ต้องรันใหม่) แยกบั๊กจริงในโค้ด service ออกจากบั๊กใน test fixture เอง (pattern ที่
  AGR-01 พบคือ permission/wiring ที่ไม่ครบใน test setup ไม่ใช่โค้ด production)
- **ผลกระทบ:** ไม่กระทบ DSA-08 เอง (agreement package เองผ่านครบหลังแก้ AGR-01) แต่เป็นสัญญาณว่าโมดูลจำนวนมาก
  มีบั๊กจริงหรือบั๊ก fixture ที่ไม่เคยถูกตรวจสอบเลยตั้งแต่สร้าง — ความเสี่ยงสำคัญที่ควร triage ก่อน production

---

## AGR-01 — `internal/agreement/service` ทดสอบหลายตัวล้มเหลวจริงเมื่อรันกับ Postgres จริง (ไม่เคยถูกรันมาก่อน)

- **สถานะ:** open
- **พบเมื่อ:** 2026-10-21 ระหว่างทำ DSA-08 (ตรวจทานและอนุมัติ — "dsa" ของ DPA-07's own review/approval
  mirror) เมื่อรัน `go test ./internal/agreement/...` จริงเป็นครั้งแรกกับ Postgres/Redis ที่ reachable ในเซสชันนี้
  — การ pass ของ test suite นี้ที่บันทึกไว้ใน CLAUDE.md ของฟีเจอร์ก่อนหน้า (DPA-10/DSA-11 เป็นต้น) ล้วนมีข้อความ
  "not verified live against a real Postgres/Redis... skips" กำกับไว้ จึงไม่เคยถูกรันจริงมาก่อนเซสชันนี้
- **อาการ (ยังไม่แก้ — อยู่นอกขอบเขตของ DSA-08):**
  1. `TestAddParty_SupportsMoreThanTwoParties` / `TestAddParty_Validation` / `TestRemoveParty`
     (`parties_test.go`) ล้มเหลวด้วย `ropa: invalid input: code` — ดูเหมือนปัญหาที่ fixture ของเทสต์เหล่านี้
     เอง (อาจเป็นรหัสกิจกรรม RoPA ที่ fixture สร้างไม่ผ่าน validation ของ ROPA-03/06)
  2. `TestSetSchedule_ValidatesAndSchedulesReminder` (`renewal_test.go:92`) ล้มเหลวเพราะ `effective_to`
     ที่อ่านกลับมาเป็น midnight UTC พอดี (`2027-01-19 00:00:00 +0000 UTC`) แต่ assert เทียบกับเวลาปัจจุบัน
     แบบเต็ม (`...08:43:27...`) — ดูเหมือนปัญหาการเทียบเวลาของเทสต์เอง (`pgtype.Date` ปัดเป็น date-only
     เมื่อ round-trip ผ่าน Postgres ตามที่ VEN-06 เคยพบปัญหาแบบเดียวกันมาแล้ว) ไม่ใช่บั๊กในโค้ด service จริง
  3. `TestFireRenewalReminder_NotifiesLegalAndNoopsWhenStale` และ
     `TestFireRenewalReminder_WorksForDsaAgreementsToo` (`renewal_test.go`, `dsa_renewal_test.go`) ล้มเหลว
     ด้วย "expected at least one platform.notifications row for role LEGAL" — การแจ้งเตือนของ DPA-10/DSA-11
     เมื่อ renewal reminder ทำงานไม่ถูกสร้างขึ้นจริงในสภาพแวดล้อมนี้
  4. `TestAgreementEndpoints_Contract` (`handler_test.go:315`): `PATCH .../schedule` ด้วย
     `renewal_notice_days: -1` ตอบ 400 (schema validation) แทนที่จะเป็น 422 ตามที่เทสต์คาดหวัง — แปลว่า
     OpenAPI schema เองมี `minimum` constraint บน `renewal_notice_days` ที่ดักค่าติดลบไว้ก่อนถึงชั้น service
     validation (ซึ่งเทสต์คาดว่าจะเป็นผู้ดัก) — ต้องดูว่า schema หรือเทสต์ผิดคาด
- **ทำไม test เดิมไม่จับ:** ทุก entry ของ DPA-10/DSA-11/VEN-06 ใน CLAUDE.md ระบุชัดว่า "not verified live" —
  Postgres/Redis ไม่ reachable ตอนฟีเจอร์เหล่านี้ถูกสร้าง เทสต์จึง skip เสมอ
- **สิ่งที่แก้แล้วระหว่างทำ DSA-08 (ไม่ใช่บั๊กเหล่านี้ แต่เป็นบั๊กจริงที่พบและแก้แล้วในรอบเดียวกัน):**
  `backend/internal/agreement/service/agreements_test.go`'s shared `permissions`/`setup()` และ
  `backend/internal/agreement/http/handler_test.go`'s `grants` map ไม่เคยให้สิทธิ์ `agreement.dsa.create`/
  `.update` หรือ wire `SetSubmitValidate`/`SetOnTransition` สำหรับ `"dsa"` เลย — ทำให้ทุกเทสต์ที่สร้าง/อนุมัติ
  agreement ประเภท `"dsa"` ล้มเหลวด้วย `docs: forbidden` / `versioning: forbidden` เมื่อรันจริงเป็นครั้งแรก
  (แก้แล้วในคอมมิตเดียวกับ DSA-08)
- **แนวทางแก้ที่เป็นไปได้:** ตรวจ fixture ของแต่ละเทสต์ทีละตัว (1–3) และตัดสินใจเรื่อง schema vs. service-layer
  validation ของ `renewal_notice_days` (4) — งานแยกจาก DSA-08 เอง
- **ผลกระทบ:** ไม่กระทบ acceptance criterion ของ DPA-07/DSA-08 เอง (ซึ่งผ่านแล้ว) แต่เป็นสัญญาณว่า
  DPA-10/VEN-06/DSA-11's own renewal-reminder mechanism อาจมีบั๊กจริงที่ยังไม่เคยถูกตรวจสอบ

---

## PNG-15/ROPA-15 — `TestCheckDueReviews_NotifiesOwnerAndReschedules`-style tests undercount notifications by half

- **สถานะ:** open (แก้แล้วเฉพาะ `internal/notice/service/review_test.go` ระหว่างทำ PNG-09; ยังไม่แก้ใน
  `internal/ropa/service/review_test.go`)
- **พบเมื่อ:** 2026-10-22 ระหว่างทำ PNG-09 — รัน `go test -count=1 ./internal/notice/...` เต็มชุดเป็นครั้งแรก
  กับ Postgres/Redis จริง
- **อาการ:** `TestCheckDueReviews_NotifiesOwnerAndReschedules` ล้มเหลวด้วย "a not-yet-due notice should not
  be notified again, got 2 notifications" แม้ `CheckDueReviews` ถูกเรียกเพียงครั้งเดียวที่ควรแจ้งเตือนจริง
  (การเรียกครั้งที่สองเป็น no-op ตามที่ตั้งใจ) ก่อนหน้านั้นยังมีบั๊กแยกที่ซ่อนอันนี้ไว้: query ของเทสต์เอง
  อ้างคอลัมน์ `template_code` ที่ไม่มีอยู่จริงบน `platform.notifications` (คอลัมน์จริงคือ `template_id` ซึ่ง
  ต้อง join กับ `platform.notification_templates.code`) — แก้ query ผิดนี้ก่อนแล้วถึงเจอบั๊ก count จริง
- **สาเหตุ:** `CheckDueReviews` (`review.go`) ส่งการแจ้งเตือนหนึ่งครั้งต่อ channel (`in_app` และ `email`) ต่อ
  notice ที่ถึงกำหนดทบทวนหนึ่งครั้ง — ดังนั้นการแจ้งเตือนที่ถูกต้องเพียงครั้งเดียวจะสร้างแถว
  `platform.notifications` สองแถวเสมอ (คนละ channel) แต่ query ของเทสต์นับทุกแถวโดยไม่กรอง channel จึงได้ 2
  ทั้งที่ควรนับเป็น "1 ครั้งของการแจ้งเตือน"
- **ทำไม test เดิมไม่จับ:** ทั้งสองโมดูล (`notice`/PNG-15 และ `ropa`/ROPA-15) ถูกสร้างตอน Postgres/Redis ไม่
  reachable ในเซสชัน — เทสต์ถูก skip มาตลอดจนกระทั่ง Postgres/Redis กลับมา reachable ครั้งนี้ (ครั้งแรกที่
  เทสต์เหล่านี้รันจริง) และบั๊ก `template_code` เดิมทำให้เทสต์ fail ก่อนจะไปถึงจุดที่ assertion การนับทำงาน
  ด้วยซ้ำ
- **สิ่งที่แก้แล้ว:** `internal/notice/service/review_test.go` — เพิ่ม `n.channel = 'in_app'` ในทั้ง 3
  query (และแก้ `template_code` → join `platform.notification_templates` ตาม `template_id`) เพื่อนับเฉพาะ
  signal "แจ้งแล้วหรือยัง" หนึ่งครั้งต่อ notice แทนที่จะนับทุก channel
- **ยังไม่แก้:** `internal/ropa/service/review_test.go`'s `TestCheckDueReviews_NotifiesOwnerAndReschedules`
  (ROPA-15) มีรูปแบบเดียวกันเป๊ะ — ยืนยันแล้วว่าล้มเหลวเหมือนกันจริง (`go test -run
  TestCheckDueReviews_NotifiesOwnerAndReschedules ./internal/ropa/service/...` → `got 2 notifications`)
  แต่เป็นไฟล์เทสต์ของอีกโมดูลหนึ่ง อยู่นอก scope ของ PNG-09 จึงไม่ได้แก้ในรอบนี้
- **แนวทางแก้ที่เป็นไปได้:** แก้ `internal/ropa/service/review_test.go` ด้วยวิธีเดียวกัน — เพิ่มเงื่อนไข
  `channel = 'in_app'` (หรือ channel อื่นที่คงที่หนึ่งค่า) ในทุก query ที่นับแถว `platform.notifications`
  เพื่อสะท้อน "จำนวนครั้งที่ถูกแจ้งเตือนจริง" แทนจำนวนแถวดิบ
- **ผลกระทบ:** ไม่กระทบ production code ของ PNG-15/ROPA-15 เอง (ซึ่งทำงานถูกต้องตามที่ตั้งใจ — ส่งทุก
  channel ต่อการแจ้งเตือนหนึ่งครั้งเป็นเรื่องปกติ) เป็นเพียง assertion ของเทสต์ที่นับผิดหน่วย แต่ถ้าไม่แก้
  `ropa`'s เวอร์ชันจะ fail ทุกครั้งที่ full suite รันกับ Postgres/Redis จริง

---

## JOBS-01 — หลาย periodic sweep job ไม่ได้อยู่ใน `jobs.GlobalKinds` ทั้งที่ comment ของตัวเองบอกว่าอยู่

- **สถานะ:** open
- **พบเมื่อ:** 2026-10-23 ระหว่างทำ DPA-12 (เพิ่ม `agreement.missing_dpa_sweep` เข้า
  `internal/platform/jobs/client.go`'s `GlobalKinds`) — ไล่ตรวจ `GlobalKinds` ทั้งไฟล์เพื่อดูว่าจะวาง
  entry ใหม่ตรงไหน แล้วพบว่า sweep job อื่นๆ ที่ comment ของตัวเอง (ในไฟล์ของแต่ละโมดูล) ระบุชัดว่า
  "platform-wide (jobs.GlobalKinds)" กลับไม่มี entry จริงในแมปนั้นเลย
- **อาการ:** `vendor.reassessment_sweep` (VEN-10), `notice.review_sweep` (PNG-15), `ropa.review_sweep`
  (ROPA-15), `consent.double_opt_in_sweep` (CON-23), `consent.expiry_sweep` (CON-20) และ
  `gov.retention_sweep` (DPX-05) ทุกตัวเป็น struct ว่าง ไม่มี `tenant_id` — เมื่อ `TenantTxMiddleware.Work`
  เจอ kind ที่ไม่อยู่ใน `GlobalKinds` และ parse `tenant_id` ไม่ได้ (เพราะไม่มี field นี้เลย) จะ
  `river.JobCancel(ErrMissingTenant)` ทันที หมายความว่า sweep job เหล่านี้ถูก cancel ทุกครั้งที่รันจริง
  ไม่เคยเดินถึง `Work` ของตัวเองเพื่อ enqueue job รายเทนแนนต์เลย
- **สาเหตุ:** แต่ละ feature (VEN-10/PNG-15/ROPA-15/CON-20/CON-23/DPX-05) เขียน comment ไว้ถูกต้องว่าต้อง
  อยู่ใน `GlobalKinds` แต่ไม่มีใครไปเพิ่ม entry จริงในแมปของ `internal/platform/jobs/client.go` — น่าจะ
  เพราะทุก pass เหล่านั้นไม่มี Postgres/Redis จริงให้รัน worker จริงตรวจสอบ (ทุก entry ใน CLAUDE.md ของ
  feature เหล่านี้ระบุว่า "not verified live against a real Postgres/Redis") จึงไม่มีใครเห็น job ถูก
  cancel จริง
- **ทำไม test เดิมไม่จับ:** ทุกโมดูลเทสต์ `CheckDueReviews`/`CheckDueReassessments`/ฯลฯ โดยเรียกฟังก์ชัน
  `Service` ตรง (เช่น `s.CheckDueReassessments(ctx)`) ภายใน tx ที่ test harness เปิดให้เองอยู่แล้ว — ไม่มี
  เทสต์ใดรัน sweep ผ่าน River worker จริงเพื่อพิสูจน์ว่า job จริงไม่ถูก cancel
- **แนวทางแก้ที่เป็นไปได้:** เพิ่ม 6 entry ข้างบนเข้า `internal/platform/jobs/client.go`'s `GlobalKinds`
  (รูปแบบเดียวกับที่ DPA-12 เพิ่ม `"agreement.missing_dpa_sweep": true` เข้าไปแล้ว) แล้วเพิ่มเทสต์ที่รัน
  sweep ผ่าน `river.Client` จริงอย่างน้อยหนึ่งกรณีเพื่อกันไม่ให้เกิดซ้ำกับ sweep job ตัวใหม่ในอนาคต
- **ผลกระทบ:** ไม่กระทบ logic ของ `CheckDueReassessments`/`CheckDueReviews`/ฯลฯ เอง (ถูกพิสูจน์ถูกต้องแล้ว
  โดยเทสต์ที่เรียกตรง) แต่หมายความว่า **ฟีเจอร์ periodic-reminder ทั้ง 6 นี้ไม่เคยทำงานจริงบน worker ที่รัน
  อยู่จริงเลย** จนกว่าจะเพิ่ม entry เข้า `GlobalKinds` — เป็นช่องว่างใหญ่กว่าที่ CLAUDE.md ของแต่ละฟีเจอร์
  เคยเปิดเผยไว้ ("not verified live" หมายถึงยิ่งกว่านั้นจริง)

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
