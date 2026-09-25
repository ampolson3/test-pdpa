# Architecture overview

> ต้นฉบับภาพ: `design/PDPA_System_Analysis.drawio` → ARC-01 (container) · ARC-02 (deployment) · ARC-03 (security) · ARC-04 (integration) · ARC-05 (code) · ARC-06 (CI/CD)

## ภาพรวม

แพลตฟอร์มบริหารการปฏิบัติตาม PDPA แบบ multi-tenant (SaaS บน cloud ที่มี data center ในไทย และชุดติดตั้ง on-prem) ประกอบด้วย

- **apps/admin** — Next.js App Router สำหรับผู้ใช้ภายใน (DPO, กฎหมาย, เจ้าของกระบวนการ, IT, ผู้ดูแลระบบ) ผ่านรูปแบบ BFF
- **apps/portal** — Next.js สำหรับเจ้าของข้อมูลและ guest (privacy center, preference center, DSAR, ประกาศ, แจ้งเหตุ, guest link) รองรับ custom domain ต่อ tenant
- **packages/cookie-sdk** — vanilla TypeScript < 30 KB gzip ฝังบนเว็บลูกค้า (แบนเนอร์, บล็อกสคริปต์, Google Consent Mode v2) ส่งผ่าน CDN
- **backend** — Go modular monolith 1 repo build เป็น 4 binary: `api` (HTTP), `worker` (River jobs + cron), `scanner` (chromedp สแกนคุกกี้), `migrate` (goose)
- **data** — PostgreSQL 16+ (schema ต่อ module, RLS ต่อ tenant, partition รายเดือน, pg_trgm, pgvector, River queue) · Valkey · S3/MinIO + ClamAV · OpenBao/KMS
- **บริการประกอบ** — Keycloak (OIDC/SAML/MFA/LDAP/ThaID broker), Gotenberg (PDF ภาษาไทย), Email/SMS/LINE, e-Signature, LLM ผ่าน AI gateway, OpenTelemetry stack

## Container และเทคโนโลยี

| ชั้น | component | เทคโนโลยี | หน้าที่ |
|---|---|---|---|
| ผู้ใช้งาน | Admin portal | Next.js (App Router, React Server Components, TypeScript) + shadcn/ui + Tailwind + TanStack Query / Table | หน้าจัดการทุกโมดูล เมนูและปุ่มตามสิทธิ์; BFF เก็บ session ฝั่ง server |
| ผู้ใช้งาน | Public portal | Next.js แอปแยก + ISR/edge cache + custom domain ต่อ tenant | Preference Center, ฟอร์มความยินยอม, ฟอร์มและพอร์ทัลคำขอใช้สิทธิ, หน้าประกาศ, แจ้งเหตุ, พอร์ทัล guest |
| ผู้ใช้งาน | Cookie & consent SDK | Vanilla TypeScript (Vite/Rollup) ขนาดไม่เกิน 30 KB gzip ผ่าน CDN; Google Consent Mode v2, GTM | แบนเนอร์ บล็อกสคริปต์ widget ความยินยอม และแสดงประกาศบนเว็บลูกค้า |
| ผู้ใช้งาน | Mobile SDK | Swift / Kotlin หรือ Flutter / React Native wrapper | ความยินยอมในแอป (P3) — อาจจ้างทีม mobile ภายนอก |
| Edge | CDN + WAF + Ingress | CDN + WAF + Kubernetes Ingress / Gateway API | ส่ง SDK และ config แบนเนอร์, กันการโจมตี, TLS |
| Backend | Go API (modular monolith) | Go + chi + oapi-codegen (OpenAPI 3.1) + pgx + sqlc + goose | API ของทุกโมดูล แยก package ตาม bounded context; deploy แยก profile ได้ เช่น consent-api สำหรับโหลดสูง |
| Backend | Worker | Go + River (job queue บน PostgreSQL) + periodic jobs | SLA timer, แจ้งเตือน, export/import, ความยินยอมหมดอายุ, webhook, retention |
| Backend | Cookie scanner | Go + chromedp (headless Chromium) แยก worker pool | สแกนเว็บไซต์ลูกค้าตามรอบ |
| Backend | Document service | Gotenberg (Chromium/LibreOffice) + ฟอนต์ไทย (Sarabun / Noto Sans Thai) + excelize | PDF / DOCX / Excel ของประกาศ สัญญา แบบแจ้ง และรายงาน |
| Identity | Keycloak | OIDC/SAML, MFA, brute-force detection, LDAP/AD federation, identity brokering | login, SSO, MFA, ThaID (ผ่าน OIDC broker), service account; หน้า login TH/EN ทำ theme ด้วย Keycloakify |
| Identity | Authorization | Go authorization module (RBAC + data scope + record rule) + PostgreSQL RLS | ตรวจสิทธิ์ทุก endpoint (x-permission), กรองข้อมูลตามหน่วยงาน, แยก tenant |
| Data | Database | PostgreSQL 16+ (HA ด้วย CloudNativePG / Patroni หรือ managed) | schema ต่อโมดูล, RLS ต่อ tenant, partition consent_transaction และ audit_log รายเดือน, JSONB สำหรับฟอร์ม |
| Data | Cache / rate limit | Valkey (Redis-compatible, license BSD) | permission cache, config แบนเนอร์, rate limit, idempotency key |
| Data | Object storage | S3-compatible (S3 ของ cloud / SeaweedFS / MinIO — ตรวจ license) + ClamAV | ไฟล์แนบ แพ็กเกจข้อมูลคำขอใช้สิทธิ ไฟล์ export (เข้ารหัส) |
| Data | Secrets & keys | KMS ของ cloud หรือ OpenBao (Transit) | envelope encryption ของ PII, secret ของ connector และ API |
| Data | Search | PostgreSQL pg_trgm → OpenSearch + Thai analyzer เมื่อจำเป็น | ค้นหาภาษาไทยที่ไม่มีช่องว่างระหว่างคำ |
| Integration | Events & webhooks | Transactional outbox + River; ทางเลือก NATS JetStream | ส่ง event (consent.withdrawn, dsar.completed ฯลฯ — ชื่อจริงดู events.yaml) แบบไม่สูญหาย, webhook ลงลายเซ็น HMAC |
| Integration | Notification | SMTP/SES, SMS gateway ไทย, LINE Messaging API | อีเมล SMS LINE และ OTP |
| Integration | Connectors | Go connector SDK (DB, REST, SaaS) + HRIS / ITSM / SIEM | ค้นหาข้อมูลข้ามระบบ, data discovery, sync ความยินยอม |
| AI | AI gateway | Go module + LLM ผ่าน API หรือ on-prem (vLLM) + pgvector | ผู้ช่วย DPO, เติมแบบประเมิน, ร่างรายงาน — ปกปิด PII ก่อนส่ง |
| Ops | Observability | OpenTelemetry + Prometheus + Grafana + Loki + Tempo | metrics, log, trace, alert |
| Ops | Deployment | Docker + Kubernetes + Helm + GitOps (Argo CD); Docker Compose สำหรับ on-prem ขนาดเล็ก | SaaS บน cloud ที่มี data center ในไทย หรือติดตั้ง on-prem |
| Ops | CI/CD & security | GitHub Actions / GitLab CI, golangci-lint, gosec, govulncheck, ESLint, Trivy, Syft, cosign | build / test / scan / sign ทุก commit |
| Ops | Testing | go test + testcontainers-go, Vitest, Playwright, k6, OWASP ZAP | unit, integration, contract, E2E, load, DAST |

