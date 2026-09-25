# CON — คุกกี้และความยินยอม (Cookies & Consent)

> ระบบบริหารจัดการความยินยอม (Cookies and Consent Management) · ขอบเขต: คุกกี้และความยินยอมทุกช่องทาง ตั้งแต่เก็บ พิสูจน์ ถอน จนเชื่อมระบบปลายทาง  
> 24 features · Must 12 / Should 11 / Nice 1 · phase: P1 (18), P3 (5), P4 (1)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/consent · backend/internal/cookie` |
| PostgreSQL schema | [`consent`](../data/consent.md) · [`cookie`](../data/cookie.md) (30 ตาราง) |
| Admin API prefix | `/admin/v1/consent` · `/admin/v1/cookie` |
| Endpoint ที่ SA กำหนดแล้ว | `GET /public/v1/collection-points/{key}` — โหลด collection point + purpose เวอร์ชันล่าสุด (BP-01)<br>`POST /public/v1/consents` — บันทึกความยินยอมจากเว็บ / แอป (Idempotency-Key) (BP-01)<br>`POST /public/v1/consents/confirm` — ยืนยัน double opt-in (BP-01)<br>`GET /public/v1/cookie/banners/{domainKey}` — config แบนเนอร์ผ่าน CDN (SEQ-03)<br>`POST /public/v1/cookie/consents` — หลักฐานความยินยอมคุกกี้ (SEQ-03)<br>`GET /public/v1/preferences` — preference center (session ของ portal) (BP-02)<br>`POST /api/v1/consents` — ระบบช่องทางส่ง consent (OAuth2 scope consent.record.create) (SEQ-04)<br>`POST /admin/v1/cookie/domains/{id}/scans` — สั่งสแกนเว็บไซต์ (BP-03) |
| หน้าจอ (Next.js) | admin: /consent/*, /cookies/*; portal: /preferences, /c/[id]; SDK |
| พึ่งพาบริการ | forms, events, crypto, notify, scanner |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-04 CON, BP-01, BP-02, BP-03, DFD-1, DFD-2.3, ERD-05, ERD-06, SEQ-03, SEQ-04, ST-01 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| DS | เจ้าของข้อมูล / ผู้เข้าชมเว็บ | Data Subject / Visitor | Portal: OTP อีเมล/SMS หรือ ThaID (ไม่ต้องมีบัญชี) |
| GUARD | ผู้ปกครอง / ผู้รับมอบอำนาจ | Guardian / Authorized Agent | Portal: OTP + เอกสารพิสูจน์อำนาจ |
| FRONT | พนักงานหน้าร้าน / Call center | Front Staff | OIDC SSO · Admin app (หน้าจอหน้าร้าน) |
| WEB | เว็บไซต์ / แอปขององค์กร (SDK) | Website / App with SDK | Public key ของ collection point + CORS allowlist |
| MKT | การตลาด / ธุรกิจ | Marketing / Business | OIDC SSO · Admin app |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| EXT | ระบบธุรกิจ CRM / POS / CDP | Enterprise Systems (API) | OAuth2 client credentials (token ≤ 15 นาที · scope) |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [CON-01](#con-01) | แบนเนอร์คุกกี้บนเว็บไซต์ | Must | P1 | DS WEB | M | XL | Y | BP-03 |
| [CON-02](#con-02) | ตั้งค่าคุกกี้รายหมวด | Must | P1 | DS | S | M | Y | BP-03 |
| [CON-03](#con-03) | บล็อกสคริปต์ก่อนได้รับความยินยอม | Must | P1 | WEB IT | S | L | N | BP-03 |
| [CON-04](#con-04) | สแกนและจัดหมวดคุกกี้อัตโนมัติ | Should | P1 | IT SCHED | L | M | Y | BP-03 |
| [CON-06](#con-06) | ปรับแต่งดีไซน์และหลายภาษา | Should | P1 | MKT | S | M | Y | BP-03 |
| [CON-09](#con-09) | แบบฟอร์มขอความยินยอม (Collection Point) | Must | P1 | MKT DPO | M | M | Y | BP-01 |
| [CON-10](#con-10) | ความยินยอมโดยชัดแจ้งสำหรับข้อมูลอ่อนไหว | Must | P1 | DPO | S | S | N | BP-01 |
| [CON-11](#con-11) | ความยินยอมผู้เยาว์และผู้ปกครอง | Must | P1 | DS GUARD | M | M | Y | BP-01 |
| [CON-12](#con-12) | เวอร์ชันข้อความยินยอม | Must | P1 | DPO | M | S | N |  |
| [CON-13](#con-13) | ถอนความยินยอม | Must | P1 | DS EXT | M | S | N | BP-02 |
| [CON-14](#con-14) | เก็บความยินยอมหลายช่องทาง | Must | P1 | FRONT EXT | M | M | Y | BP-01 |
| [CON-15](#con-15) | หลักฐานความยินยอม (Receipt) | Must | P1 | DS DPO | L | S | N | BP-01 |
| [CON-16](#con-16) | REST API และ Webhook | Must | P1 | EXT | L | S | N | BP-01 |
| [CON-17](#con-17) | ความปลอดภัยและที่เก็บข้อมูล | Must | P1 | IT | S | XS | N |  |
| [CON-18](#con-18) | ศูนย์ตั้งค่าความเป็นส่วนตัว | Should | P1 | DS | M | M | Y | BP-02 |
| [CON-19](#con-19) | ยืนยันตัวตนก่อนบันทึก | Should | P1 | DS | S | S | N | BP-02 |
| [CON-21](#con-21) | ค้นหา แดชบอร์ด และส่งออก | Should | P1 | DPO MKT EXT | M | M | Y | BP-02 |
| [CON-22](#con-22) | นำเข้าความยินยอมเดิม | Should | P1 | IT | M | S | N |  |
| [CON-05](#con-05) | นโยบายคุกกี้อัตโนมัติ | Should | P3 | DPO | S | S | N | BP-03 |
| [CON-07](#con-07) | หลายโดเมนและ Mobile SDK | Should | P3 | IT | M | L | N | BP-03 |
| [CON-20](#con-20) | วันหมดอายุและต่ออายุ | Should | P3 | SCHED DS | M | S | N | BP-02 |
| [CON-23](#con-23) | Double Opt-In | Should | P3 | DS | S | S | N | BP-01 |
| [CON-24](#con-24) | ส่งคำขอความยินยอมแบบ Bulk | Should | P3 | MKT DS | M | M | Y |  |
| [CON-08](#con-08) | A/B testing และวัด consent rate | Nice | P4 | MKT | M | M | Y | BP-03 |

### ความสัมพันธ์ระหว่าง use case

- CON-01 «include» CON-03 (ทุกครั้งที่ทำ CON-01 ต้องทำ CON-03)
- CON-02 «extend» CON-01 (CON-02 เป็นทางเลือก/ส่วนขยายของ CON-01)
- CON-14 «include» CON-15 (ทุกครั้งที่ทำ CON-14 ต้องทำ CON-15)
- CON-18 «include» CON-19 (ทุกครั้งที่ทำ CON-18 ต้องทำ CON-19)
- CON-13 «extend» CON-18 (CON-13 เป็นทางเลือก/ส่วนขยายของ CON-18)
- CON-11 «extend» CON-09 (CON-11 เป็นทางเลือก/ส่วนขยายของ CON-09)
- CON-23 «extend» CON-09 (CON-23 เป็นทางเลือก/ส่วนขยายของ CON-09)

## กระบวนการ / sequence / state machine

- [BP-01 เก็บความยินยอมทุกช่องทาง (Consent capture)](../processes/BP-01.md)
- [BP-02 ถอนความยินยอม หมดอายุ และกระทบยอด (Withdrawal, expiry & reconcile)](../processes/BP-02.md)
- [BP-03 แบนเนอร์คุกกี้และการสแกนเว็บไซต์ (Cookie consent & scanning)](../processes/BP-03.md)
- [SEQ-03 แบนเนอร์คุกกี้และบันทึกความยินยอม (Cookie SDK)](../sequences/SEQ-03.md)
- [SEQ-04 Consent API: idempotency + transaction + outbox + webhook](../sequences/SEQ-04.md)
- [ST-01 สถานะความยินยอม (consent.consent_status.status)](../states/ST-01.md)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [consent.data_elements](../data/consent.md#consent-data-elements) | Data Element: ข้อมูลที่ใช้ในแต่ละวัตถุประสงค์ |
| [consent.purposes](../data/consent.md#consent-purposes) | Purpose: วัตถุประสงค์ที่ขอความยินยอม |
| [consent.purpose_versions](../data/consent.md#consent-purpose-versions) | ข้อความของ Purpose แต่ละเวอร์ชัน |
| [consent.purpose_preferences](../data/consent.md#consent-purpose-preferences) | Purpose Preference: ตัวเลือกย่อย เช่น ช่องทาง หัวข้อ ความถี่ |
| [consent.purpose_data_elements](../data/consent.md#consent-purpose-data-elements) | Data Element ที่ใช้ในแต่ละ Purpose |
| [consent.collection_points](../data/consent.md#consent-collection-points) | Collection Point: จุดเก็บความยินยอม |
| [consent.collection_point_purposes](../data/consent.md#consent-collection-point-purposes) | Purpose ที่แสดงใน Collection Point |
| [consent.data_subjects](../data/consent.md#consent-data-subjects) | เจ้าของข้อมูล (identifier เข้ารหัส + blind index) |
| [consent.subject_identifiers](../data/consent.md#consent-subject-identifiers) | identifier ของเจ้าของข้อมูล (อีเมล เบอร์ เลขบัตร รหัสลูกค้า) |
| [consent.consent_receipts](../data/consent.md#consent-consent-receipts) | Receipt: หลักฐานการตอบแต่ละครั้ง (hash chain) |
| [consent.consent_transactions](../data/consent.md#consent-consent-transactions) | Consent Transaction: รายการต่อ Purpose (partition รายเดือน) |
| [consent.consent_status](../data/consent.md#consent-consent-status) | สถานะล่าสุดต่อเจ้าของข้อมูล × Purpose (projection) |
| [consent.double_optin_requests](../data/consent.md#consent-double-optin-requests) | คำขอยืนยัน Double Opt-In |
| [consent.guardian_approvals](../data/consent.md#consent-guardian-approvals) | การให้ความยินยอมโดยผู้ปกครอง / ผู้อนุบาล (ม.20) |
| [consent.campaigns](../data/consent.md#consent-campaigns) | แคมเปญขอความยินยอมแบบ bulk |
| [consent.campaign_recipients](../data/consent.md#consent-campaign-recipients) | ผู้รับของแคมเปญ |
| [consent.reconcile_runs](../data/consent.md#consent-reconcile-runs) | Reconcile Report: รอบเทียบสถานะกับระบบปลายทาง |
| [consent.reconcile_items](../data/consent.md#consent-reconcile-items) | รายการที่สถานะไม่ตรงกัน |
| [consent.downstream_syncs](../data/consent.md#consent-downstream-syncs) | การยืนยันจากระบบปลายทางหลังเปลี่ยนสถานะ |
| [cookie.domains](../data/cookie.md#cookie-domains) | โดเมน / เว็บไซต์ที่ใช้แบนเนอร์ |
| [cookie.apps](../data/cookie.md#cookie-apps) | แอป / LINE LIFF ที่ใช้ SDK |
| [cookie.banner_configs](../data/cookie.md#cookie-banner-configs) | config แบนเนอร์ (มีเวอร์ชัน publish ไป CDN) |
| [cookie.categories](../data/cookie.md#cookie-categories) | หมวดคุกกี้ |
| [cookie.cookies](../data/cookie.md#cookie-cookies) | คุกกี้ที่พบ / ประกาศไว้ |
| [cookie.cookie_kb](../data/cookie.md#cookie-cookie-kb) | ฐานข้อมูลคุกกี้กลาง (ใช้จัดหมวดอัตโนมัติ) |
| [cookie.scans](../data/cookie.md#cookie-scans) | รอบสแกนเว็บไซต์ |
| [cookie.scan_findings](../data/cookie.md#cookie-scan-findings) | คุกกี้ / tracker ที่พบในแต่ละรอบ |
| [cookie.script_rules](../data/cookie.md#cookie-script-rules) | กฎบล็อกสคริปต์ก่อนได้รับความยินยอม |
| [cookie.ab_variants](../data/cookie.md#cookie-ab-variants) | variant แบนเนอร์สำหรับ A/B test |
| [cookie.consent_records](../data/cookie.md#cookie-consent-records) | หลักฐานความยินยอมคุกกี้ของผู้เข้าชม (partition รายเดือน) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `cookie.banner.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `cookie.banner` | แบนเนอร์คุกกี้และโดเมน | DPO `CRUDAP` · PRIVACY `CRU` · LEGAL `R` · IT `RU` · MKT `R` · AUDIT `R` | IT คัดลอก script และแก้ค่าทางเทคนิค |
| `cookie.scan` | ผลสแกนคุกกี้ | DPO `CRUDX` · PRIVACY `CRUX` · IT `CRUX` · MKT `R` · AUDIT `R` |  |
| `consent.purpose` | Purpose / Data Element / Purpose Preference | DPO `CRUDAP` · PRIVACY `CRU` · LEGAL `RU` · MKT `CRU` · AUDIT `R` · API `R` | Marketing ร่างได้ DPO อนุมัติและเปิดใช้ |
| `consent.collectionpoint` | Collection Point / แบบฟอร์ม | DPO `CRUDAP` · PRIVACY `CRU` · IT `RU` · MKT `CRU` · AUDIT `R` · API `R` |  |
| `consent.record` | ความยินยอมรายบุคคล / โปรไฟล์ | DPO `RE` · PRIVACY `RE` · IT `R` · MKT `R` · FRONT `R` · AUDIT `RE` · API `CR` | identifier แสดงแบบปกปิด ยกเว้นมีสิทธิ์ pii.unmask |
| `consent.onbehalf` | บันทึกความยินยอมแทนลูกค้า | DPO `CR` · PRIVACY `CR` · FRONT `CR` · AUDIT `R` | Front Staff เห็นเฉพาะรายการที่ตนบันทึกในสาขา |
| `consent.bulk` | นำเข้า / ส่งออกความยินยอม | DPO `XE` · PRIVACY `XE` · IT `X` · MKT `E` · AUDIT `E` | export จำนวนมากต้องมีผู้อนุมัติ |
| `consent.campaign` | แคมเปญขอความยินยอม | DPO `CRUDA` · PRIVACY `CRU` · MKT `CRUX` · AUDIT `R` |  |
| `pii.unmask` | เปิดดูข้อมูลส่วนบุคคลแบบไม่ปกปิด | DPO `X` · PRIVACY `X` · SEC `X` | ต้องระบุเหตุผล ถูกบันทึก log ทุกครั้ง และอาจต้องยืนยัน MFA ซ้ำ |

