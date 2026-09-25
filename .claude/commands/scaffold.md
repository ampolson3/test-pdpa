---
description: P0 scaffold — create the monorepo skeleton, tooling, local stack and CI exactly as documented
argument-hint: "[part: all | backend | frontend | infra | ci]"
---

Create the P0 skeleton of the PDPA platform. Part requested: "$ARGUMENTS" (if empty, do all parts in the order below). Follow `docs/architecture/code-structure.md`, `docs/architecture/deployment.md`, `api/openapi/README.md` and `CLAUDE.md`. Use the latest stable versions of every tool and pin them.

Backend (`backend/`)
- Go module with `cmd/{api,worker,scanner,migrate}` and `internal/pkg/{db,httpx,authz,otel,validate}`.
- `internal/pkg/db`: pgx pool (role `pdpa_app`) + `WithTenantTx(ctx, tenantID, userID, fn)` that runs `BEGIN` and then `SELECT set_config('app.tenant_id', <tenant id>, true), set_config('app.user_id', <user id>, true)` (the parameterised form of SET LOCAL) and puts the tx in the context. It is called only by the Tx middleware (one transaction per request) and by the worker job wrapper (one per job); services and stores read the tx from the context. Add a lint rule or test that forbids `pool.Begin` / `WithTenantTx` elsewhere. A second pool for `pdpa_platform` is visible only to `internal/platform/provider`.
- chi router + middleware chain in the order listed in `docs/architecture/code-structure.md` (request id, otel, recover, access log without PII, CORS/CSRF, rate limit, authn, tenant — from the `tid` claim or `platform.public_keys`, authz from x-permission using grants cached in Valkey and loaded on a miss with its own read-only `WithTenantTx`, idempotency, tx, handler, audit).
- oapi-codegen (strict server) wired to `api/openapi/openapi.yaml`; implement `GET /admin/v1/me` end to end as the reference slice.
- `backend/db/sqlc.yaml` with `schema: "migrations"` (relative to that file); `cmd/migrate` embeds the migrations, connects as `pdpa_migrator` with `options=-c role=pdpa_owner`, runs goose, River migrations, then `deploy/db/10-grants.sql`.
- Worker: River client; job wrapper that opens `WithTenantTx` from `tenant_id` in the job args; `outbox.dispatch` (enqueued per tenant in the same transaction as the outbox row) + a sweeper; `partition.maintain` calling `SELECT platform.ensure_monthly_partitions(3)` daily.
- Problem+json helper with stable codes; x-permission middleware that loads the permission catalogue from `iam.permissions` and denies unknown operations.

Frontend
- pnpm workspace + Turborepo with `apps/admin`, `apps/portal`, `packages/{ui,api-client,i18n,form-renderer,authz,cookie-sdk}`.
- Admin: App Router with `[locale]` (th default, en), Auth.js + Keycloak (OIDC code + PKCE), server-side session in Valkey, BFF proxy route that attaches the JWT and checks CSRF, a layout whose menu is filtered by `packages/authz`.
- `packages/api-client`: openapi-typescript + TanStack Query hooks generated from the spec.

Infra (local stack and tooling)
- `deploy/compose/docker-compose.yml`: postgres 16+ (with citext, ltree, pg_trgm, pgvector; init runs `deploy/db/00-bootstrap.sql`), valkey, minio, keycloak (dev realm `pdpa` with Organizations enabled, one organization per seeded tenant, a protocol mapper that emits the `tid` claim — the default of decisions Q-18), gotenberg, clamav, mailpit.
- `Makefile` targets from `CLAUDE.md` (`dev`, `gen`, `migrate`, `migrate-down`, `test`, `test-int`, `e2e`, `lint`).
- `.env.example` only (no secrets committed).

CI (GitHub Actions or GitLab CI, ask me which)
- lint → codegen diff check → unit → build → security scans (govulncheck, osv-scanner, semgrep, trivy) → integration (testcontainers; applies migrations up/down/up) → SBOM + sign — as in `docs/architecture/deployment.md`.

Finish by running the build and tests you can run, update the Commands section of `CLAUDE.md`, and list what still needs credentials or decisions.
