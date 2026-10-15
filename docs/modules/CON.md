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

**Implementation note (done):** `internal/cookie` — the first module on the `cookie` schema (`cookie.banner.*`,
already seeded in the baseline permission migration). Deliberately scoped to the three literal acceptance
criteria only: `cookie.apps`/`scan*`/`cookie_kb`/`script_rules`/`ab_variants` (CON-02/03/04/07/08's own
tables) stay untouched, unused until their own feature passes. `domains.go`/`categories.go` are plain CRUD
(`RegisterDomain` FK-checks `legal_entity_id` via `orgservice.GetLegalEntity`, rule 1; domain verification is
this pass's own manual confirmation — no outbound DNS resolution is available in this environment for a real
TXT/meta-tag check, so the already-issued `verification_token` is shown but staff mark it verified by hand).
`banners.go` is `cookie.banner_configs`' own simple `draft → published → archived` versioning (not PLT-08):
publishing archives every sibling published version in the same call and issues the domain's own
`platform.public_keys` entry (`publickeys.EntityCookieDomain`, already in the baseline schema's CHECK
constraint) only on first publish — a new shared `publickeys.Get` helper was added for this, since
`cookie.domains` (unlike `breach.report_forms`/`dsar.intake_forms`) has no `public_key` column of its own to
read it back from. `CookieBannerConfig.id` is returned to the SDK as `banner_config_id` on the public banner
read, closing the one gap found while building the SDK client: the wire schema otherwise named a config only
by `domain_id`+`version_no`. `consent.go`'s `RecordConsent` is the acceptance criterion's own evidence write —
append-only, one call per decision — hashing the IP (SHA-256, rule 3: `cookie.consent_records.ip_hash` never
stores the raw address).

