# Code structure

> ต้นฉบับ: ARC-05 ใน draw.io และชีต Architecture ของ Development Plan (REPO / MODMAP)

## Monorepo

```
pdpa-platform/
├── CLAUDE.md                     คำสั่งถาวรสำหรับ Claude Code
├── .claude/                      commands + skills ของโปรเจกต์
├── api/openapi/                  OpenAPI 3.1 (source of truth) + x-permission
├── apps/
│   ├── admin/                    Next.js App Router (ผู้ใช้ภายใน, BFF)
│   └── portal/                   Next.js (เจ้าของข้อมูล / guest, custom domain)
├── packages/
│   ├── ui/                       shadcn/ui + Tailwind + Radix (design system)
│   ├── api-client/               openapi-typescript + TanStack Query hooks (generated)
│   ├── i18n/                     ข้อความ th / en (next-intl)
│   ├── form-renderer/            renderer ของ form engine (admin + portal)
│   ├── authz/                    usePermission() · <Can permission="…">
│   └── cookie-sdk/               vanilla TS SDK → build ไป CDN
├── backend/                      Go module
│   ├── cmd/{api,worker,scanner,migrate}/
│   ├── internal/
│   │   ├── platform/<service>/   tenant · workflow · forms · docs · files · jobs · events · audit · crypto · notify · report · importer · search · ai
│   │   ├── iam/ org/ consent/ cookie/ notice/ ropa/ dataflow/ risk/ assess/ dsar/ breach/ vendor/ agreement/ dpo/ gov/
│   │   └── pkg/                  authz · db · httpx · otel · validate (ไม่มี business logic)
│   └── db/
│       ├── migrations/           goose (00001 … ) — ห้ามแก้ไฟล์ที่ deploy แล้ว
│       ├── queries/<module>/     SQL ของ sqlc
│       ├── schema.sql            DDL รวม (อ่านอย่างเดียว / อ้างอิง)
│       └── sqlc.yaml
├── deploy/                       helm/ · compose/
├── infra/                        OpenTofu / Terraform
├── tests/                        e2e/ (Playwright) · load/ (k6)
├── docs/                         ความรู้ของระบบ (ชุดนี้)
└── design/                       ไฟล์ต้นฉบับ draw.io / Excel
```

## Module → package → schema → API

