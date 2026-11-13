-- DSAR-14: secure delivery packages (dsar.packages). token_hash holds the SHA-256 hex of a separate
-- access code (the module doc's own "รหัสผ่าน"), never the plaintext; the download link itself is a
-- platform.public_keys key pointing at the package's own id.

-- name: InsertPackage :one
INSERT INTO dsar.packages (id, tenant_id, request_id, file_id, format, token_hash, password_protected, expires_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, @request_id, @file_id, @format, @token_hash, @password_protected, @expires_at,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_id, file_id, format, token_hash, password_protected, expires_at, download_count, last_downloaded_at, purged_at, row_version, created_at;

-- name: GetPackage :one
SELECT id, request_id, file_id, format, token_hash, password_protected, expires_at, download_count, last_downloaded_at, purged_at, row_version, created_at
FROM dsar.packages WHERE id = $1;

-- name: LockPackage :one
SELECT id, request_id, file_id, format, token_hash, password_protected, expires_at, download_count, last_downloaded_at, purged_at, row_version, created_at
FROM dsar.packages WHERE id = $1 FOR UPDATE;

-- name: ListPackagesForRequest :many
SELECT id, request_id, file_id, format, token_hash, password_protected, expires_at, download_count, last_downloaded_at, purged_at, row_version, created_at
FROM dsar.packages WHERE request_id = $1 ORDER BY created_at DESC;

-- name: RecordPackageDownload :exec
UPDATE dsar.packages
SET download_count = download_count + 1, last_downloaded_at = now(), row_version = row_version + 1
WHERE id = $1;

-- name: MarkPackagePurged :exec
UPDATE dsar.packages SET purged_at = now(), row_version = row_version + 1
WHERE id = $1 AND purged_at IS NULL;
