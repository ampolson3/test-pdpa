-- DSAR-01: the public web intake form (dsar.intake_forms). Publish/retire follow the same pattern
-- consent.collection_points (CON-09) and notice.notices (PNG-06) already use: coalesce the public key so a
-- re-publish never issues a second one, lock the row for the version check.

-- name: InsertIntakeForm :one
INSERT INTO dsar.intake_forms (id, tenant_id, legal_entity_id, name, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, name, status, public_key, row_version, updated_at;

-- name: GetIntakeForm :one
SELECT id, legal_entity_id, name, status, public_key, row_version, updated_at
FROM dsar.intake_forms WHERE id = $1;

-- name: LockIntakeForm :one
SELECT id, legal_entity_id, name, status, public_key, row_version, updated_at
FROM dsar.intake_forms WHERE id = $1 FOR UPDATE;

-- name: ListIntakeForms :many
SELECT id, legal_entity_id, name, status, public_key, row_version, updated_at
FROM dsar.intake_forms
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (updated_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @lim;

-- name: PublishIntakeForm :exec
UPDATE dsar.intake_forms
SET status = 'active', public_key = coalesce(public_key, @public_key), row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: RetireIntakeForm :exec
UPDATE dsar.intake_forms SET status = 'retired', row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1;