## Event ที่ module นี้ปล่อย (ผ่าน outbox)

| event | ฟิลด์หลักใน data | ผู้รับ |
|---|---|---|
| `consent.granted` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.denied` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.withdrawn` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.expired` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.preferences_changed` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `consent.mismatch_found` | subject_ref · purpose_code · purpose_version · channel · occurred_at | webhook subscriber (CRM / CDP) · reconcile |
| `cookie.banner_published` | domain · banner_version · new_cookies | DPO dashboard · CDN purge |
| `cookie.scan_completed` | domain · banner_version · new_cookies | DPO dashboard · CDN purge |

## Background jobs (River)

| job | รอบ | หน้าที่ | อ้างอิง |
|---|---|---|---|
| `consent.expiry_sweep` | River cron รายวัน | ACTIVE ที่ครบ expires_at → EXPIRED + event consent.expired | ST-01 / BP-02 |
| `consent.reconcile` | ตามรอบ / หลัง webhook dead | เทียบสถานะกับระบบปลายทาง → consent.mismatch_found | BP-02 |
| `cookie.scan` | ตามรอบของโดเมน / สั่งเอง | scanner (chromedp) crawl เว็บ → cookie.scan_completed | BP-03 |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P1:** CON-01, CON-02, CON-03, CON-09, CON-10, CON-11, CON-12, CON-13, CON-14, CON-15, CON-16, CON-17, CON-04, CON-06, CON-18, CON-19, CON-21, CON-22
- **P3:** CON-05, CON-07, CON-20, CON-23, CON-24
- **P4:** CON-08

