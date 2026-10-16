-- PLT-16 document composer.

-- name: InsertDocument :one
INSERT INTO platform.documents (id, tenant_id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @doc_type, @title, sqlc.narg(template_id), sqlc.narg(legal_entity_id),
        sqlc.narg(entity_type), sqlc.narg(entity_id),
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, status, current_version_id, created_at, updated_at, row_version;

-- name: GetDocument :one
SELECT id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, status, current_version_id, created_at, updated_at, row_version
FROM platform.documents WHERE id = $1;

-- name: LockDocument :one
SELECT id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, status, current_version_id, created_at, updated_at, row_version
FROM platform.documents WHERE id = $1 FOR UPDATE;

-- name: ListDocuments :many
SELECT id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, status, current_version_id, created_at, updated_at, row_version
FROM platform.documents
WHERE doc_type = ANY (@types::text[])
  AND (sqlc.narg(doc_type)::text IS NULL OR doc_type = sqlc.narg(doc_type))
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q) || '%' ESCAPE '\')
  AND (sqlc.narg(after_updated)::timestamptz IS NULL OR (updated_at, id) < (sqlc.narg(after_updated), sqlc.narg(after_id)::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @lim;

-- name: UpdateDocumentDraft :one
UPDATE platform.documents
SET title = @title, legal_entity_id = sqlc.narg(legal_entity_id), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id
RETURNING id, doc_type, title, template_id, legal_entity_id, entity_type, entity_id, status, current_version_id, created_at, updated_at, row_version;

-- name: SetDocumentPublished :exec
UPDATE platform.documents
SET title = @title, status = 'published', current_version_id = @version_id,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: NextDocumentVersionNo :one
SELECT (coalesce(max(version_no), 0) + 1)::int FROM platform.document_versions WHERE document_id = $1;

-- name: InsertDocumentVersion :one
INSERT INTO platform.document_versions (id, tenant_id, document_id, version_no, content, languages, change_summary, effective_from,
                                        approved_by, approved_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @document_id, @version_no, @content, @languages, sqlc.narg(change_summary),
        sqlc.narg(effective_from), sqlc.narg(approved_by), sqlc.narg(approved_at),
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id;

-- name: ListDocumentVersions :many
SELECT id, document_id, version_no, languages, pdf_file_id, docx_file_id, pdf_en_file_id, docx_en_file_id, render_status,
       change_summary, effective_from, approved_by, approved_at, created_at, created_by
FROM platform.document_versions WHERE document_id = $1 ORDER BY version_no DESC;

-- name: GetDocumentVersion :one
SELECT id, document_id, version_no, content, languages, pdf_file_id, docx_file_id, pdf_en_file_id, docx_en_file_id, render_status,
       change_summary, effective_from, approved_by, approved_at, created_at, created_by
FROM platform.document_versions WHERE id = $1;

-- name: SetVersionFiles :exec
UPDATE platform.document_versions
SET pdf_file_id = sqlc.narg(pdf_file_id), docx_file_id = sqlc.narg(docx_file_id),
    pdf_en_file_id = sqlc.narg(pdf_en_file_id), docx_en_file_id = sqlc.narg(docx_en_file_id), render_status = @render_status
WHERE id = @id;

-- name: SetVersionRenderStatus :exec
UPDATE platform.document_versions SET render_status = @render_status WHERE id = @id;

-- name: GetDocumentVersionByNo :one
SELECT id, document_id, version_no, content, languages, pdf_file_id, docx_file_id, pdf_en_file_id, docx_en_file_id, render_status,
       change_summary, effective_from, approved_by, approved_at, created_at, created_by
FROM platform.document_versions WHERE document_id = $1 AND version_no = $2;
