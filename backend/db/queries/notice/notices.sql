-- name: InsertNotice :one
INSERT INTO notice.notices (id, tenant_id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id,
    owner_user_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at;

-- name: GetNotice :one
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at
FROM notice.notices
WHERE id = $1;

-- name: ListNotices :many
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at
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

-- name: GetNoticeByDocumentID :one
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at
FROM notice.notices
WHERE document_id = $1;

-- PNG-06: version tracking + the public page. notice.notice_versions was already fully specified in the
-- baseline migrations (effective_from, document_version_id, checklist_result, public_url, ...).

-- name: SetNoticePublished :one
-- First publish sets status + current_version_id together with the newly issued public key; a later publish
-- only needs the first two (public_key stays NULL in the params and the COALESCE keeps the existing one).
UPDATE notice.notices
SET status = 'published', current_version_id = $2, public_key = COALESCE(sqlc.narg(public_key)::varchar, public_key),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1
RETURNING id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at;

-- name: GetNoticeByPublicKeyEntity :one
-- The public key's entity_id is the notice's own id (publickeys.EntityNotice) — same row GetNotice reads,
-- named separately so the public handler's intent is obvious at the call site.
SELECT id, legal_entity_id, subject_type_id, notice_type, title, slug, document_id, status, current_version_id,
    owner_user_id, review_cycle_months, public_key, row_version, updated_at
FROM notice.notices
WHERE id = $1;

-- name: InsertNoticeVersion :one
INSERT INTO notice.notice_versions (id, tenant_id, notice_id, version_no, document_version_id, languages,
    effective_from, checklist_result, public_url, published_at, published_by, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, now(),
    NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, notice_id, version_no, document_version_id, languages, effective_from, is_material_change,
    changes_purpose, checklist_result, public_url, published_at, published_by;

-- name: ListNoticeVersions :many
SELECT id, notice_id, version_no, document_version_id, languages, effective_from, is_material_change,
    changes_purpose, checklist_result, public_url, published_at, published_by
FROM notice.notice_versions
WHERE notice_id = $1
ORDER BY version_no DESC;

-- name: GetNoticeVersion :one
SELECT id, notice_id, version_no, document_version_id, languages, effective_from, is_material_change,
    changes_purpose, checklist_result, public_url, published_at, published_by
FROM notice.notice_versions
WHERE notice_id = $1 AND version_no = $2;
