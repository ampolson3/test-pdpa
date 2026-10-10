-- name: InsertIndirectCollection :one
INSERT INTO notice.indirect_collections (id, tenant_id, source_party_id, activity_id, obtained_at, subject_count,
    notify_due_at, status, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'pending',
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, source_party_id, activity_id, obtained_at, subject_count, notify_due_at, method, notified_at,
    evidence_file_id, status, row_version;

-- name: GetIndirectCollection :one
SELECT id, source_party_id, activity_id, obtained_at, subject_count, notify_due_at, method, notified_at,
    evidence_file_id, status, row_version
FROM notice.indirect_collections
WHERE id = $1;

-- name: ListIndirectCollections :many
SELECT id, source_party_id, activity_id, obtained_at, subject_count, notify_due_at, method, notified_at,
    evidence_file_id, status, row_version
FROM notice.indirect_collections
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::date IS NULL
       OR (notify_due_at, id) > (sqlc.narg(cursor_at)::date, sqlc.narg(cursor_id)::uuid))
ORDER BY notify_due_at, id
LIMIT @lim;

-- name: RecordIndirectNotice :one
UPDATE notice.indirect_collections
SET method = $2, notified_at = now(), evidence_file_id = $3, status = 'notified', row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $4
RETURNING id, source_party_id, activity_id, obtained_at, subject_count, notify_due_at, method, notified_at,
    evidence_file_id, status, row_version;

-- name: MarkIndirectCollectionOverdue :execrows
UPDATE notice.indirect_collections SET status = 'overdue', row_version = row_version + 1
WHERE id = $1 AND status = 'pending';
