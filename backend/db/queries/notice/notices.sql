-- name: InsertNotice :one
INSERT INTO notice.notices (id, tenant_id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id,
    owner_user_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, row_version, updated_at;

-- name: GetNotice :one
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, row_version, updated_at
FROM notice.notices
WHERE id = $1;

-- name: ListNotices :many
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, row_version, updated_at
FROM notice.notices
WHERE (sqlc.narg(notice_type)::text IS NULL OR notice_type = sqlc.narg(notice_type))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (updated_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @lim;

-- name: InsertNoticeActivityLink :exec
INSERT INTO notice.notice_activity_links (notice_id, activity_id, tenant_id)
VALUES ($1, $2, current_setting('app.tenant_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: ListNoticeActivityLinks :many
SELECT activity_id FROM notice.notice_activity_links WHERE notice_id = $1 ORDER BY activity_id;
