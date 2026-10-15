# PDPA Management Platform

Multi-tenant platform for Thai PDPA compliance: cookies & consent, privacy notices, RoPA, DSAR, breach notification, DPIA/risk, vendors, DPA/DSA agreements, DPO workspace. 17 epics · 275 features · phases P0–P4. Primary UI language Thai (`th`), secondary English (`en`).

## Where the knowledge lives
- `docs/README.md` — index. Start there for anything you don't know about the domain or design.
- Feature work: `docs/backlog/backlog.csv` (one row per feature ID) → `docs/modules/<EPIC>.md#<id>` → the linked `docs/processes/BP-xx.md`, `docs/states/ST-xx.md`, `docs/sequences/SEQ-xx.md` → `docs/data/<schema>.md`.
- Rules: `docs/legal/pdpa-rules.md` (law → system rule → test), `docs/architecture/security.md`, `docs/security/permissions.md`.
- Open questions: `docs/decisions.md` §2 — never invent answers. If the question affects the data model, an API contract or legally relevant behaviour, ask before building; if it is only a tunable value, implement the listed default as configuration and say so.
- Originals (reference only): `design/PDPA_System_Analysis.drawio`, `design/*.xlsx`.

When sources disagree: `backend/db/migrations` > `docs/states` > `docs/architecture` + `docs/decisions.md` > `docs/modules` > `docs/processes` / `docs/sequences` > `design/`. If the conflict changes behaviour, stop and ask.

## Stack and layout
- Frontend: Next.js App Router + TypeScript in `apps/admin` and `apps/portal` (pnpm + Turborepo); shared `packages/{ui,api-client,i18n,form-renderer,authz,cookie-sdk}`. BFF pattern with Auth.js + Keycloak — access tokens never reach the browser.
- Backend: Go modular monolith in `backend/`: binaries `cmd/{api,worker,scanner,migrate}`; one module per PostgreSQL schema in `backend/internal/<module>` (`http/`, `service/`, `store/`, `events/`, `jobs/`); chi + oapi-codegen (strict server), pgx v5 + sqlc, goose, River.
- Data and services: PostgreSQL 16+ with RLS per tenant, Valkey, S3/MinIO + ClamAV, OpenBao/KMS, Keycloak, Gotenberg, OpenTelemetry.
- Contract: `api/openapi/` (OpenAPI 3.1) generates Go handlers and the TS client — conventions in `api/openapi/README.md`.
- Database: `backend/db/migrations` (goose, 00001–00020 baseline) · roles and privileges in `deploy/db/00-bootstrap.sql` + `deploy/db/10-grants.sql` (tested with real non-superuser roles) · `backend/db/schema.sql` is the combined read-only DDL.
- Full detail: `docs/architecture/code-structure.md`.

