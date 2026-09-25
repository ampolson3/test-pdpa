# ภาพรวมระบบ

**PDPA Management Platform** — แพลตฟอร์มบริหารการปฏิบัติตาม พ.ร.บ.คุ้มครองข้อมูลส่วนบุคคล พ.ศ. 2562 สำหรับองค์กรในไทย แบบ multi-tenant (SaaS บน cloud ที่มี data center ในไทย และชุดติดตั้ง on-prem) ครอบคลุม 15 โมดูลตาม Function List + บริการกลางของแพลตฟอร์ม (PLT) + ผู้ใช้และสิทธิ์ (IAM) รวม 17 epic 275 feature

จุดต่างที่ต้องรักษา: ภาษาไทยเป็นภาษาหลัก (TH/EN), ข้อมูลอยู่ในประเทศ, ครอบคลุมประกาศ สคส. ล่าสุด, ต่อระบบ Consent เดิม (FSD V3.2), ทุกการกระทำพิสูจน์ย้อนหลังได้ (audit + hash chain)

## Epic

| epic | ชื่อ | features | Must | Should | Nice | phase |
|---|---|---|---|---|---|---|
| [PLT](modules/PLT.md) | โครงสร้างพื้นฐานแพลตฟอร์ม (Platform Foundation) | 22 | 18 | 3 | 1 | P0:15 · P1:4 · P3:2 · P4:1 |
| [IAM](modules/IAM.md) | ระบบจัดการผู้ใช้และสิทธิ์ (User & Permission) | 21 | 11 | 9 | 1 | P0:9 · P1:6 · P3:5 · P4:1 |
| [ORG](modules/ORG.md) | ข้อมูลหน่วยงาน (Organization) | 10 | 4 | 5 | 1 | P0:5 · P1:1 · P3:3 · P4:1 |
| [CON](modules/CON.md) | คุกกี้และความยินยอม (Cookies & Consent) | 24 | 12 | 11 | 1 | P1:18 · P3:5 · P4:1 |
| [PNG](modules/PNG.md) | ประกาศความเป็นส่วนตัว (Privacy Notice Generator) | 16 | 7 | 8 | 1 | P1:9 · P3:6 · P4:1 |
| [ROPA](modules/ROPA.md) | บันทึกกิจกรรมการประมวลผล (RoPA) | 20 | 11 | 8 | 1 | P1:13 · P3:6 · P4:1 |
| [RTG](modules/RTG.md) | RoPA ฉบับมาตรฐาน (ROPA Template Generator) | 13 | 5 | 7 | 1 | P1:2 · P2:3 · P3:7 · P4:1 |
| [DFG](modules/DFG.md) | แผนผังการไหลของข้อมูล (Data Flow Generator) | 11 | 5 | 4 | 2 | P2:5 · P3:4 · P4:2 |
| [DSAR](modules/DSAR.md) | คำขอใช้สิทธิของเจ้าของข้อมูล (DSAR) | 21 | 13 | 8 | 0 | P1:13 · P3:8 |
| [BRE](modules/BRE.md) | แจ้งเหตุละเมิดข้อมูล (Data Breach Notification) | 17 | 12 | 3 | 2 | P1:12 · P3:3 · P4:2 |
| [DPO](modules/DPO.md) | งานของ DPO (DPO Module) | 12 | 3 | 8 | 1 | P1:4 · P3:7 · P4:1 |
| [DPIA](modules/DPIA.md) | แบบประเมินผลกระทบ (DPIA) | 18 | 12 | 4 | 2 | P2:12 · P3:4 · P4:2 |
| [RRA](modules/RRA.md) | ประเมินความเสี่ยงกิจกรรม (ROPA Risk Assessment) | 14 | 6 | 8 | 0 | P2:6 · P3:8 |
| [VEN](modules/VEN.md) | ประเมินคู่ค้า (Vendor Assessment) | 15 | 7 | 7 | 1 | P2:7 · P3:7 · P4:1 |
| [DPA](modules/DPA.md) | ข้อตกลงการประมวลผลข้อมูล (DPA) | 14 | 6 | 7 | 1 | P2:6 · P3:7 · P4:1 |
| [DSA](modules/DSA.md) | ข้อตกลงการแบ่งปันข้อมูล (DSA) | 14 | 6 | 6 | 2 | P2:6 · P3:6 · P4:2 |
| [DPX](modules/DPX.md) | DPO ส่วนต่อขยาย (DPO Extension) | 13 | 2 | 6 | 5 | P2:2 · P3:6 · P4:5 |
| **รวม** |  | 275 | 140 | 112 | 23 |  |

