# Modules (Epics)

1 epic = 1 ไฟล์ · Go package และ PostgreSQL schema ตาม [code-structure](../architecture/code-structure.md) · ID ของ feature ตรงกับ Function List, Development Plan และ `docs/backlog/backlog.csv`

| Epic | ชื่อ | features | phases | schema | Go package |
|---|---|---|---|---|---|
| [PLT](PLT.md) | โครงสร้างพื้นฐานแพลตฟอร์ม (Platform Foundation) | 22 | P0, P1, P3, P4 | platform | `backend/internal/platform/<service>` |
| [IAM](IAM.md) | ระบบจัดการผู้ใช้และสิทธิ์ (User & Permission) | 21 | P0, P1, P3, P4 | iam | `backend/internal/iam` |
| [ORG](ORG.md) | ข้อมูลหน่วยงาน (Organization) | 10 | P0, P1, P3, P4 | org | `backend/internal/org` |
| [CON](CON.md) | คุกกี้และความยินยอม (Cookies & Consent) | 24 | P1, P3, P4 | consent · cookie | `backend/internal/consent · backend/internal/cookie` |
| [PNG](PNG.md) | ประกาศความเป็นส่วนตัว (Privacy Notice Generator) | 16 | P1, P3, P4 | notice | `backend/internal/notice` |
| [ROPA](ROPA.md) | บันทึกกิจกรรมการประมวลผล (RoPA) | 20 | P1, P3, P4 | ropa | `backend/internal/ropa` |
| [RTG](RTG.md) | RoPA ฉบับมาตรฐาน (ROPA Template Generator) | 13 | P1, P2, P3, P4 | ropa | `backend/internal/ropa/templates` |
| [DFG](DFG.md) | แผนผังการไหลของข้อมูล (Data Flow Generator) | 11 | P2, P3, P4 | dataflow | `backend/internal/dataflow` |
| [DSAR](DSAR.md) | คำขอใช้สิทธิของเจ้าของข้อมูล (DSAR) | 21 | P1, P3 | dsar | `backend/internal/dsar` |
| [BRE](BRE.md) | แจ้งเหตุละเมิดข้อมูล (Data Breach Notification) | 17 | P1, P3, P4 | breach | `backend/internal/breach` |
| [DPO](DPO.md) | งานของ DPO (DPO Module) | 12 | P1, P3, P4 | dpo | `backend/internal/dpo` |
| [DPIA](DPIA.md) | แบบประเมินผลกระทบ (DPIA) | 18 | P2, P3, P4 | assess | `backend/internal/assess` |
| [RRA](RRA.md) | ประเมินความเสี่ยงกิจกรรม (ROPA Risk Assessment) | 14 | P2, P3 | risk | `backend/internal/risk` |
| [VEN](VEN.md) | ประเมินคู่ค้า (Vendor Assessment) | 15 | P2, P3, P4 | vendor | `backend/internal/vendor` |
| [DPA](DPA.md) | ข้อตกลงการประมวลผลข้อมูล (DPA) | 14 | P2, P3, P4 | agreement | `backend/internal/agreement` |
| [DSA](DSA.md) | ข้อตกลงการแบ่งปันข้อมูล (DSA) | 14 | P2, P3, P4 | agreement | `backend/internal/agreement` |
| [DPX](DPX.md) | DPO ส่วนต่อขยาย (DPO Extension) | 13 | P2, P3, P4 | gov | `backend/internal/gov` |

หมายเหตุ: ORG-09 ถึง ORG-19 (ผู้ใช้และสิทธิ์) อยู่ใน epic IAM ตาม Development Plan · PLT-xx และ IAM-xx คือ feature ที่ PM/SA เพิ่ม
