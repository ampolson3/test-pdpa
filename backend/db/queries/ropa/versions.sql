-- ROPA-13: append-only version history, one row per approved round.

-- name: InsertActivityVersion :one
INSERT INTO ropa.activity_versions (id, tenant_id, activity_id, version_no, snapshot, approved_by, approved_at)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
    NULLIF(current_setting('app.user_id', true), '')::uuid, now())
RETURNING id, activity_id, version_no, snapshot, approved_by, approved_at, created_at;

-- name: ListActivityVersions :many
SELECT id, activity_id, version_no, snapshot, approved_by, approved_at, created_at
FROM ropa.activity_versions
WHERE activity_id = $1
ORDER BY version_no DESC;

-- name: GetActivityVersion :one
SELECT id, activity_id, version_no, snapshot, approved_by, approved_at, created_at
FROM ropa.activity_versions
WHERE id = $1;

-- name: MaxActivityVersionNo :one
SELECT COALESCE(MAX(version_no), 0)::int FROM ropa.activity_versions WHERE activity_id = $1;