## รายละเอียด feature

<a id="con-01"></a>
### CON-01 แบนเนอร์คุกกี้บนเว็บไซต์

*Cookie consent banner*

- **Priority / Phase:** Must · P1 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19, ม.23
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), WEB (เว็บไซต์ / แอปขององค์กร (SDK))
- **ขนาดงาน:** BE M (5 วัน) · FE XL (20 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-17, PLT-11
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** แสดงเมื่อเข้าเว็บครั้งแรก ปุ่มยอมรับทั้งหมด / ปฏิเสธทั้งหมด / ตั้งค่า น้ำหนักเท่ากัน ไม่ติ๊กไว้ล่วงหน้า

**Backend (Go):** config แบนเนอร์ต่อโดเมน publish เป็น JSON บน CDN, รับ consent event แบบ anonymous (cookie ID) + เก็บหลักฐาน, จัดการโดเมนและยืนยันความเป็นเจ้าของ

**Frontend (Next.js):** cookie SDK (vanilla TypeScript ≤ 30 KB gzip): banner / modal ปุ่มยอมรับ-ปฏิเสธ-ตั้งค่า น้ำหนักเท่ากัน, first-party cookie, TH/EN; หน้า admin ตั้งค่าแบนเนอร์ + โค้ดติดตั้ง

**Acceptance criteria:** ไม่มีช่องติ๊กล่วงหน้า; ปฏิเสธทั้งหมดได้ในคลิกเดียว; ขนาด SDK ไม่เกิน 30 KB gzip

<a id="con-02"></a>
### CON-02 ตั้งค่าคุกกี้รายหมวด

*Granular cookie preferences*

- **Priority / Phase:** Must · P1 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** CON-01
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** แยกหมวด Strictly Necessary / Functional / Analytics / Marketing ให้เลือกเปิด-ปิดรายหมวด

**Backend (Go):** หมวดคุกกี้ (ค่าเริ่มต้น 4 หมวด) + รายการคุกกี้ต่อหมวด, บันทึกทางเลือกรายหมวด

**Frontend (Next.js):** preference modal ใน SDK + หน้า admin จัดหมวด

**Acceptance criteria:** เปิด/ปิดรายหมวดแล้วมีผลทันทีกับสคริปต์ในหมวดนั้น

<a id="con-03"></a>
### CON-03 บล็อกสคริปต์ก่อนได้รับความยินยอม

*Prior blocking*

- **Priority / Phase:** Must · P1 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19
- **Actor:** WEB (เว็บไซต์ / แอปขององค์กร (SDK)), IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE S (3 วัน) · FE L (10 วัน) · UX —
- **ขึ้นกับ:** CON-02
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** ไม่โหลดคุกกี้/แท็กที่ไม่จำเป็นจนกว่าจะได้รับความยินยอม รองรับ Google Consent Mode v2 และ GTM

**Backend (Go):** กฎ blocking ต่อ script / โดเมน

**Frontend (Next.js):** auto-blocking ใน SDK (MutationObserver + rewrite type=text/plain), Google Consent Mode v2, GTM template, event สำหรับ tag manager

**Acceptance criteria:** ก่อนยินยอมไม่มีคุกกี้ Analytics / Marketing ถูกตั้ง (Playwright test บนเว็บตัวอย่าง)

<a id="con-04"></a>
### CON-04 สแกนและจัดหมวดคุกกี้อัตโนมัติ

*Cookie scanner*

- **Priority / Phase:** Should · P1 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** IT (เจ้าของระบบ / IT), SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE L (10 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-10
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** สแกนตามรอบ ตรวจจับคุกกี้/แท็กใหม่ และจัดหมวดจากฐานข้อมูลคุกกี้

**Backend (Go):** scanner worker (Go + chromedp): crawl sitemap / ลิงก์, เก็บคุกกี้ localStorage และ request ไปโดเมนภายนอก, จัดหมวดจากฐานข้อมูลคุกกี้ + กฎของ tenant, สแกนตามรอบ, แจ้งคุกกี้ใหม่

**Frontend (Next.js):** หน้าเริ่มสแกน / ผลสแกน, จัดหมวดคุกกี้ที่ไม่รู้จัก, เทียบกับรอบก่อน

**Acceptance criteria:** สแกนเว็บ 200 หน้าเสร็จภายใน 30 นาที และคุกกี้ใหม่ถูกแจ้งเตือน

**หมายเหตุ:** ดึงเข้า P1: แบนเนอร์ที่ไม่มี scanner แข่งในตลาดยาก

<a id="con-06"></a>
### CON-06 ปรับแต่งดีไซน์และหลายภาษา

*Branding & multi-language*

- **Priority / Phase:** Should · P1 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** MKT (การตลาด / ธุรกิจ)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** CON-01
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** ปรับสี โลโก้ ตำแหน่ง CSS ดูตัวอย่างหลายขนาดจอ และตรวจภาษา browser

**Backend (Go):** theme (สี โลโก้ ตำแหน่ง layout), custom CSS แบบจำกัด, ข้อความ TH/EN + ภาษาอื่น, ตรวจภาษา browser

**Frontend (Next.js):** editor ปรับดีไซน์ + preview หลายขนาดจอ

**Acceptance criteria:** เปลี่ยนดีไซน์แล้วเห็นผลใน preview และบนเว็บหลัง publish

**หมายเหตุ:** ดึงเข้า P1 เพราะเป็นส่วนหนึ่งของแบนเนอร์ที่ลูกค้าใช้จริง

<a id="con-09"></a>
### CON-09 แบบฟอร์มขอความยินยอม (Collection Point)

*Consent form builder*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19 วรรคสาม-สี่
- **Actor:** MKT (การตลาด / ธุรกิจ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-06, CON-12
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** สร้างฟอร์มแบบ no-code ข้อความชัดเจนแยกจากเรื่องอื่น แยกวัตถุประสงค์ และไม่ผูกเป็นเงื่อนไขสัญญาเกินจำเป็น

**Backend (Go):** Purpose, Data Element, Purpose Preference, Collection Point ตาม FSD V3.2; ฟอร์มผูกหลาย Purpose, checklist ม.19 (แยกข้อความ / ไม่ผูกเงื่อนไขเกินจำเป็น) ก่อน publish

**Frontend (Next.js):** form builder (PLT-06) + ลิงก์ / embed / QR / API endpoint

**Acceptance criteria:** ฟอร์มที่ publish แล้วรับความยินยอมแยกรายวัตถุประสงค์และออก receipt ทุกครั้ง

**หมายเหตุ:** ย้าย domain model จากระบบ Consent เดิม

<a id="con-10"></a>
### CON-10 ความยินยอมโดยชัดแจ้งสำหรับข้อมูลอ่อนไหว

*Explicit consent for sensitive data*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.26
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** CON-09, ORG-07
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** แยกข้อความและช่องยินยอมเฉพาะสำหรับข้อมูลตาม ม.26

**Backend (Go):** Purpose ที่ใช้ข้อมูล ม.26 ต้องมีช่องยินยอมแยกและข้อความเฉพาะ ตรวจตอน publish

**Frontend (Next.js):** ส่วนตั้งค่าข้อมูลอ่อนไหวใน Purpose

**Acceptance criteria:** publish ไม่ได้ถ้า Purpose ข้อมูลอ่อนไหวไม่มีช่องยินยอมแยก

<a id="con-11"></a>
### CON-11 ความยินยอมผู้เยาว์และผู้ปกครอง

*Minor / guardian consent*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.20
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), GUARD (ผู้ปกครอง / ผู้รับมอบอำนาจ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** CON-09, IAM-05
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** ตรวจอายุตามเงื่อนไข ม.20 ผูกผู้ปกครองกับผู้เยาว์ และส่งคำขอยืนยันไปยังผู้ปกครอง

**Backend (Go):** ตรวจอายุตาม ม.20 (ไม่เกิน 10 ปีผู้ปกครองให้ความยินยอม, 10-20 ปีตามเงื่อนไข), ผูกโปรไฟล์ผู้ปกครอง-ผู้เยาว์, ส่งคำขอยืนยันให้ผู้ปกครอง

**Frontend (Next.js):** ขั้นตอนผู้เยาว์ในฟอร์ม + หน้าผู้ปกครองยืนยัน

**Acceptance criteria:** ความยินยอมที่ต้องได้จากผู้ปกครองมีผลเมื่อผู้ปกครองยืนยันแล้วเท่านั้น

<a id="con-12"></a>
### CON-12 เวอร์ชันข้อความยินยอม

*Consent versioning*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19, ม.21
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** —

**คำอธิบาย:** เก็บทุกเวอร์ชัน ระบุเวอร์ชันที่ยินยอม และขอความยินยอมใหม่เมื่อเปลี่ยนวัตถุประสงค์

**Backend (Go):** Purpose และข้อความมีเวอร์ชัน, receipt ผูกเวอร์ชัน, เปลี่ยนวัตถุประสงค์แล้วทำเครื่องหมาย re-consent

**Frontend (Next.js):** หน้าเวอร์ชันของ Purpose + diff

**Acceptance criteria:** ย้อนดูได้ว่าแต่ละคนยินยอมข้อความเวอร์ชันใด

<a id="con-13"></a>
### CON-13 ถอนความยินยอม

*Consent withdrawal*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19 วรรคห้า
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** CON-15, PLT-11
- **Process:** [BP-02](../processes/BP-02.md)

**คำอธิบาย:** ถอนได้ง่ายเท่ากับการให้ ทุกช่องทาง มีผลทันที เลือกเหตุผลการถอน และหยุดการประมวลผลในระบบปลายทาง

**Backend (Go):** ถอนผ่าน preference center / API / พนักงาน, เลือกเหตุผล, มีผลทันที, event consent.withdrawn → webhook ไประบบปลายทาง + ติดตามการยืนยัน

**Frontend (Next.js):** ปุ่มถอนใน preference center และหน้าพนักงาน

**Acceptance criteria:** ถอนแล้วสถานะเปลี่ยนทันทีและระบบปลายทางได้รับ event ภายใน 1 นาที

<a id="con-14"></a>
### CON-14 เก็บความยินยอมหลายช่องทาง

*Omni-channel capture*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19
- **Actor:** FRONT (พนักงานหน้าร้าน / Call center), EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** CON-16, IAM-02
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** เว็บ แอป POS/หน้าร้าน Call center Kiosk LINE เอกสารกระดาษ พนักงานบันทึกแทนลูกค้า และ QR code

**Backend (Go):** API สำหรับ POS / Call center / Kiosk, บันทึกแทนลูกค้า (on behalf) พร้อมผู้บันทึกและสาขา, QR code ต่อ collection point, แนบไฟล์สแกนเอกสารกระดาษ

**Frontend (Next.js):** หน้าพนักงานบันทึกความยินยอม (โหมดแท็บเล็ต), สร้าง QR, ตัวอย่าง LINE LIFF

**Acceptance criteria:** ทุกช่องทางได้ receipt รูปแบบเดียวกันและระบุช่องทาง / ผู้บันทึก

<a id="con-15"></a>
### CON-15 หลักฐานความยินยอม (Receipt)

*Consent receipt & log*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-13, PLT-12
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** บันทึกที่แก้ไขไม่ได้: ใคร เมื่อไร ช่องทาง เวอร์ชัน IP อุปกรณ์ และประเภทรายการ (ยินยอม / ไม่ยินยอม / ถอน / หมดอายุ / ขยายอายุ)

**Backend (Go):** Consent Transaction แบบ append-only (partition รายเดือน), receipt ID, hash chain ต่อเจ้าของข้อมูล, ประเภทรายการ, IP / อุปกรณ์ / ช่องทาง / เวอร์ชัน; Data Subject Profile รวมสถานะล่าสุด; ส่ง receipt ทางอีเมล

**Frontend (Next.js):** หน้า profile + ประวัติรายการ

**Acceptance criteria:** แก้หรือลบ transaction ไม่ได้ และตรวจย้อนหลังได้ว่าความยินยอมมาจากเวอร์ชันและช่องทางใด

**หมายเหตุ:** Consent Transaction ตาม FSD V3.2

<a id="con-16"></a>
### CON-16 REST API และ Webhook

*Consent API & webhook*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE L (10 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-15, ORG-16
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** ให้ CRM/ERP/CDP ตรวจสถานะความยินยอมแบบ real-time สร้าง/แก้โปรไฟล์ลูกค้า และรับ event เมื่อสถานะเปลี่ยน

**Backend (Go):** API ตรวจสถานะความยินยอม real-time (cache ใน Valkey), สร้าง/แก้ profile, บันทึก transaction (idempotency key), bulk query; webhook consent.changed

**Frontend (Next.js):** หน้าเอกสาร API และตัวอย่างใน developer portal

**Acceptance criteria:** ผ่าน load test ตามเป้า TPS ที่ตกลง (T22) และ p95 ต่ำกว่า 200 ms

<a id="con-17"></a>
### CON-17 ความปลอดภัยและที่เก็บข้อมูล

*Security & data residency*

- **Priority / Phase:** Must · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ประกาศมาตรการความปลอดภัย พ.ศ. 2565
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE S (3 วัน) · FE XS (1 วัน) · UX —
- **ขึ้นกับ:** PLT-13
- **Process:** —

**คำอธิบาย:** เข้ารหัสข้อมูล ควบคุมสิทธิ์ และเลือกเก็บข้อมูลในประเทศ

**Backend (Go):** ใช้บริการกลาง: เข้ารหัส identifier, สิทธิ์ตาม role/scope, เก็บข้อมูลบน cloud ในไทย; งานที่เหลือคือ config และทดสอบ

**Frontend (Next.js):** แสดงตำแหน่งที่เก็บข้อมูลในหน้าตั้งค่า

**Acceptance criteria:** ผลทดสอบยืนยันว่าข้อมูลความยินยอมเข้ารหัสและอยู่ใน region ที่กำหนด

<a id="con-18"></a>
### CON-18 ศูนย์ตั้งค่าความเป็นส่วนตัว

*Preference center*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.19
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** IAM-05, PLT-17
- **Process:** [BP-02](../processes/BP-02.md)

**คำอธิบาย:** เจ้าของข้อมูลดูและแก้ความยินยอมและช่องทางการสื่อสาร ผ่านลิงก์ยืนยันตัวตน (magic link)

**Backend (Go):** API preference center: ดู/แก้ความยินยอมและช่องทางสื่อสาร (Purpose Preference), session ของเจ้าของข้อมูล

**Frontend (Next.js):** หน้า preference center ใน portal (magic link / OTP, TH/EN, branding)

**Acceptance criteria:** เจ้าของข้อมูลแก้ทางเลือกได้เองและได้ receipt ทุกครั้ง

**หมายเหตุ:** ดึงเข้า P1: ต้องมีช่องทางถอนที่ง่ายเท่ากับการให้ (ม.19)

<a id="con-19"></a>
### CON-19 ยืนยันตัวตนก่อนบันทึก

*Identity verification*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-05
- **Process:** [BP-02](../processes/BP-02.md)

**คำอธิบาย:** OTP ทาง SMS/อีเมล ตั้งค่าได้ต่อจุดเก็บความยินยอม และรองรับ IdP ภายนอก

**Backend (Go):** ตั้งวิธียืนยันตัวตนต่อ collection point (ไม่ต้อง / OTP / magic link / IdP ภายนอก) ผ่าน IAM-05

**Frontend (Next.js):** ตัวเลือกวิธียืนยันในหน้าตั้งค่า collection point

**Acceptance criteria:** collection point ที่บังคับ OTP บันทึกได้เมื่อยืนยันผ่านเท่านั้น

<a id="con-21"></a>
### CON-21 ค้นหา แดชบอร์ด และส่งออก

*Search, dashboard & export*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), MKT (การตลาด / ธุรกิจ), EXT (ระบบธุรกิจ CRM / POS / CDP)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-18
- **Process:** [BP-02](../processes/BP-02.md)

**คำอธิบาย:** ค้นหาตามบุคคล/วัตถุประสงค์ อัตรายินยอม/ถอน ส่งออกจำนวนมาก และรายงานเทียบกับระบบปลายทาง

**Backend (Go):** ค้นหาตามบุคคล (blind index) / Purpose, สถิติอัตรายินยอม/ถอน, export แบบ async, Reconcile Report เทียบสถานะกับระบบปลายทาง

**Frontend (Next.js):** หน้าค้นหา, dashboard และหน้า reconcile

**Acceptance criteria:** ค้นหาด้วยอีเมลหรือเบอร์ได้โดยไม่ต้องถอดรหัสทั้งตาราง และรายงาน reconcile แสดงรายการที่ไม่ตรง

**หมายเหตุ:** Reconcile Report ตาม FSD V3.2; ดึงเข้า P1

<a id="con-22"></a>
### CON-22 นำเข้าความยินยอมเดิม

*Bulk import*

- **Priority / Phase:** Should · P1 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-14, CON-15
- **Process:** —

**คำอธิบาย:** นำเข้าความยินยอมเดิมพร้อมวันที่ จากไฟล์หรือระบบเก่า

**Backend (Go):** นำเข้าความยินยอมเดิมพร้อมวันที่และช่องทางเดิม, map Purpose, ตรวจซ้ำ, ทำเครื่องหมายว่าเป็นข้อมูลนำเข้า

**Frontend (Next.js):** wizard นำเข้า (PLT-14) + รายงานผล

**Acceptance criteria:** ข้อมูลจากระบบ Consent เดิมนำเข้าครบและ reconcile ยอดตรง

**หมายเหตุ:** ดึงเข้า P1 เพื่อย้ายข้อมูลตอน go-live (T26)

<a id="con-05"></a>
### CON-05 นโยบายคุกกี้อัตโนมัติ

*Auto cookie policy*

- **Priority / Phase:** Should · P3 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.23
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** CON-04
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** สร้างและอัปเดตตารางคุกกี้ในหน้า Cookie Policy ภาษาไทย-อังกฤษ

**Backend (Go):** สร้างตารางคุกกี้ TH/EN จากผลสแกน อัปเดตเมื่อ publish

**Frontend (Next.js):** embed script / iframe สำหรับหน้า Cookie Policy

**Acceptance criteria:** ตารางในหน้า Cookie Policy ตรงกับผลสแกนล่าสุด

<a id="con-07"></a>
### CON-07 หลายโดเมนและ Mobile SDK

*Multi-domain & mobile SDK*

- **Priority / Phase:** Should · P3 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาดไทย
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE L (10 วัน) · UX —
- **ขึ้นกับ:** CON-16
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** จัดการหลายเว็บไซต์ SDK iOS/Android และรองรับ LINE OA / LIFF

**Backend (Go):** จัดการหลายโดเมน / แอปต่อ tenant, config แยก, API สำหรับ mobile

**Frontend (Next.js):** Mobile SDK (iOS Swift, Android Kotlin หรือ Flutter / React Native wrapper) + ตัวอย่าง LINE LIFF ด้วย JS SDK

**Acceptance criteria:** แอปตัวอย่างบันทึกและอ่านสถานะความยินยอมผ่าน SDK ได้

**หมายเหตุ:** Mobile SDK อาจจ้างนักพัฒนา mobile ภายนอก

<a id="con-20"></a>
### CON-20 วันหมดอายุและต่ออายุ

*Consent expiry & renewal*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** SCHED (ระบบ: Scheduler / Event), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-10
- **Process:** [BP-02](../processes/BP-02.md)

**คำอธิบาย:** กำหนดอายุต่อวัตถุประสงค์ แจ้งเตือนต่ออายุ และขยายอายุ

**Backend (Go):** อายุต่อ Purpose, job ตรวจหมดอายุรายวัน → สถานะ EXPIRED + event, แจ้งเตือนต่ออายุ, transaction EXTEND

**Frontend (Next.js):** ตั้งอายุใน Purpose + รายการที่ใกล้หมดอายุ

**Acceptance criteria:** ความยินยอมที่ครบอายุเปลี่ยนสถานะอัตโนมัติและระบบปลายทางได้รับ event

<a id="con-23"></a>
### CON-23 Double Opt-In

*Double opt-in*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** IAM-05
- **Process:** [BP-01](../processes/BP-01.md)

**คำอธิบาย:** ความยินยอมมีผลเมื่อเจ้าของข้อมูลยืนยันผ่านลิงก์ทางอีเมล/SMS

**Backend (Go):** สถานะ PENDING → CONFIRMED / CANCELLED ผ่านลิงก์อีเมล / SMS ที่มีวันหมดอายุ

**Frontend (Next.js):** ตั้งค่า double opt-in ต่อ collection point + หน้ายืนยัน

**Acceptance criteria:** ความยินยอมมีผลหลังยืนยันเท่านั้น

<a id="con-24"></a>
### CON-24 ส่งคำขอความยินยอมแบบ Bulk

*Bulk consent request campaign*

- **Priority / Phase:** Should · P3 · กลุ่ม: ความยินยอม
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** MKT (การตลาด / ธุรกิจ), DS (เจ้าของข้อมูล / ผู้เข้าชมเว็บ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-04, CON-18
- **Process:** —

**คำอธิบาย:** ส่งลิงก์ขอความยินยอมทางอีเมล/SMS ให้ฐานลูกค้าเดิม และติดตามว่าใครตอบแล้ว

**Backend (Go):** เลือกกลุ่มเป้าหมาย, ส่งลิงก์ขอความยินยอมทางอีเมล / SMS / LINE แบบทยอย, ติดตามผู้ตอบ, ส่งซ้ำ

**Frontend (Next.js):** หน้าสร้างแคมเปญ + สถิติการตอบ

**Acceptance criteria:** แคมเปญแสดงจำนวนส่ง / เปิด / ตอบ และส่งซ้ำเฉพาะผู้ยังไม่ตอบได้

<a id="con-08"></a>
### CON-08 A/B testing และวัด consent rate

*Banner A/B testing*

- **Priority / Phase:** Nice · P4 · กลุ่ม: คุกกี้
- **ที่มา:** Function List: 11_Consent
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** MKT (การตลาด / ธุรกิจ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** CON-01, PLT-18
- **Process:** [BP-03](../processes/BP-03.md)

**คำอธิบาย:** ทดสอบแบนเนอร์หลายแบบและวัดอัตราการยินยอม

**Backend (Go):** variant แบนเนอร์, สุ่มแบ่งผู้เข้าชม, วัด opt-in rate ต่อ variant

**Frontend (Next.js):** หน้าสร้าง variant และรายงานผลเปรียบเทียบ

**Acceptance criteria:** ผลการทดลองแสดงอัตรายินยอมต่อ variant พร้อมจำนวนตัวอย่าง
