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
- `make gen` — sqlc, oapi-codegen, openapi-typescript (all three re-run and produced correct output while scaffolding; CI should fail if generation leaves a diff — not wired into CI yet, there is no CI)
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
- Not started: CI (explicitly deferred — ask before adding), lint/test tooling beyond `go vet`/`tsc`, every module besides `iam`'s `/admin/v1/me` slice and `platform`'s audit log.

### PLT-02 Multi-tenant และการแยกข้อมูล (`docs/modules/PLT.md#plt-02`) — partial
The DB-level half (`tenant_id` on every table + RLS) was already in the migrations from the
knowledge kit, not new work here. What this pass added, against the acceptance criterion
("ผู้ใช้ tenant A เข้าถึงข้อมูล tenant B ไม่ได้แม้เรียก API ตรง (automated test ทุก endpoint)"):
- `internal/pkg/dbtest` — a reusable two-tenant test harness (seeds a tenant via `pdpa_platform`, a
  user inside it via `pdpa_app` + `WithTenantTx`, cleans both up) so every future repository can
  write its own isolation test the same way, per CLAUDE.md rule 1. Points at a real Postgres via
  `TEST_DATABASE_URL` / `TEST_PLATFORM_DATABASE_URL` (defaults match the port-5433 local setup) and
  skips cleanly if neither is reachable — a stand-in for the testcontainers-go setup
  `docs/architecture/code-structure.md`'s testing table calls for, which needs Docker.
- `internal/iam/store/isolation_test.go` — proves tenant B's own transaction cannot read tenant A's
  `iam.users` row (gets `pgx.ErrNoRows`, not a permission error — RLS filters it, doesn't reject
  the query), with a sanity check that a tenant can still read its own data. **Run and passing**
  against the real local Postgres.

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

## Definition of done
Acceptance criteria from the module doc are automated tests · lint and codegen check clean · RLS and permission tests present · no PII in logs · audit + outbox written for state changes · i18n th/en · docs updated · `/review-compliance` on the diff has no blocking findings.

## Style
Code, identifiers, comments and commit messages in English; UI copy and domain docs may be Thai. Thin handlers, explicit SQL via sqlc, small packages with clear interfaces. Ask instead of guessing when a rule, deadline or permission is unclear.