## หลักการออกแบบ (ต้องรักษาไว้)

- **Modular monolith** — แยก module ตาม bounded context (1 module = 1 schema = 1 Go package) สื่อสารกันผ่าน service interface หรือ domain event เท่านั้น เพื่อแยกเป็น service ภายหลังได้ (เช่น consent-api)
- **API-first** — OpenAPI 3.1 ใน `api/openapi/` เป็นต้นแบบ → generate Go server (oapi-codegen) และ TypeScript client (openapi-typescript) · สิทธิ์ผูกกับ endpoint ด้วย `x-permission`
- **BFF** — browser ไม่ถือ access token · Next.js เก็บ session แบบ httpOnly cookie และเรียก Go API ด้วย JWT ฝั่ง server
- **Multi-tenant** — shared database + `tenant_id` + PostgreSQL RLS (กันข้อมูลรั่วข้าม tenant แม้โค้ดผิด) · ลูกค้าใหญ่ / on-prem ใช้ DB แยกด้วยโค้ดเดียวกัน
- **งานนาน / ต้อง retry** ทำใน worker ผ่าน River ซึ่ง enqueue ใน transaction เดียวกับข้อมูลธุรกิจ · event ออกภายนอกผ่าน transactional outbox
- **Privacy by design** — เข้ารหัส identifier ระดับฟิลด์, ปกปิดบนหน้าจอเป็นค่าเริ่มต้น, ไม่มี PII ใน log, append-only + hash chain สำหรับหลักฐาน

## ความต้องการด้านคุณภาพ (NFR เริ่มต้น — ยืนยันใน SLA)

| หัวข้อ | เป้าหมาย |
|---|---|
| Availability (prod) | 99.9% ต่อเดือน |
| RPO / RTO | ≤ 15 นาที / ≤ 4 ชั่วโมง (warm standby DR) |
| Backup | PITR (WAL → object storage) เก็บ 35 วัน · ทดสอบ restore ทุกไตรมาส · ซ้อม failover ปีละครั้ง |
| Cookie SDK | < 30 KB gzip · โหลด async ไม่บล็อก render · config แบนเนอร์ cache ที่ CDN 5 นาที (ทำงานต่อได้ระหว่าง API ขัดข้อง) |
| Data residency | ข้อมูลส่วนบุคคลจัดเก็บใน region / data center ประเทศไทย |
| Scalability | api HPA 3–12 · worker 2–8 ตาม queue depth · portal 2–10 · scanner node pool แยก |
| Security baseline | OWASP ASVS L2 · ประกาศมาตรการความปลอดภัย พ.ศ. 2565 · TLS 1.2+ ทุกเส้น |
| Latency / throughput | ยังไม่กำหนดตัวเลข — วัดใน load test (T22) ดู [decisions](../decisions.md) |

## เอกสารในหมวดนี้

- [adr.md](adr.md) — การตัดสินใจทางสถาปัตยกรรม
- [code-structure.md](code-structure.md) — monorepo, layer, middleware, codegen, test
- [security.md](security.md) — authN/Z, tenant resolution, DB roles, PII, key hierarchy
- [integration.md](integration.md) — API surfaces, outbox/webhook, event catalog, jobs
- [deployment.md](deployment.md) — Kubernetes, environment, CI/CD, migration
- API conventions: [`api/openapi/README.md`](../../api/openapi/README.md)