| epic | ชื่อ | Go package | schema | admin API | หน้าจอ (Next.js) |
|---|---|---|---|---|---|
| PLT | โครงสร้างพื้นฐานแพลตฟอร์ม (Platform Foundation) | `backend/internal/platform/<service>` | platform | `/admin/v1/platform` | admin: /settings/* |
| IAM | ระบบจัดการผู้ใช้และสิทธิ์ (User & Permission) | `backend/internal/iam` | iam | `/admin/v1/iam` · `/admin/v1/me` | admin: /admin/users, /admin/roles; portal: /auth/* |
| ORG | ข้อมูลหน่วยงาน (Organization) | `backend/internal/org` | org | `/admin/v1/org` | admin: /org/* |
| CON | คุกกี้และความยินยอม (Cookies & Consent) | `backend/internal/consent · backend/internal/cookie` | consent · cookie | `/admin/v1/consent` · `/admin/v1/cookie` | admin: /consent/*, /cookies/*; portal: /preferences, /c/[id]; SDK |
| PNG | ประกาศความเป็นส่วนตัว (Privacy Notice Generator) | `backend/internal/notice` | notice | `/admin/v1/notices` | admin: /notices/*; portal: /notice/[slug] |
| ROPA | บันทึกกิจกรรมการประมวลผล (RoPA) | `backend/internal/ropa` | ropa | `/admin/v1/ropa` | admin: /ropa/* |
| RTG | RoPA ฉบับมาตรฐาน (ROPA Template Generator) | `backend/internal/ropa/templates` | ropa | `/admin/v1/ropa/templates` | admin: /ropa/templates/* |
| DFG | แผนผังการไหลของข้อมูล (Data Flow Generator) | `backend/internal/dataflow` | dataflow | `/admin/v1/dataflow` | admin: /data-flow/* (React Flow) |
| DSAR | คำขอใช้สิทธิของเจ้าของข้อมูล (DSAR) | `backend/internal/dsar` | dsar | `/admin/v1/dsar` | admin: /requests/*; portal: /request, /request/status |
| BRE | แจ้งเหตุละเมิดข้อมูล (Data Breach Notification) | `backend/internal/breach` | breach | `/admin/v1/breach` | admin: /incidents/*; portal: /report-incident |
| DPO | งานของ DPO (DPO Module) | `backend/internal/dpo` | dpo | `/admin/v1/dpo` | admin: /dpo/*, /dashboard |
| DPIA | แบบประเมินผลกระทบ (DPIA) | `backend/internal/assess` | assess | `/admin/v1/assessments` | admin: /assessments/*; portal: /guest/assessment/[token] |
| RRA | ประเมินความเสี่ยงกิจกรรม (ROPA Risk Assessment) | `backend/internal/risk` | risk | `/admin/v1/risk` | admin: /risk/* |
| VEN | ประเมินคู่ค้า (Vendor Assessment) | `backend/internal/vendor` | vendor | `/admin/v1/vendors` | admin: /vendors/*; portal: /guest/vendor/[token] |
| DPA | ข้อตกลงการประมวลผลข้อมูล (DPA) | `backend/internal/agreement` | agreement | `/admin/v1/agreements` | admin: /agreements/dpa/* |
| DSA | ข้อตกลงการแบ่งปันข้อมูล (DSA) | `backend/internal/agreement` | agreement | `/admin/v1/agreements` | admin: /agreements/dsa/* |
| DPX | DPO ส่วนต่อขยาย (DPO Extension) | `backend/internal/gov` | gov | `/admin/v1/governance` | admin: /governance/*; portal: /learn |

หมายเหตุ: DPA และ DSA ใช้ agreement engine เดียวกัน (แยกด้วย `agreements.agreement_type`) · RTG เป็น sub-package ของ ropa · endpoint สาธารณะดู [integration.md](integration.md) และ [`api/openapi/README.md`](../../api/openapi/README.md)

## Layer ภายใน module

```
backend/internal/<module>/
├── http/        handler ที่ generate interface จาก OpenAPI (oapi-codegen strict-server) — แปลง request/response เท่านั้น
├── service/     business rules · state machine · เปิด transaction · เรียก module อื่นผ่าน interface
├── store/       sqlc generated + wrapper (ไม่มี ORM) — อ่าน/เขียนเฉพาะ schema ของตัวเอง
├── events/      นิยาม domain event + เขียน outbox ใน tx เดียวกัน
├── jobs/        River job args + worker
└── <module>.go  ประกาศ Service interface ที่ module อื่นใช้ได้ + wiring
```

- handler ไม่มี business logic และไม่เรียก store ตรง
- **transaction model = unit of work**: API เปิด 1 transaction ต่อ request ที่ middleware Tx (#11) ผ่าน `db.WithTenantTx` (BEGIN → set_config tenant / user → handler → audit → COMMIT หรือ ROLLBACK เมื่อ error) · worker เปิด 1 transaction ต่อ job · service และ store ใช้ tx จาก context เท่านั้น **ห้าม BEGIN เอง**
- AuthZ (#9) ทำงานก่อน Tx จึงอ่านสิทธิ์จาก cache ใน Valkey (60 วินาที) และเมื่อ cache miss ให้โหลดผ่าน `db.WithTenantTx` แบบอ่านอย่างเดียวของตัวเอง (tenant มาจาก #8 แล้ว)
- ห้ามเรียกระบบภายนอก (HTTP, SMTP, LLM) ภายใน request transaction — enqueue River job ด้วย `InsertTx` แล้วให้ worker ทำ
- FK constraint ของ PostgreSQL ไม่ผ่าน RLS: ก่อนบันทึก id ที่อ้างถึงแถวอื่นใน tenant ให้ตรวจว่า id นั้นมองเห็นได้ภายใต้ RLS (helper `store.MustExist…` / `SELECT … FOR KEY SHARE`) และมี test ว่าอ้าง id ของ tenant อื่นแล้วได้ 404 / 422
- ห้าม import `internal/<other>/store` หรือ `internal/<other>/service` implementation — ใช้ interface ที่ module นั้นประกาศ (ตรวจด้วย depguard / go-arch-lint ใน CI)
- งานที่ต้อง retry / ใช้เวลานาน → enqueue River job ใน transaction เดียวกัน (`InsertTx`) · job args มี `tenant_id` เสมอ
- job framework อยู่ที่ `internal/platform/jobs` (PLT-10): args ของ job ต่อ tenant ฝัง `jobs.TenantArgs` · enqueue ด้วย `jobs.Enqueue(ctx, client, args, opts)` ซึ่งใช้ tx จาก context และปฏิเสธ job ของ tenant อื่น · worker สร้างด้วย `jobs.NewWorkerClient` ซึ่งมี `TenantTxMiddleware` เปิด `WithTenantTx` 1 ครั้งต่อ job จาก `tenant_id` ใน args (worker อ่าน tx ด้วย `db.MustTxFromContext`; job ที่ไม่มี tenant_id และไม่อยู่ใน `jobs.GlobalKinds` ถูก cancel) · job args เก็บใน `river_job` แบบไม่เข้ารหัสและไม่มี RLS จึงใส่ได้เฉพาะ id ห้ามมีข้อมูลส่วนบุคคล · worker ต้อง idempotent (at-least-once: job ที่ถูกตัดกลางคันจะรันซ้ำ) · job ที่ต้องไม่ซ้ำใช้ `UniqueOpts{ByArgs: true}` (tenant_id อยู่ใน args จึง unique ต่อ tenant) · periodic job รันเฉพาะ leader
- เวลา: ใช้ clock ที่ inject ได้ (ทดสอบ SLA / 72 ชม. / 30 วัน ได้โดยไม่รอเวลาจริง)

## Middleware ของ Go API (ตามลำดับ)

| # | middleware | หน้าที่ |
|---|---|---|
| 1 | RequestID | X-Request-Id |
| 2 | OTel | trace · metric |
| 3 | Recover | panic → 500 |
| 4 | Access log | JSON · no PII |
| 5 | CORS / CSRF | ตาม surface |
| 6 | Rate limit | Valkey |
| 7 | AuthN | JWT · guest · OTP session |
| 8 | Tenant | claim tid → ctx |
| 9 | AuthZ | x-permission + scope |
| 10 | Idempotency | Idempotency-Key |
| 11 | Tx | BEGIN + SET LOCAL |
| 12 | Handler | → service → store |
| 13 | Audit | audit_log + outbox |

Tx (#11) ครอบ Handler (#12) และ Audit (#13): audit ระดับ request เขียนใน transaction เดียวกันก่อน COMMIT · audit / outbox ระดับ domain เขียนใน service · `/public/v1` หา tenant จาก `platform.public_keys` ที่ขั้น Tenant (#8) ก่อนเปิด Tx

## Codegen

| จาก | เครื่องมือ | ผลลัพธ์ |
|---|---|---|
| api/openapi/*.yaml | oapi-codegen (strict-server, chi) | backend/internal/<module>/http/*.gen.go |
| api/openapi/*.yaml | openapi-typescript + hooks | packages/api-client |
| backend/db/queries/<module>/*.sql | sqlc (pgx/v5) | backend/internal/<module>/store/*.gen.go |
| docs/states/state-machines.yaml | สคริปต์ของโปรเจกต์ (ทำใน P0) | ตาราง transition ใน service + test |
| docs/security/permissions.yaml | migration / seed | iam.permissions, iam.roles |

CI ต้อง fail ถ้า generate แล้วมี diff (ARC-06 ขั้น Codegen check)

## Frontend

- `apps/admin/app/[locale]/(dashboard)/<module>/…` — หน้าต่อ module ตามตารางด้านบน · เมนูและปุ่มตาม `packages/authz`
- `apps/admin/app/api/auth/[...nextauth]` — Auth.js + Keycloak (OIDC code + PKCE) · session อยู่ฝั่ง server (Valkey) · cookie httpOnly/Secure/SameSite=Lax
- `apps/admin/app/api/bff/[...path]` — proxy ไป Go API แนบ JWT ฝั่ง server + ตรวจ CSRF token สำหรับ method ที่เปลี่ยนข้อมูล
- Server Components ดึงข้อมูลผ่าน BFF · Client Components ใช้ TanStack Query hooks จาก `packages/api-client`
- ฟอร์ม: React Hook Form + Zod (schema สร้างจาก OpenAPI) · ตาราง: TanStack Table · rich text: TipTap (เก็บ JSON, sanitize ก่อนแสดง) · data map: React Flow + ELK
- ข้อความทุกตัวผ่าน next-intl (`packages/i18n`, th เป็นค่าเริ่มต้น + en) · วันที่แสดง Asia/Bangkok และปี พ.ศ. ตาม locale
- portal: tenant จาก Host (custom domain) → public key ใน `platform.public_keys` · ISR/edge cache เฉพาะหน้าที่ไม่ผูกผู้ใช้

## การทดสอบ

| ระดับ | เครื่องมือ | สิ่งที่ต้องครอบคลุม |
|---|---|---|
| unit | go test -race · Vitest | business rules, state machine (ทุก transition), deadline calculator |
| integration | testcontainers-go (PostgreSQL + Valkey) | repository + RLS (tenant A มองไม่เห็น B), migration up/down, outbox |
| contract | OpenAPI (schemathesis / kin-openapi) | ทุก endpoint: 401 ไม่ login, 403 ไม่มีสิทธิ์, problem+json, validation |
| E2E | Playwright | flow หลักใน BP-01 ถึง BP-12 (admin + portal) |
| load | k6 | Consent API, banner config ผ่าน CDN, portal |
| security | gosec · semgrep · govulncheck · trivy · OWASP ZAP | ทุก PR / ก่อน release |
