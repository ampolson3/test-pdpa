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
other schemas when merging — nothing writes a real row referencing `org.external_parties` yet (RoPA,
DSAR, Vendor and Agreement aren't built; even `breach.incidents.processor_party_id`, which already has
the column, is never set) so there's nothing to reassign today — the first module that does must follow
`merged_into_id` itself. API `/admin/v1/org/external-parties` (cursor pagination, same shape as PLT-16's
document list), `/external-parties/duplicates`, `/external-parties/{id}/merge`. UI
`/settings/external-parties` (list + form + a duplicates panel with a merge-into picker). Tests: unit
(validation, update, merge rules, duplicate detection matches the normalization exactly, two-tenant
isolation), HTTP contract (401/403/400 schema/412/428).

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
