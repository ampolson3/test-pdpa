# อภิธานศัพท์

| คำ | ความหมายในระบบนี้ |
|---|---|
| PDPA | พ.ร.บ.คุ้มครองข้อมูลส่วนบุคคล พ.ศ. 2562 · อ้างถึงมาตราเป็น “ม.xx” |
| สคส. (PDPC) | สำนักงานคณะกรรมการคุ้มครองข้อมูลส่วนบุคคล / คณะกรรมการฯ — ผู้ออกประกาศและผู้รับแจ้งเหตุละเมิด |
| ผู้ควบคุมข้อมูล (Controller) | นิติบุคคลที่ตัดสินใจเรื่องการประมวลผล — ใน data model คือ `org.legal_entities` ที่ `is_controller` |
| ผู้ประมวลผล (Processor) | ผู้ประมวลผลตามคำสั่งผู้ควบคุม — คู่ค้าใน `vendor.vendors` · ต้องมี DPA (ม.40) |
| เจ้าของข้อมูล (Data subject) | บุคคลที่ข้อมูลระบุถึง — `consent.data_subjects` · ใช้ระบบผ่าน portal ไม่มีบัญชีผู้ใช้ |
| DPO | เจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล (ม.41–42) — ผู้อนุมัติหลักในหลาย workflow |
| Tenant | องค์กรลูกค้าหนึ่งรายบนแพลตฟอร์ม (`platform.tenants`) · หนึ่ง tenant มีได้หลายนิติบุคคล |
| Purpose | วัตถุประสงค์ที่ขอความยินยอม (FSD V3.2) — `consent.purposes` + ข้อความรายเวอร์ชัน `purpose_versions` |
| Data Element | รายการข้อมูลที่ใช้ในวัตถุประสงค์ (อีเมล, เบอร์โทร …) — `consent.data_elements` |
| Purpose Preference | ตัวเลือกย่อยของ purpose เช่น ช่องทาง หัวข้อ ความถี่ — `consent.purpose_preferences` |
| Collection Point | จุดเก็บความยินยอม (ฟอร์มเว็บ แอป หน้าร้าน API) — `consent.collection_points` |
| Consent Transaction | รายการตอบรับ / ปฏิเสธ / ถอน ต่อ purpose (append-only, partition รายเดือน) — ประเภทตาม FSD: CONSENTED, NOT_CONSENTED, WITHDRAWN, EXPIRED, EXTENDED, PENDING, CONFIRMED, CANCELLED, CHANGED_PREFERENCES |
| Consent Receipt | หลักฐานการตอบแต่ละครั้ง (hash chain) ส่งให้เจ้าของข้อมูลได้ |
| Consent status | สถานะล่าสุดต่อเจ้าของข้อมูล × purpose (projection) — ACTIVE, NOT_GIVEN, WITHDRAWN, EXPIRED, PENDING |
| Double Opt-In | ยืนยันความยินยอมซ้ำผ่านลิงก์อีเมล / SMS ก่อนมีผล |
| Reconcile | การเทียบสถานะความยินยอมกับระบบปลายทาง (CRM / CDP) และแก้รายการที่ไม่ตรง |
| Privacy Notice | ประกาศความเป็นส่วนตัวตาม ม.23 — `notice.notices` / `notice_versions` |
| RoPA | บันทึกรายการกิจกรรมการประมวลผล (ม.39) — `ropa.processing_activities` |
| DSAR | คำขอใช้สิทธิของเจ้าของข้อมูล (ม.30–36) รวมเรื่องร้องเรียน — `dsar.requests` |
| Breach | เหตุละเมิดข้อมูลส่วนบุคคล — `breach.incidents` · แจ้ง สคส. ภายใน 72 ชม. (ม.37(4)) |
| DPIA / LIA / TIA | แบบประเมินผลกระทบ / ประเมินประโยชน์โดยชอบด้วยกฎหมาย / ประเมินการโอนต่างประเทศ — assessment engine ใน schema `assess` |
| DPA | Data Processing Agreement — ข้อตกลงผู้ควบคุมกับผู้ประมวลผล (ม.40) |
| DSA | Data Sharing Agreement — ข้อตกลงแบ่งปันข้อมูลระหว่างผู้ควบคุม (ม.27, ม.37(2)) |
| Data scope | ขอบเขตข้อมูลของ role assignment: tenant / legal_entity / org_unit / self |
| x-permission | extension ใน OpenAPI ที่ประกาศสิทธิ์ของ endpoint — code จาก `docs/security/permissions.yaml` รูปแบบ `<area>.<resource>.<action>` (area ตาม RBAC ไม่ใช่ชื่อ package) |
| Maker-checker | ผู้สร้างกับผู้อนุมัติต้องเป็นคนละคน (`platform.approvals`) |
| Break-glass | สิทธิ์ฉุกเฉินชั่วคราวที่ต้องมีเหตุผลและแจ้ง DPO / Security ทันที |
| Unmask | การดูข้อมูลส่วนบุคคลแบบไม่ปกปิด — ต้องมี `pii.unmask.execute` + เหตุผล + บันทึก |
| Guest link | magic link ให้บุคคลภายนอกทำงานเดียว (ตอบแบบประเมิน ลงนาม แจ้งเหตุ) — `iam.guest_tokens` |
| Public key | key สาธารณะที่ใช้หา tenant ของ request ที่ไม่มีการ login — `platform.public_keys` |
| Outbox | ตาราง event ที่เขียนใน transaction เดียวกับข้อมูล แล้ว worker ส่งออก — `platform.outbox_events` |
| LFK | logical foreign key — อ้างถึงตาราง partition โดยไม่มี constraint ตรวจใน service |
| Blind index | HMAC ของค่าที่ normalize แล้ว ใช้ค้นหาข้อมูลที่เข้ารหัสแบบตรงตัว |
| FSD V3.2 | เอกสาร functional spec ของระบบ Consent เดิม (.NET) ที่ใช้เป็นต้นแบบ domain model |
| Phase P0–P4 | P0 Foundation + User & Permission · P1 MVP กฎหมายหลัก · P2 Governance & คู่ค้า · P3 Should · P4 Nice-to-have + AI |
| Function ID / UC ID | รหัส feature เช่น `CON-09` — ใช้ร่วมกันใน Function List, Development Plan, use case diagram และ backlog |