API: `/admin/v1/cookie/{domains,categories}` + `.../domains/{id}/verify` + `.../domains/{id}/banner-configs` +
`.../banner-configs/{id}` + `.../banner-configs/{id}/publish`; public `GET /public/v1/cookie-banners/{key}`
(banner + categories, no login) and `POST .../consents` (`Idempotency-Key` required, rule 10). UI:
`/settings/cookie-banners` (domain registration + verify, category CRUD, a per-domain banner draft
editor/publish, the install snippet shown once a domain's banner has published at least once). The SDK itself
is `packages/cookie-sdk` — vanilla TypeScript, no React/`@pdpa/api-client` dependency (a standalone CDN bundle
has a different size budget than a Next.js app) — fetches the published banner, renders three genuinely
equal-weight buttons (accept-all / reject-all / settings, the same CSS class, no category pre-ticked unless
`is_required`), and posts the decision with a first-party visitor cookie. Built with esbuild
(`pnpm --filter @pdpa/cookie-sdk build`), verified at **2.48 KB gzip** (`pnpm check-size`, failing the build
past 30 KB) — comfortably inside the acceptance criterion's own budget, with headroom for CON-02/03's own
later scripts. Tests: unit (no category pre-ticked unless required; domain FK-visibility and uniqueness
checks; publish lifecycle archives siblings and issues the public key only once; `RecordConsent`'s evidence
row carries the right action with a hashed, never-raw IP; two-tenant isolation of domains/categories/banner
configs), HTTP contract for both the admin and public packages (401/403/404/412/428/422/201 through the real
validator + AuthZ/public-key/Idempotency/Tx chain, an Idempotency-Key replay returning the original result).
Not verified live against a real Postgres/Redis — the same environment limitation DSAR-01/BRE-01/DSAR-14
already disclosed; every test compiles and vets clean and skips the same way the rest of this codebase's own
test suite already does here. Not done, deliberately (sibling features, not this one's literal criterion):
CON-02's own immediate script-blocking effect, CON-03's prior-to-consent script interception, CON-04's
automated scanner, CON-06's branding/multi-language beyond th/en, CON-07's mobile/LIFF SDK, CON-08's A/B
testing — every one of their own baseline tables stays unused until that pass.

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

**Implementation note (done):** Builds on CON-01's own `cookie` module and schema — no migration needed,
every table this feature touches (`cookie.cookies`, `cookie.script_rules`) was already in the baseline.
`domains.go`'s `RegisterDomain` now seeds the module doc's own "ค่าเริ่มต้น 4 หมวด" (Strictly Necessary /
Functional / Analytics / Marketing, only "necessary" required) on a tenant's *first* domain
(`seedDefaultCategoriesIfNone`) — a tunable convenience, not legal content (rule 8 doesn't apply: these are
internal category labels, never shown as notice/consent wording to a data subject), so no `docs/decisions.md`
entry was needed; a later domain never re-seeds, so a DPO who already edited the set keeps their own.
`cookies.go` is "รายการคุกกี้ต่อหมวด" — plain CRUD on `cookie.cookies` (manual entry only, `source='manual'`;
`scan`/`kb` stay CON-04's own unused columns), FK-checking `category_id` via a new `GetCategory` service
method (added to `categories.go`, since CON-01 never needed a single-row read). `script_rules.go`'s
`cookie.script_rules` is the acceptance criterion's own enforcement mechanism: a DPO declares which URL
pattern belongs to which category (`action` block/allow, default block); `PublicScriptRules` exposes them to
the SDK keyed by the category's own *code* (a new sqlc query joining `cookie.categories`, not by id — the
SDK already only knows categories by code), added to `GET /public/v1/cookie-banners/{key}`'s own response
right alongside the categories it already returned.

The SDK itself (`packages/cookie-sdk/src/scripts.ts`) is where "มีผลทันที" (immediate effect) actually
happens — the same two mechanisms every commercial consent-management platform uses, since there is no way
to literally un-execute code that has already run (a universal limitation, not specific to this SDK):
declarative gating (a site marks a script `<script type="text/plain" data-pdpa-category="analytics"
data-pdpa-src="...">`; the browser never executes `type="text/plain"` by spec, and this SDK activates it the
moment that category turns on — turning it back off after that only prevents re-activation, since the script
already ran) and pattern-based removal (a still-present, not-yet-loaded `<script src>` matching a `block`
rule's pattern is removed the instant its category is disallowed). A `pdpa:consent` `CustomEvent` is
dispatched on every change too, so a site's own already-running trackers (which this SDK can't reach
directly) can react on their own. Applied both right after a fresh decision (the literal acceptance
criterion) and on every page load for a returning visitor's already-recorded choice (so the effect isn't
only ever "immediate," it's also *persistent*). `choicesForAction`/`categoryAllowed`/`matchesPattern`/
`scriptsToBlock` are pure, DOM-free functions — `applyPreferences` is the one function that actually touches
the DOM — so the gating logic itself is unit-tested directly (`scripts.test.ts`, the package's first test
file; `vitest` added as a new devDependency) rather than only exercised through a browser nobody has
available in this environment. The bundle grew from CON-01's own 2.48 KB to **2.98 KB gzip** — still
comfortably inside the 30 KB budget `pnpm --filter @pdpa/cookie-sdk check-size` enforces.

API: `GET`/`POST /admin/v1/cookie/domains/{id}/cookies`, `DELETE /admin/v1/cookie/cookies/{id}`,
`GET`/`POST /admin/v1/cookie/domains/{id}/script-rules`, `DELETE /admin/v1/cookie/script-rules/{id}` — all on
the already-seeded `cookie.banner.*` codes (child-table pattern reusing the parent resource's own
permissions, the same move ROPA-03's own child tables use); `script_rules` added to `CookiePublicBanner`.
UI: `/settings/cookie-banners` gained a "Cookies" and a "Script rules" section per domain (name/provider/
category/storage-type form + list; pattern/category/action form + list), both inside the existing
per-domain banner-management panel. Tests: unit (`internal/cookie/service/con02_test.go` — the 4-defaults
seed fires once per tenant and never re-seeds, cookie/script-rule validation incl. unknown domain/category
and bad storage_type/action, `PublicScriptRules` keyed by category code and reflecting a delete, two-tenant
isolation of both new tables) and `packages/cookie-sdk/src/scripts.test.ts` (choicesForAction's three modes,
category-allowed lookup, substring and `/regex/` pattern matching incl. a malformed pattern never throwing,
which scripts a given choice set blocks), HTTP contract for both the admin and public packages (403 without
`cookie.banner.create`, 201/204/404, the public banner response now carrying `script_rules`). Existing CON-01
tests that created a category named "analytics" or asserted an empty category list right after
`RegisterDomain` were updated for the new 4-seeded-defaults baseline. Not verified live against a real
Postgres/Redis — same environment limitation CON-01 already disclosed; every Go test compiles and vets clean
and skips the same way the rest of this codebase's own test suite already does here. `go build ./...`,
`go vet ./...`, `go test -p 1 ./...` (no regressions), `pnpm --filter @pdpa/api-client typecheck`,
`pnpm --filter @pdpa/i18n test`, `pnpm --filter @pdpa/cookie-sdk test` (vitest + build + size check) and
`pnpm --filter @pdpa/admin build`/`typecheck` all verified clean.

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

**Implementation note (done):** Backend scope is already fully covered by CON-02's own `cookie.script_rules`
(blocking rules per script/domain) — no new migration, endpoint, or permission. This feature's real scope is
entirely `packages/cookie-sdk`.

A MutationObserver-based "remove after insertion" mechanism was tried first and rejected by this package's
own Playwright E2E test: a mutation callback fires as a microtask *after* the synchronous DOM insertion that
already starts the browser's fetch (and, per spec, scripts inserted via `appendChild`/`insertBefore` are not
guaranteed to stop executing once removed) — a real, observed failure, not a theoretical one. `scripts.ts`'s
`guardDynamicScripts()` instead patches `Node.prototype.appendChild`/`insertBefore` directly: a `<script src>`
is rewritten *before* the original call ever runs — stripped of `src`, given `type="text/plain"` and a
`data-pdpa-src` — so the browser never fetches it at all. This is exactly "rewrite type=text/plain", just
applied *before* insertion instead of after, which is what makes it deterministic rather than racy. A script
intercepted before the tenant's real `script_rules` are even known (the realistic case: GTM's own container
injecting a sub-tag can run before this SDK's async banner fetch resolves) is held pending with no category
yet; `guard.resolve(choices, rules)` — called once the banner loads, and again on every later decision —
classifies each one: matches a block rule and still disallowed → stays inert with `data-pdpa-category` set,
so `applyPreferences`' own CON-02 mechanism 1 activates it the moment that category is allowed; matches no
rule at all → released immediately, since nothing the tenant flagged as tracking should stay blocked forever
on a guess. `activatePlaceholder` was extracted out of `applyPreferences` so both call sites share one
activation code path.

A second real bug, found by the same E2E test: `autoInit()` previously deferred the *entire* `init()` call to
`DOMContentLoaded` whenever `document.readyState === 'loading'` — which is always true for a `<script>` tag
placed in `<body>`, the embed snippet's own recommended placement. That meant `guardDynamicScripts()` and
`gtagDefault()` (both synchronous, needing nothing but `document.currentScript`) ran only *after* every other
script on the page had already executed — the opposite of "prior." Fixed by calling `init()` unconditionally
and immediately, deferring only the one step that actually needs `document.body` (the banner's own DOM
insertion) to `DOMContentLoaded` if `document.body` doesn't exist yet.

Google Consent Mode v2 ("รองรับ Google Consent Mode v2"): `consentMode.ts`'s `gtagDefault()`/`gtagUpdate()`
push the fixed-vocabulary gtag consent signals (`ad_storage`/`ad_user_data`/`ad_personalization` ← marketing,
`analytics_storage` ← analytics, `functionality_storage`/`personalization_storage` ← functional,
`security_storage` always granted) onto `window.dataLayer` directly — this works whether or not gtag.js/GTM
has actually loaded yet, since GTM reads its own queued dataLayer array once it initializes; no real GTM
container exists in this environment to test against, so this is proven by unit test (the exact signals
pushed) rather than a live container. `gtagDefault()` is called synchronously at the very top of `init()`
(before the banner fetch), `gtagUpdate()` on every later preference change — the same timing CON-03's own
"prior blocking" needs. A literal downloadable GTM community template (the module doc's own "GTM template"
bullet) is deliberately not built — no real GTM container exists here to design or test one against, the
same "no consumer yet" deferral this codebase uses throughout; the `pdpa:consent` CustomEvent (CON-02) and
the dataLayer consent pushes are what such a template, or a custom GTM trigger, would read.

The acceptance criterion itself is proven by a real Playwright test (`packages/cookie-sdk/e2e/`, `pnpm
test:e2e`), not just asserted — this environment has a pre-installed Chromium (unlike most of this
codebase's own "no browser available here" E2E disclaimers), so this is a genuine first for this session.
`e2e/fixtures/index.html` is the literal "เว็บตัวอย่าง": a declaratively-gated analytics tag, and an inline
script simulating a tag manager injecting a marketing sub-tag after its own container loads (the realistic
case `guardDynamicScripts` targets) — a plain `<script src>` already present *earlier* in the HTML than the
SDK's own tag is the one case nothing can retroactively stop (CON-02's own documented "can't un-execute code
that already ran"), so the fixture places the SDK first, the documented real-world recommendation. `e2e/
server.mjs` is a minimal static file server (no real backend — the test mocks `/public/v1` at the network
layer via Playwright's own route interception). Three tests: no analytics/marketing cookie before any
decision (the acceptance criterion directly), accepting all activates the gated script immediately (CON-02's
own criterion, still true here), rejecting all keeps both blocked. `playwright.config.ts` points
`launchOptions.executablePath` at this environment's own pre-installed Chromium (`/opt/pw-browsers/chromium`)
since the pinned `@playwright/test` version would otherwise try to download a second, incompatible revision.
`vitest.config.ts` was added to scope vitest's own glob to `src/**/*.test.ts` — needed once `e2e/*.spec.ts`
existed, since vitest would otherwise try to run Playwright's own test files and fail immediately.

Bundle grew from CON-02's own 2.98 KB to **3.51 KB gzip** — still comfortably inside the 30 KB budget. Tests:
`pnpm --filter @pdpa/cookie-sdk test` (vitest incl. new `consentMode.test.ts`, build, size check) and `pnpm
--filter @pdpa/cookie-sdk test:e2e` (the 3 Playwright tests above, all passing against a real Chromium). No
backend or admin-UI changes — `go build/vet/test`, `pnpm --filter @pdpa/api-client typecheck` and `pnpm
--filter @pdpa/i18n test` re-verified clean regardless.

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

**Implementation (CON-04) — done:** `internal/cookie/scan` เป็น crawler จริง (chromedp + headless Chromium
ที่ติดตั้งไว้ในสภาพแวดล้อมนี้) — BFS ภายในโดเมนเดียวกัน (same-origin) หลาย tab พร้อมกัน (`Concurrency`,
ดีฟอลต์ 4), เก็บคุกกี้ทุกตัวต่อหน้าผ่าน CDP `Storage.getCookies` (เห็นคุกกี้ของบุคคลที่สามด้วย ไม่ใช่แค่
`document.cookie`), มีการ "warm up" tab ด้วย `about:blank` ก่อน timeout ครั้งแรก (บั๊กจริงที่พบ: chromedp ผูก
อายุของ tab กับ context ของคำสั่งแรก) วัดจริงได้ ~378ms/หน้าที่ concurrency 6 → นอกเวลาเหลือเฟือสำหรับ 200
หน้าใน 30 นาที (เกณฑ์ตอบรับ). `cookie.scans`/`cookie.scan_findings` (migration ที่มีอยู่แล้วในฐานข้อมูล
baseline) + migration ใหม่ 00068 สำหรับ `cookie.cookie_kb` (23 รายการ tracker ที่รู้จักทั่วไป เช่น GA/Meta/
TikTok — ข้อมูลสาธารณะทางอุตสาหกรรม ไม่ใช่ข้อความทางกฎหมาย จึงไม่ต้องมีสถานะ DRAFT ตามกติกาข้อ 8).
`internal/cookie/service/scan.go`: `StartScan`/`RunScan`/`ClassifyFinding` — คุกกี้ที่พบใหม่จะถูกสร้างแถว
`cookie.cookies` อัตโนมัติ (`source='scan'`) พร้อมหมวดแนะนำจาก `cookie_kb` (ถ้าตรงกับหมวดที่ tenant มีอยู่
จริง) แล้วแจ้งเตือน DPO/IT ทันที (เทมเพลต `cookie.scan_new_cookies`, migration 00069) — ครอบคลุม
"คุกกี้ใหม่ถูกแจ้งเตือน" ตามเกณฑ์ตอบรับโดยตรง. สแกนตามรอบ (`scan_schedule` ของ `cookie.domains`: daily/
weekly/monthly) ผ่าน River สองขั้นตอน — `cookie.scan_schedule_sweep` (global job อ่านแค่ `platform.tenants`)
enqueue `cookie.scan_schedule_check` ต่อ tenant ซึ่งอ่านโดเมนของตัวเองใน RLS tx เดียวกัน เหมือนแพทเทิร์นของ
PLT-11's outbox sweep. API: `GET/POST .../scans`, `GET .../scans/{id}`, `GET .../scans/{id}/findings`,
`POST .../scan-findings/{id}/classify` (ทั้งหมดใช้ permission `cookie.scan.*` ที่ seed ไว้แล้วตั้งแต่
migration baseline — ไม่มี permission code ใหม่). Frontend: ส่วน "สแกนคุกกี้อัตโนมัติ" บนหน้า
`/settings/cookie-banners` ต่อโดเมน (ปุ่มเริ่มสแกน, ตารางประวัติพร้อม polling ขณะกำลังสแกน, แผงดูผลสแกน +
จัดหมวดคุกกี้ที่ไม่รู้จัก). ทดสอบ: unit (`matchesKBPattern`, `scanDue`/`scheduleInterval` — pure functions),
service (DB-backed: คุกกี้ใหม่ถูกจัดหมวด+นับถูก, จัดหมวดแก้ไขได้, crawl ล้มเหลวไม่ทำให้ state เสีย, permission
gate แต่ละตัวแยกกัน, two-tenant isolation), **real-browser crawler test** (ไม่ใช่ mock — รันจริงกับ Chromium
ที่ติดตั้งไว้ ต่อเว็บ fixture 3 หน้าที่ตั้งคุกกี้คนละตัว พิสูจน์ว่าเจอทุกหน้า+ทุกคุกกี้ และไม่ไหลตามลิงก์
off-site, กับทดสอบ `MaxPages` อีกเคส), HTTP contract (401/403/404/422/200/201 ผ่าน validator จริง). ยังไม่ทำ
(ไม่ใช่ขอบเขตของเกณฑ์ตอบรับนี้): sitemap.xml เป็นจุดเริ่มสแกนเพิ่มเติม (เดินตามลิงก์บนหน้าเว็บเพียงพอสำหรับ
เกณฑ์ตอบรับแล้ว), การตรวจ localStorage/sessionStorage (schema ของ `cookie.scan_findings` ไม่มีคอลัมน์แยก
ประเภท storage เหมือน `cookie.cookies.storage_type` — เพิ่มเมื่อมีจอที่ต้องใช้จริง), เทียบกับรอบก่อน
("เทียบกับรอบก่อน" ใน FE note — ตารางประวัติที่มีอยู่แล้วให้เห็น new_cookies ต่อรอบ แต่ยังไม่มี diff เจาะจงราย
คุกกี้; เป็น CON-05's "ตารางในหน้า Cookie Policy ตรงกับผลสแกนล่าสุด" ที่แยกเป็นฟีเจอร์ของตัวเอง).

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

**Implementation (CON-09):** `backend/internal/consent/service/collection_points.go` — จุดเก็บความยินยอม = ชื่อ, ช่องทาง, นิติบุคคล, วัตถุประสงค์ (แต่ละข้อ "จำเป็น" ได้ ยกเว้นข้อมูลอ่อนไหว), origin ที่อนุญาต · draft → active (publish) → retired · publish ต้องผ่าน `cpChecks`: มีวัตถุประสงค์, ทุกวัตถุประสงค์เผยแพร่แล้ว, และผู้ publish ยืนยัน checklist ม.19 ทั้ง 4 ข้อ (`separate_text`, `not_bundled`, `plain_language`, `withdrawal_info`, เก็บใน `publish_checklist`) — ไม่ผ่าน = 422 `consent.publish_checks` พร้อม `errors[].code` · publish ครั้งแรกออกคีย์สาธารณะ (`platform.public_keys`, 192 บิต base64url, entity `collection_point`) · retire เพิกถอนคีย์ทันที · ฟอร์มสาธารณะ: `GET /public/v1/collection-points/{key}` + `POST /public/v1/consents` (`X-Public-Key`, `Idempotency-Key`) — ไม่มี AuthN, `publickeys.Middleware` หา tenant จากคีย์ (ไม่รู้จัก = 404), ปฏิเสธ `Origin` ที่ไม่อยู่ในรายการ (403), ตั้ง principal `data_subject` แล้วผ่าน Idempotency + Tx + audit ตามปกติ · ฟอร์มสาธารณะต้องตัดสินทุกวัตถุประสงค์ที่แสดง (ติ๊ก = ยินยอม, ไม่ติ๊ก = ไม่ยินยอม) และถอนไม่ได้ (ต้องผ่านศูนย์ตั้งค่า CON-18/19) · หน้าจอ admin `/consent/collection-points` (แก้ไข, checklist, ลิงก์ + คีย์) · หน้าฟอร์มใน portal `/[locale]/c/[key]` (server action, Idempotency-Key ต่อการส่ง, ส่ง `X-Forwarded-For`/`User-Agent` ของผู้ใช้ต่อ — ต้องใส่ portal ใน `TRUSTED_PROXIES`) · migration 00034 · ยังไม่ทำ: QR code, embed script / SDK และ CORS สำหรับเว็บไซต์ลูกค้า (CON-07/14), CAPTCHA, `/api/v1` (CON-16 ต้องมี ORG-16), age gate (CON-11)

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

**Implementation (CON-10):** ความอ่อนไหวของวัตถุประสงค์มาจากหมวดข้อมูลที่เลือก (`purposes.data_category_codes` เทียบ `is_sensitive` ของ master data ORG-07) · วัตถุประสงค์ที่อ่อนไหวต้องมีข้อความยินยอมโดยชัดแจ้ง (`explicit_text`, `purpose_versions.explicit_text_th/en`) — ไม่มี = ส่งเผยแพร่ไม่ได้ (`explicit_text_required`) · เผยแพร่แล้ว lawful basis = `EXPLICIT_CONSENT`, `requires_explicit = true` · จุดเก็บความยินยอม publish ไม่ได้ถ้าวัตถุประสงค์อ่อนไหวถูกตั้งเป็น "จำเป็น" (`sensitive_required`) หรือไม่มีข้อความชัดแจ้ง (`sensitive_without_explicit_text`) · ฟอร์มแสดงข้อความชัดแจ้งกับช่องติ๊กของตัวเองเสมอ

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

**Implementation (CON-12):** วัตถุประสงค์ใช้ PLT-08 (ประเภท `consent_purpose`, สิทธิ์ `consent.purpose.read/update/publish`, 1 ขั้นอนุมัติโดย role DPO — maker-checker: ผู้ร่างอนุมัติเองไม่ได้) · เนื้อหาร่าง = ชื่อ, คำอธิบาย, ข้อความยินยอม, ข้อความชัดแจ้ง, หมวดข้อมูล, อายุขั้นต่ำ, อายุความยินยอม (วัน), ประเภทการเปลี่ยนแปลง (`minor`/`material`), ต้องขอใหม่, ตัวเลือกย่อย · `OnPublish` เพิ่ม `consent.purpose_versions` แถวใหม่ (ฉบับแรก = `initial`) และปรับ `current_version_id` ใน tx เดียว; ฉบับที่เผยแพร่ไม่ถูกแก้ · ทุก transaction อ้าง `purpose_version_id` ที่ผู้ใช้เห็น — ส่งเลขฉบับเก่า = 422 `stale_version` · โปรไฟล์เจ้าของข้อมูลแสดงฉบับที่ยินยอมและธง "ต้องขอใหม่" เมื่อฉบับปัจจุบัน `requires_reconsent` · หน้าจอ `/consent/purposes` (ตัวแก้ร่าง + `RecordVersions`) · ยังไม่ทำ: เปลี่ยนสถานะ ACTIVE → PENDING อัตโนมัติเมื่อมีฉบับ material (ST-01 ยังไม่มี PENDING + CONSENTED จึงยินยอมใหม่ไม่ได้ — decisions Q-22)

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

**Implementation (CON-13) — done:** การถอนเปลี่ยน `consent_status` เป็น `WITHDRAWN` ใน tx เดียวกับ transaction + receipt และเขียน `consent.withdrawn` ลง outbox ใน tx เดียวกัน (ไม่ใช่ async — รับประกัน "ภายใน 1 นาที" ของ acceptance criterion ได้เกินพอ; PLT-11's sweeper ส่งต่อให้ subscriber ภายในไม่กี่วินาที) · ถอนได้เฉพาะจาก ACTIVE (อื่น ๆ = 422 `withdraw_not_allowed`) พร้อมเหตุผล (`reason_code`) · ถอนได้ทางเจ้าหน้าที่ (`POST /admin/v1/consent/records`, `consent.onbehalf.create`, หน้าโปรไฟล์ `/consent/subjects`) ครอบคลุม acceptance criterion เต็มรูปแบบสำหรับช่องทางนี้ — ทดสอบแล้วทั้ง service (สถานะเปลี่ยนทันที, เหตุผลบังคับ, ถอนซ้ำถูกปฏิเสธ, แถว `consent.withdrawn` ใน outbox) และ HTTP contract (422/200) · **ยังไม่ทำ (ฟีเจอร์พี่น้องแยกต่างหาก ไม่ใช่ขอบเขตของ CON-13):** เจ้าของข้อมูลถอนเอง (ศูนย์ตั้งค่า CON-18 + ยืนยันตัวตน CON-19 — ทั้งคู่ Should, ขึ้นกับ IAM-05 ซึ่งเสร็จแล้ว, CON-18 ยังขึ้นกับ PLT-17), ส่ง HMAC webhook จริงถึงระบบปลายทางภายนอก เช่น CRM/POS (PLT-15 ต้องมี ORG-16 — ทั้งคู่ยัง todo; event ภายในระบบผ่าน outbox/subscriber ทำงานแล้ววันนี้ ตามกติกา CLAUDE.md ข้อ 6)

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

**Implementation (CON-15):** `Record()` ใน `consent/service/record.go` — ทุกการส่ง = 1 receipt (`CR-YYYYMMDD-XXXXXXXXXX`) + 1 transaction ต่อวัตถุประสงค์ (ฉบับ, ช่องทาง, จุดเก็บ, ที่มา web/app/staff, IP, user agent, ผู้บันทึก, idempotency key) · สถานะตาม ST-01 (`Decide`: ยินยอมซ้ำ = `EXTENDED`, เปลี่ยนตัวเลือก = `CHANGED_PREFERENCES`, ไม่ติ๊กครั้งแรก = `NOT_GIVEN`, ไม่ติ๊กขณะ ACTIVE = ไม่เปลี่ยนแปลง — decisions Q-21) · event `consent.granted/denied/withdrawn/preferences_changed` · receipt ต่อกันเป็น hash chain ต่อเจ้าของข้อมูล (SHA-256 บน canonical JSON แท็ก `consent-receipt/v1` ครอบคลุม receipt, `prev_hash` และ transaction ทุกแถว; advisory lock ต่อ subject) · `consent_transactions`/`consent_receipts` app role INSERT ได้อย่างเดียว · ตรวจห่วงโซ่ได้ที่ `POST /admin/v1/consent/subjects/{id}/verify` (บอก receipt แรกที่ผิดและสาเหตุ `hash_mismatch`/`prev_hash_mismatch`) · ส่ง receipt ทางอีเมล (template `consent.receipt`, PLT-04) เมื่อมีอีเมล · ค้นหาเจ้าของข้อมูลด้วย blind index แบบตรงตัว (`POST …/subjects/search`, ตัวระบุอยู่ใน body ไม่อยู่ใน URL) · โปรไฟล์: ตัวระบุแบบปกปิด, สถานะรายวัตถุประสงค์, ประวัติพร้อมเลข receipt · migration 00034 (index chain) + 00035 (index subject บนตาราง partitioned) · หน้าจอ `/consent/subjects`

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

**Implementation (CON-17) — เสร็จเท่าที่ทำได้ในสภาพแวดล้อมนี้:** ตัวระบุ (อีเมล, โทรศัพท์, เลขบัตร ฯลฯ) เก็บใน `subject_identifiers.value_enc` ด้วย PLT-13 (AES-256-GCM, DEK ต่อ tenant คลาส `consent`, ผูก associated data กับคอลัมน์) + `blind_index` สำหรับค้นหา — ทดสอบแล้วว่าไบต์ที่เก็บไม่มีค่าเดิม · `GET /admin/v1/consent/settings` แสดง region ของ tenant (`platform.tenants.data_region`) และการจัดการกุญแจ (OpenBao Transit / กุญแจพัฒนา) บนหน้า `/consent/subjects`. "หลักฐานทดสอบ region" ที่ขาดอยู่ก่อนนี้ปิดแล้ว:
`TestGetSettings_ReportsRegionAndEncryption` (ไม่มี endpoint เขียน `data_region` ให้เรียก — คอลัมน์นี้ตั้งค่าได้แค่ตอน provision tenant เท่านั้น ไม่มีหน้าจอแก้ไขในระบบนี้ — จึงตั้งค่าตรงผ่าน `platform.tenants` เหมือนการ provision จริง) พิสูจน์ว่า `GetSettings` อ่าน `data_region` ของ tenant ตนเองจริง ไม่ใช่ค่าตายตัว และไม่รั่วข้าม tenant (tenant A เป็น TH ค่าเริ่มต้น, tenant B ตั้งเป็น SG แล้วอ่านกลับถูกต้องทั้งสอง tenant ในการเรียกคนละครั้ง) บวก HTTP contract (401 ไม่มี principal, 403 ไม่มี `consent.record.read`, 200 คืน `data_region`/`identifier_encryption` ที่ถูกต้องผ่าน validator จริง). ยังไม่ทำโดยตั้งใจ: บังคับ region ที่ระดับ deployment จริง (ฐานข้อมูล/บักเก็ตแยกต่อ region) — ไม่มี infra หลาย region ให้ทดสอบจริงในสภาพแวดล้อมนี้ (`docs/decisions.md` Q-02 ยังเปิดอยู่), เหตุผลเดียวกับที่ PLT-02 เลื่อน DB-per-tenant deployment mode ไว้ก่อน

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

**Implementation (CON-18) — done, OTP only; magic link deferred (no consumer, same as CON-19):** no new table —
`platform.public_keys` already had `entity_type` value `"portal"` seeded in the baseline migration, unused
until now (`publickeys.EntityPreferenceCenter`); the key stands directly for an `org.legal_entities` row, the
same granularity CON-09's own collection points use, so one legal entity gets one preference-centre link.
`internal/consent/service/preferences.go`: `PublishPreferenceCenter`/`RetirePreferenceCenter`/
`GetPreferenceCenterLink` (admin, `consent.collectionpoint.{read,publish,delete}` — a preference centre is
just another publishable public consent surface, no new permission code); `StartPreferenceVerification` reuses
CON-19's own IAM-05 OTP primitive under a distinct purpose (`"preference_center"`, already reserved in
`iamservice.verificationPurposes`) so a plain consent-form OTP can never double as a preference-centre
session; `GetPreferenceCenter` shows every active purpose in the tenant (not one form's own list) with the
subject's current decision and preferences overlaid; `UpdatePreferences` lets the subject grant, refuse *or
withdraw* any of them in one call — `Record()`'s own "a public submission can't withdraw" rule (CON-19) simply
doesn't apply, since every preference-centre call is OTP-verified first. `Record()` was refactored to extract
its own core (`writeDecisions`) so `UpdatePreferences` reuses the exact same hash-chained receipt + transaction
+ status-projection + outbox logic without duplicating it, validating decisions against a synthetic
`CollectionPoint{Purposes: ...}` built from every active purpose instead of one real form's list.
`consent.consent_receipts.collection_point_id` is a hard NOT NULL FK and the preference centre isn't modelled
as a collection point, so `channelForPreferenceUpdate` attributes the receipt to a real active collection
point — one already showing the decided purpose when there is one, else any active point of the tenant — a
disclosed, deliberate limitation (CLAUDE.md's "minimal slice" pattern): the receipt's channel then names
wherever that purpose happens to also be collected, not literally "the preference centre."
`verifyPreferenceIdentity` lets one OTP code cover both a view and a later update in the same session: once
`VerifyOTP` has decided a verification "verified", calling it again errors `ErrVerificationDecided` rather than
re-checking the code (and would otherwise burn IAM-05's own 5-attempt cap twice for one legitimate code), so
the gate falls back to the side-effect-free `VerifiedIdentifier` check once a code has already been consumed.

API: `GET`/`POST .../publish`/`POST .../retire` on `/admin/v1/consent/preference-centers/{legalEntityId}`;
public `POST /public/v1/preference-centers/{key}/start-verification`, `.../view`, `.../update` (`Idempotency-Key`
required on all three, rule 10 read literally). UI: `/consent/collection-points` gained a "Preference centre"
section (legal-entity picker, publish/retire, the hosted link) — no new page, since the existing collection-
points screen already has the legal-entity picker pattern this needed. The portal gained
`/[locale]/preferences/[key]` (`apps/portal/src/lib/preferences.ts` + `actions.ts` +
`preference-center-form.tsx`): a three-phase flow — send code → verify → a list of every active purpose with
radio choices (keep / consent / withdraw, withdraw only shown once a purpose is already `ACTIVE`) — ending in
the same kind of receipt number the public consent form shows. Tests: unit
(`internal/consent/service/preferences_test.go` — the acceptance criterion directly: grant then withdraw both
record and both return a receipt; every read/write refused without a correct OTP against the exact identifier;
publish/retire/re-publish link lifecycle; two-tenant isolation of the verification id), HTTP contract
(`internal/consent/http/handler_test.go` — the full publish → start-verification → view → update(grant) →
update(withdraw) → retire chain through the real validator + AuthZ + public-key + Idempotency + Tx chain, plus
404s for an unknown or retired key). Not verified live against a real Postgres/Redis — the same environment
limitation this codebase's other recent passes already disclosed; every test compiles and vets clean and skips
the same way the rest of this module's own test suite already does here. Not done: magic_link/idp methods (no
consumer or design for either yet, same as CON-19); "ช่องทางการสื่อสาร" (communication-channel preferences
beyond a purpose's own `Preferences` sub-choices) — no separate channel-opt-out model exists in this schema
distinct from a purpose's preferences, so none was invented for this pass.

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

**Implementation (CON-19) — done, OTP only; magic_link/idp left in the CHECK constraint for later:**
`consent.collection_points.verification_method` (`none`/`otp`/`magic_link`/`idp`) was already in the baseline
migration, unused until now — no new migration. `internal/consent/service/collection_points.go` validates it
on create/update (empty normalizes to `none`); `StartConsentVerification`
(`internal/consent/service/record.go`) refuses unless the collection point is `active` with
`verification_method = "otp"`, resolves the identifier's kind to IAM-05's own `otp_email`/`otp_sms` method,
and calls `iamservice.StartVerification(Purpose: "consent", ...)` — the same OTP primitive CON-11's guardian
flow already uses, now given a second, direct-subject consumer. Unlike CON-11's separate confirm-then-submit
flow, CON-19 verifies inline inside `Record()` itself: `Submission` gained `VerificationID`/`VerificationCode`,
and `verifyIdentityIfRequired` calls `VerifyOTP` then a new `iamservice.VerifiedIdentifier` (constant-time blind
-index comparison) to prove the completed OTP was started against *this* submission's own identifier, not a
stranger's — closing a real gap a naive `status == "verified"` check would have left open. A missing/invalid
code or an identifier mismatch is a `DecisionError` (422 `consent.invalid_decisions`, field `verification`),
the exact shape every other field-level consent validation error already uses.

API: `POST /public/v1/consents/start-verification` (`Idempotency-Key` required, rule 10) returns
`{verification_id, channel}`; `ConsentSubmitPublicConsent`'s body gained an optional
`verification: {verification_id, code}`; `verification_method` added to both the admin
`CollectionPointInput`/`ConsentCollectionPoint` wire schemas and the public `GET
/public/v1/collection-points/{key}` response (previously hard-coded to `"none"`). UI: `/consent/collection
-points` gained a verification-method selector (OTP enabled, magic_link/idp shown disabled — not built yet);
the portal's `/[locale]/c/[key]` form gained a two-phase flow when `verification_method === "otp"` — a
send-code step (`startVerification`, `apps/portal/src/lib/consent.ts`) followed by the existing purpose
checklist with a code field and the `verification_id`/`code` carried as hidden inputs into the final submit
(`apps/portal/src/app/[locale]/c/[key]/actions.ts`/`consent-form.tsx`). Tests: unit
(`internal/consent/service/verification_test.go` — the acceptance criterion directly: an OTP-gated collection
point refuses a submission without a prior, correct verification and accepts one once verified; starting
verification against a non-OTP or non-active collection point is refused; two-tenant isolation),
HTTP contract (`internal/consent/http/handler_test.go` — create/publish an OTP collection point, 422 without
verification, 201 with a real OTP round-trip through the fake IAM notifier, 404 for an unknown public key on
start-verification). Not verified live against a real Postgres/Redis — the same environment limitation this
codebase's other recent passes already disclosed. Not done: magic_link/idp methods (no consumer or design for
either yet — the column already anticipates them); a portal-side resend/rate-limit UI beyond what IAM-05's own
5-attempt OTP cap already provides.

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
