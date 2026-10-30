-- name: InsertSnapshot :one
-- DFG-07: publish the current live Graph (DFG-01/03/04) as an immutable, versioned row — version_no is
-- this (view_type, scope_id)'s own next number, computed from the rows already committed at statement
-- start; wrapped in pdb.Savepoint by the caller since two concurrent publishes could still race the real
-- unique constraint uq_snapshots_view_type_scope_id_version_no (migration 00009).
INSERT INTO dataflow.snapshots (tenant_id, view_type, scope_id, version_no, graph, published_by, published_at, created_by, updated_by)
SELECT current_setting('app.tenant_id')::uuid, sqlc.arg(view_type)::varchar(20), sqlc.narg(scope_id)::uuid, COALESCE(MAX(version_no), 0) + 1, sqlc.arg(graph)::jsonb,
    NULLIF(current_setting('app.user_id', true), '')::uuid, now(),
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid
FROM dataflow.snapshots
WHERE view_type = sqlc.arg(view_type)::varchar(20) AND (scope_id = sqlc.narg(scope_id)::uuid OR (scope_id IS NULL AND sqlc.narg(scope_id)::uuid IS NULL))
RETURNING id, view_type, scope_id, version_no, graph, published_by, published_at, created_at;

-- name: ListSnapshots :many
-- Newest first — the version history, "ดูแผนผังที่ publish ย้อนหลังได้" (the acceptance criterion); the
-- graph jsonb itself is left out here (can be large) and read on demand via GetSnapshot.
SELECT id, view_type, scope_id, version_no, published_by, published_at, created_at
FROM dataflow.snapshots
WHERE view_type = sqlc.arg(view_type)::varchar(20) AND (scope_id = sqlc.narg(scope_id)::uuid OR (scope_id IS NULL AND sqlc.narg(scope_id)::uuid IS NULL))
ORDER BY version_no DESC;

-- name: GetSnapshot :one
SELECT id, view_type, scope_id, version_no, graph, published_by, published_at, created_at
FROM dataflow.snapshots
WHERE id = $1;
