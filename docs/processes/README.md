# Business processes

กระบวนการทางธุรกิจแบบ swimlane แปลงจากหน้า BP-xx ใน draw.io เป็น Mermaid + ตารางขั้นตอน ใช้คู่กับ use case ใน `docs/modules/` และ state machine ใน `docs/states/`

| รหัส | กระบวนการ | lanes | จำนวน UC |
|---|---|---|---|
| [BP-01](BP-01.md) | เก็บความยินยอมทุกช่องทาง (Consent capture) | เจ้าของข้อมูล · ช่องทาง: เว็บ / แอป / หน้าร้าน · PDPA Platform · Consent · ระบบปลายทาง CRM / CDP | 7 |
| [BP-02](BP-02.md) | ถอนความยินยอม หมดอายุ และกระทบยอด (Withdrawal, expiry & reconcile) | เจ้าของข้อมูล · PDPA Platform · Consent · ระบบปลายทาง CRM / CDP · DPO / การตลาด | 5 |
| [BP-03](BP-03.md) | แบนเนอร์คุกกี้และการสแกนเว็บไซต์ (Cookie consent & scanning) | ผู้เข้าชมเว็บไซต์ · Browser · Cookie SDK · CDN + Public API · PDPA Platform · Scanner / Admin · DPO / เจ้าของเว็บไซต์ | 8 |
| [BP-04](BP-04.md) | จัดทำและเผยแพร่ประกาศความเป็นส่วนตัว (Privacy notice lifecycle) | ฝ่ายกฎหมาย / เจ้าของประกาศ · PDPA Platform · Notice · DPO · เจ้าของข้อมูล / เว็บไซต์ | 16 |
| [BP-05](BP-05.md) | จัดทำและอนุมัติ RoPA (Record of Processing Activities) | เจ้าของกระบวนการ / Champion · PDPA Platform · RoPA · DPO | 24 |
| [BP-06](BP-06.md) | จัดการคำขอใช้สิทธิของเจ้าของข้อมูล (DSAR) | เจ้าของข้อมูล / ผู้แทน · PDPA Platform · DSAR · DPO / Privacy team · เจ้าของระบบ / IT / ผู้ประมวลผล | 21 |
| [BP-07](BP-07.md) | จัดการและแจ้งเหตุละเมิดข้อมูลส่วนบุคคล (Data breach 72 ชม.) | ผู้พบเหตุ (พนักงาน / คู่ค้า / ภายนอก) · ทีม Security / Incident · DPO · PDPA Platform · Breach · สคส. / เจ้าของข้อมูล | 17 |
| [BP-08](BP-08.md) | ประเมินผลกระทบด้านการคุ้มครองข้อมูล (DPIA) | เจ้าของโครงการ / กระบวนการ · PDPA Platform · Assessment · IT / Security · DPO · ผู้บริหาร / ผู้มีอำนาจอนุมัติ | 18 |
| [BP-09](BP-09.md) | รับคู่ค้าใหม่และติดตามความเสี่ยงคู่ค้า (Vendor onboarding & monitoring) | จัดซื้อ / เจ้าของงาน · PDPA Platform · Vendor · คู่ค้า (guest link) · DPO / Security · ฝ่ายกฎหมาย | 15 |
| [BP-10](BP-10.md) | จัดทำและบริหารข้อตกลง DPA / DSA (Agreement lifecycle) | ฝ่ายกฎหมาย · PDPA Platform · Agreement · DPO · คู่สัญญา (guest link) · e-Signature | 28 |
| [BP-11](BP-11.md) | ระยะเวลาเก็บรักษาและการทำลายข้อมูล (Retention & disposal) | PDPA Platform · Scheduler · เจ้าของข้อมูลในองค์กร (Data owner) · DPO · เจ้าของระบบ / IT | 3 |
| [BP-12](BP-12.md) | บริหารผู้ใช้และสิทธิ์ (Joiner-Mover-Leaver & access review) | HRIS / Directory (SCIM) · PDPA Platform · IAM · Keycloak / IdP · ผู้ใช้ / หัวหน้างาน · ผู้ดูแลระบบองค์กร | 21 |
