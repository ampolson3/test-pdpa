# State machines

สถานะของ entity หลัก ตรงกับ CHECK constraint ใน migration (ตรวจอัตโนมัติตอน generate ชุดนี้แล้ว) · ไฟล์ [`state-machines.yaml`](state-machines.yaml) คือฉบับ machine-readable สำหรับ generate โค้ดและ test

| รหัส | เรื่อง | คอลัมน์ |
|---|---|---|
| [ST-01](ST-01.md) | สถานะความยินยอม | `consent.consent_status.status` |
| [ST-02](ST-02.md) | คำขอใช้สิทธิ | `dsar.requests.status` |
| [ST-03](ST-03.md) | เหตุละเมิดข้อมูล | `breach.incidents.status` |
| [ST-04](ST-04.md) | ข้อตกลง DPA / DSA และประกาศความเป็นส่วนตัว | `agreement.agreements.status` · `notice.notices.status` |
| [ST-05](ST-05.md) | กิจกรรม RoPA และแบบประเมิน | `ropa.processing_activities.status` · `assess.assessments.status` |
| [ST-06](ST-06.md) | คู่ค้า / ผู้ประมวลผล | `vendor.vendors.status` |
| [ST-07](ST-07.md) | บัญชีผู้ใช้ | `iam.users.status` · `platform.webhook_deliveries.status` |