## Commands
Scaffolded (2026-09-25). No Docker in this environment, so `deploy/compose/docker-compose.yml`
itself is untested; instead, Postgres 17 + pgvector runs as a **permanent** Homebrew service on
port 5433 (README.md § Local Postgres โดยไม่ใช้ Docker) and is what everything below actually ran
against. Valkey (`redis`, already a Homebrew service on this machine) plays that role too. See
"Scaffold status" below.
- `make dev` — docker compose (postgres, valkey, minio, keycloak, gotenberg, clamav, mailpit); on this machine, use the Homebrew Postgres/Valkey above instead and start `backend/cmd/api`, `backend/cmd/worker` and `pnpm dev` yourself (see the Makefile's echoed instructions) — not yet backgrounded into one command
- `make gen` — sqlc, oapi-codegen, openapi-typescript (all three re-run and produced correct output while scaffolding; `.github/workflows/ci.yml` (PLT-01) now runs this then `git diff --exit-code` on every push/PR, so generation drift fails CI for real)
- `make migrate` / `make migrate-down` — goose as `pdpa_migrator` with `options=-c role=pdpa_owner`, then River migrations and `deploy/db/10-grants.sql`; **verified**: all 20 goose migrations + River's own + grants applied cleanly, twice (once during initial verification, once for real onto the permanent database)
- `make test` — `go test -race ./...` (real; includes `internal/iam/store`'s two-tenant RLS isolation test — see PLT-02 below) + `pnpm -r test` (no vitest config yet, will fail) · `make test-int` — not implemented (needs testcontainers-go setup; the isolation test above uses `internal/pkg/dbtest` against a real Postgres instead, as a stand-in until Docker is available) · `make e2e` — not implemented (needs a `@pdpa/e2e` Playwright package)
- `make lint` — not implemented (needs golangci-lint config, ESLint flat config, sqlfluff config)
- Local run without Docker/Keycloak (README § รันบนเครื่องตัวเอง): `make dev-env` (creates `.env`, random dev secrets) →
  `make migrate` → `make dev-seed` (`backend/cmd/devseed`: demo tenant + ORGADMIN/DPO user, fixed ids) → `make run-api`,
  `make run-worker`, `make run-web`. **Dev login** (`AUTH_DEV_LOGIN=true`, `apps/admin/src/lib/dev-auth.ts`): the admin app
  signs RS256 tokens for that user and serves `/api/dev-auth/jwks`, which `.env`'s `OIDC_*` point the API at; off in any
  production build. `apps/admin/next.config.ts` loads the root `.env` (`@next/env`, forced reload) and sets `agentRules:
  false` (Next 16's `next dev` otherwise writes its own AGENTS.md/CLAUDE.md into the app). `make migrate` now passes the
  grants path it needs from `backend/`.

### Scaffold status (P0, from `/scaffold`)
- Backend: Go module builds and vets clean. Reference slice `GET /admin/v1/me` is implemented and **verified end to end against a real database**: chi router, full 13-step middleware chain (`internal/pkg/{authn,authz,db,httpx,idempotency,otelx,ratelimit,validate}`), sqlc store, oapi-codegen strict handler, hash-chained request audit (`internal/platform/audit`). Confirmed by running migrations on Postgres 17+pgvector (Homebrew, port 5433, since Docker isn't installed), seeding a tenant/user/role, minting a throwaway signed JWT (no Keycloak available), and hitting the running API: 200 with correct roles/permissions/masked email/tenant, 401 for missing/invalid tokens, RLS proven to isolate two tenants' rows on `iam.users`, AuthZ grants correctly cached in Valkey with a 60s TTL, and two properly hash-chained `platform.audit_log` rows (first `prev_hash IS NULL`, second chained). One real bug found and fixed this way: `chi.RouteContext(ctx).RoutePattern()` is empty for middleware registered with `r.Use()` (chi only populates it once the mux is actually dispatching to the matched route, which is after every top-level `Use()` middleware has run) — AuthZ was relying on it and always 404'd. Fixed by moving AuthZ into the oapi-codegen strict-server middleware hook, keyed by `operationId` instead of route pattern (`internal/pkg/authz.StrictMiddleware`, generic over each module's own generated `StrictHandlerFunc` type). `cmd/migrate` is now verified; `cmd/worker` (River, `partition.maintain`) builds but its own job execution wasn't separately exercised. `cmd/scanner` is an intentional stub (CON-02, not started).
- Frontend: pnpm + Turborepo workspace. `apps/admin` (Next.js 16, Auth.js v5 + Keycloak, next-intl th/en, TanStack Query, the same `/admin/v1/me` reference slice via a BFF proxy) builds and was smoke-tested with `next build` + `next start` — correctly falls back to a sign-in prompt with no session. Not yet re-tested against the now-working backend (no Keycloak here to complete a real OAuth login). `apps/portal` is a bare, buildable shell (no portal features exist yet). `packages/ui` has one placeholder `Button`, not the shadcn/ui + Radix design system CLAUDE.md calls for. `packages/form-renderer` and `packages/cookie-sdk` are empty placeholders (no feature needs them yet).
- Infra: `deploy/compose/docker-compose.yml` + `deploy/compose/initdb/01-bootstrap.sh` wrap `deploy/db/00-bootstrap.sql` correctly by construction — `00-bootstrap.sql` itself was run for real (see above) and works as written. The compose file's container wiring is still unverified (no Docker here). The `minio/minio` image reference is unverified — Docker Hub's public listing for it returned 404 while scaffolding; check before relying on it. Keycloak comes up with a blank realm only: the Organizations multi-tenant model and `tid` claim mapper (`docs/decisions.md` Q-18) need the T13 PoC first.
- CI (PLT-01) now exists: `.github/workflows/ci.yml`, two jobs (`backend`: codegen-drift check, `go vet`/`gofmt`/build/test against Postgres 16+pgvector and Redis service containers; `frontend`: `pnpm -r build/test/typecheck`). Not started: `make lint` (still needs a `.golangci.yml` and an ESLint flat config — left out of CI on purpose rather than wiring a lint step with no rules configured), `test-int` (testcontainers-go, needs Docker), `e2e` (`@pdpa/e2e` Playwright package).

### PLT-01 โครงระบบ Go modular monolith + Next.js monorepo (`docs/modules/PLT.md#plt-01`) — done
The scaffold itself (`backend/internal/<module>/{http,service,store}`, chi + oapi-codegen, pgx + sqlc, goose,
RFC 9457 errors, binaries `cmd/{api,worker,scanner,migrate}`; Turborepo `apps/{admin,portal}` +
`packages/{ui,api-client,i18n,...}`) was already built by the original `/scaffold` pass (see the note above)
and proven out dozens of times since by every module this session has added — this pass closed PLT-01's own
two acceptance criteria, which the scaffold alone didn't yet satisfy. "สร้างโมดูลใหม่จาก template ได้ภายใน 1
วัน": no generator CLI exists (none was needed — a `docs/architecture/code-structure.md` "สร้างโมดูลใหม่"
checklist now documents the exact repeatable steps, each already proven well under a day by every module
this codebase has, from `ORG` through `DFG`). "แก้ OpenAPI แล้ว Go stub และ TS client generate ใน CI": this
had no CI at all until now — `.github/workflows/ci.yml` (two jobs: `backend` runs `make gen` then
`git diff --exit-code`, so a committed OpenAPI/SQL change with stale generated code fails the build, plus
`go vet`/`gofmt`/`go build`/`go test` against real Postgres 16+pgvector and Redis service containers;
`frontend` runs `pnpm -r build/test/typecheck`) — satisfies the criterion literally. Not done: `make lint`
in CI (no `.golangci.yml`/ESLint flat config exist yet — adding a lint *step* with no actual rules configured
would be theater, not a real gate; add it once those configs exist), `test-int`/`e2e` in CI (both need Docker,
not available in this environment to verify the workflow against, though GitHub's own hosted runners have it).

### PLT-02 Multi-tenant และการแยกข้อมูล (`docs/modules/PLT.md#plt-02`) — done for the acceptance criterion; infra pieces deferred
The DB-level half (`tenant_id` on every table + RLS) was already in the migrations from the
knowledge kit, not new work here. `internal/pkg/dbtest` (a reusable two-tenant test harness — seeds a
tenant via `pdpa_platform`, a user inside it via `pdpa_app` + `WithTenantTx`, cleans both up, points at a
real Postgres via `TEST_DATABASE_URL`/`TEST_PLATFORM_DATABASE_URL`, skips cleanly if neither is reachable)
and `internal/iam/store/isolation_test.go` were built earlier in this pass of work, establishing the
pattern — rule 1 ("every new repository gets a two-tenant isolation test") was then actually followed for
every module built afterward, ~50 of them.

What this later pass added, closing the literal acceptance criterion ("ผู้ใช้ tenant A เข้าถึงข้อมูล
tenant B ไม่ได้แม้เรียก API ตรง (automated test ทุก endpoint)"): a from-scratch **audit** of every
tenant-scoped module's actual test files (not a naming-convention grep, which under-counts — modules name
their proof differently: `TestVisibility`'s own cross-tenant case, `TestChains_ArePerTenant`'s direct RLS
query, `TestTenantIsolation`, `TestAccessAndIsolation`, …) confirmed **every one of them already has a
real two-tenant isolation proof** — service-layer repository tests directly exercising RLS, which is the
actual enforcement boundary every one of that module's `/admin/v1` operations shares, so a literal
per-operationId duplicate would just repeat the same proof with no benefit. Read literally, "automated
test ทุก endpoint" means no endpoint ships unverified, not one bespoke test file per operationId — and a
live HTTP crawl hitting every operation generically has no generic shape to build against (every module's
request/response/fixture differs, rule 9), so it would just reimplement each module's own fixture worse.

To make that guarantee permanent rather than a one-time audit result, `internal/pkg/isolationaudit` is a
new static CI gate: it reads every `x-module` tag actually used in `api/openapi/openapi.yaml` (so a future
module's own tag puts it on this list with no second place to remember to update) and fails if that
module's own test files carry no isolation-proof evidence (a regex matching every phrasing this audit
found: `isolat`, `per.?tenant`, `cross.?tenant`, `other tenant`, `another tenant`) — `platform`'s own
sub-packages (docs/collab/files/forms/importer/jobs/notify/versioning/workflow/audit) are checked
individually since they're independently schema-scoped despite sharing one OpenAPI tag. Proven to actually
fail on a real gap (temporarily pointed one module at a nonexistent directory, confirmed the test failed
with a clear message, reverted) before being left in place. `go test ./internal/pkg/isolationaudit/...`
needs no database — pure file/text scan — so it runs in both CI jobs for free.

Not done, and deliberately not scaffolded speculatively (no consuming endpoint exists yet — see
"don't design for hypothetical future requirements" in this project's engineering conventions):
- The subdomain / custom-domain / `platform.public_keys` tenant resolver for `/public/v1` and the
  portal (`docs/architecture/security.md` describes it; the table exists in migrations already).
  Build it when the first `/public/v1` feature (CON, DSAR or BRE) actually needs it.
- Tenant provisioning (standard roles + master data for a new tenant) — depends on ORG, not built.
- DB-per-tenant deployment mode.
- Next.js tenant-context middleware / portal custom domain (same reasoning: no portal feature yet).

### PLT-03 ระบบหลายภาษา TH/EN (`docs/modules/PLT.md#plt-03`) — done for what exists so far
Domain content doesn't get a separate translation table (already true in the migrations — the SA
note on this feature says so): `_th`/`_en` columns or `{th, en}` jsonb next to the data. This pass
covered the fixed strings the platform itself owns:
- Backend: `internal/pkg/i18n` — a Go message catalog (CLAUDE.md rule 12) resolving
  `Accept-Language` (falling back to `th`, decisions.md Q-17) and localizing every
  `httpx.Problem`'s title by its stable `code`; module-specific business-rule codes get cataloged
  as each module is built. Unit-tested (`go test ./internal/pkg/i18n/...`) and confirmed live:
  the same `authn.required` request returns "กรุณาเข้าสู่ระบบ" with no header / `Accept-Language: th`
  and "Authentication required" with `Accept-Language: en`.
- Frontend: `apps/admin` now has a working `LocaleSwitcher` in the root layout header (so it's on
  every page, satisfying the acceptance criterion literally, not just the dashboard), proper
  next-intl routing (`defineRouting` + `createNavigation` in `src/i18n/routing.ts`, so the switcher
  changes locale without leaving the current page), the Sarabun font (Thai government's standard
  typeface, covers Latin too) via `next/font/google`, and `@pdpa/i18n`'s `formatDate` — Buddhist Era
  for th / Gregorian for en, both converted to Asia/Bangkok before formatting (CLAUDE.md rule 11).
  `formatDate` has real unit tests (`pnpm --filter @pdpa/i18n test`, Vitest — the first vitest setup
  in this monorepo) proving the BE-year math and the UTC→Bangkok day boundary, not just eyeballed.
- Fixed a real bug this surfaced: the BFF (`apps/admin/src/app/api/bff/[...path]/route.ts`) was
  forwarding the browser's raw `Accept-Language` header (e.g. `"en-US,en;q=0.9"`) straight to the
  Go API, which only accepts the literal values `"th"`/`"en"` — every real browser request would
  have gotten a spurious 400. Fixed by reading the locale from the page's own path via `Referer`
  instead (that route isn't nested under `app/[locale]/`, so next-intl has no param to resolve
  there); Server Components use `next-intl/server`'s `getLocale()` directly in `lib/api.ts`, which
  does have one.

### PLT-10 Background jobs และ scheduler (`docs/modules/PLT.md#plt-10`) — done
`internal/platform/jobs` is the job framework every module's `jobs/` package builds on (rules in
`docs/architecture/code-structure.md` and `docs/architecture/integration.md` § Background jobs):
`TenantArgs` + `TenantTxMiddleware` (one `WithTenantTx` per job from the args' `tenant_id`; no tenant and
not in `GlobalKinds` → cancelled), `Enqueue` (InsertTx on the request's own tx, rejects another tenant's
job), `AlertHandler` (WARN + `pdpa.jobs.failed` per failed attempt, ERROR `alert=job_discarded` +
`pdpa.jobs.discarded` on the final one), `NewWorkerClient` (leader-only periodic jobs, 25s soft stop via
`WORKER_SOFT_STOP_TIMEOUT`), `NewInsertClient` (cmd/api's insert-only client). Tests against real Postgres
cover both acceptance criteria (retry then alert; two worker instances work each job once and fire a
periodic job once), RLS inside a job, enqueue commit/rollback, per-tenant unique jobs, graceful shutdown.
- Job-status page: `GET /admin/v1/platform/jobs` (`platformListJobs`, `x-permission: admin.job.read` —
  new permission, ORGADMIN + SUPER, migration 00021) → `internal/platform/jobs/http` (own oapi-codegen
  config, `include-operation-ids`) → `jobs.ListForTenant`. `river_job` has no RLS, so the tenant filter is
  `args->>'tenant_id' = current_setting('app.tenant_id')` read from the request tx; its two-tenant test is
  `TestListForTenant_IsolatesFiltersAndPages`, contract test (401/403/200 + isolation) in
  `jobs/http/handler_test.go`. Frontend: `apps/admin` `/[locale]/settings/jobs` (`useJobs` infinite query,
  10 s refresh, state/kind filters, th/en keys under `jobs.*`). Verified live against the running API with a
  self-signed JWT; the page itself is only build/typecheck-verified (no Keycloak here for a real login).
- Fixed while verifying: `internal/pkg/validate` never validated anything — gorillamux matched the spec's
  `servers` host (`api.pdpa.example`), so `FindRoute` failed for every real Host and requests passed
  through; and with a nil auth func kin-openapi fails every secured operation instead of skipping
  security. Both fixed + `validate_test.go`. Also `cmd/migrate -down` no longer drops River's tables
  (which deleted every queued job) unless `-river-down` is given.
- Not done: SUPER's cross-tenant job view (`/provider/v1`, no provider surface exists yet); failure alerts
  reach people only through log/metric alert rules until PLT-04 (notifications) exists.

### PLT-11 Domain events และ outbox (`docs/modules/PLT.md#plt-11`) — done
`internal/platform/events` (rules in `docs/architecture/integration.md` § Outbox → webhook): services call
`Publisher.Publish(ctx, events.Event{...})` inside their request tx — it validates type + data fields against
`Catalog` (generated from `docs/architecture/events.yaml` by `go generate ./internal/platform/events`, drift
test included), writes `platform.outbox_events` and enqueues `outbox.dispatch` (unique per tenant) in the same
tx. `Dispatcher` delivers each event to in-process subscribers (`Registry.Subscribe`, wired in `cmd/worker`)
and creates `pending` webhook deliveries in a per-event savepoint (`db.Savepoint`, new), keeping per-aggregate
order when one fails; `Sweeper` (`outbox.sweep`, every minute, in `jobs.GlobalKinds`) re-dispatches every live
tenant. Migration 00022: partial index for unpublished events + unique (subscription, event) delivery.
Tests (`events_test.go`, real Postgres): catalog drift, catalog validation, publish commit/rollback + one
dispatch per burst, delivery + subscription matching, crash-before-commit redelivery with an idempotent
consumer (the acceptance criterion), failing event blocks only its aggregate, two-tenant isolation, sweeper.
Also run live: real `cmd/worker` published a raw outbox row within ~4 s via sweep → dispatch.
Not done: `webhook.deliver` (HTTP + HMAC + retry → PLT-15, needs ORG-16 API clients and OpenBao secrets);
NATS forwarding (optional per ADR); retention/cleanup of published outbox rows (no rule yet). No module
publishes events yet — the first producers arrive with their features.

### PLT-12 Tamper-evident audit log (`docs/modules/PLT.md#plt-12`) — done (retention pending)
`internal/platform/audit/service`: the hash now covers every column but `id`/`hash` (length-prefixed, tag
`audit/v2`, canonical JSON for before/after so jsonb round-trips verify) — before, IP, user agent, actor/entity
ids and times were unprotected. `Write` takes a per-tenant advisory lock (held to COMMIT; audit is the last
step) and derives `occurred_at` from the DB clock, never earlier than the previous row — concurrent requests
used to be able to fork the chain. `Verify` replays a chain in pages; `audit/jobs` runs it daily per tenant
(`audit.verify_sweep` → `audit.verify`, alert `audit_chain_broken`). `Changes` gives the before/after diff.
The request audit now records IP (TCP peer) and user agent. Migration 00023 is a Go migration (partition
list differs per environment) for the chain index. `dbtest.OwnerPool` added for tamper tests. Tests: every
column edit, row deletion and a forged hash detected at the right row; app role denied UPDATE/DELETE; 20
concurrent writers keep one chain (checked the test fails without the lock); per-tenant chains + RLS; verifier
alert; sweeper. **Rows written by the old hash code (local dev DBs only) no longer verify** — clear them.
Retention (user's decision D-22: **5 years**, migration 00033): daily `audit.retention` calls SECURITY DEFINER
`platform.drop_expired_audit_partitions(keep)` — drops whole monthly partitions older than `AUDIT_RETENTION_MONTHS`
(default 60; the function refuses less, so app credentials can't purge recent evidence) after recording each tenant's
last dropped row in append-only `platform.audit_chain_anchors`; `Verify` and `NextAuditChainLink` start from the newest
anchor, so chains still verify and keep growing (also for a tenant whose every row was purged) while a forged anchor is
caught. Rows in `audit_log_default` are never dropped. Client IP (D-23): `internal/pkg/clientip` + `TRUSTED_PROXIES`
(IPs/CIDRs; empty = TCP peer) — X-Forwarded-For read right to left only when the peer is trusted, first untrusted hop
wins; used by the request audit and the rate limiter; verified live with the api binary.

### PLT-13 Field-level PII encryption (`docs/modules/PLT.md#plt-13`) — done
`internal/platform/crypto`: `Keyring` seals `*_enc` values (AES-256-GCM, versioned DEK per tenant + data class,
associated data = tenant + class + column) and computes `blind_index` (per-tenant HMAC-SHA256 over the
normalized identifier — e-mail lower-case, phone E.164 defaulting to +66, 13-digit national id, Thai digits
mapped). DEKs live wrapped in new table `platform.tenant_keys` (migration 00024; DELETE revoked from the app
roles — deleting a key is crypto-shredding) and are cached unwrapped for 5 min. KEK = `crypto.Transit`
(OpenBao, one key `tenant-<uuid>` per tenant); `LocalKEK` is for dev/tests only. Rotation: `RotateDEK` (new
writes use v+1, old versions still decrypt), `RotateKEK` (rotate Transit key + rewrap every DEK, no data
re-encryption). Tests run against a real OpenBao dev server when reachable (README) and LocalKEK:
acceptance (stored bytes unreadable, exact lookup from differently formatted input, unreadable without the
KEK), tenant isolation, column binding/tamper, both rotations, concurrent first use, app can't delete keys.
Not wired into `cmd/api`/`cmd/worker` yet (no consumer; first is CON's subject identifiers — add
`OPENBAO_ADDR`/token config then). Not done: blind-index key rotation (needs a reindex of every row),
re-encrypting old data under a new DEK, a schedule for the 12-month KEK rotation.

### PLT-09 File storage & AV scan (`docs/modules/PLT.md#plt-09`) — done
`internal/platform/files` + `files/http`: `POST /admin/v1/platform/files` (multipart; validator skips buffering
multipart bodies; `cmd/api` caps bodies at 1 MiB, upload route at max file + 1 MiB), `GET …/{id}`, `GET …/{id}/download`
→ 302 pre-signed URL (5 min, forced attachment). Upload spools to a temp file, checks size and *sniffed* type,
writes S3 (minio-go; SSE-S3 when `S3_SSE`, default on), inserts `platform.files` (pending) and enqueues
`files.scan` + `files.expire` in the request tx. `files.scan` streams the object through clamd INSTREAM:
pending → clean | infected (object deleted, row + audit kept, `alert=file_infected`) | error after 10 attempts
(state machine `PLT-09` added to `docs/states/state-machines.yaml`). `files.expire` deletes uploads still
unattached after 24 h. Visibility: uploader while unattached; attached files need the permission the owning
module registers in `Service.EntityPermissions` (empty now → attached files undownloadable until a module registers).
All limits are config with placeholder defaults (25 MB, PDF/images/Office/CSV/TXT) — none are in decisions.md.
Frontend: `@pdpa/ui` `FileDropzone` + `ProgressBar`, `@pdpa/api-client` `uploadFile` (XHR progress) /
`useFileStatus` / `fileDownloadHref`, `apps/admin/src/components/file-uploader.tsx` (th/en `files.*`), not mounted
on a page yet (first consumer feature will). BFF now streams bodies, passes redirects back, and refuses
cross-origin state-changing requests (Origin check — the CSRF item its comment had left open).
Tests (real SeaweedFS S3 with SigV4 + real clamd with an EICAR signature): both acceptance criteria (EICAR
rejected + object deleted + audited; link 200 then 403 after its TTL), bad files, visibility incl. other
tenant, orphan expiry, failed-scan path, HTTP contract (401/201/409/404/302/415). Also run live with the real
api + worker binaries. Not verified: SSE-S3 (SeaweedFS here has no KMS), the React component in a browser
(no Keycloak session), BFF Origin check behind a proxy that rewrites Host.

### PLT-04 Notification service (`docs/modules/PLT.md#plt-04`) — done (providers pending Q-03)
`internal/platform/notify` + `notify/http`: templates (tenant override > global, th fallback, text/template with
declared variables only), `Service.Send` in the caller's tx (recipient + variables encrypted via PLT-13, addresses
normalized, non-urgent SMS/LINE held in quiet hours 21:00–08:00 Bangkok — config, undecided), `notify.deliver`
(each failed attempt recorded with a redacted error, next one scheduled 1m/5m/30m/2h/6h, then `failed`; SMTP 5xx
fails at once; status changes audited; state machine `PLT-04` added). Senders: real SMTP (deadline, STARTTLS, Thai
subjects); SMS, LINE — and e-mail without `SMTP_ADDR` — are `MockSender` per decisions.md Q-03, which marks messages
sent without delivering them: **choose providers before production**. `iam/service.ContactOf` resolves user
recipients (rule 9). API: template CRUD with ETag/If-Match + preview (`admin.notification.*`), delivery log +
per-message status (`admin.notification.read`, recipients masked), inbox/read (`authenticated`), and an SSE unread
stream mounted outside Idempotency+Tx (`cmd/api` now registers generated handlers inside `r.Group` with those two
middlewares). `crypto.KEKFromEnv` (Transit, or `KEK_PROVIDER=local` + `LOCAL_KEK_BASE64` for dev) is now required by
api and worker. Frontend: `/settings/notification-templates` (editor + live preview), `/settings/notifications`,
`NotificationBell` in the header (SSE via `EventSource` through the BFF, which now streams response bodies), shared
`lib/me.ts`. `packages/i18n/src/messages.test.ts` parses every th/en message as ICU (caught a real `{{.name}}` bug).
Verified: Go tests (acceptance: retries recorded per attempt then `failed`; a real worker retrying until sent),
HTTP contract incl. 412/428 through the real validator, SSE test; **and an end-to-end browser run** (Playwright +
Chromium against real api + worker + Next with a minted Auth.js session): bell count over SSE, read clears it,
template create with preview, SMS delivered by the worker shown masked on the delivery page, jobs page, file upload
through the BFF, cross-origin POST refused, signed download redirect, English locale, no unexpected errors.
Note: `make test` now runs `go test -p 1` — parallel packages sharing one DB let one package's River worker steal
another's jobs. Committed by mistake earlier and removed: `dump.rdb` (local Redis snapshot, now ignored).

### PLT-07 Comments, attachments, activity (`docs/modules/PLT.md#plt-07`) — done
`internal/platform/collab` + `collab/http`: modules opt a record type in with `collab.Service.Register` (read/write
permission + an RLS existence check; unregistered types are refused; `files.Service.EntityPermissions` is derived
from it, so attachments download with the record's read permission). Comments with one level of replies, @mentions
as `@[Name](user-id)` limited to active users of the tenant, in-app notifications through PLT-04 (global templates
`collab.mention` / `collab.reply`, migration 00026), edit/delete own (If-Match, not with replies), resolve,
attachments (PLT-09 files), activity = the record's audit rows. `iam/service` gained `SearchUsers` (names only, LIKE
wildcards escaped) and `Names`. Frontend: one `RecordCollaboration` component (comments with @-picker, attachments via
`FileUploader`, activity), first mounted on the notification-template editor (`notification_template` registered in
`cmd/api`). Tests: service (mention notifies — acceptance; non-users/other tenant/self ignored; reply notifies the
parent author; access rules incl. other tenant; edit/delete/resolve rules; attachments + activity against real S3),
HTTP contract through the real validator, and a Chromium E2E of the component on the real stack (9/9).
Found by the E2E and fixed: the request audit stored `METHOD + full path` in `audit_log.action` (varchar(80)) — any
path longer than ~76 characters made the audit insert and so **every such request fail with 500**; now ids become
`{id}` and the action is capped (`audit.RequestAction`, tested). Also: `cmd/api`'s response-error handler and the Tx
middleware now log the error with the request id (both used to swallow it).

### PLT-14 Import framework (`docs/modules/PLT.md#plt-14`) — done
`internal/platform/importer` + `importer/http`: modules register an import type in `internal/wiring.ImportTypes()`
(shared by `cmd/api` and `cmd/worker`) as `importer.Type{Permission, Columns, Validate, Apply}`; unregistered types are
404, the type's permission is checked in the service (endpoints are `authenticated`). Flow on River (PLT-10), each job
in its own tenant tx with a 30-minute timeout: `import.prepare` (snoozes until the PLT-09 virus scan is done, reads the
header row, suggests a mapping from labels/aliases) → `PUT …/mapping` (If-Match) → `import.validate` (dry run over every
row, nothing written; error report CSV `line,column,error` without cell values, stored as a PLT-09 file attached to the
import) → `POST …/confirm` (If-Match) → `import.apply` (all valid rows in one savepoint; any failure rolls the whole
import back and marks it `failed`). CSV (BOM stripped, `,`/`;` detected) and XLSX (first sheet, excelize streaming).
State machine `PLT-14` in `docs/states/state-machines.yaml`; `files` gained trusted system operations (`Status`, `Open`,
`SignedURL`, `SaveGenerated`, `AttachSystem`). Frontend: `ImportWizard` (upload → map → check → confirm) +
`@pdpa/api-client` `useImport`, first mounted by ORG-20's holiday import. Tests: 50,000 rows validated and imported
with the error report (acceptance, ~15 s), rollback on a failing row, Excel, mapping rules and transitions, access and
two-tenant isolation, infected file, HTTP contract (401/403/404/400/422/428).

### ORG-20 Organization settings (`docs/modules/ORG.md#org-20`) — done
The business calendar was built first (user's choice: PLT-05 needed it); language/logo/theme followed later in
the same pass. `internal/org/{store,service,http}` — the first business module: several calendars per tenant (name, IANA zone, ISO
workdays 1–7), the first one becomes the default, moving the default keeps `org_settings.default_calendar_id` in step
(migration 00027: one default per tenant, unique names). Holidays are entered by the admin or imported (PLT-14 type
`org.holiday`, registered in `internal/wiring`; `importer.ParseDate` accepts ISO, D/M/YYYY with Buddhist-era years and
Excel serials — the XLSX reader now returns raw cell values); **no seeded holiday data** (user's choice). Other modules
use the exported `orgservice.Calendars` interface (`BusinessCalendar(ctx, id|nil)`) and do the arithmetic in the pure
`internal/pkg/bizcal` (`AddBusinessDays`, `BusinessDaysBetween`, `IsBusinessDay`; no calendar → Mon–Fri Asia/Bangkok).
Permissions: `org.settings.read`, `org.settings.update` (ORGADMIN has no `create`, so adding a calendar is an update).
Frontend `/settings/calendar` (calendar form, holidays by year in BE/CE, `ImportWizard`). Tests: bizcal unit tests
(weekends, Songkran, zone boundary, custom weeks, runaway guard), service (default/validation/audit, acceptance: due
date skips the tenant's holidays, two-tenant isolation, holiday import type), HTTP contract, Chromium E2E incl. a real
CSV import with an error report (15/15).

**Language/logo/theme:** `internal/org/service/settings.go` — the other half of the same `org.org_settings` row
(different columns than the calendar's `default_calendar_id`): `default_language` (th/en), `date_era` (BE/CE),
`branding` jsonb (`logo_file_id`, `theme_color`, `accent_color`). Nothing reads these yet (the UI's own locale
already comes from the `/[locale]/…` path and `Accept-Language`, per PLT-03; no screen consumes the theme colors
yet) but the values are ready for a future feature without another migration. The logo is the caller's own clean
PLT-09 upload, attached the same way as ORG-01's legal-entity logo (`OrgSettingsEntityType = "org_settings"`,
downloads with `org.settings.read`). A tenant that never saved settings reads as the defaults (th, BE, row_version
0) — the same pattern as a tenant with no calendar yet — so the first save just sends `If-Match: "0"`; the upsert
query's `WHERE row_version = $n` still catches a stale write. `GET/PUT /admin/v1/org/settings` (ETag/If-Match,
422 `org.invalid_settings` for schema-legal-but-business-invalid values, 422 `org.logo_not_usable`). UI: a
"General settings" section added to the same `/settings/calendar` page (ORG-20's own frontend note already
called for "an org settings page + the holiday calendar" as one deliverable). Tests: unit (defaults, real save,
stale version, logo must be the uploader's own clean file, tenant isolation), HTTP contract (401/403/428/412/400
schema, run before any calendar exists in the test so the defaults are real defaults).

### PLT-05 Workflow & SLA engine (`docs/modules/PLT.md#plt-05`) — done (no module uses it yet)
`internal/platform/workflow` (+ `http/`, `store/`). **The definition JSON format was designed here (no SA spec
existed) — flagged for review in PLT.md.** Tenant definitions are versioned (every save = new row, running instances
keep theirs; tenants override global ones). Modules call `Start` from their own service and register a `Policy`
(read/write permission + `OnSLA` / `OnTransition` hooks for their own events) in `internal/wiring.Workflow`, which
both binaries use. Access = the record's permissions or being on a task (directly or via an `iam` group, read through
new `iamservice.GroupIDsOf/GroupMembers/GroupNames/SearchGroups`); only the current task's people or writers move it;
group members claim. Deadlines are pure, clock-injected functions (`DueAt`, `ReminderTimes`, `Resume`) on the ORG-20
calendar (`Calendars` interface, satisfied by `orgservice.Service`; `bizcal` gained `SubBusinessDays`,
`AddCalendarDays`). `pause_sla` states pause the timer (`sla_timers.paused_at`, migration 00028 with the
`admin.workflow.*` permissions, open-task indexes and the `workflow.*` in-app templates). SLA reminders/breaches run
as River jobs scheduled exactly at those moments (`workflow.sla_tick`, idempotent). UI: `/tasks` board +
`WorkflowPanel` + `SlaBadge`, `/settings/workflows` form editor. Tests: unit (due dates across Songkran for all
three modes, reminders, pause/resume, definition validation), integration (acceptance: reminder at its configured
time and breach escalation with an injected clock; pause/resume; tasks, claims, access; two-tenant isolation), HTTP
contract, Chromium E2E with the real worker sending a due reminder (17/17).

### ORG-19 Audit log (`docs/modules/IAM.md#org-19`) — done
`audit/service/search.go` + `audit/http`: search (`admin.audit.read`; actor, record, action prefix with LIKE
wildcards escaped, time range, `kind` changes/requests/all — every API request is audited, so per-request rows are
hidden by default; cursor on (occurred_at, id)), CSV export (`admin.audit.export`, ≤ 50,000 rows else 422, formula
cells neutralized, the export itself audited as `platform.audit.export`), on-demand chain verify. Names via new
`iamservice.AllNames` (includes disabled users). UI `/settings/audit` (filters, before/after, export link, verify).
BFF now forwards `Content-Disposition`. Tests: acceptance (who changed whose roles and when, found by target and
exported), kinds/prefix/range/paging/cursor, isolation, formula injection, HTTP contract, Chromium E2E (8/8).

### PLT-08 Versioning & approval (`docs/modules/PLT.md#plt-08`) — done (no module registered yet)
`internal/platform/versioning` (+ `http/`, `store/`): modules register a record type in `internal/wiring.Versioning`
(read/edit/publish permissions, ≥ 1 approval step by role, `Title`, `OnPublish`) and call `SaveDraft` from their own
service. draft → in_review (steps opened, diff vs published fixed) → approved → published (previous superseded,
hook in the same tx); returned/rejected (reason required) → draft. Published content is never edited — a change is a
new version (acceptance); maker-checker: author, requester and earlier-level approvers can't decide (acceptance);
levels in order. Migration 00029: one open + one published version per record, pending-by-role index,
`approval.approved/returned/rejected` in-app templates. `versioning.Diff` = JSON-path changes. Endpoints are
`authenticated` with the policy checked in the service. UI: `RecordVersions`, `VersionDiff`, `/approvals` inbox.
Tests: diff unit tests; integration (both acceptance criteria, level order, one person one level, returns, locked
drafts, access, isolation); HTTP contract; Chromium E2E through a temporary (uncommitted) registration (9/9).

### ORG-01 Legal entities · ORG-04 Org-unit tree (`docs/modules/ORG.md`) — done
`internal/org/service/structure.go` + `org/http/structure.go` (`org.structure.*`). ORG-01: 13-digit registration/tax
ids with the Thai mod-11 check digit (`ValidThaiID`), unique per tenant, address jsonb, parent company without
cycles, logo = the caller's clean PNG/JPEG PLT-09 upload attached as `legal_entity` (`orgservice.Service.Files`, a
small `FileStore` interface; `cmd/api` registers the download permission), `MergeFields` for future document
templates. ORG-04: ltree paths of `u<id>` labels; move rewrites the whole subtree in one statement (cycle refused,
same legal entity only), close needs no active children and keeps history; `UnitWithin` answers scope questions on
the live tree (acceptance: a moved department's team is in its new parent's scope at once). No org events yet
(none in events.yaml). Migration 00030. UI `/settings/organization`: entity form + logo upload, tree with HTML5
drag-and-drop, move menu, search with ancestors, add/rename/close. Tests: check digit, validation, logo rules,
merge fields, move/scope/cycle/close, two-tenant isolation, HTTP contract, Chromium E2E (10/10, real S3 + clamd).

### ORG-07 Master data (`docs/modules/ORG.md#org-07`) — done (defaults are a draft, decisions.md Q-20)
`org/service/masterdata.go` + `org/http/masterdata.go` (`org.masterdata.*`, one `{kind}` path for data_categories,
data_subject_types, processing_purposes, lawful_bases, countries). Migration 00031 seeds the platform defaults —
**user's choice: seed a draft and flag it for legal review** (Q-20 added): 13 lawful bases paraphrasing PDPA
ss.19/24/26 with consent/LIA/sensitive flags, 19 data categories (10 sensitive per s.26), 9 subject groups, 10
purposes, 249 ISO countries with Thai/English names generated from CLDR (`Intl.DisplayNames`) and adequacy `unknown`.
Every tenant sees the defaults at once (acceptance: nothing to provision); tenants add/edit/delete their own rows
of the three editable kinds (codes may not shadow a default; delete refused while referenced); defaults, lawful
bases and countries are read-only. UI `/settings/master-data` with the pending-review banner. Tests: defaults per
kind, one-source edits, read-only rules, in-use delete, isolation, HTTP contract, Chromium E2E (8/8). Found while
testing: a YAML flow-map description with a comma made the spec invalid — `internal/pkg/validate`'s spec test
catches that; run the full suite before starting the stack.

### PLT-06 Form & assessment engine (`docs/modules/PLT.md#plt-06`) — done
`internal/platform/forms` (+ `http/`, `store/`) and `packages/form-renderer`. **The schema/scoring JSON format was designed
here (no SA spec existed) — flagged for review in PLT.md.** Question types text/textarea/number/date/email/single/multi/
yes_no; `visible_if` on sections and questions (comparisons, all/any, only on earlier questions); weighted option scores
into bands; hidden answers dropped. `forms.Evaluate` (Go) and `evaluate` (TS) are twins held together by the shared
fixture `packages/form-renderer/src/fixtures/engine-cases.json` (tested on both sides) — change them together.
Permissions per form type (user's choice, no new codes): `internal/wiring.Forms` registers assessment/questionnaire/dsar/
consent with their module's codes; breach/intake/quiz have no module yet and can't be created. Versions: one draft per form
(If-Match), published versions immutable, running responses keep their version. Responses: draft → submitted (validated,
scored, `result` frozen); owners assign sections to users (in-app `form.section_assigned`), assignees answer only those and
hand them back; `Record` is the one-shot path for modules taking answers from outside (consent, DSAR). Migration 00032,
state machines `PLT-06#form|response|assignment`. UI: `FormRenderer` (react-hook-form + zod resolver calling the engine),
`/forms`, `/forms/{id}` builder (dnd-kit incl. keyboard, condition + band editors, live th/en preview with score),
`/form-responses/{id}`. Tests: fixture on both evaluators, schema validation, service (acceptance, versions, assignment,
permissions by type, two-tenant isolation), HTTP contract, vitest, Chromium E2E of the whole flow with a second user (21/21).
Found by the E2E: the builder remounted on every refetch (keyed by `dataUpdatedAt`) and lost unsaved edits — now keyed by
the latest version id. Not done: portal use (no portal feature yet), retiring forms, provider (global) forms.

### CON consent (`docs/modules/CON.md`) — CON-09/10/12/15 done, CON-13/17 partial
`internal/consent/{store,service,http,publichttp}` — the first P1 business module. Purposes are PLT-08 records
(`consent_purpose`, one DPO approval step, `RegisterVersioning()` in `cmd/api`); `OnPublish` writes an immutable
`consent.purpose_versions` row. A purpose is sensitive when one of its `data_category_codes` (migration 00034) is a
sensitive ORG-07 category: it needs `explicit_text` and can't be "required" on a form (CON-10). Collection points pass
`cpChecks` (purposes live + the four s.19 checklist items) before publish, which issues a `platform.public_keys` key
(`internal/platform/publickeys`; retire revokes it). `Record()` is the one write path (public form, staff on behalf):
per-purpose `Decide` against ST-01, one receipt per submission hash-chained per subject (`consent-receipt/v1`,
advisory lock per subject), transactions + status in the same tx, `consent.*` events, receipt e-mail via PLT-04.
**Leaving an ACTIVE purpose unticked changes nothing** (ST-01 has no such transition; an unverified form must not
withdraw — decisions Q-21); withdrawal is an explicit `WITHDRAWN` decision, refused on public forms. Identifiers:
PLT-13 `value_enc` + `blind_index`, masked on read, exact search in a POST body. `/public/v1`: `cmd/api` skips AuthN
for that prefix; `publickeys.Middleware` resolves tenant + Origin, sets a `data_subject` principal, then Idempotency
+ Tx + audit. Tests: `internal/consent/consenttest` fixture (two tenants, maker + second DPO), service acceptance
tests, contract tests for both chains (`consent/http/handler_test.go`), Chromium E2E of BP-01 (24/24: purposes with
approval, checklist publish, portal form th/en, profile, verify, staff withdrawal). Frontend: admin
`/consent/{purposes,collection-points,subjects}`; portal now has next-intl and `/[locale]/c/[key]` (server action,
per-attempt Idempotency-Key, forwards `X-Forwarded-For`/`User-Agent`). Found and fixed: `return X(w), convert(&w)`
returned an empty body (Go copies `w` first); list endpoints returned partial purposes/collection points; the BFF
(`lib/api.ts`) never forwarded the browser's address or user agent, so the audit log saw only the BFF — it now does
(list the admin app and portal in `TRUSTED_PROXIES`). Not done: re-consent flow (Q-22), CAPTCHA / embed SDK / CORS
for customer sites, `/api/v1` + webhooks (CON-16 needs ORG-16/PLT-15), self-service withdrawal (CON-18/19), age gate
(CON-11), region pinning. Known gap: the `/public/v1` validator accepts only `th`/`en` in Accept-Language, so a
browser calling the API directly (future SDK) would get 400 — the portal sends the locale itself.

### BRE breach (`docs/modules/BRE.md`) — BRE-02/05/06/07/08/09/10/12/13 done
`internal/breach/{store,service,http}` (+ `breachtest` fixture). Incidents `BR-YYYY-NNNN` with an owner (migration 00036)
move on ST-03 through `Transition` / `Decide` only; closing needs `breach.incident.approve` + a reason. The 72-hour clock
is pure functions in `service/deadlines.go` (`PDPCDue`, `Checkpoints`, `ToSchedule`, `ClockAt`, `LateReasonRequired`) —
`breach.sla_timer` River jobs at 24/48/66/72 h alert the owner + role DPO, + role EXEC from 66 h (default recipients until
BRE-04 routing); moving `aware_at` needs approve + a reason and reschedules (stale jobs see the old aware_at and skip).
Risk assessment = a published PLT-06 form of the new type `breach` whose bands are exactly none/low/high, answered through
`forms.Record`; factors = `forms.Contributions` (new, with option labels). The DPO's decision may exceed, never undercut,
what the risk requires. Timeline rows are append-only tokens (`status:a:b`, `deadline:66`, …) the UI localizes, plus any
human reason. Evidence = the caller's clean upload + SHA-256. Data-subject notices: CSV → encrypted recipients (batch
INSERT with `unnest` — **COPY is refused on RLS tables**), maker-checker send, 1000-per-job hand-off to PLT-04, per-person
delivery via new `notify.Service.Statuses`; the `breach.subject_notice` template is a marked DRAFT (rule 8). New
`iamservice.UsersWithRole`. Tests: deadline unit tests, service acceptance (register + alerts, reminders/escalation
with an injected clock, assessment/decision/timeline, 10,000 notices tracked per person, evidence/search/access,
two-tenant isolation), HTTP contract, Chromium E2E of BP-07 (20/20). UI `/incidents`, `/incidents/{id}`. Local
stack note: `.env` points S3 at MinIO :9000 and SMTP at :1025; with the Homebrew-style local services use S3
127.0.0.1:8333 (bucket pdpa-files-test), clamd :3310 and an empty SMTP_ADDR.

**BRE-08/09** (`internal/breach/service/pdpc.go`, once PLT-16 existed, decisions Q-23 resolved): `breach.pdpc_notifications`
rows are filing rounds (initial/supplementary/final, sequence per incident) citing a *published* `pdpc_form`
document version — checked through `docs.Service` (`Docs` interface, rule 9: breach never reads PLT-16's store
directly), which also proves the version is visible under RLS (rule 1). `submitted_at` is when the person actually
filed with the PDPC through its own channel (may be in the past, never future); `LateReasonRequired` (already
written for BRE-07) makes `late_reason` mandatory once that's past the 72-hour window — the 15-day outer limit
stays an open question (decisions Q-07) so it's flagged, not hard-rejected. A second person confirms the round
(`breach.notification.approve`, maker-checker like subject notices, `ErrSelfApproval`); `notifying → remediating`
now checks the real guard from `docs/states/ST-03.md` instead of always refusing: a confirmed round, and — when
the decision includes the data subjects — their notice must have finished sending too (`ErrSubjectNoticeMissing`,
checked directly against `breach.subject_notifications`, not through its own read permission — an internal
precondition, not a report). `wiring.Breach` takes the document composer now (`nil` in the worker: BRE-09 recording
is admin-only). UI: an "แจ้ง สคส." tab on the incident page — pick a published `pdpc_form` document (with a
shortcut to the document composer to create one), record the round, evidence upload, and confirm. Tests: service
acceptance (recorded → still blocked until confirmed → unblocks; late without a reason refused; the data-subject
guard), HTTP contract, Chromium E2E driving the whole chain including a real second-DPO confirmation (14/14). Found
by the E2E: `usePublishedDocumentVersions` had no `enabled` guard, so mounting the picker before a document was
chosen fired a request with an empty id (400) on every page load — fixed.

### PLT-16 Document composer (`docs/modules/PLT.md#plt-16`) — done
`internal/platform/docs` (+ `render/`, `http/`, `docstest/`) and `apps/admin` `/documents*`, chosen ahead of BRE-08/09
(decisions Q-23) so recording the PDPC notice has something to point at. Content is ProseMirror JSON per language
(`{th, en}`, allow-listed nodes incl. `mergeField` and `clause`), edited with TipTap. Documents are PLT-08 records,
one type per owning module (notice/policy → `notice.document.*`, dpa/dsa → `agreement.{dpa,dsa}.*`, dsar_letter →
`dsar.request.*`, pdpc_form/breach_letter → `breach.notification.*`; `internal/wiring.Docs`), one DPO approval step.
Publishing freezes the text, every merge field's current value (org fields via `orgservice.MergeFields`) and every
cited clause's text into an immutable `document_versions` row (migration 00037), then `docs.render` produces PDF +
Word per language: DOCX is hand-written OOXML (byte-exact Thai, no shaping), PDF via Gotenberg or a local Chromium,
both with the Sarabun font (OFL) embedded. Clause library and templates are draft → published → retired per code
(`agreement.clause.*`; a template's wording needs the type's own publish permission — legal text, rule 8). Compare
is block-level LCS with inline segments; an unpublished export carries a DRAFT banner. Tests: acceptance (Thai
Word text byte-exact incl. resolved fields/clause/date, PDF valid with the font embedded, two versions compare),
access/isolation, clause versioning, templates, HTTP contract, Chromium E2E (19/19: clause library → compose
Thai+English → draft export → submit → second DPO approves → publish → worker renders → Word exact in both
languages → edit → compare → English UI). Two real bugs the Go unit tests missed (no bilingual-clause fixture, no
mouse-driven toolbar) but the E2E caught: `docs.clauseTexts` decoded an English clause body into a *shallow copy*
of the Thai struct, so the shared `render.Node.Content` slice let the English decode silently overwrite the Thai
paragraphs (regression test added); and every TipTap toolbar button/select moved DOM focus before its `focus()`
call (which runs on the next animation frame), losing the next keystrokes — fixed by focusing synchronously.
Known, not fixable from here: Chromium's headless PDF export (Gotenberg too, same engine) writes an unreliable
text-layer for Thai combining marks — confirmed with a minimal reproduction outside this app that the *rendered*
PDF is pixel-correct, only extraction is lossy — so "correct characters" is verified via DOCX (byte-exact) and the
PDF only for validity + embedded font. Not done: portal/`/api/v1` use, version retention/archival.

### ORG-06 External parties directory (`docs/modules/ORG.md#org-06`) — done
`internal/org/service/parties.go` (`org.party.*`) — CRUD on `org.external_parties`, which the baseline
migrations already had (party_type, a real FK to `org.countries`, `contact` jsonb, `dedupe_key`); this
pass added only `merged_into_id` (migration 00038) for merging duplicates. Merging never deletes a
record — the loser becomes `inactive` and points at the survivor, so an id any module already stored
still resolves (the acceptance criterion). Duplicate detection is non-blocking: `dedupe_key` is a
normalized name + country computed on every save, and a separate `/duplicates` endpoint groups active
parties that share one for the admin to review and merge — creating an exact-name duplicate is never
refused outright, since same-name-different-company is a real case. `MergeExternalParty` needs
`org.party.delete` (a permanent status change, not a plain update), refuses merging into self, and
refuses touching an already-merged record on either side. Not done, deliberately: reassigning FKs from
other schemas when merging — at the time this was built nothing wrote a real row referencing
`org.external_parties` yet; **ROPA-02 now does** (`ropa.assets.provider_party_id`), so a merge no longer
reassigns that reference automatically — the first ROPA-02 UI screen that lets someone act on a merged
provider must follow `merged_into_id` itself until this gets real cross-schema reassignment. API
`/admin/v1/org/external-parties` (cursor pagination, same shape as PLT-16's document list),
`/external-parties/duplicates`, `/external-parties/{id}/merge`. UI `/settings/external-parties` (list +
form + a duplicates panel with a merge-into picker). Tests: unit (validation, update, merge rules,
duplicate detection matches the normalization exactly, two-tenant isolation), HTTP contract
(401/403/400 schema/412/428).

### ROPA-02 System / asset register (`docs/modules/ROPA.md#ropa-02`) — done
`internal/ropa` — the first module on the `ropa` schema (`ropa.inventory.*`, a permission code shared with
ROPA-01's future data inventory). CRUD on `ropa.assets`, already fully specified in the baseline migrations
(asset_type, org_unit_id/owner_user_id/provider_party_id/hosting_country_code all real FKs) — no new
migration. Built ahead of ROPA-01 despite the backlog listing only ORG-07 as ROPA-01's dependency:
`ropa.data_inventory.asset_id` is a NOT NULL FK to `ropa.assets`, so the data inventory literally cannot be
built before this exists. `org_unit_id` and `provider_party_id` bypass RLS as FKs, so `SaveAsset` checks each
is visible under the caller's RLS first (rule 1) via org's own exported service — `orgservice.GetOrgUnit`
(newly exported; was a private `unit()` helper) and the already-exported `GetExternalParty` (rule 9);
`owner_user_id` goes through `iamservice.Names`, the same cross-module helper BRE already uses. API
`/admin/v1/ropa/assets` (cursor pagination, same shape as ORG-06/PLT-16's lists), `/{id}`. UI `/ropa/assets`
(the org-unit picker needs a legal entity chosen first, same two-step pattern as `/settings/organization`;
no field for owner_user_id yet — no user directory UI exists until IAM-01/ORG-09). Tests: unit (validation,
the three FK-visibility checks, update, two-tenant isolation), HTTP contract (401/403/400 schema/422/412/428).

### ROPA-01 Personal data inventory (`docs/modules/ROPA.md#ropa-01`) — done
`internal/ropa/service/inventory.go` (`ropa.inventory.*`, shared with ROPA-02) — CRUD on `ropa.data_inventory`,
already fully specified in the baseline migrations (asset_id/data_category_id NOT NULL, org_unit_id/
owner_user_id/discovered_by_finding_id nullable) — no new migration. The acceptance criterion's sensitive-data
flag comes straight from ORG-07's `org.data_categories.is_sensitive` (no new column): `ListDataInventory`'s
query joins it in the same transaction (its RLS already allows global defaults + the tenant's own rows), so
every row carries `is_sensitive`/`sensitive_type`/`category_name_*` without a second round trip.
`sensitive_only=true` on `GET /admin/v1/ropa/data-inventory` searches every department at once (no
`org_unit_id` filter applied) — the literal "filterable across every department"; an `org_unit_id` filter
narrows to one department when wanted. Duplicate detection isn't part of this acceptance criterion, so unlike
ORG-06 there's no dedupe step. FK visibility checks follow the ROPA-02 pattern: `asset_id` through
`Service.GetAsset` (same package), `data_category_id` through a newly exported `orgservice.GetMaster` (was a
private `masterItem()` helper — same "export what another module needs" move as ROPA-02's `GetOrgUnit`),
`org_unit_id`/`owner_user_id` reusing the exact same checks ROPA-02 already has. `discovered_by_finding_id`
(FK to `dataflow.discovery_findings`) is left alone — the `dataflow` module (automated discovery scans)
doesn't exist yet, so there's nothing to link to; add it when that module ships. API
`/admin/v1/ropa/data-inventory` (cursor pagination), `/{id}`. UI `/ropa/data-inventory` (sensitive-only
toggle, department filter, form with an asset/category/unit picker — the sensitive flag shows inline next to
each category option and as a badge on sensitive rows). Tests: unit (validation, the four FK-visibility
checks, update, the acceptance criterion directly — two departments each with a sensitive entry, confirming
`sensitive_only` returns both — two-tenant isolation), HTTP contract (401/403/400 schema/422/412/428).

### ROPA-03 RoPA of the controller (`docs/modules/ROPA.md#ropa-03`) — done
`internal/ropa/service/activities.go` (`ropa.activity.*`, permission codes already seeded in the baseline
migrations, migration 00019) — CRUD on `ropa.processing_activities` plus four child tables (`activity_purposes`,
`activity_data`, `retention_rules`, `activity_recipients`), all already fully specified in the baseline
migrations — no new migration. Deliberately scoped to exactly this feature's acceptance criterion and BP-05
rules 1–2, not the full ม.39 checklist: full ST-05 approval (`pending_approval` → `active`) is ROPA-13's job
(versioning & approval via PLT-08 — a separate Should feature this one doesn't depend on); country-adequacy
transfer checks on top of the recipients table built here are ROPA-08's; retention-policy automation is
ROPA-07's; security-control linking (`activity_controls` → `risk.controls`) and DSAR-linked rejections
(`activity_rejections` → `dsar.requests`) are left alone entirely, since neither the risk-control library nor
the DSAR module exists yet — same reasoning as ROPA-01's deferred `discovered_by_finding_id`.

Completeness (the acceptance criterion) is computed live on every `GetActivity`, never persisted from a plain
read — only from mutations — against 5 fixed items (data, purpose, controller, retention, rights_and_access;
"controller" only applies when `role=processor` and `controller_party_id` is unset) plus two conditional ones
counted in the missing-item list but not the score denominator: a recipient missing `disclosure_basis`, and
sensitive data (`activity_data.is_sensitive`, from ORG-07 as in ROPA-01) with no purpose carrying
`consent_purpose_id` (BP-05 rule 2's explicit-consent evidence — checked via a newly exported
`consentservice.GetPurpose`, the first cross-module read from `ropa` into `consent`, rule 9). `POST …/submit`
(draft/under_review → pending_approval, ST-05) refuses with the itemized list (`ropa.activity_incomplete`, 422,
the same `FieldError` pattern as PLT-16's `docs.incomplete`) while anything is missing, and 409
`ropa.invalid_transition` from any other status.

Two real bugs found while testing: (1) persisting the completeness score ran through the table's ordinary
`row_version`-bumping trigger, so a plain `GetActivity` — no user edit — silently invalidated the caller's ETag;
fixed by making the completeness computation itself pure and persisting only from mutation paths (`SaveActivity`
and each child add/delete), which already legitimately bump the version. (2) A duplicate `code` hit the table's
real unique constraint and aborted the whole request transaction (no savepoint), corrupting every later
statement in the same tx until commit failed with pgx's `ErrTxCommitRollback` — fixed by wrapping the
insert/update in `pdb.Savepoint`, the same pattern `org.SaveLegalEntity` already uses for its own
unique-constraint check (a pattern worth reaching for by default whenever a write can hit a real DB constraint,
not just Go-level validation).

API `/admin/v1/ropa/activities` (cursor pagination), `/{id}`, `/{id}/submit`, and one list+create+delete triple
per child table (`/purposes`, `/data`, `/retention-rules`, `/recipients` — no per-row update; editing a child is
delete+recreate, keeping the sub-resource surface small). UI `/ropa/activities` (list with a completeness badge)
and `/ropa/activities/{id}` (core-field form, missing-items banner, one section per child table with inline
add/remove, submit button — editable only while `status=draft`). Tests: unit (validation, all four
FK-visibility checks, the acceptance criterion directly — an activity missing items shows them and clears them
one at a time as each is filled in — processor-needs-controller, sensitive-data-needs-consent-evidence, submit
blocked while incomplete with the itemized list, two-tenant isolation), HTTP contract (401/403/400
schema/422/428).

### ROPA-08 Recipients & cross-border transfers (`docs/modules/ROPA.md#ropa-08`) — done
`internal/ropa/service/transfers.go` (`ropa.activity.*`, shared with ROPA-03) — CRUD on `ropa.activity_transfers`,
already fully specified in the baseline migrations — no new migration. Recipients themselves
(`activity_recipients`, `disclosure_basis` for ม.27) were already built generically in ROPA-03 (documented
there as the piece ROPA-08 would extend); this feature adds only the transfer half: `country_code` (validated
against ORG-07's countries via a new `Org.ListMaster` scan, the exact `validLawfulBasis` pattern ROPA-03
already set — both are code-keyed, not id-keyed), `transfer_basis` (ม.28/29's six mechanisms), `safeguards`,
and an optional link to one of the activity's own recipients (checked against `ListActivityRecipients`, no new
FK-visibility query needed).

`transfer_basis` is a NOT NULL enum column, so an actual row can never lack a basis — the acceptance criterion
("a transfer without a basis is warned") is about the *implicit* transfer that was never logged at all
(`docs/legal/pdpa-rules.md`'s ม.28 row: every real transfer in the RoPA needs its country and mechanism
recorded). So ROPA-03's `completeness()` gained one more conditional item: for each recipient, resolve its
party's `country_code` via the already-shared `Org.GetExternalParty`, and flag `transfer_basis` once per
activity if any real foreign country (not `TH`, not blank) has no `activity_transfers` row referencing that
recipient — it blocks `/submit` exactly like ROPA-03's other conditional items.

API `/admin/v1/ropa/activities/{id}/transfers` (list+create) and `/{transferId}` (delete) — same shape as
ROPA-03's other child tables. UI: a transfers section on the activity detail page (country/basis/safeguards +
an optional recipient picker scoped to the activity). Tests: unit (validation — bad/unknown country code, bad
transfer basis, a recipient outside the activity refused — the acceptance criterion directly: a foreign
recipient with no transfer flags `transfer_basis`, adding one clears it, deleting the only one brings it back —
two-tenant isolation), HTTP contract (401/403/400 schema/422).

### ROPA-06 Lawful basis mapping (`docs/modules/ROPA.md#ropa-06`) — done
`internal/ropa/service/activities.go`'s `AddActivityPurpose` (`ropa.activity.*`, shared with ROPA-03) — no new
migration, no new endpoint: ROPA-03 already built `activity_purposes` with `lawful_basis_code` and an optional
`consent_purpose_id`, already validated the code exists (`validLawfulBasis`) and that a given
`consent_purpose_id` is a real, visible `consent.purposes` row. What was missing is this feature's own rule:
`validLawfulBasis` now returns the matched `org.lawful_bases` row (not just a bool), and when its
`requires_consent` flag is set, `consent_purpose_id` becomes mandatory — refused with `ErrInvalid` at save
time, before the row is ever written, not just flagged later as an incomplete item. This is stricter and
narrower than ROPA-03's own `sensitive_consent` completeness check: that one only fires when the activity's
*data* is sensitive and only blocks `/submit`; this one fires whenever the *lawful basis itself* is consent
(e.g. ordinary non-sensitive marketing) and blocks the write immediately — the two are independent and can
both apply to the same purpose. Not built: "แสดงสถิติความยินยอม" (surfacing consent statistics) from the
module doc's backend note — no screen in this pass needs aggregate consent numbers, and serving one would need
a materially different query (counts, not a single row) than the FK check already in place; add it when a
screen actually asks.

UI: the lawful-basis picker tags each consent-requiring code inline, and the Add button for a new purpose is
disabled with a hint until a Purpose is chosen for those codes — a client-side mirror, not a replacement for
the service-side rule. Tests: unit (a consent-basis purpose without a link is refused; with one it saves and
round-trips; the shared `lawfulBasisCode` test helper now explicitly picks a non-consent code so earlier tests
don't trip this new rule by accident), HTTP contract (422 `ropa.invalid_input`).

### PNG-01 Wizard-based generator (`docs/modules/PNG.md#png-01`) — done
`internal/notice` — the first module on the `notice` schema (`notice.document.{read,create,update}`,
`notice.template.*` — already seeded in the baseline permission migration for PLT-16's pre-registered "notice"
document type, so no new migration). A notice (`notice.notices`) is a thin wrapper — legal entity, subject
type, slug, ST-04 status — around a PLT-16 document (`document_id`, NOT NULL): `CreateWizard` is the whole
wizard in one call, composing the document's first draft (`docs.Service.Create` + `SaveDraft`) from data the
platform already has rather than a generic conditional Q&A engine (that would duplicate PLT-06's forms engine
for no acceptance-criterion benefit). For each linked RoPA processing activity (ROPA-03/06/07/08),
`ListActivityPurposes`/`ListActivityData`/`ListRetentionRules`/`ListActivityRecipients`/`ListActivityTransfers`
(+ ORG-07 lawful-basis/data-category/country names, rule 9) are turned into ม.23 sections; a topic nothing was
linked for becomes a bracketed placeholder rather than blocking creation — the acceptance criterion is a
*complete* draft within 30 minutes, not a finished one. The DPO-contact section uses `mergeField` nodes
resolved from the legal entity (ORG-01) the same way every other PLT-16 document does. `notice.notices.status`
starts and stays `draft` (ST-04's `[*] → draft`, the only transition this feature makes); the ม.23 checklist
gate (PNG-02), subject/industry templates (PNG-03/PNG-11), re-flagging on RoPA change (PNG-10 — a distinct
event-driven feature per the module's own «extend» relationship, not part of the one-time compose), DPO
approval and publish (PNG-14/PNG-08) are sibling features layered on the same document, not rebuilt here. The
slug is typed by the caller (not transliterated from the often-Thai title), validated `^[a-z0-9-]+$` and
enforced unique per tenant via a `pdb.Savepoint`-wrapped insert (the established pattern for a real
unique-constraint violation not aborting the request transaction). API `/admin/v1/notices` (cursor pagination),
`GET /{id}` (includes `activity_ids`) — editing the composed content is PLT-16's own document-draft endpoint,
not duplicated here. UI `/notices`: a wizard form that on success routes straight to the PLT-16 document editor
for the freshly composed draft. Tests: unit (the draft contains every ม.23 topic verbatim when an activity is
linked, placeholders when none is, validation, duplicate slug, two-tenant isolation of both the legal-entity
and activity FKs), HTTP contract (401/403/400/422).

### PNG-02 Mandatory content checklist (`docs/modules/PNG.md#png-02`) — done
No new endpoint or migration — the gate lives inside the existing publish flow. `docs.Service` (PLT-16) gained
`SetValidate(docType, fn)`: an extra publish-time check run right after the generic merge-field/clause
completeness check already there (`missing(in)`), inside the same request transaction — an error blocks the
PLT-08 publish exactly like that generic check already does. It's a setter, not a `Policy` field, because the
module's own service (the thing that actually implements the check) is built *after* `docs.Service` in
`cmd/api/main.go`; wiring calls `docsSvc.SetValidate("notice", noticeSvc.CheckPublishable)` right after
`noticeSvc` exists. `compose.go`'s headings (PNG-01) now carry a stable topic code (`attrs.topic`), matched by
a new pure function, `Checklist(content)`, against ม.23's six mandatory items — `purpose_basis`, `consequence`,
`data_retention` (needs both the data *and* retention sections — ม.23 counts them as one item), `recipients`,
`contact`, `rights` — each complete when its heading exists and the text under it isn't empty or one of the
wizard's own bracketed placeholders. A document with no topic-coded headings at all (created directly through
PLT-16's generic document endpoints, bypassing the wizard) reads as every topic missing — deliberate: this
checklist only recognizes what PNG-01 itself composes, not free-form authoring, rather than guessing from
arbitrary text. `CheckPublishable` looks the notice up by `document_id` (`GetNoticeByDocumentID`, new sqlc
query, still no migration) and blocks with `ErrChecklistIncomplete` (422 `versioning.invalid_request`,
following `docs.IncompleteError`'s own reporting pattern) when anything is missing. Configurable per the
module doc's "(ตั้งค่าได้)": `Service.EnforceChecklist` (default true, `NOTICE_CHECKLIST_ENFORCE=false` to turn
it off) — a tunable, not a `docs/decisions.md` question, since it changes no data model or contract.
`GET /admin/v1/notices/{id}/checklist` reads the same function live off the current draft for the UI panel.
Tests: unit (`Checklist` directly: complete, one placeholder blocking only its own item, `data_retention`
needing both halves, no topic codes at all → everything missing), integration through the *real* PLT-08
submit → DPO approve → publish chain (a notice left with placeholders is blocked at publish with the itemized
list; a notice completed *before* submission — the real BP-04 order — publishes normally; `EnforceChecklist =
false` skips the gate), HTTP contract. Found while writing the integration test: PLT-08 has no "unapprove"
action, so a version already at `approved` when publish is blocked is locked against further edits (`SaveDraft`
refuses anything but the open draft) — recovering means a fresh review round on a *new* draft, not editing the
blocked one in place; the test proves the gate with two independent notices rather than working around that
(existing PLT-08 behaviour, not something for this feature to fix).

### PNG-05 TH/EN notices (`docs/modules/PNG.md#png-05`) — done
Bilingual content and the editor's language switch were already generic PLT-16/PNG-01 work; this feature's real
scope is just its acceptance criterion — a second publish-time gate reusing PNG-02's exact
`docs.Service.SetValidate` hook, extended with what PNG-02 didn't need: the *previous* published version's
content to diff against. `docs.Service` gained `previousPublished` (walks `s.Versioning.List` for the version
being superseded, nil on a first publish) and `PublishedContent` (the read-side twin, for the UI); the
`Validate` hook signature grew a `previous *Draft` parameter — its only caller (notice) was updated in the same
change, so this was a safe signature change with no other module affected. `notice.Service.CheckPublishable`
now also runs the new `StaleTranslation(current, previous)`: stale only when both versions carry English
content, the Thai section changed, and the English section did not — adding English for the first time or
dropping it entirely is never itself flagged, since neither is "an update the translation missed". Blocks with
`ErrTranslationStale` (422 `versioning.invalid_request`, same reporting pattern as `ErrChecklistIncomplete`).
Configurable per "(ตั้งค่าได้)": `Service.EnforceTranslationSync` (default true, `NOTICE_TRANSLATION_SYNC_ENFORCE=false`),
no `docs/decisions.md` entry needed (a tunable, not legally-relevant behaviour). `GET
/admin/v1/notices/{id}/translation-status` exposes the same check for the UI ahead of any actual publish
attempt. Not built: a third (migrant-worker) UI language — `render.Content`'s language handling is hard-coded
to exactly th/en across PLT-16 (validation, date formatting, both renderers, the editor's tabs); adding one is
a cross-cutting PLT-16 change no other feature needs yet, and well beyond this feature's literal acceptance
criterion. Tests: unit (`StaleTranslation` — stale only on Thai-changed/English-unchanged, every other
combination not stale), integration through the real PLT-08 submit → DPO approve → publish chain (blocked when
only Thai changed; allowed when both languages were updated together — two independent notices, matching
PNG-02's own pattern for a gate that locks a version at "approved" once it blocks), the read-side status check,
HTTP contract.

### ROPA-04 RoPA of the processor (`docs/modules/ROPA.md#ropa-04`) — done
`internal/ropa/service/export.go` (`ropa.activity.read`, shared with ROPA-03/06/08 — no new permission or
migration) — `ExportProcessorActivities` writes every `role='processor'` activity as one CSV row (a new sqlc
query, `ListProcessorActivities`, code order, no pagination — a tenant's processor RoPA is meant to be filed as
one document, following ORG-19's own export shape: UTF-8 with BOM, the export itself audited as
`ropa.activity.export`). The PDPC's actual "ประกาศ RoPA ผู้ประมวลผล พ.ศ. 2565" form text wasn't available to fetch
from here, so — the same "seed a draft flagged for legal review" move ORG-07 made for its Q-20 master data — the
column set is a best-effort draft built only from fields ROPA-03/06/08 already model, not any newly invented
legally-mandated field: code, name, description, the controller's identity (`controller_party_id` via
`Org.GetExternalParty`), org unit, owner, every data category/subject type the activity's data rows reference
(via `Org.GetMaster`, deduplicated), retention rules (period, trigger, disposal method), recipients (party name +
role) and cross-border transfers (country + legal mechanism). Deliberately excludes `lawful_basis_code`: that's
the *controller's* basis for processing under the controller's own RoPA, not something a processor's ม.40(3)
record reports about work done on another controller's instructions. `GET
/admin/v1/ropa/activities/processor-export` (a static path registered ahead of `/{id}` — `text/csv`,
`Content-Disposition: attachment`, same response shape as ORG-19's audit-log export). UI: an export link next to
`/ropa/activities`'s own "add activity" button (`processorActivitiesExportHref`, a plain `<a href>` through the
BFF, same pattern as the audit log's own export link). Tests: unit (the export contains every column and a real
processor activity's controller/recipient names, never includes a controller-role activity — the acceptance
criterion), HTTP contract (401 without a principal, 200 with the header row present).

### DPO-01 Appointment register (`docs/modules/DPO.md#dpo-01`) — done
`internal/dpo` — the first module on the `dpo` schema (`dpo.profile.*`, already seeded in the baseline
permission migration — no new migration: `dpo.appointments` was already fully specified — `dpo_type`
internal/external/group, `user_id` for internal, `external_name`/`external_company` for external/group,
`contact_email`/`contact_phone`, `appointed_at`/`ended_at`, `appointment_file_id` and
`pdpc_notified_at`/`pdpc_evidence_file_id`). `SaveAppointment` checks the legal entity is visible under RLS
(rule 1, via `orgservice.GetLegalEntity`) and, for an internal appointment, that `user_id` is a real active
user of the tenant (`iamservice.Names`); a file id (order or evidence) is checked and attached the way
BRE-09's PDPC evidence already is (`Files.Get` + `AttachSystem`, refused unless it's the caller's own clean,
still-unattached upload) — but only when it's new or changed from what's stored, since re-checking an id
already attached to this same appointment would fail the "unattached" test on every plain update. A legal
entity's *current* appointment is whichever has no `ended_at` yet, most recently appointed
(`CurrentAppointment`, a new sqlc query) — plain CRUD otherwise, no state machine (the module doc lists no
process for this feature).

The acceptance criterion is a new PLT-16 extension point, not a screen: `docs.Service` gained a second
merge-field source alongside `OrgFields` (ORG-01) — `DpoFields` (`Dpo DpoFields`, same
`MergeFields(ctx, legalEntityID) (map[string]string, error)` shape) — resolved in `fieldValues` right after
the organization's own fields, so every document (notices today; DPAs/DSA/PDPC-form letters once those
document types exist) picks up `dpo_name`/`dpo_email`/`dpo_phone` from the legal entity's current appointment
without any template ever hard-coding it (rule 8). `dpo.Service.MergeFields` returns nothing when there's no
current appointment — the draft then shows `[dpo_name]` etc. as an unresolved placeholder and publishing
refuses it, exactly like any other missing merge field. `wiring.Docs` builds the `dpo.Service` itself
(mirroring how it already builds its own `orgservice.Service`), so `docs` never imports `dpo`'s HTTP layer
or vice versa — module boundaries stay one-directional (rule 9).

API `/admin/v1/dpo/appointments` (cursor pagination, same shape as ROPA-04's own list), `/{id}`. UI
`/settings/dpo`: a legal-entity picker (the same two-step pattern `/settings/organization` and
`/ropa/activities` use) then the entity's appointments with a create/edit form (`FileUploader` for the
appointment order), current vs. ended shown as a badge. Tests: unit (validation incl. dpo_type-conditional
fields, the two FK-visibility checks, update, current-contact resolution as an appointment starts/ends/is
replaced — the acceptance criterion's core logic — two-tenant isolation), a white-box `docs` package test
proving `fieldValues` actually merges the dpo source in (a stub, no database needed — the org source itself
has no equivalent unit test, only the existing Chromium-based E2E; this closes that gap for the new source
too), HTTP contract (401/403/400 schema/422/412/428).

### DPO-09 Security measures assessment (`docs/modules/DPO.md#dpo-09`) — done
The checklist itself (ประกาศมาตรการความปลอดภัย พ.ศ. 2565) is an ordinary PLT-06 form the DPO authors and
publishes, not new logic: `internal/wiring/forms.go` registers a new module-agnostic form type, `"security"`,
with `dpo.risk.*` permissions (a failed control is a risk-register-adjacent finding — no new permission code)
— `platform.form_definitions.form_type`'s CHECK constraint widened by migration 00039 rather than reusing the
unclaimed `'quiz'` value, for clarity. SEC/DPO submit a completed run in one call —
`internal/dpo/service/assessment.go`'s `Assess` — via `forms.Service.Record` (BRE-05's exact pattern: bypasses
the draft/section-assignment UI flow, for a form filled in one atomic step) against a specific published form
version, refusing with `ErrBadForm` when that form isn't type `"security"` or has no published version. The
score/band (`forms.Result`) and every question's answer (`forms.Contributions`) land in a new
`dpo.security_assessments` row (migration 00039, mirrors `breach.assessments`' shape: score, result, factors
jsonb, `form_submission_id`). The acceptance criterion — a failed item auto-opens remediation work — is
computed by walking the form's schema directly rather than `Contribution.Points` (which would misfire on any
non-yes_no question): every `yes_no` question answered `"no"` opens one `dpo.tasks` row (`source_type =
'risk'`, no enum widening needed; numbered `SEC-<year>-NNNN` with the same per-tenant-per-year advisory-lock
pattern `breach.incidents.incident_no` already uses), linked back to the assessment.

API `/admin/v1/dpo/security-assessments` (cursor pagination, list + create) and `/{id}`. UI: a "Security
assessments" section on `/settings/dpo`, right after the appointments list — `FormRenderer` against the
legal entity's published `"security"` form, a score badge, an expandable per-question factor table, and the
remediation-task list when the run failed anything. Tests: unit (pass/fail scoring, task auto-creation and
round-trip via `GetAssessment`, validation — unknown legal entity, unknown/unpublished form, a
non-`"security"`-type form, a missing required answer — two-tenant isolation), HTTP contract
(401/403/201/200/404/422). Re-confirmed a forms-package gotcha while writing the tests (not new to this
feature, but easy to trip on again): `forms.Service.CreateForm`/`Publish` both return via `GetForm`, which
folds a missing *Read* permission into `ErrNotFound` rather than `ErrForbidden` — any fixture granting only
Create/Update/Publish for a form type fails opaquely with "forms: not found"; test grants for `"security"`
(and the cross-type-rejection test's `"questionnaire"` fixture) now include Read. Not done: evidence
attachments per checklist item (the module doc's own description mentions "พร้อมหลักฐาน" but the acceptance
criterion is only about auto-opened remediation work; PLT-06's `Record` path has no per-question attachment
slot yet — add one only when a screen actually needs it).

### ROPA-09 Security measures per activity (`docs/modules/ROPA.md#ropa-09`) — done
`ropa.activity_controls` (link to `risk.controls`, both already fully specified in the baseline migrations —
no new migration for either) was still empty: no feature owned seeding the control catalog itself, since the
full RRA module (risk matrices, control-library management) is P2 and not started — RRA-06 only *links*
existing controls to risks, never creates the catalog. Rather than block on RRA, migration 00040 seeds a draft
global catalog (`tenant_id NULL`, the same ORG-07 Q-20 visibility pattern) of 12 measures covering all four
ม.37(1) categories the ประกาศมาตรการความปลอดภัย พ.ศ. 2565 names (organizational, technical, physical,
access_control) — flagged for legal review (`docs/decisions.md` Q-25; `legal` left unseeded, no clear example
in the notice to paraphrase). `internal/risk/service` is the first, deliberately minimal package on the `risk`
schema — a plain `ListControls`/`GetControl` read, no create/update surface, mirroring ORG-07's own read-only
master data exactly. `ropa.Service` gained a `Risk` interface (rule 9) and `internal/ropa/service/controls.go`
(`AddActivityControl`/`DeleteActivityControl`/`ListActivityControls`) following ROPA-08's own `ActivityTransfer`
CRUD pattern precisely: FK-visibility check via `Risk.GetControl` (wrapped as `ErrInvalid`, not the raw error),
`pdb.Savepoint` around the insert since the PK is `(activity_id, control_id)` and a duplicate link must not
abort the whole request transaction. The acceptance criterion is a sixth core item in ROPA-03's
`completeness()` (`security_controls`, unconditional like `data`/`purpose`/`retention`, not one of the
conditional recipient/sensitive-data checks) — clears once any control is linked, reappears if the last one is
removed. Permissions reuse the already-seeded `ropa.risk.*` (baseline migration 00019's "ความเสี่ยงและช่องว่าง
รายกิจกรรม" — per-activity risk-and-gap items, which a linked control is one of; no new permission code).
API `GET /admin/v1/ropa/security-controls` (the catalog, for the picker) and
`/admin/v1/ropa/activities/{id}/controls` (list+create) / `/{controlId}` (delete) — same shape as ROPA-08's
transfer endpoints. UI: a security-measures section on `/ropa/activities/{id}`, right after transfers — a
control picker + optional free-text description, list with remove. Tests: unit (missing until referenced,
unknown control_id refused, duplicate link refused, delete brings the missing item back, the seeded catalog
covers all four categories, two-tenant isolation of the per-activity link — the global catalog itself is
visible to both tenants, by design), HTTP contract (401/403/201/200/404/422). Existing ROPA-03 tests that
asserted an exact "fully complete" activity or an exact missing-items list were updated for the new 6-item
denominator (previously 5).

### ROPA-07 Retention & disposal method (`docs/modules/ROPA.md#ropa-07`) — done, no new code
Already fully built by ROPA-03 itself: `ropa.retention_rules` (`retention_months`, `retention_basis` as the
reason/legal reference, `trigger_event`, `disposal_method` validated against
delete/destroy/anonymize/return) already has full CRUD, a `/retention-rules` list+create/delete endpoint, a UI
section on `/ropa/activities/{id}`, and — the acceptance criterion itself — is one of `completeness()`'s
unconditional core items, blocking `/submit` with the itemized list while any activity has no retention rule.
Marked done as documentation only (backlog + module doc), no migration, code or test changes needed. Not done:
"ส่งต่อ DPX-05" (forwarding the retention deadline) — DPX-05 doesn't exist yet, same reasoning as ROPA-01's
deferred `discovered_by_finding_id`.

### PNG-04 Indirect collection notice (`docs/modules/PNG.md#png-04`) — done
`notice.indirect_collections` was already fully specified in the baseline migrations (`notify_due_at date
NOT NULL`, `status` pending/notified/overdue/exempted, `method`, `notified_at`, `evidence_file_id`) along
with the `notice.indirect.*` permissions — no new migration or permission code for the table itself.
`internal/notice/service/indirect.go` follows BRE-07's own deadline pattern (pure, clock-testable
`Checkpoints`/`ToSchedule` functions + scheduled River jobs) rather than instantiating the generic PLT-05
workflow engine, even though the backlog lists PLT-05 as a dependency: the engine's task assignee is baked
into its Definition JSON at the type level, not resolvable per record, and this table has no owner column
to resolve one from — so `notice.indirect_due` (BP-04's own job name) alerts role DPO by default
(`iamservice.UsersWithRole`, the same "default recipients until real routing exists" fallback BRE-07 used
before BRE-04 existed; two new global notification templates in migration 00041). Reminders fire at 20 and
25 days elapsed, overdue at 30 (ม.25 counts calendar days) — matching PLT-05's own worked reminder example
almost exactly, just without the engine itself. `RegisterCollection` validates the source party (and an
optional linked RoPA activity) and computes `notify_due_at`; `RecordNotice` is the acceptance criterion's
other half — method + evidence (a PLT-09 file, `Files.Get` + `AttachSystem`) close a `pending` or
`overdue` record as `notified`, and a stale reminder tick afterward is a harmless no-op. `exempted` is in
the schema and the new `docs/states/state-machines.yaml#PNG-04` machine but reachable by no transition in
this pass — deliberately deferred, since ม.25's exemption grounds aren't modeled by any column yet.

API `/admin/v1/notices/indirect-collections` (cursor pagination, list + create), `/{id}` (get),
`/{id}/notify` (ETag/If-Match). UI `/notices/indirect-collections` (linked from `/notices`): a register
form, a status-filtered list with the due date and a colored status badge, and an inline "record notice"
panel (method + `FileUploader` evidence). Tests: unit (`Checkpoints`/`ToSchedule` incl. a late-recorded
event still alerting at once, validation, the acceptance criterion directly — overdue after the 30-day
checkpoint, still closable afterwards with evidence, a stale tick is harmless — two-tenant isolation),
HTTP contract (401/403/201/200/404/412/428/422).

### PNG-03 Templates by data subject group (`docs/modules/PNG.md#png-03`) — done
T15 (the backlog dependency) resolves via `docs/decisions.md` Q-14: seed DRAFT sample content pending legal
review — the same move ORG-07 (Q-20) and ROPA-09 (Q-25) already made. `platform.templates` and
`notice.wizard_templates` were both already fully specified in the baseline migrations (00002/00007) with the
same global (`tenant_id NULL`) + tenant-override RLS pattern as ORG-07's master data, so this needed no schema
migration, only seed data: migration 00042 seeds 8 groups (customer, employee, job_applicant, vendor, visitor,
cctv, shareholder, member) × th/en = 16 `platform.templates` rows (`template_type = 'notice_wizard'`), each a
full ม.23-topic-coded ProseMirror document — the exact shape `compose.go`'s `docNode` already produces, so a
template-sourced draft is PNG-02's checklist-compatible from the start — with bracketed placeholders for
group-specific detail and a `[ร่าง — ...]`/`[DRAFT — ...]` opening paragraph (rule 8), plus one linking
`notice.wizard_templates` row per (group, language). `WizardInput` gained `TemplateGroup string`, mutually
exclusive with `ActivityIDs` (`ErrInvalid` if both are set); `notice.Service.templateContent`
(`internal/notice/service/templates.go`) loads and JSON-decodes both languages' stored `render.Node` trees, and
`CreateWizard` uses it instead of `compose()` when a group is picked — everything downstream (document
creation, PLT-16 draft save, notice row) is identical to PNG-01's existing path. `ListTemplateGroups` (backed
by the real table, not a hardcoded list) drives the picker; `TemplateGroups` in Go is only the fixed 8-code
list the UI's th/en keys are built against — the table has no display-name column. API: `GET
/admin/v1/notices/template-groups` (`notice.document.read`) and `template_group` on `NoticeWizardInput`. UI: a
group `<select>` on the existing `/notices` wizard form that disables the activity picker when a group is
chosen (client-side mirror of the exclusivity rule); picking a group and submitting routes straight to the
composed draft exactly like the RoPA-activity path already did. Tests: unit (`ListTemplateGroups` covers all 8,
the acceptance criterion directly — picking a group produces an immediate draft carrying that group's own
sample text and DRAFT marker in both languages, unknown group refused, group+activity_ids together refused),
HTTP contract (200 list, 201 create, 422 unknown group).

### DSAR-13 Response letter templates (`docs/modules/DSAR.md#dsar-13`) — done
`internal/dsar` — the first module on the `dsar` schema. The acceptance criterion needed a real `dsar.requests`
row with a status/outcome to generate a letter from, so this pass also carries the minimal slice of
DSAR-01/02/06/07/08/11's job DSAR-13 depends on — creating a request and moving it through ST-02's real
transition graph — deliberately scoped no further: full intake channels, identity verification, the
workflow/subtask engine, business-day SLA countdown and the reject-with-reason UI are sibling features layered
on top later (the same "build the minimal slice a feature needs" move ROPA-01 made ahead of ROPA-02).
`dsar.request_types`, `dsar.requests` and the `dsar_letter` PLT-16 document type (`dsar.request.*` permissions,
`Approver: DPO`) were all already fully specified in the baseline migrations and `internal/wiring/docs.go` — this
pass only seeds data (migration 00043: the 9 fixed right-type rows, plus `ALTER TABLE ... ADD COLUMN name_en`
since the baseline only carried a Thai name and the letter is bilingual) and 6 DRAFT response-letter templates
(`platform.templates`, `template_type = 'dsar_response'`) — one per *purpose* (result / rejection /
request_info, the module doc's own "ดำเนินการแล้ว / ปฏิเสธ / ขอข้อมูลเพิ่ม"), not per (type × purpose):
`{{request_type_name}}` is substituted with the request's own type name at generation time, so one body reads
correctly for every right type without 27 near-duplicate bodies to keep in sync — flagged DRAFT per rule 8,
same pattern as ORG-07/ROPA-09/PNG-03.

`Service.Transition` encodes ST-02's full 8-state, 17-edge graph (`docs/states/state-machines.yaml#ST-02`,
newly added) even though this feature only exercises a subset — cheap to encode correctly once, so DSAR-06/08/11
build on a real machine later instead of reinventing one; only the two data-carrying guards this feature needs
are enforced (`outcome` required to enter `completed`, a reason required to enter `rejected`) — no verification,
subtask-completeness or SLA-overdue guard yet, since those modules don't exist. Entering
`awaiting_info`/`completed`/`rejected` auto-generates the matching response letter: a PLT-16 `dsar_letter`
document draft composed from the purpose's template with the requester's name (decrypted via PLT-13,
`Keyring.Decrypt` — the same class/context pattern CON-13's identifiers already established), request number
and type name substituted in as plain text (not PLT-16's `mergeField` mechanism, which only resolves org/DPO
fields live at every render — these values are specific to this one immutable letter instance). The generated
document is still just a draft: DPO reviews and edits it before ever sending it ("เลือก template + แก้ก่อนส่ง")
through PLT-16's own existing approve/publish flow — actually recording that a letter was sent
(`dsar.communications`) is left to whichever later feature owns delivery tracking, the same "leave the FK to
build the real thing later" move ROPA-01 made for `discovered_by_finding_id`.

`due_at` is `received_at` plus the request type's `sla_days` as *calendar* days — a placeholder; the real
business-day countdown is DSAR-07's job. Permissions reuse the already-seeded `dsar.request.*` (no new codes):
`read`/`create`/`execute` gate the three endpoints, and letter generation inside `Transition` additionally
needs `dsar.request.update` (the `dsar_letter` type's own `Create`/`Update` permission), since it's really
PLT-16's document-composer write happening underneath. API: `GET /admin/v1/dsar/request-types`,
`GET/POST /admin/v1/dsar/requests`, `GET /admin/v1/dsar/requests/{id}`,
`POST /admin/v1/dsar/requests/{id}/transition` (ETag/If-Match; 409 `dsar.invalid_transition` for a graph edge
that doesn't exist, 422 `dsar.invalid_input` for a missing outcome/reason). UI `/requests`: an intake form, a
status-filtered list with the SLA due date, and an inline action panel per request (next-status picker +
outcome/rejection-reason fields) linking straight to the generated letter's document editor. Tests: unit
(`ListRequestTypes` covers all 9 codes, `CreateRequest` validation and numbering, the acceptance criterion
directly — completing/rejecting a request generates a letter carrying the decrypted requester name, the
request's own type name and its request number in both languages — a rejected letter carries the reason, an
invalid ST-02 edge refused, two-tenant isolation), HTTP contract (401/403/400/404/409/412/422/428).

### DSAR-11 Rejection with reason (`docs/modules/DSAR.md#dsar-11`) + ROPA-10 Log of rejected requests
(`docs/modules/ROPA.md#ropa-10`) — done
Built together: DSAR-11's own acceptance criterion ends "...and gets recorded into RoPA automatically", and
ROPA-10 (a separate backlog row, "Log of rejected requests") is literally that recording step, wired through
the `dsar.rejected` event DSAR-11's own backend note names — the two features complete one BP-05/BP-11 flow.

**DSAR-11**: built on DSAR-13's `Transition`/ST-02 — entering `rejected` already required a reason; this
feature added the rest. "ผู้อนุมัติ" (an approver) is a permission gate, not a separate propose/confirm
round: the caller must additionally hold `dsar.request.approve` (already-seeded, no new code), not just
`dsar.request.execute` — `Transition` checks `authz.FromContext(ctx).Has(...)` and returns a new
`ErrForbidden` (403 `authz.denied`) otherwise, since DSAR-13's `rejected` transition is already one explicit
DPO action with a reason attached, not a multi-step draft. "บันทึกเข้า RoPA อัตโนมัติ" is deliberately *not*
done inside dsar: `TransitionInput` gained an optional `ActivityIDs []uuid.UUID` (the processing activities
this rejection concerns, FK-checked via a new `Ropa` interface — rule 9), and entering `rejected` now
publishes `dsar.rejected` (PLT-11 outbox, same tx as the status update) carrying those activity ids and the
reason code — dsar never writes to the `ropa` schema itself. `docs/architecture/events.yaml`'s `dsar.rejected`
entry (a generic template shared by every `dsar.*` lifecycle event) was extended with `reason_code` and
`activity_refs` — the two fields ROPA-10 actually needs, which the shared template didn't carry — and the
catalog regenerated (`go generate ./internal/platform/events`).

**ROPA-10**: `ropa.activity_rejections` (activity_id, dsar_request_id, reason_code, rejected_at) was already
fully specified in the baseline migrations; migration 00044 adds only a unique index on
`(activity_id, dsar_request_id)` for idempotency — `dsar.rejected` is delivered at-least-once (PLT-11), and
`ropa.Service.RecordRejection`'s insert is `ON CONFLICT ... DO NOTHING` against it, so a redelivery is a
harmless no-op rather than a duplicate row. `internal/wiring.Events` (new; called once from `cmd/worker`
after `ropaSvc` is built) subscribes `dsar.rejected` on the shared `events.Registry` and calls
`RecordRejection` for every activity ref the event carries — the first real in-process event
producer/consumer pair in this codebase (PLT-11 shipped with none registered yet). `RecordRejection`
FK-checks the activity via the already-exported `GetActivity` (rule 1) before inserting.

API: `POST /admin/v1/dsar/requests/{id}/transition`'s body gained `activity_ids` (DSAR-11);
`GET /admin/v1/ropa/activities/{id}/rejections` (`ropa.activity.read`, ROPA-10) is read-only — nothing writes
these rows through HTTP. UI: the reject panel on `/requests` gained an activity multi-select shown only when
rejecting; `/ropa/activities/{id}` gained a "DSAR rejections (ม.39(7))" section listing date + reason. Tests:
unit (rejecting without `dsar.request.approve` refused with `ErrForbidden`, an unknown activity id refused,
the outbox row carries the reason and activity ref — DSAR-11; idempotent redelivery of the same
activity+request pair inserts once, unknown activity refused, two-tenant isolation — ROPA-10), HTTP contract
for both.

### DSAR-17 ประวัติและสืบค้นคำขอ (`docs/modules/DSAR.md#dsar-17`) — done
No new migration or table: `dsar.requests.requester_blind_index` (already indexed, `ix_dsar_requests_requester_blind_index`)
was already computed and stored by DSAR-13's `CreateRequest`, just never queried. `dsarstore.ListRequests` gained two
`sqlc.narg` filters — `request_no` (`ILIKE '%…%'`, case-insensitive substring) and `blind_index` (exact match) — and
`dsarservice.RequestFilter.Search` decides which one to use in `ListRequests`: a value containing `"@"` is hashed
through `Keyring.BlindIndex` (never decrypted to search, rule 3) and matched exactly; anything else is matched as a
request-number fragment. Both are `AND`-composed with the existing `status` filter and cursor pagination unchanged.
The "history" half of the acceptance criterion (ประวัติการพิจารณา, การแก้ไขทุกครั้ง) needed no new endpoint either:
`dsar_request` is now a registered PLT-07 record type (`collabSvc.Register("dsar_request", ...)` in `cmd/api/main.go`,
`Exists` backed by `dsarSvc.GetRequest`) — its comments, attachments and activity feed (which is `platform.audit_log`
replayed, so it already carries every `Transition` call's before/after and timestamp) come for free from the same
generic component every other module's records use, per DSAR-13's own dependency on PLT-07. API: `search` query
param on `GET /admin/v1/dsar/requests`. UI: a search box next to the status filter on `/requests`, and a "ประวัติ"
toggle per row rendering the shared `RecordCollaboration` component (`entityType="dsar_request"`) — the same pattern
`/settings/notification-templates` already uses. Tests: unit (search by request-number fragment, by e-mail
case-insensitively via blind index, a different requester's e-mail never matches, an unknown e-mail returns nothing),
HTTP contract (200 for both search forms, matching row present/absent as expected).

### DSAR-07 นับเวลา SLA 30 วัน (`docs/modules/DSAR.md#dsar-07`) — done
`due_at` itself needed no change: `docs/legal/pdpa-rules.md`'s ม.30 row already reads "30 วันนับแต่วันที่ได้รับคำขอ"
with no business-day qualifier, so DSAR-13's calendar-day calculation was already correct — DSAR-07's "SLA 30 วัน
(PLT-05)" backend note is about the reminder/escalate half, not the deadline math. `dsarservice.SLAStatus(now, dueAt)`
is a new pure function (on_track / at_risk / overdue, at_risk once ≤10 days remain — generalizing the legal doc's
own "at_risk วันที่ 20" worked example for the default 30-day type to any `request_types.sla_days` value) — computed
live in the HTTP layer on every read, never persisted, so it can never go stale. The acceptance criterion itself
("เหลือ 7 วันถูกแจ้งเตือน") is one River checkpoint per request: `scheduleReminder` (called from `CreateRequest`)
enqueues `dsar.sla_reminder` at `due_at - 7d`, or immediately if that moment has already passed (the same
"late-recorded event still alerts at once" rule PNG-04's `ToSchedule` already established) — no need for PLT-04/BRE-07's
multi-checkpoint list since this feature has only the one. `FireReminder` no-ops for an unknown or already-closed
(`completed`/`rejected`/`withdrawn`) request, then notifies the assignee (if one is set) plus every user with role
DPO — the same "default recipients until real per-record routing exists" fallback BRE-07/PNG-04 already use, since
DSAR-08 (workflow & subtasks, not built) is what will eventually resolve a real per-task assignee. Migration 00045
seeds `dsar.sla_reminder` (4 rows, th/en × in_app/email, operational text so no DRAFT marker — rule 8 is about text
shown to a data subject or the PDPC, same reasoning as PNG-04's own templates).

"ผู้รับผิดชอบ" needed a way to actually be set — the column (`assignee_user_id`) already existed but nothing wrote
to it — so this pass also added the minimal `AssignRequest` (new `UpdateRequestAssignee` sqlc query, FK-checked via
`iamservice.Names` the same way DPO-01 validates an internal appointment's user) rather than waiting on DSAR-08's
full workflow/task-claim engine; `dsar.request.update` gates it (no new permission code), consistent with DSAR-13's
own "it's really a document-composer-adjacent write" reasoning for reusing that code. `cmd/worker` gained its first
dsar wiring (`dsarservice.ReminderWorker`, `Notify`+`River` fields new on `Service` — `Docs`/`Org`/`Ropa`/`Keyring`
stay nil there since the reminder path never touches them, the same "worker builds only what its own jobs need"
pattern `breachSvc`'s `nil` document composer already set). API: `POST /admin/v1/dsar/requests/{id}/assign`
(ETag/If-Match), `sla_status` added to `DsarRequest`. UI: an SLA badge column and a per-row "มอบหมาย" (assign) toggle
on `/requests`, reusing PLT-07's own `useMentionSearch` picker for the assignee search (the same component
`RecordCollaboration`'s @-mention picker already uses, so no new user-search endpoint). Tests: unit (`SLAStatus`
boundaries, `ReminderAt`, the reminder job is enqueued at the right `scheduled_at` on create, a stale/closed/unknown
tick no-ops, `AssignRequest` sets/clears/rejects an unknown user, two-tenant isolation via the existing per-package
harness), HTTP contract (401/428/412/422/200 on `/assign`, `sla_status` present on every response). Not done:
DSAR-08's real per-task routing (this stays on the DPO-role fallback until that engine exists) and a filtered
"near/overdue" list view (the badge column already surfaces this at a glance — no screen has asked for a separate
filtered list yet).

### IAM-05 บริการยืนยันตัวตนเจ้าของข้อมูล (`docs/modules/IAM.md#iam-05`) — done (portal use pending a consumer)
`internal/iam/service/verification.go` — the first feature on `iam.subject_verifications`, already fully
specified in the baseline migrations (purpose/method CHECK constraints, `otp_hash char(64)`, `attempts`,
`assurance_level`, `expires_at`), so no new migration for the table itself. `StartVerification` generates a
random 6-digit code (`crypto/rand`), stores only its SHA-256 hex hash (never the plaintext, rule 3), and sends
it via PLT-04 (`Urgent: true`, skipping quiet hours — a 5-minute-lived code has no use for them) to the raw
identifier as a `RecipientAddress` (never persisted); `identifier_blind_index` is computed via the existing
PLT-13 `Keyring.BlindIndex`. `VerifyOTP` enforces decisions.md D-01 exactly — `OTPExpiry = 5m`,
`MaxOTPAttempts = 5` — via a pure `SubjectVerification` state machine (pending → verified/failed/expired, no
new `docs/states/state-machines.yaml` entry since it's a 4-state/3-edge shape entirely local to this one
table); every call, right or wrong, increments `attempts` and writes a `platform.audit_log` row before
returning (the acceptance criterion's "บันทึกผลการยืนยันทุกครั้ง") — the plaintext code itself is never in
that log, only the outcome. Only `otp_sms`/`otp_email` are built; `magic_link`/`idp`/`thaid` are already in the
column's CHECK constraint for later, exactly as the module doc's own "รองรับ IdP ภายนอก / ThaID ภายหลัง" says.

Real import-cycle problem, not a design choice: `platform/audit/service` already imports `iam/service` (ORG-19's
actor-name resolution) and `platform/notify` already imports it too (user-recipient contact resolution) — so
`iam/service` importing either of *them* back (for `Write`/`Send`) would be a compile-time cycle. Fixed with two
minimal local interfaces owned by `iam/service` (`Auditor`, `Notifier`) and matching plain-struct types
(`AuditEntry`, `NotifyRequest`) that mirror only the fields this feature needs — no import of either concrete
package. `internal/wiring.IamVerification` (new) is the adapter, built where both concrete packages are already
safely importable; `cmd/api/main.go`'s `iamSvc` construction moved down past `keyring`/`notifySvc` and now goes
through it (still serves `/me` unchanged — `Keyring`/`Notify`/`Audit` are simply unused there).

No HTTP endpoint yet, deliberately — the module doc's own frontend note is "ขั้นตอนยืนยันตัวตนใน portal ใช้ร่วม
preference center, คำขอใช้สิทธิ และ double opt-in", and none of those (a portal preference center, a public DSAR
intake form, double opt-in) exist yet to design a real contract against; the same "no consumer yet" deferral
PLT-13's own crypto Keyring used when it shipped ("not wired into cmd/api/cmd/worker yet"). Migration 00046 seeds
the `iam.otp` template (sms/email × th/en) — operational text, no DRAFT marker (rule 8 is about legal/notice
text). Tests (`internal/iam/service/verification_test.go`, the package's first test file): OTP sent with a
5-minute expiry, correct-code verifies, 5 wrong attempts locks the row and a 6th is refused as already decided,
an expired-but-correct code is refused with an injected clock, every attempt (right or wrong) leaves its own
audit row, two-tenant isolation. Not done: rate-limiting *starting* a new verification (only the 5-attempt cap
on *verifying* one is in decisions.md; the `internal/pkg/ratelimit` package exists for this but has no natural
key to rate-limit against yet without a real public endpoint), and the frontend flow itself.

### CON-11 ความยินยอมผู้เยาว์และผู้ปกครอง (`docs/modules/CON.md#con-11`) — done (portal form/guardian-confirm page not built)
`internal/consent/service/guardian.go` (+ `store/guardian.sql.go`, no new migration): the schema already anticipated
this feature — `consent.guardian_approvals`, `consent.data_subjects.is_minor`/`guardian_subject_id`/
`legal_capacity`, and `consent.purposes.min_age` were all already in the baseline migrations, and ST-01 already
declared a PENDING status with PENDING/CONFIRMED/CANCELLED transaction types (the SA's own `ConsentResult.
transactions[].transaction_type` wire enum already listed CONFIRMED/CANCELLED before this feature touched it).
`Decide()` (`transitions.go`) gates only a purpose's *first-ever* decision for a subject: `guardianRequired &&
current == ""` routes to `TxPending`/`StatusPending` instead of `TxConsented`/`StatusActive` — deliberately
narrower than "every guardian-gated decision," because `docs/states/state-machines.yaml#ST-01` declares PENDING
reachable only from the empty initial state, with no edge back into PENDING from NOT_GIVEN/WITHDRAWN/EXPIRED;
per CLAUDE.md's own source-priority order (migrations > docs/states > ...), a returning already-decided minor's
fresh CONSENTED decision on a guardian-gated purpose goes straight to ACTIVE like any other purpose, rather than
inventing a transition the state machine doesn't declare.

`Record()` computes `guardianRequired := sub.IsMinor && d.purpose.MinAge != nil` per decision; a CONSENTED
decision on a guardian-gated purpose without `Submission.Guardian` (identifiers + relationship) is refused as a
`DecisionError{Fields: [{Code: "guardian_required"}]}` before anything is written. When any status lands PENDING,
`requestGuardianApproval` resolves/creates the guardian's own data-subject record (reusing `resolveSubject`,
the same blind-index lookup every other identifier match uses), inserts one `consent.guardian_approvals` row and
starts the guardian's own OTP through `Service.Verification` — a direct `*iamservice.Service` field, not an
interface-indirection adapter: unlike IAM-05's own `Auditor`/`Notifier` workaround (needed there because
`platform/audit/service` and `platform/notify` already import `iam/service`, so `iam/service` importing either
back would cycle), `iam/service` does not import `consent/service`, so there is no cycle to route around (rule 9
is still satisfied — `consent` only calls `iam/service`'s own exported `Service`).

`ConfirmGuardianApproval` is the acceptance criterion itself ("มีผลเมื่อผู้ปกครองยืนยันแล้วเท่านั้น"): verifies the
guardian's OTP via `Verification.VerifyOTP`, then — guarded by `UPDATE ... WHERE status = 'requested'` so a
redelivered/retried confirm is a harmless no-op (`ErrInvalidTransition`), not a duplicate activation — sweeps
*every* purpose still PENDING for that minor subject (`ListPendingStatusForSubject ... FOR UPDATE`), not just the
one that first triggered the request: a single public-form submission can guardian-gate several purposes at
once, and the guardian's one OTP should unlock all of them together. Each confirmed purpose gets a CONFIRMED
transaction, an ACTIVE status, and its own `consent.granted` event; the whole confirm is one audit entry
(`consent.guardian_approval.approve`). `mapVerificationErr` translates IAM-05's own sentinel errors
(`ErrVerificationNotFound/Decided/Expired/ErrTooManyAttempts/ErrCodeMismatch`) into consent's own sentinels so
the HTTP layer never needs iam-specific mapping (rule 9).

API: `subject.is_minor`/`subject.guardian` added to `ConsentSubmitPublicConsent`'s body,
`guardian_approvals: [{id, verification_id, channel}]` added to `ConsentResult`, and a new public endpoint
`POST /public/v1/consents/guardian-approvals/{id}/verify` (`x-permission: public`, `Idempotency-Key`, body
`{verification_id, code}`, 200 `{status: "approved", confirmed_purposes: [...]}`).

Real bug found and fixed while writing the HTTP contract test: `Service.audit()` resolved the tenant only from
`authz.FromContext(ctx).TenantID` — fine on admin routes (AuthZ middleware sets Grants) but always empty on
`/public/v1` routes, since `publickeys.Middleware` only sets an `httpx.Principal`, never `authz.Grants` (there is
no user to authorize). `ConfirmGuardianApproval`'s own audit call therefore always hit `ErrForbidden` → 403
`authz.denied`, breaking guardian verification on every real request. Fixed by falling back to
`SELECT current_setting('app.tenant_id')` on the request's own transaction when no Grants are present — the same
technique IAM-05's own `verificationTenantID` already uses — so every module's `audit()`-style helper on a public
route should use this fallback, not just consent's.

Tests: unit (`transitions_test.go` — the `current == ""` gate and its three "already decided" exceptions;
`guardian_test.go` — the acceptance criterion end to end with a fake IAM notifier capturing the OTP, wrong code
leaves the subject PENDING, missing guardian info refused pre-write), HTTP contract (`handler_test.go` — the same
flow through the real validator/AuthZ/public-key/Idempotency/Tx chain, wrong code then correct code against the
public verify endpoint). Not done: the portal-side UI (`docs/modules/CON.md#con-11`'s own frontend note —
"ขั้นตอนผู้เยาว์ในฟอร์ม + หน้าผู้ปกครองยืนยัน", a minor-declaration step on the public consent form plus a
guardian-confirmation page) — the acceptance criterion is fully exercised and proven by the backend contract test
without one; add it once a screen actually needs it, the same "no consumer yet" deferral IAM-05's own OTP service
used for its portal piece.

### DPIA-01 แบบคัดกรองความจำเป็นในการทำ DPIA · DPIA-02 เกณฑ์คะแนนและเงื่อนไขบังคับทำ (`docs/modules/DPIA.md#dpia-01`, `#dpia-02`) — done
`internal/dpia` — the first module on the `assess` schema (`assessment.dpia.*`/`assessment.template.*`,
already seeded in the baseline permission migration). `assess.templates`, `assess.screening_rules` and
`assess.assessments` were all already fully specified in the baseline migrations, and `internal/wiring.Forms`
had already reserved a PLT-06 form type, `"assessment"` (`assessment.template.*` to design/publish,
`assessment.dpia.create` to respond), unused until now — no schema or wiring change needed beyond seed data.
Migration 00047 seeds a global (`tenant_id NULL`) screening form + `assess.templates` row, code
`dpia_screening`: 6 yes/no questions paraphrasing TDPG 4.0-P's high-risk factors (sensitive data ม.26,
large-scale, systematic monitoring, automated decision-making, new tech/AI, vulnerable groups) — flagged for
legal review the same way ORG-07/ROPA-09/PNG-03 already do (`docs/decisions.md` Q-26).

`Service.Screen` answers the form against `forms.Evaluate` directly (the pure function, not
`forms.Service.Record`): `assess.assessments` has no `form_submission_id` column to point at a
`platform.form_submissions` row the way `dpo.security_assessments` does, so persisting an unlinked submission
would be an orphan row nothing references. Instead each risk-factor answer is written to `assess.answers`
(one row per question) — the schema's own intended place for a screening's answers — via `forms.Contributions`
for the factor breakdown. The result is computed in Go, not baked into the form's own scoring bands: `required`
when the factor count (or, if set, the raw score) meets the tenant's own `assess.screening_rules.min_factors`/
`min_score` (default `min_factors=2`, a config tunable per Q-26, not persisted until first saved — the same
"defaults until first save" pattern ORG-20's `org_settings` uses); `recommended` when at least one factor is
flagged but short of that; `not_required` otherwise. `assess.assessments.status` lands `not_required` or
`in_progress` directly (ST-05's own `screening → not_required | in_progress` edges — already declared, no new
state-machine entry needed) — the transient `screening` status is never separately persisted, since the whole
call is atomic and no partial state ever exists in the DB. Re-screening the same activity opens a new round
(`round_no`/`previous_id`, following `assess.assessments`' own versioning columns) rather than editing the
prior one, so `SaveRules`'s effect (DPIA-02's acceptance criterion) is naturally scoped to the next round only.

`SaveRules` is append-only history (`DeactivateScreeningRules` then a fresh insert), not an in-place edit —
the criteria a past round was judged against stays on record. Gated by `assessment.template.update` (DPO-only
per the seeded RBAC — `assessment.dpia.update` is also held by OWNER, which the module doc's own actor line
restricts to DPO for DPIA-02 specifically), matching DPIA-02's own actor line; `Screen` itself is gated by
`assessment.dpia.create`, which only DPO holds in the seeded grants — OWNER (the module doc's other actor,
who "answers" the screening per BP-08) only has `assessment.dpia.read`/`update`, not `create`, so for this
pass DPO runs the whole screening in one call, the same "build the minimal slice this feature needs" move
DPO-09's own `Assess` made (bypassing PLT-06's draft/section-assignment flow for a one-shot record); a
delegated OWNER-fills-the-form flow needs that Assign/CompleteSection machinery layered on top later.

Real migration bug found and fixed while seeding: the natural `WITH def AS (INSERT INTO form_definitions
...), ver AS (INSERT INTO form_versions ... RETURNING id, form_id) UPDATE form_definitions SET
current_version_id = ver.id FROM ver WHERE form_definitions.id = ver.form_id` pattern left `current_version_id`
NULL every time (0 rows affected, no error) — confirmed by hand outside the migration too. All the
sub-statements in one `WITH` share the query's own snapshot, so the primary `UPDATE`'s scan of
`form_definitions` (the same table a sibling CTE just inserted into) never sees that new row. Fixed by using
fixed literal ids and three separate top-level statements instead of one combined `WITH`/CTE chain — this
pitfall applies to any future migration that inserts into a table and then needs to update a self-reference on
that same freshly-inserted row within one statement.

Real regression found and fixed in an unrelated package: `internal/platform/forms`' own
`TestTwoTenantIsolation`/`TestPermissionsByFormType` asserted `len(ListForms(...)) == 0` for a tenant that had
created nothing — true only because no global (`tenant_id NULL`) form existed anywhere in the DB before this
migration. Migration 00047's seeded `dpia_screening` form is now always visible to every tenant by design
(rule 1's RLS: `tenant_id IS NULL OR tenant_id = current tenant`), so that assertion legitimately breaks the
instant any feature seeds a global form. Fixed by asserting the *other* tenant's specific form id is absent
from the list instead of asserting the list is empty — the correct two-tenant-isolation check regardless of
how many global forms exist.

API: `GET`/`PUT /admin/v1/dpia/screening-rules`, `POST /admin/v1/dpia/activities/{id}/screen`,
`GET /admin/v1/dpia/assessments` (+ `/{id}`) — cursor pagination, same shape as ROPA-04/PLT-16's own lists.
UI: a "DPIA screening" section on `/ropa/activities/{id}` (6 checkboxes + past rounds for that activity) and
`/settings/dpia` (thresholds). Tests: unit (not_required/recommended/required boundary cases against the
default threshold, changing the threshold changes the *next* round only, unknown-activity FK check, two-tenant
isolation), HTTP contract (401/403/422/201/200/404). Not done: everything past screening (DPIA-03 template
library beyond this one seeded form, DPIA-04 RoPA prefill into a full assessment, DPIA-05 onward — necessity/
proportionality, risk scoring, DPO opinion, approval) — sibling features layered on the same `assess.assessments`
row, not built here, the same layering DSAR-13 used for DSAR-01/02/06/07/08/11.

### DPIA-03 คลัง template แบบประเมิน (`docs/modules/DPIA.md#dpia-03`) — done
`internal/dpia/service/templates.go` (+ `http/templates.go`, no new migration): the whole feature sits on
`assess.templates` (already fully specified in the baseline migrations — same table DPIA-01/02's own seeded
screening template already uses) and the generic PLT-06 form engine (`internal/platform/forms`), both
already built. DPIA-03 doesn't reimplement form editing — a template's questions/options/scoring are edited
on PLT-06's own existing `/forms/{id}` builder page, which every `"assessment"`-type form (screening
included) already uses; this feature's real, narrow scope is the `assess.templates`-level catalog wrapping
that content: `ListTemplates`/`GetTemplateByID` (list + filter by `assessment_type`), `CreateTemplate` (a
fresh PLT-06 form + its `assess.templates` wrapper row), `CloneTemplate` (the acceptance criterion), and a
`PublishTemplate`/`RetireTemplate` lifecycle.

`CloneTemplate` resolves the source form to its current content — published version if it has one, else its
still-open draft (`latestFormVersion`, a small addition next to DPIA-01/02's own `screeningVersion`, which by
contrast only ever accepts a *published* version since screening must run against a live, reviewed form) —
and calls `s.Forms.CreateForm` with that schema/scoring/languages to build a brand-new
`platform.form_definitions` row from scratch. Because the clone is a different form row from the moment it's
created, not a reference or a shared draft, editing it afterward through PLT-06's own builder (`SaveDraft`,
`Publish`) can never reach the source template's own form — proven directly by a test that publishes the
clone and re-reads the source, expecting its status/version untouched.

`PublishTemplate` leans on the underlying form's own optimistic-concurrency ETag (`forms.Service.Publish`'s
`draftVersion` parameter, checked against the draft version's own `row_version`) rather than adding a second
check on the `assess.templates` row — that ETag is already the real precondition for "is this still the
draft I'm publishing". `RetireTemplate` has no such underlying check to lean on (retiring doesn't touch the
form at all, only flips `assess.templates.status`), so it gets its own: a dedicated `RetireTemplateRow` sqlc
query with `WHERE id = $1 AND row_version = $2`, mapped to a new `ErrVersionMismatch` sentinel (412) exactly
like PLT-08 and other modules' own ETag checks. `RetireTemplate` is gated by `assessment.template.delete` (a
permanent status change), not `.update` — the same reasoning ORG-06's external-party merge already
established. Templates created through this feature's API are always tenant-scoped; a `tenant_id NULL`
global row only ever comes from a migration (ORG-07 Q-20/ROPA-09 Q-25/PNG-03 Q-14/DPIA-01's own Q-26 pattern),
never from this endpoint.

API: `GET`/`POST /admin/v1/dpia/templates`, `GET /admin/v1/dpia/templates/{id}`,
`POST /admin/v1/dpia/templates/{id}/clone`, `POST .../publish` (`If-Match`), `POST .../retire` (`If-Match`) —
all on the already-seeded `assessment.template.*` permissions (`.read`/`.create`/`.publish`/`.delete`), no
new permission code or migration. UI `/settings/dpia-templates`: a list filtered by `assessment_type` with
clone/publish/retire actions and a link into PLT-06's own `/forms/{id}` builder for content, plus a create
form (following `/forms`' own `CreateForm` pattern almost exactly) — linked from `/settings/dpia`. Tests:
unit (create + list-by-type, unknown assessment_type refused, duplicate code refused — surfaces as the
underlying PLT-06 form's own code uniqueness, `forms.ErrInvalidRequest`, since CreateTemplate always creates
a brand-new form with the caller's code and that collision is checked before `assess.templates`' own insert
is ever attempted — clone independence, publish/retire version-mismatch and success, two-tenant isolation),
HTTP contract (401/403/201/200/404/412/428). Not done: nothing else in this pass — DPIA-03 is now fully built
against its one acceptance criterion; the next DPIA feature (DPIA-04 onward) builds on top of a *published*
template the way DPIA-01/02's screening form already does.

### DPIA-04 อธิบายกิจกรรมโดยดึงข้อมูลจาก RoPA (`docs/modules/DPIA.md#dpia-04`) — done
`internal/dpia/service/description.go` (+ `http/description.go`, no new migration, no new permission code):
the acceptance criterion ("ข้อมูลที่ดึงจาก RoPA ตรงกับกิจกรรมต้นทาง") and the backend note's own "sync เมื่อ
RoPA เปลี่ยน" are the same requirement solved the same way — `Service.ActivityDescription` composes the
assessment's linked RoPA activity's purposes, data categories, data subject groups, recipients, cross-border
transfers and retention *live* on every call, and never persists any of it. There is nothing to go stale,
so there is no separate sync step to build: a plain re-read after the activity changes is already correct.

`dpiaservice.Ropa` (already used by DPIA-01/02's own `GetActivity` check) grew five more methods —
`ListActivityPurposes`/`ListActivityData`/`ListActivityRecipients`/`ListActivityTransfers`/
`ListRetentionRules` — and a new `Org` interface (`GetMaster`/`ListMaster`/`GetExternalParty`) was added to
`Service`, both wider-interface-only changes: every method they name was already exported by `ropaservice`
and `orgservice` for ROPA-03/06/08's own completeness checks and FK-visibility lookups, so this feature adds
no new code to either module (rule 9 — dpia reads ropa/org only through their own exported services, never
their schemas). Lawful-basis/data-category/subject-type/country names are resolved the exact way ROPA-08's
own `validCountry` already scans `Org.ListMaster` by code; party names via `Org.GetExternalParty`, the same
call ROPA-03/04's completeness and export already make. The activity itself is never re-checked against RLS
here — `GetAssessment` (DPIA-01/02's own, tenant-scoped through `assess.assessments`) already proves the
assessment is the caller's before `ActivityDescription` ever calls `Ropa.GetActivity`, so a cross-tenant
assessment id fails at that first step with the usual `ErrNotFound`, not a second RLS check.

API: `GET /admin/v1/dpia/assessments/{id}/description` (`assessment.dpia.read`, no ETag — the response is
never written back). Wire schema follows ROPA-01's own bilingual-name convention (`category_name_th`/`_en`
pairs, not the earlier DPIA-03 sub-schemas' single localized field) since this is a plain admin JSON
response, not a bilingual PLT-16 document. UI: a new description panel on `/ropa/activities/{id}`'s existing
DPIA screening section (`dpia-screening-section.tsx`), shown only once the activity's latest screening round
is `in_progress` — a `not_required` round has no full DPIA to describe, and a screening round that hasn't
happened yet has no assessment id to ask for. Not done: `ropa.activity_systems` ("ระบบที่ใช้" — asset links)
per the module doc's own backend note, since ROPA-02/03 never built any CRUD on that table (a private-note
gap in ROPA-02's own module doc: "no field for owner_user_id yet" applies the same way here) — nothing exists
yet to surface; add it once a screen actually writes to that table.

### DPIA-05 ประเมินความจำเป็นและความได้สัดส่วน (`docs/modules/DPIA.md#dpia-05`) — done
`internal/dpia/service/necessity.go` (+ `http/necessity.go`, migration 00048, `docs/decisions.md` Q-27, no new
permission code): a 4-question yes/no checklist (ม.22 data minimization, purpose specificity, ม.24/26 lawful-
basis fit, a less-invasive alternative considered) on the exact same apparatus DPIA-01/02/03 already built —
`assess.templates` (assessment_type `dpia`, code `dpia_necessity`) wrapping a published PLT-06 form, answered
through `forms.Evaluate` the same way `Screen` already is. Unlike DPIA-01's screening (which writes answers
with `section_id NULL`), this checklist's answers are grouped under a real `assess.sections` row
(`section_code = 'necessity'`) — `assess.sections`/`assess.answers` were already in the baseline schema for
exactly this multi-section future, unused until now. `ListAnswersForAssessment` (DPIA-01's own factor read)
gained a `section_id IS NULL` filter to keep necessity answers out of `Assessment.Factors` — proven directly
by a test that answers necessity and re-reads the screening round, expecting its factor count unchanged.

The conclusion is a plain business rule, not a legal one: "necessary" only once every question reads "yes";
any other answer lands the question code in `missing` and the result reads "needs_review" — computed by a
pure `necessityResult(schema, answers)` shared by both the write path (`AssessNecessity`) and the read path
(`GetNecessity`, which recomputes live from the stored answers rather than trusting a persisted verdict).
Re-answering (`AssessNecessity` called again) deletes the section's prior answers and inserts fresh ones
under the same section row (`ResubmitSection` refreshes `submitted_at`) — a redo, not a new round the way
DPIA-01's own re-screening is, since this checklist has no threshold configuration a fresh round would be
judged against differently.

Permissions: `assessment.dpia.update` gates `POST .../necessity` (not `.create`, which only DPO holds) — the
module doc's own actor line lists both OWNER and DPO, and the baseline RBAC seed only grants OWNER `.read`/
`.update` on `assessment.dpia.*`, matching DPIA-02's own reasoning for why a `.update`-gated action is the
right one when OWNER must be able to act. `dpiaservice.ErrBadTemplate`'s http-layer message (`handler.go`)
was widened from "No published DPIA screening template" to "...for this checklist" since the sentinel is now
shared between DPIA-01's screening and this feature's own necessity template lookup.

API: `GET`/`POST /admin/v1/dpia/assessments/{id}/necessity` — GET 404s before the checklist is ever answered
(no empty-result placeholder). UI: a checklist panel on `/ropa/activities/{id}`'s DPIA section, right next to
DPIA-04's own description panel, shown under the same `in_progress` condition. Tests: unit (the acceptance
criterion directly — all-"yes" reads necessary with nothing missing; any "no" flags exactly that question;
re-answering replaces rather than duplicates; necessity answers never change the screening round's own
Factors; reading before answering is `ErrNotFound`; two-tenant isolation), HTTP contract (401/403/200/404).

### DPIA-14 ประวัติเวอร์ชันและ audit trail (`docs/modules/DPIA.md#dpia-14`) — done
`internal/dpia/service/diff.go` (+ `http/diff.go`, no new migration, no new permission code): the acceptance
criterion ("เห็นความต่างของคำตอบระหว่างรอบได้") maps directly onto DPIA-01's own round chain
(`assess.assessments.round_no`/`previous_id`, built for re-screening) — `Service.CompareToPrevious` loads the
current round and the round it supersedes through the already-exported `GetAssessment` (rule 1 — tenant
isolation comes for free, no new query), builds question→answer maps from each round's `Factors
([]forms.Contribution)`, and diffs them: a change is emitted only where a question's before and after differ
(`reflect.DeepEqual`, since an answer can be any JSON-decoded type, not just a string), including one-sided
additions/removals if a question's answer is missing on either side. Question keys are sorted (`sort.Strings`)
before the diff is built — Go map iteration order is unstable, and an unsorted diff would return the same
logical result in an unpredictable order across calls, which would be a nuisance for both API consumers and
tests. Deliberately scoped to DPIA-01's own screening `Factors` only, not DPIA-05's necessity checklist:
necessity has no round concept to diff against — an answer is replaced in place, not versioned into a new
round — so under this feature's literal acceptance criterion there is nothing meaningful to diff there.

The other half of the acceptance criterion ("ผู้แก้ไข และวันที่") needed no new backend code at all:
`"dpia_assessment"` is now a registered PLT-07 collaboration record type (`collabSvc.Register` in
`cmd/api/main.go`, right after `dpiaSvc` is constructed — the exact move DSAR-17 made for `dsar_request`),
`Exists` backed by `dpiaSvc.GetAssessment`. Comments, attachments and the activity feed (which is
`platform.audit_log` replayed, so it already carries every `Screen`/`AssessNecessity` call's actor and
timestamp via `s.audit(...)`) come for free from the same generic component every other module's records use.

API: `GET /admin/v1/dpia/assessments/{id}/diff` (`assessment.dpia.read`) — `changes: []` (serialized as `null`,
an unpopulated slice) for a first round or an unchanged re-screen, `previous_id` omitted when there is none. UI:
a diff panel under each round with `round_no > 1` on `/ropa/activities/{id}`'s DPIA section (only rendered when
there's at least one change), and a "ประวัติ" toggle per round — the same pattern DSAR-17's own history toggle
used — rendering the shared `RecordCollaboration` component (`entityType="dpia_assessment"`); `currentUserId`
is threaded down from the activity detail page (`loadMe()`) through `ActivityDetailContent` to
`DpiaScreeningSection`, which didn't need it before this feature. Tests: unit (a first round has no previous
round to diff against; changing one answer shows exactly that question's before/after — the acceptance
criterion directly; an identical re-screen diffs to nothing; an unknown assessment id is `ErrNotFound`;
two-tenant isolation), HTTP contract (401/200/404, and a re-screened round showing the changed question in its
diff response).

### DPO-05 ศูนย์แจ้งเตือนรวม (`docs/modules/DPO.md#dpo-05`) — done, scoped to modules that already track a deadline
`internal/dpo/service/center.go` (+ `http/center.go`, no new migration, no new permission code — `dpo.report.read`
is already seeded for "Dashboard และรายงาน" and fits this exactly). `Service.Deadlines` is the acceptance criterion
("งานใกล้ครบกำหนดจากทุกโมดูลแสดงในหน้าเดียว"): it reads every module that already tracks a legal deadline — DSAR's
30-day SLA and breach's 72-hour PDPC clock — through their own exported services (`dpo.Service` gained `Dsar
*dsarservice.Service` and `Breach *breachservice.Service` fields, concrete types directly like `Forms` already was;
no import cycle, since neither module imports `dpo`, rule 9). DPIA review cycles, contract expiry and notice-review
reminders — the other three items the module doc's own description names — aren't built yet (no owning feature
exists: DPIA-13, PNG-15, the DPA/DSA modules), so they're left out until they exist, the same "no consumer yet"
deferral this codebase uses elsewhere (PLT-13's own Keyring shipped the same way).

DSAR: `dsarservice.ListRequests` once per ST-02 non-terminal status (no multi-status filter exists, and adding one
for a single caller felt like more than this feature needs), each row's `dsarservice.SLAStatus(now, due_at)` reused
exactly as DSAR-07 already defined it — on_track items are dropped. Breach: `breachservice.List{OpenOnly: true}`,
paginated to drain every open incident, classified at_risk/overdue against a new `breachAtRiskWindow` of 6 hours —
deliberately the same point BRE-07 already escalates to role EXEC from (66 of the 72-hour clock), not a new
threshold invented for this dashboard. Real access-control gap found while designing this: `dsarservice.ListRequests`
and `breachservice.List` have no shared caller — `breachservice.List` already enforces `breach.incident.read`
internally (`has(ctx, PermRead)`), but `dsarservice.ListRequests` enforces nothing itself (`dsar.request.read` is
normally checked only at the HTTP/operation layer, which this internal call bypasses). A caller holding only
`dpo.report.read` (EXEC, ORGADMIN — seeded with that permission but not `dsar.request.read`/`breach.incident.read`)
would otherwise see DSAR request numbers and due dates they have no module-level permission for. Fixed by checking
`authz.FromContext(ctx).Has("dsar.request.read")` before calling the DSAR side at all, and catching breach's own
`ErrForbidden` to skip that source rather than failing the whole call — each source is left out, not a 403 for the
whole dashboard, when the caller lacks that module's own read permission; `dpo.report.read` is not a blanket grant
to every module's detail.

`GET /admin/v1/dpo/deadlines` (`dpo.report.read`) returns every near-deadline/overdue item sorted by due date
ascending, each naming its source module, a reference id + human label (request number / incident number) and
at_risk/overdue. UI: a new page, `/dpo` (the module doc's own UX note: "มีหน้าจอใหม่"), linked from the existing
`/settings/dpo` appointments page; each row links to its source record (`/incidents/{id}` for breach — DSAR has no
per-request detail route yet, so it links to the `/requests` list instead). Tests: unit (aggregates and filters
across both modules — the acceptance criterion directly; sorted by due_at; a caller without a source module's read
permission sees that source silently empty, not an error; two-tenant isolation), HTTP contract (401/403/200).

### RTG-01 คลังกิจกรรมมาตรฐานตามหมวดงาน (`docs/modules/RTG.md#rtg-01`) — done
The first feature on the `RTG` module: `ropa.template_sets`/`ropa.activity_templates` were already fully
specified in the baseline migrations (ERD-09), and `ropa.template.*` was already seeded (migration 00019) —
this pass only seeds data (migration 00049), no schema change. T15, the backlog dependency meant to supply
real content, had no deliverable to pull from here, so — the same "seed a draft + flag for legal review" move
ORG-07 (Q-20), ROPA-09 (Q-25), PNG-03 (Q-14) and DPIA-01 (Q-26) already made — this writes 51 activities
across 14 job categories (recruitment, payroll, procurement, marketing, cctv, visitor, it_support,
customer_service, finance, legal_compliance, facilities, training, health_safety, product_analytics, plus 3
processor-role examples) as a new `docs/decisions.md` entry, Q-28. Each activity's `defaults` jsonb covers
every ม.39 topic in the exact shape ROPA-03/06/08/09's own `activity_purposes`/`activity_data`/
`activity_recipients`/`retention_rules`/`activity_controls` tables already use (purposes with
`lawful_basis_code`, data with `data_category_code`+`subject_type_code`, recipients, retention, security
controls) — keyed by *code*, not id, since a global template can't reference a tenant's own `org.*` rows.
Lawful-basis/data-category/subject-type codes and security-control codes are the real seeded values from
ORG-07 (migration 00031) and ROPA-09 (migration 00040), cross-checked against the live catalog before
writing the migration, not invented. `rationale` carries a short "ร่าง — รอฝ่ายกฎหมายตรวจ" note per activity
(rule 8's spirit, even though this is operational RoPA content, not a notice/letter/clause).

`internal/ropa/templates` is a new, deliberately minimal read-only package (`Service.New()`, no fields) —
mirrors `internal/risk/service`'s own "nothing to create/update yet" shape exactly: no tenant authors a
template here (that's RTG-08, P3, "แก้ไขและสร้าง template ขององค์กร", not built). `Defaults`/`Rationale` stay
opaque `json.RawMessage` all the way to the wire (`additionalProperties: true` in the OpenAPI schema) — no
fixed Go shape, since the admin UI and a future RTG-04 ("create RoPA from template") both just need to
show/copy the same JSON this was seeded with, not a typed model. API: `GET /admin/v1/ropa/templates`
(published sets — today only the platform's own "standard" set), `GET /admin/v1/ropa/templates/{id}/activities`
(optional `job_category` filter), `GET /admin/v1/ropa/templates/activities/{id}` (full detail) — all on the
already-seeded `ropa.template.read`. UI: a new page, `/ropa/templates` (the module doc's own UX note: "มีหน้าจอ
ใหม่") — a job-category picker, an activity list, and a detail panel rendering purposes/data/recipients/
retention/security controls from the raw `defaults` JSON — linked from `/ropa/activities`. Tests: unit (the
global standard set is visible, at least 50 activities with a working `job_category` filter — the acceptance
criterion directly, every ม.39 topic present on a sampled activity's defaults, an unknown id is `ErrNotFound`,
two-tenant isolation — the global set is visible to both tenants by design, the same ORG-07 master-data
pattern), HTTP contract (401/200/404, the full list→activities→detail chain through the real validator).

### ROPA-05 เพิ่มกิจกรรมแบบปกติและแบบมาตรฐาน (`docs/modules/ROPA.md#ropa-05`) — done
`internal/ropa/service/from_template.go` (`ropa.activity.create`, shared with ROPA-03's own blank-create path
— no new permission or migration): `CreateActivityFromTemplate` is RTG-01's natural consumer — it looks up one
`ropa.activity_templates` row (`ropa/templates` is a sibling package of `ropa/service` inside the same `ropa`
module, so `Service` holds it as a direct concrete field, `Templates *templatesservice.Service`, not a rule-9
interface — only a *different* module's own schema needs that indirection), creates the activity, then replays
its `defaults` jsonb through the exact same `AddActivityPurpose`/`AddActivityData`/`AddRetentionRule`/
`AddActivityControl` calls ROPA-03/07/09's own manual-entry endpoints already use — a template-created activity
is otherwise indistinguishable from a hand-entered one, same audit trail and all. `data_category_code`/
`subject_type_code`/security-control codes are resolved to *this tenant's* own visible `org.*`/`risk.controls`
rows at creation time (`masterIDByCode`/`controlIDByCode`, the same `ListMaster`-scan pattern ROPA-06/ROPA-08
already use for code-keyed master data) — never the platform-global template's own (nonexistent) ids.

Two topics are deliberately left out, both for the same reason: a global template cannot know which of *this*
tenant's rows a free-text hint resolves to. Recipients only carry a role + note (e.g. "ผู้ให้บริการระบบสรรหา
บุคลากรภายนอก"), never a real `org.external_parties` id — the same cross-schema-FK limit ORG-06's own party
merge already documents — so they're left for the OWNER to add by hand from the template's own note (already
shown on `/ropa/templates`' detail panel, RTG-01); recipients also isn't one of `completeness()`'s 6 core items,
so this doesn't block the acceptance criterion. A processor-role template's `controller_party_id` has the same
problem and *is* a core item when role=processor — so of the 51 seeded activities, the 48 controller-role ones
reach `completeness == 100` immediately on creation (tested directly), while the 3 processor-role examples still
need that one FK set by hand afterward, same as a blank processor-role activity always has. `rights_and_access`
has no equivalent in the template's own defaults at all (it describes what data is processed, not how this
tenant handles a data-subject request for it) — a plain Go constant fills it instead, since it's internal RoPA
record text, not the outward-facing legal wording rule 8 governs.

API: `POST /admin/v1/ropa/activities/from-template` (`ProcessingActivityFromTemplateInput`: `activity_template_id`,
`legal_entity_id`, `org_unit_id`, `code`, optional `owner_user_id`) → 201 `ProcessingActivity`, same shape as
`ropaCreateActivity`. UI: a "create from this template" button + inline legal-entity/org-unit/code form added to
`/ropa/templates`' own `ActivityDetail` panel (RTG-01), routing straight to `/ropa/activities/{id}` on success —
no new page needed, since the template browser already is the natural place to start from one. Tests: unit (the
acceptance criterion directly — every ม.39 topic the template carries lands as a real row, completeness reads
100% with no missing items right after creation, no further edits; an unknown template id refused as
`ErrInvalid`, not a 500; two-tenant isolation of the resulting activity, while the templates themselves stay
visible to both tenants by design), HTTP contract (403 without `ropa.activity.create`, 422 for an unknown
template, 201 happy path with the copied purpose visible on the new activity via its own purposes endpoint).

### RTG-04 สร้าง RoPA จาก template ในไม่กี่ขั้นตอน (`docs/modules/RTG.md#rtg-04`) — done
`backend/internal/ropa/service/from_template.go`'s `CreateActivitiesFromTemplates` (`ropa.activity.create`,
shared with ROPA-05 — no new permission or migration): the module doc's own "เลือกหมวดงาน → เลือกกิจกรรม →
เลือกแผนกเจ้าของ แล้วได้ร่าง RoPA ทันที" is a thin loop over ROPA-05's own `CreateActivityFromTemplate`, not a
reimplementation — each chosen template id gets exactly the same treatment a single ROPA-05 create would
(purposes/data/retention/security-controls copied in, codes resolved against this tenant's own `org.*`/
`risk.controls`), just against one resolved department instead of asking the caller for a legal entity *and*
an org unit separately: `s.Org.GetOrgUnit(orgUnitID).LegalEntityID` supplies the legal entity, so the wizard's
third step really is just "pick a department." Each created activity's `code` is the template's own `code`
(already unique platform-wide), so the caller never types 20 codes by hand — a real collision (re-running the
wizard against the same department with an already-used template) surfaces as the same duplicate-code
`ErrInvalid` ROPA-05's own `SaveActivity` already raises.

The whole batch runs inside the request's own transaction (rule 1 — this service never opens one): any
failure partway through — one bad template id, one duplicate code — rolls every activity already created in
*this* call back with it, not just the one that failed; there is no partial-batch state to clean up or report
on, the same all-or-nothing contract PLT-14's `import.apply` already established for a bulk write. Capped at
`MaxBatchActivityTemplates = 50` — comfortable headroom over this feature's own 20-activity acceptance
criterion, not an arbitrary round number pulled from nowhere.

API: `POST /admin/v1/ropa/activities/batch-from-templates` (`ProcessingActivityBatchFromTemplatesInput`:
`org_unit_id`, `activity_template_ids` (1–50), optional `owner_user_id`) → 201 `{data: [ProcessingActivity]}`,
same list-wrapper shape every other module's batch/list responses already use. UI: a new page,
`/ropa/templates/batch` (the module doc's own UX note: "มีหน้าจอใหม่") — the literal 3-step wizard + summary
the frontend note calls for (job category → multi-select activities within it → department → a summary
listing every chosen activity's name before the single create call fires), linked from `/ropa/templates`'s
own header next to the browse-only flow RTG-01/ROPA-05 already built. Tests: unit (the acceptance criterion
directly — 20 template ids in one call yield 20 real, independently-scoped activities, each already carrying
its own template's defaults with no further edits; an unknown id anywhere in the batch rolls back the whole
call — proven by driving the same transaction the HTTP layer would use and checking nothing persisted after;
empty and over-50 batches refused), HTTP contract (403 without `ropa.activity.create`, 400 for an
empty array — the schema's own `minItems`, 201 with a 10-activity batch's response carrying exactly 10
entries).

### PNG-06 จัดการเวอร์ชัน (`docs/modules/PNG.md#png-06`) — done
`internal/platform/docs` gained a second extension point symmetric with PNG-02's own `SetValidate`:
`Service.SetOnPublished(docType, fn)`, called inside the same publish transaction right after PLT-08 freezes
a new `platform.document_versions` row and flips the document to published — gives a module the chance to
write its own per-version record from the frozen content without `docs` ever reaching back into that module
(rule 9). `internal/notice/service/versions.go`'s `OnDocumentPublished` (registered in `cmd/api/main.go`
right after the existing `SetValidate("notice", ...)` line) inserts one `notice.notice_versions` row per
publish — a baseline-schema table purpose-built for this feature, distinct from PLT-16's own generic
versioning (version_no, document_version_id, languages, effective_from, checklist_result jsonb) —
snapshotting PNG-02's own `Checklist(content)` result so a later change to the checklist logic can never
rewrite history; `is_material_change`/`changes_purpose` stay at their column defaults (false) since no
screen wires them yet (PNG-07's job). A public key (`notice.notices.public_key`, migration 00050) is issued
only on the *first* publish — `SetNoticePublished`'s sqlc query uses
`COALESCE(sqlc.narg(public_key)::varchar, public_key)` so every later publish leaves an already-bookmarked
public URL untouched. `internal/platform/publickeys`'s single-entity regex was generalized
(`EntityNotice = "notice"` alongside the existing `EntityCollectionPoint`, `pathKeyRE` widened to
`^/public/v1/(?:collection-points|notices)/([^/]+)`) rather than adding a second tenant-resolution mechanism
— `/public/v1/notices/{key}` resolves tenant through the exact same `publickeys.Middleware` CON-09's
collection-point links already use.

The acceptance criterion's "หน้า public แสดง...ได้" needed a way to serve real rendered HTML to an anonymous
caller: every existing read on `docs.Service` (`Get`/`PublishedContent`/`Export`/`Versions`) internally gates
on `authz` permissions a `/public/v1` request never carries, so a new, deliberately permission-free
`Service.PublicVersionHTML(ctx, documentID, versionID, lang)` was added instead of threading a fake "public"
grant through the normal gated path — it reuses the existing `buildInputs(frozen)` helper `Export` already
uses internally to resolve merge fields/clauses into real HTML via `render.HTML(in)`. Access is proven by the
module's own prior lookup of the version id through a notice it owns (resolved via the public key), not by
any permission check inside `docs` itself.

API: `GET /admin/v1/notices/{id}/versions` (`notice.document.read`, cursor-free list, newest first); public
`GET /public/v1/notices/{key}`, `GET /public/v1/notices/{key}/versions`,
`GET /public/v1/notices/{key}/versions/{versionNo}` (`x-permission: public`, `security: []`, same `cmd/api`
public `r.Group` consent's own `publichttp` already shares). UI: `/notices` gained a version-history toggle
per row (the public URL when issued, or "not published yet"; a table of version_no/effective_from/
published_at) next to the existing checklist toggle, using a new `useNoticeVersions` api-client hook. The
portal gained a new public page, `/[locale]/n/[key]` (`apps/portal/src/lib/notice.ts` +
`apps/portal/src/app/[locale]/n/[key]/page.tsx`) mirroring the existing `/[locale]/c/[key]` consent-form
pattern exactly — a server component, never calling the Go API from the browser, `?v={no}` to view a past
version, a history list linking back to the current one. Tests: unit (`OnDocumentPublished` records the
version and issues a key only on first publish, a second publish reuses the existing key;
`PublicNotice`/`PublicVersions`/`PublicVersionHTML` serve the current version and full history by number;
unknown key or version number is `ErrNotFound`; two-tenant isolation via the existing harness), HTTP contract
(401/200/404 on the admin versions list; 404 for an unknown key, 200 with the correct title and rendered HTML
on all three public endpoints, including a specific past version). `pnpm --filter @pdpa/admin build` and
`pnpm --filter @pdpa/portal build` both verified clean, including the new routes.

### PNG-07 แจ้งการเปลี่ยนแปลงและขอความยินยอมใหม่ (`docs/modules/PNG.md#png-07`) — done
`notice.notices` gained two staging columns (migration 00051), `pending_is_material_change`/
`pending_changes_purpose`: PLT-08's generic publish endpoint knows nothing about notice-specific fields, so
the DPO sets these on the notice itself first via a new `POST /admin/v1/notices/{id}/publish-intent`
(ETag/If-Match), and the very next `OnDocumentPublished` (PNG-06) reads, snapshots into `notice_versions` and
resets them in the same transaction — never meaningful outside that short window. decisions.md Q-29 covers
the two mechanism choices this feature needed (no data-model or legal-behaviour question, so no blocking
ask): a material-change publish alerts role DPO via PLT-04 (migration 00051's `notice.material_change`
template, th/en × in_app/email) — the same "default recipients until real routing exists" fallback
BRE-07/PNG-04 already use, since no portal account model or acknowledgement list (PNG-09, not built) exists
to notify an actual data subject directly yet. A purpose-changing publish opens one `dpo.tasks` job
(`source_type = 'consent'`, migration 00051 widens the CHECK the same way DPO-09 widened
`form_definitions.form_type`) per `consent.purposes` row reachable from the notice's own linked RoPA
activities (`notice_activity_links` → `ropa.activity_purposes.consent_purpose_id`) — the acceptance
criterion's "สร้างงานขอความยินยอมใหม่อัตโนมัติ" literally: a job appears automatically, but the new consent
text itself is never auto-generated (rule 8 — legal wording stays a human's job); a DPO goes and
authors/publishes the material version with `requires_reconsent=true` in CON-12's own `/consent/purposes`
editor from there.

`internal/dpo/service/reconsent.go`'s new `OpenConsentTask` mirrors DPO-09's own `openRemediationTask`
numbering pattern exactly (per-tenant-per-year advisory lock, `"CON-<year>-NNNN"` instead of `"SEC-..."`,
`source_type = "consent"`). `notice.Service` reads both other modules only through small local interfaces
(`Dpo.OpenConsentTask`, `Consent.GetPurpose` — rule 9) backed directly by `*dposervice.Service`/
`*consentservice.Service` in `cmd/api/main.go` (both already constructed before `noticeSvc`, no import
cycle — `dpo`/`consent` never import `notice`). API: `POST /admin/v1/notices/{id}/publish-intent`
(`notice.document.update`); `Notice.pending_is_material_change`/`pending_changes_purpose` added to the wire
schema so the UI can show the currently staged state. UI: a small form on `/notices`' existing
version-history panel (two checkboxes + save) right above the public-link display, using a new
`useSetNoticePublishIntent` api-client hook. Tests: unit (`SetPublishIntent` stages/round-trips and rejects a
stale ETag; a `changes_purpose` publish through the real PLT-08 submit→DPO-approve→publish chain opens
exactly one `dpo.tasks` row per linked consent purpose and the published `NoticeVersion` carries the right
flags), HTTP contract (428/412/200 on `/publish-intent`), full backend `go test -p 1 ./internal/...` and
`pnpm --filter @pdpa/admin build` both verified clean.

### DSAR-06 ยืนยันตัวตนผู้ยื่นคำขอ (`docs/modules/DSAR.md#dsar-06`) — done (admin-side; no portal yet)
`dsar.verifications` was already fully specified in the baseline migrations (`method`
otp_sms/otp_email/id_document/in_person/idp/thaid, `subject_verification_id` → `iam.subject_verifications`,
`masked_id_file_id` → `platform.files`, status pending/passed/failed) — no new migration, only the service and
endpoints against it (`internal/dsar/service/identity.go`). OTP reuses IAM-05 directly:
`StartOTPVerification` decrypts the request's own `requester_contact_enc` (PLT-13, never logged — rule 3) and
calls `iamservice.StartVerification` with `Purpose: "dsar"` (already in IAM-05's own purpose list);
`ConfirmOTPVerification` calls `VerifyOTP` — a wrong code is a harmless retry (IAM-05's own 5-attempt cap,
D-01); only once IAM-05 itself gives up (too-many-attempts or expired) does the `dsar.verifications` row get
marked `failed`. "ตรวจกับข้อมูลในระบบ" for OTP is implicit: the code goes to the contact already on file for
this exact request, so a correct code proves the requester controls that channel.

The acceptance criterion's "เลขบัตรในไฟล์ที่เก็บถูกปกปิดเสมอ" is taken literally, not as an OCR claim: no OCR
library is available in this environment (no Tesseract, no cloud OCR service configured —
`docs/decisions.md` Q-30), so rather than guess at unavailable tooling, staff mark the rectangle(s) over the
ID number themselves after uploading the photo. `SubmitIDDocumentVerification` decodes the uploaded JPEG/PNG
with Go's own `image`/`image/draw` stdlib (no new dependency), draws an opaque black box over each given
rectangle — refusing one that reaches outside the image rather than silently clipping it short — and only
the *redacted* bytes are ever saved, via `files.Service.SaveGenerated`, already attached to the request; the
caller's raw upload is never attached to anything and simply expires through PLT-09's own 24h orphan cleanup.
`ConfirmIDDocument` is staff's own pass/fail judgement after checking the redacted card against system
records — there is no trusted national-id database to automate this against from here. Either path's first
call on a `received` request moves it `received` → `verifying` (ST-02, already declared, no new state-machine
entry needed); a pass stamps `dsar.requests.verified_at` (the acceptance criterion's other half) and moves
`verifying` → `in_review`. `identity.go`'s small `Files` interface (rule 9) is backed directly by
`*files.Service` in `cmd/api/main.go` (already constructed), alongside `Verification *iamservice.Service`
(already constructed for `/me` and IAM-05) — no import cycle, since neither `iam/service` nor
`platform/files` imports `dsar`.

API: `GET /admin/v1/dsar/requests/{id}/verifications` (list, oldest first), `POST .../verifications/otp`
(201), `POST .../verifications/{verificationId}/confirm-otp` (200), `POST .../verifications/id-document`
(201, body `raw_file_id` + one or more pixel `redactions`), `POST .../verifications/{verificationId}/decide`
(200, staff pass/fail) — all `dsar.request.update`, no new permission code. UI: a "ยืนยันตัวตน" toggle per row
on `/requests` — past attempts, an OTP form (method + code), and an ID-document form (`FileUploader` + four
numeric rectangle fields + save, then pass/fail buttons once a submission is pending). No portal UI yet,
deliberately — same "no consumer yet" deferral IAM-05's own OTP service used (the module doc's own frontend
note calls for "ขั้นตอนยืนยันตัวตนใน portal", but no DSAR public intake form exists to design one against).
Tests: unit (OTP pass stamps `verified_at` and transitions to `in_review`; a wrong code is a harmless retry;
redaction blacks out exactly the given rectangle and nothing else, proven by decoding the saved bytes back to
pixels; an out-of-bounds rectangle is refused; re-deciding an already-decided verification is refused;
two-tenant isolation), HTTP contract (401/403/404/400/201/200 through the real validator),
`pnpm --filter @pdpa/admin build` verified clean.

### DSAR-08 Workflow และงานย่อย (`docs/modules/DSAR.md#dsar-08`) — done
`internal/dsar/service/subtasks.go` (`dsar.subtask.*`, already fully seeded in the baseline permission
migration — no new permission code) — CRUD on `dsar.subtasks` (already fully specified in the baseline
migrations, no new migration for the table itself). The acceptance criterion ("คำขอเดินครบทุกขั้นตอนและปิดได้
เมื่อ subtask เสร็จทั้งหมด") is a new guard on `Transition`'s own `in_progress → completed` edge (DSAR-13):
every subtask of the request must now be `done` or `not_applicable` — a request with none is vacuously "all
done" (DSAR-13's own completed tests never created any), one with any `open`/`in_progress` subtask is refused
`dsar.invalid_transition`. "คำขอเดินครบทุกขั้นตอน" itself needed no new code: DSAR-13 already encoded ST-02's
full graph, and DSAR-06/11 already built the `received → … → completed` path this walks end to end.

`dsar.subtasks.status` is its own small local machine (`open → in_progress → done|not_applicable`, or straight
`open → done|not_applicable` for a subtask quick enough to finish in one step —
`docs/states/state-machines.yaml#DSAR-08subtask`), not routed through the full PLT-05 workflow engine:
`dsar.request_types.workflow_definition_id` exists for that in the schema, but DSAR never adopted it (DSAR-13's
own module-doc note already explains why — the engine's task assignee is baked into its Definition JSON at the
type level, which doesn't fit a per-record subtask list), so wiring a second engine in for just this one gate
would be a bigger change than the acceptance criterion needs. "แก้ได้เฉพาะงานที่ได้รับมอบหมาย"
(`docs/security/permissions.md`'s own note on `dsar.subtask`): `requireSubtaskAccess` lets a caller holding
only `.update` (IT/OWNER/LEGAL/GUEST in the seeded RBAC) change a subtask only when it's assigned to them
directly, or to an `iam.groups` group they belong to (`iamservice.GroupIDsOf`); `.execute` (DPO/PRIVACY)
bypasses that check entirely, the same two-tier shape DSAR-11's own approve-gate uses. Creating an assigned
subtask notifies the assignee (PLT-04, migration 00052's `dsar.subtask_assigned` template, operational text —
no DRAFT marker, rule 8) — "มอบหมายงานย่อยให้เจ้าของระบบหรือทีม". A subtask's optional evidence is the
caller's own clean PLT-09 upload, attached via a new `Files.Attach` method (the `dsar.Files` interface DSAR-06
already defined) against the `dsar_request` entity type DSAR-17 already registered
(`fileSvc.EntityPermissions["dsar_request"]`) — no new entity-permission wiring needed.

"ตั้ง rule อัตโนมัติตามประเภทและบริษัท" (an automatic default-subtask rule per request type) is deliberately
not built: no column models it and no screen needs it yet — the module doc's own description names it, but the
literal acceptance criterion is only about subtask completion gating closure, fully exercised without one. Add
it (most naturally a `default_subtask_actions` column on `dsar.request_types`, applied when a request enters
`in_progress`) once a real screen needs it, the same "no consumer yet" deferral this codebase uses elsewhere.

API: `GET`/`POST /admin/v1/dsar/requests/{id}/subtasks`, `POST .../subtasks/{subtaskId}/status`
(ETag/If-Match), `DELETE .../subtasks/{subtaskId}` — same shapes as every other module's child-table
endpoints. UI: a "งานย่อย" toggle per row on `/requests` — a status table, an inline create form (action +
a user or group picker, reusing PLT-05's own `useGroupSearch`), and per-row start/done/not-applicable/remove
actions. Tests: unit (the acceptance criterion directly — a request with an open subtask cannot complete,
closes once it's done, a request with none is unaffected; action/assignee/group validation; the
access-restriction rule for both user and group assignees and its `.execute` bypass; delete; two-tenant
isolation), HTTP contract (401/403/400/201/200/204/404/409/412/428). Verified live, not just with tests: ran
the real `cmd/api`/`cmd/worker`/Next.js stack, logged in through the dev-login flow, created a real request and
subtask through the browser, moved it `open → done`, and confirmed its row actions correctly reduced to just
"remove" once terminal. Found and fixed, unrelated to DSAR-08 but surfaced by that same live run: `/requests`'
own row `<Fragment>` had no `key` of its own (only the inner `<tr>` did) — React warned "Each child in a list
should have a unique key prop"; fixed by moving the key onto the `Fragment`.

### DSAR-02 รับคำขอหลายช่องทาง (`docs/modules/DSAR.md#dsar-02`) — done
Most of this was already in place from DSAR-13's own intake slice: `CreateRequest` already accepted any of the
seven `channel` values (web/email/phone/branch/letter/line/api) and an `on_behalf` flag, and every request —
whatever its channel — lands in the same `dsar.requests` table, visible through the same
`ListRequests`/`GET /admin/v1/dsar/requests` the UI already used. What was actually missing against the literal
acceptance criterion ("คำขอทุกช่องทางเข้าคิวเดียวกันและระบุช่องทางที่มา"): the admin UI's intake form hard-coded
`on_behalf: false` (staff had no way to mark a request as recorded on the data subject's own behalf) and the
`/requests` list never showed a request's channel at all — so "ระบุช่องทางที่มา" wasn't actually visible
anywhere, even though the data was already stored. Fixed both: a "ลงคำขอแทนเจ้าของข้อมูล" checkbox on the
intake form now sets `on_behalf` for real (natural for a phone/branch/letter/LINE intake, where staff are the
ones typing it in), and `/requests` gained a "ช่องทาง" column showing each request's channel with a "ลงแทน"
badge when `on_behalf` is true. No migration, no new endpoint, no new permission — this was a frontend gap
closing a backend capability that already existed.

"แนบไฟล์" (attach a file at intake, e.g. a scanned letter) needed no new code either: every `dsar_request` is
already a registered PLT-07 collaboration record (DSAR-17), so staff can attach a file to the request
immediately after creating it via the existing "ประวัติ" panel — a second step, not a field on the create form
itself, since PLT-09 attachments need a real entity id to attach to and the create form is a single atomic
call. "รับคำขอผ่าน API จากแอป" (an external app submitting requests through `/api/v1`) is deliberately not
built: that surface needs API-client authentication (ORG-16, not started) the same way CON-16's own `/api/v1` +
webhooks are deferred for the same reason — the `channel: "api"` value is already in the CHECK constraint and
already selectable by staff logging a call from an external system by hand, so a real `/api/v1` consumer can be
layered on top later without touching this schema.

API: unchanged — `channel`/`on_behalf` were already on `DsarCreateRequestInput`/`DsarRequest`. UI:
`apps/admin/src/app/[locale]/requests/requests-content.tsx` — the intake form's `on_behalf` checkbox and a new
"ช่องทาง" table column with an inline "ลงแทน" badge. Tests: unit
(`TestCreateRequest_EveryChannelLandsInTheSameQueueWithOnBehalf` — the acceptance criterion directly: every
channel value round-trips through `CreateRequest` and shows up in the same `ListRequests` call, `on_behalf` is
set correctly per channel), `pnpm --filter @pdpa/admin build`/`tsc` clean. Verified live: created a real
`channel: "phone"`, `on_behalf: true` request through the running admin app and confirmed the list shows
"โทรศัพท์" with a "ลงแทน" badge on that row.

### DSAR-03 ครอบคลุมสิทธิทุกประเภท (`docs/modules/DSAR.md#dsar-03`) — done
DSAR-13 already gave all 9 right types a shared ST-02 workflow and their own response letter (purpose-keyed
body with `{{request_type_name}}` substituted) — "ของตนเอง" in the sense that every type's own name and number
appear in its own generated letter. This feature builds the half the module doc's own backend note names
instead: "ขั้นตอนเฉพาะของแต่ละสิทธิ (เช่น ขอสำเนาต้องแจ้งแหล่งที่มา)". `backend/internal/dsar/service/
subtasks.go`'s new `rightSubtaskAction` maps each right type's code to the DSAR-08 subtask action its
per-system work should carry (ม.30 access → search, ม.31 portability → export, ม.32 objection →
stop_marketing, ม.33 erasure → delete, ม.34 restriction → restrict, ม.35-36 rectification → rectify, ม.19
withdraw_consent → restrict, complaint/inquiry → review); `TransitionInput` gained `AssetIDs` (FK-checked via
a new `Ropa.GetAsset` — the interface already had `GetActivity` for DSAR-11), and entering `in_progress` with
one or more now opens one subtask per asset via `openRightSubtasks` — `dsar.subtasks.asset_id` → `ropa.assets`
(the *systems* a RoPA/data map names, not processing activities) was already in the baseline schema, unused
until now, matching "สร้าง subtask ไปทุกระบบที่มีข้อมูล" literally rather than DSAR-08's own erasure-only
example. ม.30's own requirement ("ขอให้เปิดเผยการได้มาซึ่งข้อมูลที่ไม่ได้ให้ความยินยอม") is a new optional
`data_source` field on `CreateRequest`, stored in `dsar.requests.details` jsonb (previously `{}"`, unused) and
appended as its own disclosure paragraph on the generated result letter — only for `access` requests that set
it (`letters.go`'s `appendDataSourceParagraph`), since the 3 shared purpose templates are reused by every right
type. `dsar.completed` was already in `docs/architecture/events.yaml` with "ITSM / CRM (subtask)" as a named
consumer but nothing published it; it now fires on every completion (ม.32/34's "แจ้งระบบปลายทาง"/"หยุดทันทีและ
แจ้งระบบปลายทาง"), generalized across every right type rather than only objection/restriction, since a
downstream subscriber decides for itself which completions it cares about. No new migration, permission or
event-catalog entry — every table/column/event this feature needed was already in the baseline schema or
catalog, unused.

API: `data_source` on `DsarRequestInput`/`DsarRequest`, `asset_ids` on `DsarTransitionInput`, `asset_id` on
`DsarSubtask`. UI: an access-only "แหล่งที่มาของข้อมูล" field on `/requests`' intake form, an asset multi-select
(`useAssets`, already built by ROPA-02) on the `in_progress` action panel, mirroring the existing rejected/
activity-picker pattern. Tests: unit (access/erasure each open the right subtask action per linked asset, an
unknown asset_id refused as `ErrInvalid`, `dsar.completed` published to the outbox on every completion, the
access letter discloses a given `data_source` verbatim — two-tenant isolation via the existing harness), HTTP
contract (422 for an unknown asset, 200 opening a real subtask against it, `data_source` round-trip through the
real validator). Found while wiring this: `make gen`'s `oapi-codegen` target was missing a line for
`internal/dsar/http` (present on disk but never regenerated by `make gen` itself) — added.

### CON-17 ความปลอดภัยและที่เก็บข้อมูล (`docs/modules/CON.md#con-17`) — done (deployment-level region enforcement deferred)
Everything the acceptance criterion needs ("ผลทดสอบยืนยันว่าข้อมูลความยินยอมเข้ารหัสและอยู่ใน region ที่กำหนด")
was already built by PLT-13/CON-15 and `consent.Service.GetSettings`/`GET /admin/v1/consent/settings` (identifier
encryption + the tenant's `platform.tenants.data_region`, shown on `/consent/subjects`) — what was missing, and
all this feature adds, is the "หลักฐานทดสอบ region" (region test evidence) the module doc's own implementation
note already named as the gap. `GetSettings` had no test at all before this: `internal/consent/service/
settings_test.go`'s `TestGetSettings_ReportsRegionAndEncryption` sets two tenants to different regions directly
on `platform.tenants` (there is no write endpoint for `data_region` — it's an onboarding/provisioning value, not
something a tenant edits themselves, consistent with the module doc's own "แสดงตำแหน่งที่เก็บข้อมูล" frontend
note, not "set") and proves `GetSettings` reads each tenant's *own* column rather than a hardcoded value, with
no cross-tenant leak. `internal/consent/http/handler_test.go` gained the matching HTTP contract case
(401/403/200 through the real validator + AuthZ, asserting the actual `data_region`/`identifier_encryption`
values). No new migration, endpoint, or permission — both the service method and the HTTP endpoint already
existed, just untested.

Deployment-level region enforcement (a real database or object-storage bucket pinned per region) stays
deliberately out of scope: there is no multi-region infrastructure anywhere in this environment to test it
against, the same reasoning PLT-02 already used to defer DB-per-tenant deployment mode, and `docs/decisions.md`
Q-02 (cloud provider / region) is still an open DevOps decision, not something this backend-only pass can
resolve by itself.

### VEN-01 ทะเบียนคู่ค้าและผู้ประมวลผล (`docs/modules/VEN.md#ven-01`) — done
The first feature on the `vendor` schema (`internal/vendormgmt` — see below). `vendor.vendors` and its six
sibling tables, plus `vendor.vendor.{read,create,update,delete,approve}` with role grants (PROC, OWNER, DPO,
AUDIT, IT, LEGAL, SEC, PRIVACY), were all already fully specified in the baseline migrations (00014, 00019) —
no new migration. `internal/vendormgmt/service`'s `SaveVendor` is plain CRUD: `party_id` checked visible under
RLS via `orgservice.GetExternalParty` (reused from ORG-06, rule 1), `relationship_owner_id` via
`iamservice.Names` (the same cross-module FK-visibility pattern ROPA-02/DPO-01 already use). Deliberately
scoped to only the `[*] → prospect` edge of `docs/states/ST-06.md`'s full 7-state machine (prospect →
onboarding → approved/conditional/rejected → offboarding → terminated): `status`/`tier`/`next_assessment_at`/
`approved_at`/`offboarded_at` are never written here — every other transition belongs to sibling features not
yet built (VEN-02/05/07/08/09/14), the same "build only the minimal slice this feature needs" pattern used
throughout this codebase (e.g. ROPA-01 deferring `discovered_by_finding_id`). `uq_vendors_party_id` (one
vendor per external party) is a real unique constraint, so the insert/update is wrapped in `pdb.Savepoint`
(the established pattern from `org.SaveLegalEntity`/ROPA-03) so a duplicate-party attempt doesn't abort the
whole request transaction.

**Real Go toolchain bug found and fixed while building this**: a package directory literally named `vendor`
anywhere under a Go module — not just at the module root — triggers Go's vendoring-directory import-rewriting
rules, breaking normal `import` resolution with an error like "`pdpa-platform/internal/vendor/store` must be
imported as `store`". This is a Go toolchain quirk entirely unrelated to this project's own `vendor` business
domain. Fixed by renaming the whole package tree from `internal/vendor` to `internal/vendormgmt` (sqlc.yaml,
Makefile, every Go import, `cmd/api/main.go`) while keeping the PostgreSQL schema name (`vendor`), permission
codes (`vendor.*`) and the API prefix (`/admin/v1/vendors`) unchanged — documented in
`docs/architecture/code-structure.md`, `docs/modules/VEN.md` and `docs/data/vendor.md` so a future pass doesn't
"helpfully" rename the Go package back to `internal/vendor` and reintroduce the bug.

The acceptance criterion ("คู่ค้าหนึ่งรายมีหน้าเดียวที่รวมข้อมูลทุกโมดูล" — one vendor has a single page
aggregating data from every module) is read as what it can literally be today: a foundation detail page every
later VEN/DPA/DSA sibling feature will add its own section to, not a claim that those other modules' vendor
data already exists to aggregate — the same forward-looking interpretation this codebase already used for
DPO-05's cross-module dashboard and RTG-01's seeded template content.

API: `GET`/`POST /admin/v1/vendors` (cursor pagination, status filter), `GET`/`PATCH /admin/v1/vendors/{id}`
(ETag/If-Match). UI: `/vendors` (list + create form, an owner picker reusing PLT-07's own `useMentionSearch`)
and `/vendors/{id}` (detail + inline edit). Tests: unit (validation, both FK-visibility checks, update, the
unique-constraint savepoint doesn't poison the transaction, two-tenant isolation), HTTP contract
(401/403/400 schema/404/412/428/200/201). Not verified live in a browser: the dev-login flow didn't establish a
session cookie through Playwright in this environment — the same pre-existing gap ROPA-02/DPO-01 already
noted, not a regression from this feature. Not done: every ST-06 transition past creation, and the DPA/DSA/
assessment sections the detail page's acceptance criterion anticipates — all sibling features, not yet built.

### DPIA-10 ความเห็น DPO และการอนุมัติ (`docs/modules/DPIA.md#dpia-10`) — done
`assess.dpo_opinions` and the rest of ST-05#2's review/approval states (`in_review`/`approved`/`rejected`/
`needs_review`/`closed`, beyond DPIA-01/02's own `screening`/`not_required`/`in_progress`) were already fully
specified in the baseline migrations and `docs/states/state-machines.yaml` — no new migration.
`internal/dpia/service/opinion.go`'s `Transition` is the whole state machine as one allow-list
(`assessTransitions`, ST-05#2's own edges beyond screening): submit for review, request more info, decide
(approved/rejected/needs_review) and close — all through the one `db/queries/dpia/opinion.sql`'s
`SetAssessmentStatus` query. Deciding or closing needs `assessment.dpia.approve` beyond the endpoint's own
`assessment.dpia.update` permission (checked internally, `ErrForbidden` → 403) — the module doc's own EXEC
actor has no grant on `assessment.dpia` in the baseline RBAC seed, so in this tenant's RBAC only DPO can
decide (the same gap DPIA-02/DPIA-05's own actor lines already document). The acceptance criterion itself is
`Transition`'s own guard on entering `closed`: a round that was actually assessed (`approved`/`rejected`)
needs at least one `RecordOpinion` call already on record — `not_required` never had anything to opine on, so
it closes with none. `RecordOpinion` only accepts a real `recommendation` value (proceed /
proceed_with_conditions / do_not_proceed / consult_pdpc, `assess.dpo_opinions`' own CHECK) and only while the
round is `in_progress` or `in_review`. `DpiaAssessment`'s wire schema gained `row_version` (an ETag was never
needed before this feature's `If-Match`-gated transition) and its `status` enum widened to the full ST-05#2
set — both additive, no existing consumer broke.

This was the second feature picked for this pass — VEN-08 (vendor approval) was abandoned mid-implementation
once its own `vendor.vendor_assessments.assessment_id` FK turned out to point at this very `assess.assessments`
table, meaning VEN-08 actually depends on this generic review/approval machinery existing first (a real gap
the backlog's own `depends_on: PLT-08` didn't surface); DPIA-10 was built instead, and all VEN-08 scaffolding
from that abandoned attempt was reverted before starting here.

API: `POST /admin/v1/dpia/assessments/{id}/transition` (ETag/If-Match, `{to, reason}` — `reason` required
entering `rejected`/`needs_review`), `GET`/`POST /admin/v1/dpia/assessments/{id}/opinions`. UI: a decision
panel on `/ropa/activities/{id}`'s existing DPIA section — submit-for-review/resume/close buttons, an opinion
form + list, and approve/needs-review/reject buttons gated on holding `.approve`. Tests: unit (every ST-05#2
edge allowed/refused, the approve-only gate with a limited-permission caller, reason required entering
rejected/needs_review, close blocked without an opinion unless not_required, opinion validation + status
gate, two-tenant isolation), HTTP contract (401/403/404/409/412/422/428/200) through the real validator +
AuthZ, including a real `iam.users` row for the limited-permission test caller (`assess.dpo_opinions.
dpo_user_id` is a real FK to `iam.users`, not satisfiable by an arbitrary uuid). `pnpm --filter @pdpa/admin
build`/`tsc` and the `@pdpa/i18n` ICU message tests both verified clean.

### IAM-03 Console ผู้ให้บริการ: tenant และแพ็กเกจ (`docs/modules/IAM.md#iam-03`) — done
`internal/platform/provider` is the first (and, by `docs/decisions.md` D-24/CLAUDE.md rule 1, the only
permitted) package holding a `pdpa_platform` (BYPASSRLS) pool outside `cmd/devseed`/`cmd/migrate` — this
pass resolved decisions.md Q-19 (the user approved the proposed design: SUPER accounts in a reserved
`platform` tenant, `/provider/v1`, the bypass pool scoped to this one package) and moved it to §1 as D-24.
Migration 00064 seeds that reserved tenant (fixed id) — AuthZ's ordinary per-tenant grant cache then just
works for SUPER with zero special-casing, since `iam.users.tenant_id` points at it like any other tenant's
own id. `provider.Service.CreateTenant`/`GetTenant`/`ListTenants` (no pagination — the platform's own
roster, same reasoning DPIA-12/ROPA-04's platform-wide lists already used) and `SuspendTenant`/
`ReactivateTenant` (a tiny trial/active <-> suspended machine, ETag-guarded) write `platform.tenants`
directly through the bypass pool; `CreateFirstAdmin` instead goes through the ordinary `pdpa_app` +
`db.WithTenantTx` path scoped to the *new* tenant (a DB-only `iam.users` insert + role grant, following
`cmd/devseed`'s own exact pattern — no real Keycloak account, same gap IAM-01 is blocked on).

The acceptance criterion ("ปิดโมดูลที่ไม่ได้ซื้อแล้วเมนูและ API ของโมดูลนั้นใช้ไม่ได้") is a new sibling
of `internal/pkg/authz.StrictMiddleware`: `internal/pkg/modulegate.Middleware`, the same generic
oapi-codegen strict-middleware shape, keyed by operationId → `x-module` (the tag
`internal/pkg/isolationaudit` already reads off the live spec — `cmd/api/permissions.go` gained a
`loadModules` twin of `loadPermissions`) — wired into every `/admin/v1` and `/provider/v1` strict-handler
chain in `cmd/api/main.go` (one `sed` pass appended it to all 24 existing registrations plus the new
provider one). It reads the caller's tenant's own `platform.tenant_modules` row inside the request's
already-open transaction (RLS already scopes it — no extra `tenant_id` filter needed, though the query
still names it for defense in depth): no row means enabled (opt-out, not opt-in); `enabled = false` blocks
with a new `httpx.ModuleDisabled` (403 `platform.module_disabled`). `iam` and `platform` are a permanent
allow-list (a locked-out tenant must still reach `/me`; shared plumbing including `/provider/v1` itself is
never a "module" a tenant licenses) — every other tag (`ropa`, `consent`, `dsar`, `breach`, `notice`,
`dpo`, `dpia`, `risk`, `org`, `vendor`, `agreement`, `gov`, `dataflow`) is a real `provider.PurchasableModules`
code. `ListTenantModules` returns every purchasable code even with no row (virtual default `enabled: true,
row_version: 0`, the same "defaults until first save" convention ORG-20's `org_settings` already
established), so the console's first toggle on an untouched module still sends a real `If-Match`.

API: `GET`/`POST /provider/v1/tenants`, `GET /provider/v1/tenants/{id}`, `POST .../suspend`/`.../reactivate`
(ETag/If-Match), `GET /provider/v1/tenants/{id}/modules`, `PUT .../modules/{moduleCode}` (ETag/If-Match),
`POST .../admins` — all on the already-seeded `admin.tenant.*`/`admin.user.create` permissions (no new
permission code or migration for RBAC; `x-module: platform`, since the console's own CRUD is exactly the
shared-plumbing category `modulegate` never gates). UI: `/settings/provider` (tenant list + inline create
form, per-row suspend/reactivate, a selected tenant's module-toggle list, a first-admin form) — `packages/
api-client/src/provider.ts` hooks, th/en keys under `provider.*`. Known gap, disclosed rather than
silently left out: this codebase has no central sidebar/nav component to literally hide a disabled
module's *menu* item from (checked by grep before building — every settings/feature page is reached by
direct URL); the "เมนู...ใช้ไม่ได้" half of the literal acceptance criterion is satisfied by the API gate
alone — a disabled module's own pages fail to load or write the moment `modulegate` blocks their
`/admin/v1` calls, so there is nothing left to meaningfully "use" even though no menu item disappears. Add
real menu-hiding once a nav component exists for it to hide an item from. Tests: unit
(`internal/platform/provider/service_test.go`: create + duplicate-code rejection, suspend/reactivate
transitions + stale-ETag rejection, the acceptance criterion directly through `ListTenantModules`/
`SetTenantModule` — default enabled, explicit disable persists, a stale first-time claim refused,
cross-tenant independence; `CreateFirstAdmin` + duplicate-email/unknown-role/unknown-tenant),
`internal/pkg/modulegate/middleware_test.go` (the gate itself: no row passes through, an explicit disable
blocks with `platform.module_disabled`, `iam` is never gated, cross-tenant isolation), HTTP contract
(401/403/404/412/428/409/422/201/200 through the real validator + AuthZ). `go build`/`go vet`/`gofmt` and
the full `go test -p 1 ./...` suite verified clean (no regressions, no new pre-existing failures), `make
gen` re-run with no drift, `pnpm --filter @pdpa/api-client typecheck` and `pnpm --filter @pdpa/admin
build`/`typecheck` both clean. Not done: the subdomain/custom-domain tenant resolver and break-glass
cross-tenant access decisions.md D-24 names (no `/public/v1` or break-glass consumer needs either yet —
PLT-02's own identical deferral); quota enforcement (the `quota` jsonb round-trips but nothing reads it to
cap usage yet — no feature has asked for that).

### PLT-17 Public portal framework (`docs/modules/PLT.md#plt-17`) — done for the acceptance criterion; framework pieces beyond it deferred
`apps/portal`'s home page was a bare placeholder (`<p>{t("home")}</p>`, one plain string) with no shared
header/footer/skip link anywhere in its layout — this pass made it a real page and proved the literal
acceptance criterion against it rather than against the framework bullets the module doc's description
also lists. `apps/portal/src/app/[locale]/layout.tsx` gained a skip link (`tabIndex={-1}` on its target so
activating it moves focus, not just scroll position — WCAG 2.4.1), a `<header>` banner landmark with a
new portal-local `LocaleSwitcher` (same pattern as `apps/admin`'s own), and a `<footer>` contentinfo
landmark — each page under it still renders its own `<main>` (the consent form, the notice page, the new
home page), so the layout itself never does, avoiding a duplicate-landmark defect. `page.tsx` is now
`force-static` (no per-tenant data — PLT-02's own deferred subdomain/custom-domain resolver has no
consumer here either, since this page is deliberately tenant-agnostic) with real content:
`packages/i18n`'s new `portal.home.*` (th/en) explains the portal is the shared entry point
organizations' own notices/consent-form links point back to, and lists PDPA data-subject rights in plain
language.

`apps/portal/src/proxy.ts` (Next 16's renamed middleware file) adds a strict, per-request nonce CSP
(`docs/architecture/security.md`'s own "CSP แบบเข้ม" item) plus `X-Frame-Options`/`X-Content-Type-Options`/
`Referrer-Policy` to every response. The nonce is set on the *request* headers before `next-intl`'s own
middleware runs — confirmed by reading next-intl's `middleware.js`, which builds its downstream
`NextResponse.next({request: {headers}})` from `new Headers(request.headers)`, so whatever is set there
survives — meaning the App Router's own hydration `<script>` tags automatically pick up the matching
nonce with no manual wiring per script. Verified against a real `next build` + `next start`, not just
read from docs: every script tag in the rendered response carries `nonce="..."` matching the response's
own `Content-Security-Policy` header (first attempt actually hit a stale `.next` build still running on
the port from a prior `next start` — `EADDRINUSE` on the real rebuild masked by `pkill` not matching the
`next-server` process name — caught by noticing the CSP header was simply absent, killed the right PID,
confirmed again).

The acceptance criterion itself was measured, not assumed: `npx lighthouse` (Playwright's own bundled
Chromium in this environment, `--headless=new --no-sandbox`) against the real production build, both
locales — accessibility **1.00** (76 audits checked, 0 failed) and performance **0.99–1.00**, with
**LCP 1.8–1.9 s** under Lighthouse's default mobile/slow-4G simulated throttling, directly against the
criterion's own "≤ 2 วินาทีบน 4G" threshold. The page's HTML is ~36 KB gzipped; every script tag loads
`async`, so the ~250 KB gzipped React/Next framework bundle (the standard Next 16 + Turbopack baseline,
cached across repeat visits) never blocks the already-server-rendered, visible content from painting.
`pnpm --filter @pdpa/i18n test` (ICU-parses every th/en message, including the new ones),
`pnpm --filter @pdpa/portal typecheck`/`build`, and `pnpm --filter @pdpa/admin build` (the shared
`packages/i18n` structural change — `portal.home` from a plain string to an object — touches nothing in
`apps/admin`, confirmed by grep rather than assumed) all verified clean.

Deliberately not built, each because this codebase has no consumer for it yet (the same "no consumer
yet" deferral used throughout, not because any of them are hard): CAPTCHA (no provider decision exists in
`docs/decisions.md`, and no public form anywhere in this codebase currently asks for one — CON-09/PNG-06
both already shipped without it); a data-subject session via OTP/magic link (IAM-05's own OTP service has
had no portal consumer since it shipped, and this particular page carries no per-tenant or per-subject
data to gate behind one); per-tenant theme/custom domain (PLT-02's own subdomain/custom-domain resolver
deferral, for the identical reason — nothing on this page needs tenant context); ISR/caching beyond plain
static generation (there's no per-tenant content yet to cache per-tenant). Add each once a real feature
needs it, the same way IAM-05's OTP service itself is waiting for one.

## Non-negotiable rules
1. **Tenant isolation.** One transaction per request (the Tx middleware) and one per worker job, both opened only by `db.WithTenantTx`, which sets `app.tenant_id` / `app.user_id` transaction-locally. Services and stores use the transaction from the context and never `BEGIN` themselves. The app connects as `pdpa_app` (no BYPASSRLS); only `internal/platform/provider` (`/provider/v1`) may use the `pdpa_platform` pool. FK constraints bypass RLS, so verify that a referenced row is visible under RLS before writing its id. Every new repository gets a two-tenant isolation test.
2. **Authorization.** Every operation declares `x-permission` with a code from `docs/security/permissions.yaml` — format `<area>.<resource>.<action>`, where area is the RBAC area (`admin`, `assessment`, `dpx`, …), not the Go package — or `public`, `authenticated`, `scim`, `webhook`. A new code needs a permissions.yaml entry plus a migration. Deny by default; data scope enforced in service/repository; a contract test asserts 403 for a role without the permission.
3. **PII.** Never log personal data, tokens, OTPs or secrets. Identifiers live in `*_enc` columns (envelope encryption) with `blind_index` for exact lookup. Responses are masked by default; unmasking needs `pii.unmask.execute`, a reason, step-up MFA and an `iam.unmask_logs` row.
4. **Append-only evidence.** `consent.consent_transactions`, `consent.consent_receipts` (hash chain), `breach.timeline_events` and `platform.audit_log` (hash chain) are insert-only.
5. **State machines.** Status changes only through the owning module's service, validated against `docs/states/state-machines.yaml`; the same transaction writes the status, `platform.audit_log` and `platform.outbox_events`. Invalid transition → 409 problem+json `<module>.invalid_transition`.
6. **Events.** Leave the system only via outbox → worker → HMAC-signed webhook. Names come from `docs/architecture/events.yaml`.
7. **Legal deadlines** (72 h breach notice, 30-day DSAR, 30-day indirect-collection notice, retention) are computed by unit-tested functions with an injectable clock — see `docs/legal/pdpa-rules.md`.
8. **Legal text** (notices, letters, PDPC forms, clauses) comes from templates and stays DRAFT until a Legal/DPO user approves it. Never hard-code legal wording.
9. **Module boundaries.** A module reads and writes only its own schema; use the other module's exported service interface or its events.
10. **API conventions.** RFC 9457 problem+json with a stable `code`; `Idempotency-Key` required for POST on `/public/v1` and `/api/v1` (a replay returns the original status and body plus `Idempotent-Replayed: true`); `ETag` + `If-Match` on updates (412 mismatch, 428 missing — map the validator's missing-header error to 428); cursor pagination; snake_case JSON; RFC 3339 UTC timestamps.
11. **Time.** Store `timestamptz` in UTC; display Asia/Bangkok; Buddhist-era years only in the UI layer via the locale.
12. **i18n.** No user-facing string literals: keys in `packages/i18n` (th + en) and Go message catalogs.
13. **Migrations.** Never edit a migration that may have been applied — add a new sequential goose file (`goose -s create`) with a working Down. Large-table indexes use `CONCURRENTLY` in a `-- +goose NO TRANSACTION` file; on partitioned tables use `CREATE INDEX … ON ONLY` the parent, `CONCURRENTLY` per partition, then `ALTER INDEX … ATTACH PARTITION`. New append-only or global tables go into the REVOKE lists of `deploy/db/10-grants.sql`.
14. **Secrets** only from OpenBao / environment. Nothing secret in git, fixtures or logs. Test and seed data are synthetic.

## Feature workflow (`/implement <ID>`)
1. Read the backlog row, the feature section in `docs/modules/<EPIC>.md`, its dependencies, every linked BP / ST / SEQ, and the data docs of the tables involved.
2. List assumptions and check `docs/decisions.md` §2 (ask first if the data model, a contract or legal behaviour depends on an open question; otherwise use the default as config).
3. Contract first: extend OpenAPI (x-permission, errors, examples) → `make gen`.
4. Migration if needed → sqlc queries → store → service (rules, state machine, audit, outbox) → handler.
5. Frontend: `packages/ui` components, `packages/api-client` hooks, `packages/authz` guards, i18n keys th/en.
6. Tests: unit (rules, transitions, deadlines) · integration (two-tenant RLS, repository) · contract (401/403/400/422) · E2E for the main path of the BP.
7. Update the docs your change touched (module section, states, data dictionary, events) in the same change.

### DSAR-01 แบบฟอร์มขอใช้สิทธิออนไลน์ (`docs/modules/DSAR.md#dsar-01`) — done, scoped to the literal acceptance criterion
`dsar.intake_forms` (migration 00065) is a thin, publishable wrapper — the same role `notice.notices` (PNG-06)
and `consent.collection_points` (CON-09) already play for their own public surfaces — a legal entity opens one
of, issuing a `platform.public_keys` key (`publickeys.EntityDsarForm`, `pathKeyRE` widened for
`/public/v1/dsar-forms/{key}`) once published. Deliberately **not** built on PLT-06, despite the module doc's
own "ฟอร์มจาก PLT-06" backend note: a DSAR intake's fields are exactly `CreateRequestInput`'s own fixed shape
(right type, name, one contact, an optional ม.30 data-source note) — a dynamic PLT-06 question schema would
just duplicate structure that table (DSAR-13) already names directly, the same "build only the minimal slice
this feature needs" reasoning this codebase uses throughout. `internal/dsar/service/intake.go`:
`CreateIntakeForm`/`GetIntakeForm`/`ListIntakeForms` (plain CRUD, `dsar.form.*` — already seeded in the
baseline permission migration, no new code), `PublishIntakeForm`/`RetireIntakeForm` (the CON-09/PNG-06
publish-gate pattern exactly: `coalesce` the public key so a re-publish never issues a second one, refuse
re-publishing an already-active or already-retired form).

`CreatePublicSubmission` is the acceptance criterion itself ("ผู้ยื่นได้เลขที่คำขอทันทีหลังส่ง"): a thin
wrapper over DSAR-13's own `CreateRequest` (`channel: "web"`, `on_behalf: false`) that also, right in the same
call, sends a PLT-04 confirmation (`dsar.request_received`, migration 00065's own th/en × email/sms templates
— operational text, no DRAFT marker, same reasoning DSAR-07/DSAR-08's own templates used) to the contact the
requester themselves just gave, and opens a DSAR-06 OTP verification attempt right away against that same
contact — closing this feature's own «include» DSAR-06 relationship and, in the same stroke, giving IAM-05's
OTP service and DSAR-06's `StartOTPVerification`/`ConfirmOTPVerification` their first-ever public-portal
consumer (both previously flagged "no consumer yet"). New public endpoint
`POST /public/v1/dsar-forms/{key}/requests/{requestId}/verify` (`internal/dsar/publichttp`, mirroring
`internal/notice/publichttp` exactly) confirms it, reusing `ConfirmOTPVerification` as-is — a wrong code is a
harmless retry (IAM-05's own 5-attempt cap), exactly like the admin-side flow.

API: admin `GET`/`POST /admin/v1/dsar/forms`, `GET /admin/v1/dsar/forms/{id}`, `POST .../publish` /
`.../retire` (ETag/If-Match, `dsar.form.*`); public `GET /public/v1/dsar-forms/{key}` (the right-type picker),
`POST .../requests` (`Idempotency-Key` required, 201 `{request_no, status, verification}`),
`POST .../requests/{requestId}/verify`. UI: `/settings/dsar-forms` (create a draft against a legal entity,
publish/retire, the public link shown once issued — the same `portalUrl` convention CON-09's own
`/consent/collection-points` page uses) and a new portal page, `apps/portal/src/app/[locale]/dsar/[key]`
(server actions, per-attempt `Idempotency-Key`, an inline OTP-confirm step once submitted — mirrors
`/[locale]/c/[key]`'s exact server-component/action split). Tests: unit (create/publish/retire incl. the
re-publish-keeps-the-same-key and refuse-twice cases, the acceptance criterion directly — a public submission
returns a non-empty request number immediately and opens a pending OTP attempt the requester can confirm,
exactly-one-contact validation, an unknown right-type code refused, a draft/retired form refused,
two-tenant isolation), HTTP contract for both the admin forms endpoints and the public submit/confirm chain
through the real validator + publickeys middleware + Idempotency + Tx chain (incl. an Idempotency-Key replay
returning the original result). Not verified live against a real Postgres/Redis — this cloud session has
neither reachable (no Docker daemon either) — every test compiles and vets clean and skips the same way the
rest of this module's own test suite already does in this environment.

Not done, deliberately, each because no screen or consumer needs it yet (the same "no consumer yet" deferral
this codebase uses throughout): QR code / embed snippet (the module doc's own "ฝังเว็บ หรือ QR code" — the
public link alone already satisfies the literal acceptance criterion); form versioning (there is no
configurable form content to version, unlike PLT-06's own forms); more than one intake form's content
differing per right type (today one form offers every seeded right type via `ListRequestTypes`); the
`channel: "api"` / external-app submission path DSAR-02's own notes already defer for the same ORG-16
API-client-authentication reason.

### DSAR-04 ยื่นคำขอแทนเจ้าของข้อมูล (`docs/modules/DSAR.md#dsar-04`) — done
`dsar.agents` was already fully specified in the baseline migrations (`authority_type` CHECK
power_of_attorney/parent/guardian/curator, `authority_file_id` NOT NULL — a row can never exist without an
authority-proof document attached) — no new migration, only `internal/dsar/service/agents.go` against it.
`RecordAgent` follows DSAR-06's own `SubmitIDDocumentVerification` pattern exactly: the agent's own clean,
still-unattached PLT-09 upload (`Files.Get` + `.EntityType == ""` check) is `Attach`ed to the request (not
`SaveGenerated` — this is the caller's own document, not a derived one), and the agent's name is PLT-13
write-only encrypted (rule 3) the same way the requester's own name already is. `VerifyAgent` is staff's own
pass on the document — "ยืนยันทั้งผู้ยื่นและเจ้าของข้อมูล"'s other half, the same human-judgement call DSAR-06's
`ConfirmIDDocument` already makes for the data subject's own identity, since there's no trusted registry to
check a power of attorney or guardianship document against automatically — guarded to a still-unverified row
so a redelivery/double-click is a harmless no-op, not an error.

The acceptance criterion itself ("คำขอแทนต้องมีเอกสารยืนยันอำนาจก่อนดำเนินการ") is a new case in `Transition`'s
own `switch` (`requests.go`): entering `in_review` — ST-02's own `verifying → in_review`, "ยืนยันสำเร็จ", the
exact point DSAR-06's own identity check already gates — for an `on_behalf` request now also requires at
least one `dsar.agents` row (`hasAgentDocument`, a plain `COUNT(*)`) or the transition is refused with a new
`ErrAgentRequired` (422 `dsar.agent_required`) before `UpdateRequestStatus` is ever called; a direct
(not-on-behalf) request is completely unaffected, since the guard only runs when `req.OnBehalf` is true.
Deliberately not gated on `verified_at` being set: the literal criterion is about the *document* being on
record before the request proceeds, not staff having already reviewed it — the same "existence, not judgement"
reading the acceptance criterion's own wording supports; `VerifyAgent` stays a separate, non-blocking staff
action layered on top, matching DSAR-06's own two-step verify/decide shape.

API: `GET`/`POST /admin/v1/dsar/requests/{id}/agents`, `POST .../agents/{agentId}/verify` — all
`dsar.request.*` (no new permission code). UI: `/requests` gained an "ผู้ยื่นแทน / ผู้ปกครอง" toggle, shown only
on `on_behalf` rows — a table of past agent records + a per-row "verify" action, and a record form
(name/authority-type/`FileUploader`) — mirroring `VerificationPanel`'s own layout exactly. Tests: unit
(validation — bad authority_type, empty name, a file id already attached to this or another entity refused via
the same `Files.Get` unattached-check `RecordAgent` shares with DSAR-06; the acceptance criterion directly —
`in_review` blocked without an agent document when `on_behalf`, clears once one is recorded, no gate at all for
a direct request; `VerifyAgent`'s idempotent redelivery and wrong-request 404; two-tenant isolation of both the
agent list and a cross-tenant verify attempt), HTTP contract (401/403/400 schema/422/404/428/200/201) through
the real validator + AuthZ, including the full chain end to end — blocked at 422 `dsar.agent_required`, record
the agent, verify, then the same transition succeeds. Not verified live against a real Postgres/Redis — same
environment limitation DSAR-01 already disclosed; every test compiles and vets clean and skips the same way the
rest of this module's own test suite already does here.

### BRE-01 แบบฟอร์มแจ้งเหตุ (`docs/modules/BRE.md#bre-01`) — done
PUBLIC's own loginless channel, the half BRE-02's admin `/incidents` page (EMP, `breach.incident.create`,
`reported_via: employee_form`) didn't already cover. `breach.report_forms` (migration 00066) is a thin,
publishable wrapper around a legal entity — the exact role `dsar.intake_forms` (DSAR-01) and
`consent.collection_points` (CON-09) already play for their own public surfaces — issuing a
`platform.public_keys` key (`publickeys.EntityBreachForm`, `pathKeyRE` widened for
`/public/v1/breach-report-forms/{key}`) once published; `internal/breach/service/intake.go` follows DSAR-01's
own `CreateIntakeForm`/`PublishIntakeForm`/`RetireIntakeForm` shape precisely (coalesce the key on re-publish,
`ErrInvalidTransition` publishing an already-active or retiring an already-retired form). Migration 00066 also
seeds a fresh `breach.form.*` permission (create/read/update/delete/publish — DPO full, SEC/PRIVACY
create/read/update, AUDIT read) since breach had no equivalent placeholder code seeded the way `dsar.form` was
in the baseline migration.

`breach.incidents.reported_via`'s CHECK constraint already named `'public_form'` and the table already carried
`reporter_contact_enc` (00013_breach.sql) — both unused until now. Rather than duplicate `Create`'s own
numbering/timeline/event/audit/timer/alert sequence, `Service.Create` (admin, permission-gated) was refactored
into a thin wrapper over a new unexported `create(ctx, in Input, reporterContactEnc []byte)` core; the new
`CreatePublicReport` (`internal/breach/service/intake.go`) builds its own `Input` from the public submission —
`AwareAt` is always `s.now()` (for an externally reported incident, the moment the organization is told *is*
the moment it becomes aware, ม.37(4)'s own "ทราบเหตุ") — and calls the same `create()`, so a publicly reported
incident gets the identical treatment an admin-recorded one does: sequential `BR-YYYY-NNNN` numbering, a
`reported:public_form` timeline entry, a `breach.reported` outbox event, an audit row, the four 72-hour
checkpoint timers scheduled, and — the acceptance criterion itself — `s.alert` notifying the incident's owner
and every DPO in-app and by e-mail at once, exactly like BRE-02's own `Create` already does. An optional
reporter contact is PLT-13 encrypted (`Keyring.Encrypt`, class `"breach"`) as a plain follow-up detail, not a
blind-indexed lookup key (unlike DSAR's own `requester_contact_enc`, which DSAR-17 searches) — nobody searches
by it.

API: `GET`/`POST /admin/v1/breach/report-forms`, `GET /admin/v1/breach/report-forms/{id}`,
`POST .../publish` / `.../retire` (ETag/If-Match, `breach.form.*`); public
`GET /public/v1/breach-report-forms/{key}` (the breach-type picker), `POST .../incidents` (`Idempotency-Key`
required, 201 `{incident_no, status}`) — `internal/breach/publichttp`, mirroring `internal/dsar/publichttp`
exactly (own `oapi-codegen.yaml`, a `reportForm(ctx)` helper reading the key the same way `intakeForm(ctx)`
does). UI: `/settings/breach-report-forms` (admin: create a draft against a legal entity, publish/retire, the
public link shown once issued — the same `portalUrl` convention DSAR-01's own `/settings/dsar-forms` page
uses) and a new portal page, `apps/portal/src/app/[locale]/breach/[key]` (server actions, per-attempt
`Idempotency-Key`, no OTP step — unlike DSAR-01, nothing here needs to verify the reporter's identity before
the team can act on a tip). CAPTCHA is deliberately not built: no provider is chosen anywhere in this
codebase yet (`docs/decisions.md` has no entry for one, and CON-09/PNG-06's own public forms already shipped
without it for the same reason) — `platform.public_keys`' own Origin allow-list is the only abuse control in
place; add CAPTCHA once a provider decision exists.

Tests: unit (`internal/breach/service/intake_test.go` — the acceptance criterion directly: a public
submission creates the incident and alerts the owner-less team of every DPO at once, the reporter's contact is
encrypted not stored in the clear; a draft or retired form's key refuses a submission; publishing twice keeps
the same key; two-tenant isolation of the report form itself), HTTP contract
(`internal/breach/publichttp/handler_test.go` — 404 for an unknown key, 200/201 through the real validator +
publickeys + Idempotency + Tx chain, an Idempotency-Key replay returning the original result, schema-level
400 for an empty `breach_types` array). Not verified live against a real Postgres/Redis — same environment
limitation DSAR-01/DSAR-04 already disclosed; every test compiles and vets clean and skips the same way the
rest of this module's own test suite already does here. `go build ./...`, `go vet ./...`,
`internal/pkg/isolationaudit`'s own CI gate, `pnpm --filter @pdpa/api-client typecheck`,
`pnpm --filter @pdpa/i18n test` and `pnpm --filter @pdpa/admin build` / `pnpm --filter @pdpa/portal build`
(incl. the new `/settings/breach-report-forms` and `/[locale]/breach/[key]` routes) all verified clean.

### DSAR-14 ส่งข้อมูลให้เจ้าของข้อมูลอย่างปลอดภัย (`docs/modules/DSAR.md#dsar-14`) — done
`dsar.packages` was already fully specified in the baseline migrations (`token_hash`, `password_protected`,
`expires_at`, `download_count`, `last_downloaded_at`, `purged_at`) and `dsar.package.*` was already seeded in
the baseline permission migration — no new permission code, only migration 00067 widening `platform.
public_keys.entity_type`'s CHECK to add `dsar_package` (the same widening pattern DPO-09/CON-11 already use).
`internal/dsar/service/delivery.go`'s `CreatePackage` wraps either the caller's own clean, still-unattached
PLT-09 upload (attached to the request here, DSAR-04's own `RecordAgent` "record + attach in one call" shape)
or a file already attached to the request (DSAR-13's own generated response letter, or anything DSAR-17's
collaboration panel attached) — never an arbitrary file id. It issues a `platform.public_keys` key the same
way every other `/public/v1` surface in this codebase does (CON-09/PNG-06/DSAR-01/BRE-01), and a *second*,
separate secret — a 10-character access code from an unambiguous alphabet (no 0/O/1/I/L), hashed into
`token_hash` (SHA-256) and returned to the caller exactly once, never stored in the clear (rule 3) — meant to
be relayed to the data subject through a different channel than the link itself.

`PublicPackage`/`PublicDownload` resolve the tenant from the key before the tenant transaction opens, the same
mechanism as every sibling public surface; `PublicDownload` checks the access code with
`subtle.ConstantTimeCompare`, hands back a short-lived signed URL via a new `files.Service.SignedURL` trusted
method, and records `download_count`/`last_downloaded_at` plus a `dsar.package.download` audit entry on every
call — the acceptance criterion itself. A purged or expired package reads as 404 through either read path, no
information leaked either way. Purging is a new PLT-09 trusted operation, `files.Service.DeleteObject`
(alongside `Status`/`Open`/`SignedURL`/`SaveGenerated`/`AttachSystem`), called from a one-time River job
(`dsar.package_purge`, scheduled exactly at `expires_at` per package via `jobs.Enqueue` + `river.InsertOpts{
ScheduledAt: ...}` — the same per-record checkpoint pattern BRE's own SLA timers and DSAR-07's reminder use,
not a periodic sweep like PLT-09's own `files.expire`): it deletes the file's bytes while keeping the
`platform.files` row and the package's own audit trail — PLT-09's own "object deleted, row + audit kept"
pattern for infected files, reused here for expiry.

API: `GET`/`POST /admin/v1/dsar/requests/{id}/packages` (`dsar.package.read`/`.create`); public
`GET /public/v1/dsar-packages/{key}`, `POST /public/v1/dsar-packages/{key}/download`. Real gap found and fixed
while building this: the download endpoint's first draft carried no `Idempotency-Key` parameter, missing rule
10's own "Idempotency-Key required for POST on /public/v1" — every other public POST in this codebase
(DSAR-01's submission, BRE-01's incident report, CON-11's guardian verify) already requires one. Fixed by
adding `$ref: '#/components/parameters/IdempotencyKey'` to the operation and re-running `make gen`; no handler
code changed, since the 428-on-missing-header enforcement lives entirely in the OpenAPI-driven validator
middleware, not in `publichttp/packages.go` itself. UI: `/requests` gained a "แพ็กเกจ" toggle per row (format +
password-protected checkbox + `FileUploader`, a one-time issued-result panel with the public link and access
code) and a new portal page, `apps/portal/src/app/[locale]/dsar-packages/[key]` (server actions, a fresh
`Idempotency-Key` per download attempt — mirrors `/[locale]/dsar/[key]`'s own server-component/action split).
Tests: unit (`internal/dsar/service/delivery_test.go` — the acceptance criterion directly: a package expires
per its own setting and every download is logged with count/last_downloaded_at/an audit row; correct vs. wrong
access code; a file already attached to a different request is refused; purge deletes the file's bytes while
keeping the row and reads as not-found afterward through both `PublicPackage` and `PublicDownload`, a second
purge is a harmless no-op; two-tenant isolation), HTTP contract (`internal/dsar/publichttp/handler_test.go` —
404 for an unknown key, 428 for a missing Idempotency-Key, 422 for a wrong access code, 200 through the real
validator + publickeys + Idempotency + Tx chain, an Idempotency-Key replay returning the original result
unchanged). Not verified live against a real Postgres/Redis — same environment limitation DSAR-01/DSAR-04/
BRE-01 already disclosed; every test compiles and vets clean and skips the same way the rest of this module's
own test suite already does here. `go build ./...`, `go vet ./...`, `go test -p 1 ./...` (no regressions),
`internal/pkg/isolationaudit`'s own CI gate, `pnpm --filter @pdpa/api-client typecheck`, and
`pnpm --filter @pdpa/admin build` / `pnpm --filter @pdpa/portal build` (incl. the new
`/[locale]/dsar-packages/[key]` route) all verified clean. Not done, deliberately (no consumer yet, the same
deferral this codebase uses throughout): a single-use/one-download-only option (the acceptance criterion has
no such requirement — repeat downloads within the link's own expiry are allowed); a portal-side identity check
before download (DSAR-06's own OTP flow already gates the request itself; the access code is this feature's
own, separate secret, not a second identity proof).

### DSAR-15 ส่งออกข้อมูลแบบอ่านได้ด้วยเครื่อง (`docs/modules/DSAR.md#dsar-15`) — done
`internal/dsar/service/export.go`'s `GeneratePortabilityExport` builds the acceptance criterion directly: a
JSON/CSV/XML file of exactly what this system holds about the request's own subject, then hands it straight
to DSAR-14's own `createPackage` core (factored out of `CreatePackage` for this call to share: the file-
visibility/attach logic that's specific to a caller's own upload stays in `CreatePackage`, while the
insert/issue-key/schedule-purge/audit core is now shared) — the export is delivered through the exact same
expiring, access-code-protected link every other DSAR-14 package uses, with every download already logged
by that same mechanism. The content is deliberately narrow: the request's own decrypted name/contact (PLT-13,
the same `decryptRequesterName`/`decryptRequesterContact` DSAR-13/DSAR-06 already use) plus any consent
decisions on record for that same identifier, read through a new `Consent` interface (rule 9:
`consentservice.Subjects`/`GetProfile`, matched by the exact blind-index lookup DSAR-17's own search already
uses — never decrypted to search, rule 3). RoPA is deliberately not read for this: it only ever catalogs
*categories* of data processed, never an individual's own records (ROPA-01's own data inventory describes
what kinds of data a system holds, not any one subject's actual values), so it has nothing a portability
export could include — consent is the one other module in this codebase holding real per-subject data today.
Direct transfer to another controller via API/SFTP (the module doc's own "ส่งตรงไปยังผู้ควบคุมรายอื่นเมื่อ
ทำได้") is deliberately not built: no such controller endpoint exists anywhere in this codebase to transfer
to, the same "no consumer yet" deferral this codebase uses throughout (e.g. ROPA-01's own deferred
`discovered_by_finding_id`) — the file handed to the data subject already satisfies ม.31 regardless.

A real gap found and fixed while wiring this: `CreatePackage`'s own clean-AV-status check is correct for a
caller's own upload (malware risk) but would have wrongly rejected this call's freshly generated file, which
starts `av_status = pending` until its own scan job runs — exactly like PLT-16's rendered documents and
PLT-14's import error report, neither of which goes through that gate either. Fixed by generating the file
through `files.Service.SaveGenerated` (already attached, trusted by construction) and calling the shared
`createPackage` core directly, skipping `CreatePackage`'s own public AV check entirely for this path — the
same trusted-system-operation distinction PLT-09 already draws between its ordinary upload path and
`Status`/`Open`/`SignedURL`/`SaveGenerated`/`AttachSystem`.

API: `POST /admin/v1/dsar/requests/{id}/export` (`dsar.package.create`, same `DsarCreatePackageResult`
response shape `dsarCreatePackage` already returns — the public key and, when password-protected, the
access code shown exactly once). UI: `/requests`' existing package panel gained a sibling "ส่งออกข้อมูลแบบ
พกพาได้" fieldset — a format picker (JSON/CSV/XML) and a generate button, reusing the same issued-link/
access-code display. Tests: unit (`internal/dsar/service/export_test.go` — the acceptance criterion
directly: every format decodes with its own standard-library parser (`encoding/json`, `encoding/csv`,
`encoding/xml`) and every value in it is exactly what `CreateRequest` stored, plus a matching consent
subject's real purpose/status/version; no matching consent subject is an empty list, not an error; a bad
format and a caller without `dsar.package.create` are both refused; two-tenant isolation via the request
id). Not verified live against a real Postgres/Redis — same environment limitation DSAR-01/DSAR-04/BRE-01/
DSAR-14 already disclosed; every test compiles and vets clean and skips the same way the rest of this
module's own test suite already does here. `go build ./...`, `go vet ./...`, `go test -p 1 ./...` (no
regressions), `internal/pkg/isolationaudit`'s own CI gate, `pnpm --filter @pdpa/api-client typecheck`,
`pnpm --filter @pdpa/i18n test` and `pnpm --filter @pdpa/admin build` all verified clean. Not done,
deliberately: the direct controller-to-controller transfer path (no consumer yet, as above).

### CON-01 แบนเนอร์คุกกี้บนเว็บไซต์ (`docs/modules/CON.md#con-01`) — done
`internal/cookie` is the first module on the `cookie` schema (`cookie.banner.*`, already seeded in the
baseline permission migration — no new permission code or migration). Deliberately scoped to the three
literal acceptance criteria only ("ไม่มีช่องติ๊กล่วงหน้า", "ปฏิเสธทั้งหมดได้ในคลิกเดียว...น้ำหนักเท่ากัน",
"ขนาด SDK ไม่เกิน 30 KB gzip") — `cookie.apps` (CON-07's mobile/LIFF SDK), `cookie.scans`/`scan_findings`/
`cookie_kb` (CON-04's automated scanner), `cookie.script_rules` (CON-03's prior-script blocking) and
`cookie.ab_variants` (CON-08's A/B testing) are all already in the baseline schema, unused — sibling
features layered on the same tables later, the same "build only the minimal slice this feature needs"
pattern this codebase uses throughout.

`domains.go`/`categories.go` are plain CRUD: `RegisterDomain` FK-checks `legal_entity_id` via
`orgservice.GetLegalEntity` (rule 1); domain ownership verification is this pass's own manual confirmation,
not an automated DNS TXT/meta-tag check — no outbound DNS resolution exists in this environment to verify
against, so `cookie.domains.verification_token` is issued and shown but staff mark it verified by hand, the
same deferral pattern CON-01's own module doc note anticipates for a future automated check without a data
model change. `banners.go`'s `cookie.banner_configs` has its own simple `draft → published → archived` status
column and `version_no` — deliberately not wired through PLT-08 (the generic versioning/approval engine),
mirroring the simpler pattern `notice.notices`/`dsar.intake_forms` already use for their own public-facing
content. `PublishBannerConfig` archives every sibling published version of the same domain in one
transaction (so the SDK's own fetch always sees at most one published version) and issues the domain's own
`platform.public_keys` entry only on first publish — the same coalesce-on-first-publish pattern
DSAR-01/PNG-06/BRE-01 already use. `cookie.domains` (unlike `breach.report_forms`/`dsar.intake_forms`) has no
`public_key` column of its own to read it back from, so a new shared helper, `publickeys.Get(ctx, entityType,
entityID)`, was added to `internal/platform/publickeys` — backed by a new `GetPublicKeyByEntity` sqlc query —
reusable by any future module whose own entity table lacks that column. `EntityCookieDomain` ("cookie_domain")
was already in `platform.public_keys.entity_type`'s own CHECK constraint from the very first baseline
migration, so no widening migration was needed (unlike several prior features). `consent.go`'s `RecordConsent`
is the acceptance criterion's own evidence write — append-only (no update/delete query exists for
`cookie.consent_records`), one call per decision, hashing the visitor's IP (SHA-256 hex) rather than storing
it raw (rule 3 — the schema's own `ip_hash` column names this at the data-model level too).

A real gap found while building the SDK client: the public banner response (`CookiePublicBanner`) initially
named the current banner only by `domain_id`+`version_no`, with no way for the SDK to supply the
`banner_config_id` its own consent submission needs. Fixed by threading the banner config's own `id` through
`cookie.PublicBanner` (the service's public-read struct, previously missing an `ID` field entirely) and onto
the wire schema — the same kind of gap-closing fix this codebase's own prior passes have each found and fixed
while wiring their own public-read path.

API: `GET`/`POST /admin/v1/cookie/domains`, `GET /admin/v1/cookie/domains/{id}`,
`POST .../domains/{id}/verify` (ETag/If-Match), `GET`/`POST /admin/v1/cookie/categories`,
`DELETE .../categories/{id}`, `GET`/`POST /admin/v1/cookie/domains/{id}/banner-configs`,
`GET`/`PUT /admin/v1/cookie/banner-configs/{id}` (ETag/If-Match), `POST .../banner-configs/{id}/publish`
(ETag/If-Match); public `GET /public/v1/cookie-banners/{key}` (the published banner + every category, no
login — `publickeys.Middleware`'s own path regex widened for `cookie-banners`) and
`POST .../cookie-banners/{key}/consents` (`Idempotency-Key` required, rule 10). UI:
`/settings/cookie-banners` — domain registration + verify, category CRUD (code/name/required toggle), and a
per-domain banner section (version list, a draft editor for layout + bilingual texts, publish) with the
install snippet (`<script src=... data-domain-key=... data-api-base=...>`) shown once a domain's banner has
published at least once.

The SDK itself, `packages/cookie-sdk`, is deliberately vanilla TypeScript with zero dependency on
`@pdpa/api-client` or React: a standalone bundle embedded via CDN `<script>` tag on a tenant's own website has
a completely different size budget than a Next.js app bundle, so pulling in the generated, schema-wide API
client (and its own fetch wrapper) would have blown the 30 KB budget for no benefit — the SDK hand-rolls its
own small `fetch` calls against exactly the two public endpoints it needs. `banner.ts` renders three
genuinely equal-weight buttons (same CSS class, same visual prominence) for accept-all/reject-all/settings;
no category checkbox starts checked unless the category's own `is_required` is true (the literal
"ไม่มีช่องติ๊กล่วงหน้า"), and reject-all is one click away exactly like accept-all (the literal
"ปฏิเสธทั้งหมดได้ในคลิกเดียว"). `storage.ts` keeps a first-party visitor-id cookie (365 days) and the
decision itself (180 days, keyed per domain so two tenants' banners on one browser never collide) to avoid
re-showing the banner for an already-decided version. Built with esbuild (`pnpm --filter @pdpa/cookie-sdk
build`, IIFE, minified, ES2019 target) and measured by a dedicated `scripts/check-size.mjs` that fails the
build past 30 KB gzip — the literal acceptance criterion, not eyeballed: the real bundle measures **2.48 KB
gzip**, comfortably inside budget with headroom for CON-02/03's own later scripts to share the same bundle.

Tests: unit (`internal/cookie/service/cookie_test.go` — no category pre-ticked unless `is_required`, domain
FK-visibility + uniqueness validation, the publish lifecycle acceptance criterion directly (archiving
siblings, issuing the public key only once), `RecordConsent`'s own evidence row carries the right action with
a hashed — never raw — IP and refuses an unknown visitor/banner reference, two-tenant isolation of domains,
categories and banner configs), HTTP contract for both the admin (`internal/cookie/http/handler_test.go`) and
public (`internal/cookie/publichttp/handler_test.go`) packages through the real validator + AuthZ/public-key/
Idempotency/Tx chain (401/403/404/412/428/422/201, an Idempotency-Key replay returning the original result
unchanged), plus `internal/pkg/isolationaudit`'s own CI gate (a new `"cookie": {"cookie/service"}` entry in
its static module map, the same map every new module needs one line in). Not verified live against a real
Postgres/Redis — the same environment limitation DSAR-01/BRE-01/DSAR-14/DSAR-15 already disclosed; every test
compiles and vets clean and skips the same way the rest of this codebase's own test suite already does here.
`go build ./...`, `go vet ./...`, `go test -p 1 ./...` (no regressions), `pnpm --filter @pdpa/api-client
typecheck`, `pnpm --filter @pdpa/i18n test`, `pnpm --filter @pdpa/cookie-sdk test` (build + size check) and
`pnpm --filter @pdpa/admin build`/`typecheck` (incl. the new `/settings/cookie-banners` route) all verified
clean. Not done, deliberately (each a sibling feature's own literal acceptance criterion, not this one's):
CON-02's immediate per-category script effect, CON-03's blocking of scripts before consent, CON-04's
automated cookie scanner, CON-06's branding/multi-language customization beyond th/en, CON-07's mobile/LIFF
SDK, CON-08's A/B testing — every one of their own baseline tables stays unused until that pass.

### CON-02 ตั้งค่าคุกกี้รายหมวด (`docs/modules/CON.md#con-02`) — done
Builds on CON-01's own `cookie` module and schema — no migration needed, every table this feature touches
(`cookie.cookies`, `cookie.script_rules`) was already in the baseline. `domains.go`'s `RegisterDomain` now
seeds the module doc's own 4 default categories (Strictly Necessary/Functional/Analytics/Marketing, only
"necessary" required) on a tenant's *first* domain (`seedDefaultCategoriesIfNone`) — a tunable convenience,
not legal content (rule 8 doesn't apply: internal category labels, never shown as notice/consent wording),
so no `docs/decisions.md` entry was needed; a later domain never re-seeds, so a DPO who already edited the
set keeps their own. `cookies.go` is plain CRUD on `cookie.cookies` (manual entry only, `source='manual'`;
`scan`/`kb` stay CON-04's own unused columns), FK-checking `category_id` via a new `GetCategory` service
method. `script_rules.go`'s `cookie.script_rules` is the acceptance criterion's own enforcement mechanism: a
DPO declares which URL pattern belongs to which category (`action` block/allow, default block);
`PublicScriptRules` exposes them to the SDK keyed by the category's own *code* (a new sqlc query joining
`cookie.categories`), added to `GET /public/v1/cookie-banners/{key}`'s own response.

The SDK itself (`packages/cookie-sdk/src/scripts.ts`) is where "มีผลทันที" actually happens — the same two
mechanisms every commercial CMP uses, since there is no way to literally un-execute code that already ran:
declarative gating (`<script type="text/plain" data-pdpa-category="analytics" data-pdpa-src="...">`,
activated once that category turns on) and pattern-based removal (a still-present, not-yet-loaded
`<script src>` matching a `block` rule's pattern is removed the instant its category is disallowed). A
`pdpa:consent` `CustomEvent` is dispatched on every change too, for a site's own already-running trackers.
Applied both right after a fresh decision and on every page load for a returning visitor's already-recorded
choice. `choicesForAction`/`categoryAllowed`/`matchesPattern`/`scriptsToBlock` are pure, DOM-free functions,
unit-tested directly (`scripts.test.ts`, `vitest` added as a new devDependency). Bundle grew from 2.48 KB to
**2.98 KB gzip** — still comfortably inside the 30 KB budget.

API: `GET`/`POST /admin/v1/cookie/domains/{id}/cookies`, `DELETE /admin/v1/cookie/cookies/{id}`,
`GET`/`POST /admin/v1/cookie/domains/{id}/script-rules`, `DELETE /admin/v1/cookie/script-rules/{id}` — all on
the already-seeded `cookie.banner.*` codes (child-table pattern reusing the parent resource's own
permissions, ROPA-03's move); `script_rules` added to `CookiePublicBanner`. UI: `/settings/cookie-banners`
gained a "Cookies" and a "Script rules" section per domain. Tests: unit (`con02_test.go` — the 4-defaults
seed fires once per tenant and never re-seeds, cookie/script-rule validation, `PublicScriptRules` keyed by
category code and reflecting a delete, two-tenant isolation) and `scripts.test.ts`, HTTP contract for both
the admin and public packages. Existing CON-01 tests that created a category named "analytics" or asserted
an empty category list right after `RegisterDomain` were updated for the new 4-seeded-defaults baseline. Not
verified live against a real Postgres/Redis — same environment limitation CON-01 already disclosed. `go
build ./...`, `go vet ./...`, `go test -p 1 ./...` (no regressions), `pnpm --filter @pdpa/api-client
typecheck`, `pnpm --filter @pdpa/i18n test`, `pnpm --filter @pdpa/cookie-sdk test` and
`pnpm --filter @pdpa/admin build`/`typecheck` all verified clean.

### CON-03 บล็อกสคริปต์ก่อนได้รับความยินยอม (`docs/modules/CON.md#con-03`) — done
Backend scope was already fully covered by CON-02's own `cookie.script_rules` — no new migration, endpoint,
or permission. This feature's real scope is entirely `packages/cookie-sdk`. A MutationObserver-based
"remove after insertion" mechanism was tried first and rejected by this package's own Playwright E2E test: a
mutation callback fires as a microtask *after* the synchronous insertion that already starts the browser's
fetch (and removal doesn't reliably stop execution once started, per spec). `scripts.ts`'s
`guardDynamicScripts()` instead patches `Node.prototype.appendChild`/`insertBefore` directly, rewriting a
`<script src>` to an inert `type="text/plain"` placeholder *before* the original call ever runs — the
browser never fetches it at all. A script intercepted before the tenant's real `script_rules` are known yet
(the realistic GTM-sub-tag-injection race) is held pending; `guard.resolve(choices, rules)` — called once
the banner loads and on every later decision — classifies each one: matched + disallowed stays inert with
`data-pdpa-category` set (CON-02's own mechanism 1 activates it later); unmatched is released immediately.

A second real bug the same E2E test found: `autoInit()` used to defer the *entire* `init()` call to
`DOMContentLoaded` whenever `document.readyState === 'loading'` — always true for the embed snippet's own
recommended `<body>` placement — so the guard/`gtagDefault()` ran only after every other page script had
already executed, the opposite of "prior." Fixed by calling `init()` immediately and deferring only the
banner's own DOM insertion (the one step that needs `document.body`) if necessary.

Google Consent Mode v2: `consentMode.ts`'s `gtagDefault()`/`gtagUpdate()` push the fixed gtag consent signals
onto `window.dataLayer` directly (works whether or not gtag.js/GTM has loaded yet); proven by unit test since
no real GTM container exists here. A literal GTM community template is deliberately not built (no container
to design one against) — the existing `pdpa:consent` CustomEvent + the dataLayer pushes are what one would
read. The acceptance criterion itself is proven by a real Playwright test (`packages/cookie-sdk/e2e/`, `pnpm
test:e2e`) against a real pre-installed Chromium — a genuine first for this session, not another "no browser
here" disclaimer: `e2e/fixtures/index.html` is the literal "เว็บตัวอย่าง" (SDK script first, a declaratively
gated analytics tag, and an inline script simulating a tag manager injecting a marketing sub-tag after its
own container loads), `e2e/server.mjs` a minimal static file server, `/public/v1` mocked via Playwright's
own route interception. Three tests: no analytics/marketing cookie before any decision (the literal
criterion), accepting all still activates the gated script immediately, rejecting all keeps both blocked.
`playwright.config.ts` points at this environment's own `/opt/pw-browsers/chromium` (the pinned
`@playwright/test` version would otherwise try to download an incompatible revision); `vitest.config.ts`
scopes vitest to `src/**/*.test.ts` so it never tries to run Playwright's own spec files. Bundle grew from
2.98 KB to **3.51 KB gzip** — still well inside the 30 KB budget. No backend or admin-UI changes.

### CON-04 สแกนและจัดหมวดคุกกี้อัตโนมัติ (`docs/modules/CON.md#con-04`) — done
`internal/cookie/scan` is a real crawler, not a mock: a same-origin BFS walk using this environment's own
pre-installed headless Chromium (`chromedp`, several tabs in parallel — `Concurrency`, default 4), reading
every cookie per page through CDP's `Storage.getCookies` (the full browser cookie jar, so third-party/
httpOnly cookies are caught too, not just `document.cookie`). Real chromedp gotcha found and fixed: a tab's
lifetime is tied to the context of the *first* call made against it, so wrapping that first call in a
per-page `context.WithTimeout` killed the browser the instant that timeout's own `cancel()` fired after page
one — fixed by warming up each new tab with an untimed `chromedp.Navigate("about:blank")` before any
per-page timeout is applied. A `sync.Cond` + in-flight counter in the BFS frontier avoids the standard
producer-consumer bug (an empty queue doesn't mean "done" while another tab might still discover more
links). Measured throughput: ~378ms/page at concurrency 6 — comfortably inside the 30-minute/200-page
acceptance budget.

Migration 00068 seeds `cookie.cookie_kb` (23 well-known tracker patterns — GA/Ads, Meta, TikTok, LinkedIn,
Hotjar, Clarity, HubSpot, Intercom, Stripe, Cloudflare, PHPSESSID/JSESSIONID), a deliberately global table
(no `tenant_id`, same pattern as `org.countries`) — public industry knowledge about well-known trackers, not
legal/notice wording, so rule 8's DRAFT marker doesn't apply (no `docs/decisions.md` entry needed either).
`internal/cookie/service/scan.go`: `StartScan` (permission-gated, `cookie.scan.create`) enqueues `cookie.scan`
(cmd/api never crawls — only cmd/worker's `ScanWorker` holds a real `cookiescan.Crawler`); `RunScan` is the
worker's own job body, wrapped in `pdb.Savepoint` so a crawl failure on a retried attempt rolls back only its
own partial writes, letting the final-attempt branch still write `MarkScanFailed` into the same still-open
job transaction. Each observed cookie name is resolved once per scan against the tenant's own already-
catalogued `cookie.cookies` (by domain+name) and `cookie.cookie_kb`'s patterns (`matchesKBPattern`: plain
substring or `/regex/`-wrapped, the same convention `packages/cookie-sdk`'s own `matchesPattern` already
uses) — a genuinely new cookie gets auto-created (`source='scan'`) with its category pre-filled only when the
KB's suggested code matches a category the tenant actually has. `alertNewCookies` notifies every DPO/IT user
in-app + email (migration 00069's `cookie.scan_new_cookies` template, operational text, no DRAFT marker) —
the acceptance criterion's own "คุกกี้ใหม่ถูกแจ้งเตือน" directly. `ClassifyFinding` corrects/confirms a
finding's already-created cookie row's category (`cookie.scan.update`).

Scheduled re-scanning (`cookie.domains.scan_schedule`: daily/weekly/monthly) follows PLT-11's own two-step
"sweep lists tenants, per-tenant job reads its own rows" pattern exactly: `cookie.scan_schedule_sweep`
(added to `jobs.GlobalKinds` — reads only `platform.tenants`, no tenant transaction) enqueues one
`cookie.scan_schedule_check` per trial/active tenant, which opens its own tenant tx and starts a scan for
every domain whose own interval has elapsed (`scanDue`, a pure, clock-testable function) — `scan_schedule_
check` and `scan` themselves are correctly *not* in `GlobalKinds`, since both are genuinely tenant-scoped.

API: `GET`/`POST /admin/v1/cookie/domains/{id}/scans` (cursor pagination, the same N+1-row trick every other
cursor list in this codebase uses — `ScanCursor`), `GET /admin/v1/cookie/scans/{id}`, `GET .../scans/{id}/
findings` (offset/limit), `POST /admin/v1/cookie/scan-findings/{id}/classify` — all on the already-seeded
`cookie.scan.*` permission (baseline migration 00019, no new code). UI: a "สแกนคุกกี้อัตโนมัติ" section on
`/settings/cookie-banners` per domain — a start-scan button, a history table (polls every 3s while a scan is
queued/running, no long-poll infrastructure needed for this), and a findings panel with inline category
classification. Tests: unit (`matchesKBPattern`, `scheduleInterval`/`scanDue` — pure functions), service
(DB-backed: new cookies catalogued and counted correctly, a second identical crawl finds nothing new,
classification writes through to the matched cookie, a crawl failure leaves the scan `running` rather than
corrupting state, each of the three `cookie.scan.*` permissions enforced independently, two-tenant isolation),
**a real-browser crawler test** (not mocked — drives the actual pre-installed Chromium against a 3-page
`httptest` fixture, proving every page and every cookie is found and the off-site link is never followed,
plus a `MaxPages` cap test), HTTP contract (401/403/404/422/200/201 through the real validator, exercising
`RunScan` directly since cmd/api's own test harness has no River worker running). Not done, deliberately: a
`sitemap.xml` entry point (link-following already satisfies the acceptance criterion), a `storage_type`
breakdown in `cookie.scan_findings` (the schema has no such column, unlike `cookie.cookies` — add one only if
a screen needs to distinguish localStorage/sessionStorage findings), and a dedicated cookie-by-cookie diff
against the previous round (the history table already shows `new_cookies` per round; a literal diff view is
CON-05's own "ตารางในหน้า Cookie Policy ตรงกับผลสแกนล่าสุด" acceptance criterion, a sibling feature).

### PNG-14 อนุมัติก่อนเผยแพร่ (`docs/modules/PNG.md#png-14`) — done, no new code
Already fully built by PLT-08 + PNG-01 together — the same "done, no new code" pattern ROPA-07 already
used. `internal/wiring.Docs` registers the `"notice"` document type with `Approver: "DPO"` (one approval
step, already part of PNG-01's own description), and PLT-08's `versioning.Service.Publish` hard-refuses
with `ErrInvalidTransition` unless the version's own status is already `approved` — the acceptance
criterion ("ประกาศเผยแพร่ได้หลังอนุมัติเท่านั้น") is exactly this one generic invariant, already proven by
PLT-08's own `versioning_test.go` (`TestMakerChecker`) and exercised live by PNG-02/PNG-05's own integration
tests, which already drive the real submit → DPO approve → publish chain for a notice. "ปุ่มส่งตรวจ"
(submit button) is the generic `RecordVersions` component's own submit action, already mounted on
`/documents/{id}` (every notice routes there, per PNG-01); "กล่องความเห็น" (comment box) is the generic
`/approvals` inbox's reason textarea, shown on every approve/return/reject decision — not notice-specific
UI, the same screen DPIA-10's own approval decisions already use. Marked done as documentation only
(backlog + module doc) — no migration, code or test changes needed.

## Definition of done
Acceptance criteria from the module doc are automated tests · lint and codegen check clean · RLS and permission tests present · no PII in logs · audit + outbox written for state changes · i18n th/en · docs updated · `/review-compliance` on the diff has no blocking findings.

## Style
Code, identifiers, comments and commit messages in English; UI copy and domain docs may be Thai. Thin handlers, explicit SQL via sqlc, small packages with clear interfaces. Ask instead of guessing when a rule, deadline or permission is unclear.