## Phase

| phase | ชื่อ | ขอบเขต |
|---|---|---|
| P0 | Foundation + User & Permission | โครง Go / Next.js, multi-tenant, บริการกลาง, ผู้ใช้ สิทธิ์ และข้อมูลหน่วยงาน |
| P1 | MVP กฎหมายหลัก | Consent & Cookie, Privacy Notice, RoPA, DSAR, Breach และ DPO พื้นฐาน พร้อมใช้งานจริง |
| P2 | Governance & คู่ค้า | DPIA, ความเสี่ยง RoPA, RoPA Template, Data Flow, Vendor, DPA, DSA, Retention และ TIA |
| P3 | Should | ฟังก์ชันมาตรฐานตลาดและความต้องการของลูกค้าองค์กร |
| P4 | Nice-to-have + AI | จุดขายเพิ่มเติม ฟีเจอร์ AI และแนวโน้มตลาด |

ลำดับบังคับ: **P0 ต้องเสร็จก่อน** (scaffold, CI/CD, migration, บริการกลาง PLT, IAM + RLS) — ทุก module พึ่งพา

## Actors (31 กลุ่ม)

actor ใน use case ≠ role ในระบบสิทธิ์ (16 role) — ดู mapping ใน [security/permissions.md](security/permissions.md)

| key | ชื่อ | English | ประเภท | การยืนยันตัวตน |
|---|---|---|---|---|
| SUPER | ผู้ให้บริการแพลตฟอร์ม | Platform Super Admin / Content team | ผู้ใช้ภายใน | OIDC + MFA + IP allowlist · Provider console |
| ORGADMIN | ผู้ดูแลระบบขององค์กร | Organization Admin | ผู้ใช้ภายใน | OIDC (Keycloak) + MFA บังคับ · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | ผู้ใช้ภายใน | OIDC SSO + MFA · Admin app |
| LEGAL | ฝ่ายกฎหมาย | Legal | ผู้ใช้ภายใน | OIDC SSO + MFA · Admin app |
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | ผู้ใช้ภายใน | OIDC SSO · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | ผู้ใช้ภายใน | OIDC SSO + MFA · Admin app |
| MKT | การตลาด / ธุรกิจ | Marketing / Business | ผู้ใช้ภายใน | OIDC SSO · Admin app |
| FRONT | พนักงานหน้าร้าน / Call center | Front Staff | ผู้ใช้ภายใน | OIDC SSO · Admin app (หน้าจอหน้าร้าน) |
| SEC | ทีม Security / Incident | Security / Incident Response | ผู้ใช้ภายใน | OIDC SSO + MFA · Admin app |
| PROC | จัดซื้อ / ผู้ดูแลคู่ค้า | Procurement / Vendor Manager | ผู้ใช้ภายใน | OIDC SSO · Admin app |
| AUDIT | ผู้ตรวจสอบ | Auditor | ผู้ใช้ภายใน | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| EXEC | ผู้บริหาร / ผู้มีอำนาจอนุมัติ | Executive / Approver | ผู้ใช้ภายใน | OIDC SSO · อนุมัติผ่านอีเมล/แอป |
| EMP | พนักงาน / ผู้ใช้งานทุกคน | Employee / Any user | ผู้ใช้ภายใน | OIDC SSO · Admin app / portal พนักงาน |
| MANAGER | หัวหน้าแผนก | Line Manager | ผู้ใช้ภายใน | OIDC SSO · Admin app |
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | บุคคลภายนอก | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| GUARD | ผู้ปกครอง / ผู้รับมอบอำนาจ | Guardian / Authorized Agent | บุคคลภายนอก | Portal: OTP + เอกสารพิสูจน์อำนาจ |
| PUBLIC | บุคคลภายนอกที่พบเหตุ | External Reporter | บุคคลภายนอก | ฟอร์มแจ้งเหตุสาธารณะ + CAPTCHA |
| VENDOR | คู่ค้า / ผู้ประมวลผล (guest) | Vendor / Processor | บุคคลภายนอก | Guest link (token หมดอายุ) + OTP |
| COUNTER | คู่สัญญา (guest) | Agreement Counterparty | บุคคลภายนอก | Guest link (token หมดอายุ) + OTP |
| PDPC | สคส. | PDPC (Regulator) | บุคคลภายนอก | ไม่ login: รับ/ส่งผ่านช่องทางของ สคส. |
| WEB | เว็บไซต์ / แอปขององค์กร (SDK) | Website / App with SDK | ระบบ | Public key ของ collection point + CORS allowlist |
| EXT | ระบบธุรกิจ CRM / POS / CDP | Enterprise Systems (API) | ระบบ | OAuth2 client credentials (token ≤ 15 นาที · scope) |
| IDP | Keycloak / IdP องค์กร / ThaID | Identity Provider | ระบบ | OIDC / SAML federation |
| DIR | Directory (Entra ID / Okta / HRIS) | Directory / SCIM | ระบบ | SCIM 2.0 bearer token |
| APICLIENT | ระบบภายนอก (API client) | API Client | ระบบ | OAuth2 client credentials (token ≤ 15 นาที · scope) |
| ESIGN | ผู้ให้บริการ e-Signature | e-Signature Provider | ระบบ | API key + callback ลงชื่อ HMAC |
| LLM | บริการ AI (LLM) | AI Service | ระบบ | ผ่าน AI gateway (mask PII ก่อนส่ง) |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ระบบ | ภายในระบบ (River worker / cron) |
| DATASRC | แหล่งข้อมูล (DB / File / Cloud) | Data Sources | ระบบ | Connector credential ใน OpenBao · read-only |
| HRIS | HRIS / ITSM / SIEM | HR, ITSM & SIEM | ระบบ | Connector / webhook / SCIM |
| GUEST | ผู้ใช้ภายนอก (guest link) | External Guest | บุคคลภายนอก | Guest link (token หมดอายุ) + OTP |

