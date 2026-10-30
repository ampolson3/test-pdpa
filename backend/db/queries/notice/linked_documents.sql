-- name: InsertLinkedDocument :one
INSERT INTO notice.linked_documents (id, tenant_id, notice_id, label, url, file_id, display_mode, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, notice_id, label, url, file_id, display_mode, row_version, created_at;

-- name: ListLinkedDocuments :many
SELECT id, notice_id, label, url, file_id, display_mode, row_version, created_at
FROM notice.linked_documents
WHERE notice_id = $1
ORDER BY created_at;

-- name: DeleteLinkedDocument :execrows
DELETE FROM notice.linked_documents WHERE id = $1 AND notice_id = $2;
