# Sequence diagrams

ลำดับการเรียกระหว่าง component ของ flow ที่มีความเสี่ยงทางเทคนิคหรือกฎหมายสูง

| รหัส | เรื่อง | module |
|---|---|---|
| [SEQ-01](SEQ-01.md) | Login ผู้ใช้ภายใน (OIDC Authorization Code + PKCE ผ่าน BFF) | IAM |
| [SEQ-02](SEQ-02.md) | การตรวจสิทธิ์ต่อ request (x-permission + data scope + RLS + optimistic lock) | IAM, PLT, ROPA |
| [SEQ-03](SEQ-03.md) | แบนเนอร์คุกกี้และบันทึกความยินยอม (Cookie SDK) | CON |
| [SEQ-04](SEQ-04.md) | Consent API: idempotency + transaction + outbox + webhook | CON, PLT |
| [SEQ-05](SEQ-05.md) | ยื่นคำขอใช้สิทธิ (DSAR) และยืนยันตัวตนด้วย OTP | DSAR, IAM |
| [SEQ-06](SEQ-06.md) | เหตุละเมิดข้อมูล: นับเวลา 72 ชม. และการแจ้ง สคส. / เจ้าของข้อมูล | BRE |
| [SEQ-07](SEQ-07.md) | สร้างเอกสาร DPA / DSA / ประกาศ เป็น PDF (Gotenberg) | PLT, PNG, DPA, DSA |
