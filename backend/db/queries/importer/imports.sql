-- name: InsertImportJob :one
INSERT INTO platform.import_jobs (id, tenant_id, import_type, file_id, mapping, dry_run, status, created_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @import_type, @file_id, '{}', true, 'queued',
        NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, import_type, file_id, mapping, dry_run, status, total_rows, success_rows, error_rows, error_file_id, row_version, created_at, created_by, updated_at;

-- name: GetImportJob :one
SELECT id, import_type, file_id, mapping, dry_run, status, total_rows, success_rows, error_rows, error_file_id, row_version, created_at, created_by, updated_at
FROM platform.import_jobs WHERE id = @id;

-- name: LockImportJob :one
SELECT id, import_type, file_id, mapping, dry_run, status, total_rows, success_rows, error_rows, error_file_id, row_version, created_at, created_by, updated_at
FROM platform.import_jobs WHERE id = @id FOR UPDATE;

-- name: ListImportJobs :many
SELECT id, import_type, file_id, mapping, dry_run, status, total_rows, success_rows, error_rows, error_file_id, row_version, created_at, created_by, updated_at
FROM platform.import_jobs
WHERE import_type = ANY (@import_types::text[])
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: UpdateImportJob :one
-- Every state change goes through here with the version the caller read (optimistic lock).
UPDATE platform.import_jobs
SET status = @status, mapping = @mapping, dry_run = @dry_run, total_rows = @total_rows, success_rows = @success_rows,
    error_rows = @error_rows, error_file_id = @error_file_id, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND row_version = @row_version
RETURNING id, import_type, file_id, mapping, dry_run, status, total_rows, success_rows, error_rows, error_file_id, row_version, created_at, created_by, updated_at;
