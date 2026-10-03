-- DSAR-06 identity verification (dsar.verifications — already fully specified in the baseline migrations).

-- name: InsertVerification :one
INSERT INTO dsar.verifications (id, tenant_id, request_id, method, subject_verification_id, masked_id_file_id,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @request_id, @method, @subject_verification_id,
    @masked_id_file_id, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_id, method, subject_verification_id, masked_id_file_id, status, verified_by, verified_at,
    created_at, row_version;

-- name: GetVerification :one
SELECT id, request_id, method, subject_verification_id, masked_id_file_id, status, verified_by, verified_at,
    created_at, row_version
FROM dsar.verifications
WHERE id = $1;

-- name: ListVerificationsForRequest :many
SELECT id, request_id, method, subject_verification_id, masked_id_file_id, status, verified_by, verified_at,
    created_at, row_version
FROM dsar.verifications
WHERE request_id = $1
ORDER BY created_at;

-- name: DecideVerification :one
-- Guarded to a still-pending row so a verification is decided once (DecideVerification refused with no rows
-- affected if it's already passed/failed — a redelivery or double-click is a harmless no-op from the caller's
-- own perspective, surfaced as the current, unchanged row by GetVerification).
UPDATE dsar.verifications
SET status = @status, verified_by = NULLIF(current_setting('app.user_id', true), '')::uuid, verified_at = now(),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND status = 'pending'
RETURNING id, request_id, method, subject_verification_id, masked_id_file_id, status, verified_by, verified_at,
    created_at, row_version;