## ภาพรวมการไหลของงานหลัก

- **ความยินยอม**: เว็บ / แอป / หน้าร้าน / API → Collection Point → Consent Transaction + Receipt → outbox → webhook ไป CRM/CDP → reconcile ([BP-01](processes/BP-01.md), [BP-02](processes/BP-02.md), [SEQ-04](sequences/SEQ-04.md))
- **คุกกี้**: สแกนเว็บ → จัดหมวด → แบนเนอร์ผ่าน CDN → หลักฐานความยินยอม ([BP-03](processes/BP-03.md), [SEQ-03](sequences/SEQ-03.md))
- **ประกาศ**: ร่าง → checklist ม.23 → อนุมัติ → เผยแพร่เวอร์ชัน → รับทราบ ([BP-04](processes/BP-04.md))
- **RoPA**: แบบสอบถาม / template → ร่าง → อนุมัติ → คะแนนความเสี่ยง → DPIA ถ้าเสี่ยงสูง ([BP-05](processes/BP-05.md), [BP-08](processes/BP-08.md))
- **DSAR**: ยื่น → ยืนยันตัวตน → คัดกรอง → subtask ไประบบต้นทาง → อนุมัติ → ส่งผล ภายใน 30 วัน ([BP-06](processes/BP-06.md))
- **เหตุละเมิด**: แจ้งเหตุ → triage → ประเมินความเสี่ยง → แจ้ง สคส. ภายใน 72 ชม. / แจ้งเจ้าของข้อมูล → แก้ไข ([BP-07](processes/BP-07.md))
- **คู่ค้าและสัญญา**: รับคู่ค้า → แบบประเมิน (guest link) → อนุมัติ → DPA / DSA → ลงนาม → ติดตามหมดอายุ ([BP-09](processes/BP-09.md), [BP-10](processes/BP-10.md))
- **Retention**: job ตรวจครบกำหนด → เจ้าของข้อมูลยืนยัน → DPO อนุมัติ → IT ทำลาย + หลักฐาน ([BP-11](processes/BP-11.md))
- **ผู้ใช้และสิทธิ์**: SCIM / invite → role + scope → MFA → ขอสิทธิ์ (maker-checker) → ทบทวนทุก 6 เดือน → ปิดบัญชีภายใน 24 ชม. ([BP-12](processes/BP-12.md))
