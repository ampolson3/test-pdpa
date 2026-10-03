-- name: InsertComment :one
INSERT INTO platform.comments (id, tenant_id, entity_type, entity_id, parent_id, author_type, author_id, body, mentions, created_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @entity_type, @entity_id, @parent_id, 'user', @author_id, @body, @mentions,
        NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, entity_type, entity_id, parent_id, author_id, body, mentions, resolved_at, row_version, created_at, updated_at;

-- name: ListComments :many
SELECT id, entity_type, entity_id, parent_id, author_id, body, mentions, resolved_at, row_version, created_at, updated_at
FROM platform.comments
WHERE entity_type = @entity_type AND entity_id = @entity_id
ORDER BY created_at, id
LIMIT 500;

-- name: GetComment :one
SELECT id, entity_type, entity_id, parent_id, author_id, body, mentions, resolved_at, row_version, created_at, updated_at
FROM platform.comments WHERE id = @id;

-- name: UpdateCommentBody :one
UPDATE platform.comments SET body = @body, mentions = @mentions, row_version = row_version + 1,
       updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND row_version = @row_version AND author_id = @author_id
RETURNING id, entity_type, entity_id, parent_id, author_id, body, mentions, resolved_at, row_version, created_at, updated_at;

-- name: SetCommentResolved :one
UPDATE platform.comments SET resolved_at = CASE WHEN @resolved::bool THEN now() ELSE NULL END, row_version = row_version + 1,
       updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND parent_id IS NULL
RETURNING id, entity_type, entity_id, parent_id, author_id, body, mentions, resolved_at, row_version, created_at, updated_at;

-- name: DeleteComment :execrows
-- A comment with replies keeps its thread: only leaf comments can be deleted, by their author.
DELETE FROM platform.comments c
WHERE c.id = @id AND c.row_version = @row_version AND c.author_id = @author_id
  AND NOT EXISTS (SELECT 1 FROM platform.comments r WHERE r.parent_id = c.id);

-- name: ListRecordAttachments :many
SELECT id, file_name, mime_type, size_bytes, av_status, created_at, created_by
FROM platform.files WHERE entity_type = @entity_type AND entity_id = @entity_id
ORDER BY created_at, id;

-- name: ListRecordActivity :many
-- The record's audit trail, newest first (PLT-12 rows written with entity_type/entity_id).
SELECT id, occurred_at, actor_type, actor_id, action, before, after
FROM platform.audit_log
WHERE entity_type = @entity_type AND entity_id = @entity_id
ORDER BY occurred_at DESC, id DESC
LIMIT @page_size;
