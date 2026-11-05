-- name: InsertFile :one
INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256,
                            encryption_key_id, av_status, retention_until, created_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @bucket, @object_key, @file_name, @mime_type, @size_bytes, @sha256,
        @encryption_key_id, 'pending', @retention_until, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING created_at;

-- name: GetFile :one
SELECT id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status,
       entity_type, entity_id, retention_until, created_at, created_by
FROM platform.files WHERE id = @id;

-- name: LockFileForScan :one
SELECT id, bucket, object_key, av_status FROM platform.files WHERE id = @id FOR UPDATE;

-- name: SetFileAVStatus :exec
UPDATE platform.files SET av_status = @av_status, row_version = row_version + 1,
       updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: AttachFile :execrows
-- Only an unattached file can be attached, so a file id can't be re-pointed at another record.
UPDATE platform.files SET entity_type = @entity_type, entity_id = @entity_id, retention_until = NULL,
       row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND entity_type IS NULL;

-- name: LockOrphanFile :one
-- An upload still unattached when its orphan deadline passes; nothing references it, so it can go.
SELECT id, bucket, object_key FROM platform.files
WHERE id = @id AND entity_type IS NULL AND retention_until <= now()
FOR UPDATE;

-- name: DeleteFile :exec
DELETE FROM platform.files WHERE id = @id;
