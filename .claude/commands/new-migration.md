---
description: Add a goose migration that follows the project's schema conventions
argument-hint: <short description of the change>
---

Create a new goose migration for: **$ARGUMENTS**

Rules (see `docs/data/README.md` and `docs/architecture/deployment.md`):
- New file in `backend/db/migrations/` with the next number (`goose -s create <name> sql`); never edit an existing migration.
- Each change goes to the schema of the owning module. New business tables need: `id uuid` PK (the app generates UUIDv7), `tenant_id uuid NOT NULL` + FK to `platform.tenants`, standard columns (`created_at`, `created_by`, `updated_at`, `updated_by`, `row_version`) + the `platform.set_updated_at()` trigger, `ENABLE` + `FORCE ROW LEVEL SECURITY` and the `tenant_isolation` policy using `NULLIF(current_setting('app.tenant_id', true), '')::uuid`, indexes that lead with `tenant_id`, `COMMENT ON TABLE` in Thai.
- Status columns get a CHECK list that matches `docs/states/state-machines.yaml` (update that file and the ST doc if you add states).
- Business keys get a unique constraint (`UNIQUE NULLS NOT DISTINCT` when `tenant_id` may be NULL).
- Follow expand → migrate → contract; for large tables use `CREATE INDEX CONCURRENTLY` in a separate file marked `-- +goose NO TRANSACTION`. Partitioned tables: `CREATE INDEX … ON ONLY <parent>`, then `CREATE INDEX CONCURRENTLY` on each partition, then `ALTER INDEX <parent index> ATTACH PARTITION <partition index>`.
- New append-only or global tables must be added to the REVOKE lists in `deploy/db/10-grants.sql`; new partitioned tables must be added to `platform.ensure_monthly_partitions()` (new migration replacing the function) and get RLS on their default partition.
- Write a real `-- +goose Down`.

Then: add/adjust sqlc queries, run the migration up → down → up against a local or test database, update `docs/data/<schema>.md` (and `backend/db/schema.sql` if the project keeps it in sync), and report the result.
