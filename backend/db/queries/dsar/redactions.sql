-- DSAR-16: dsar.redactions was already fully specified in the baseline migrations (00012_dsar.sql) —
-- source_file_id/output_file_id/regions jsonb/status detecting|review|done/reviewed_by — unused until now.
-- No OCR is available in this environment (the same gap DSAR-06/decisions.md Q-30 already resolved), so a
-- round skips 'detecting' straight to 'review'/'done' in one call: staff mark every rectangle themselves.

-- name: InsertRedaction :one
INSERT INTO dsar.redactions (tenant_id, request_id, source_file_id, output_file_id, regions, status, reviewed_by,
    created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, sqlc.arg(request_id), sqlc.arg(source_file_id), sqlc.arg(output_file_id),
    sqlc.arg(regions), 'done',
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_id, source_file_id, output_file_id, regions, status, reviewed_by, created_at;

-- name: ListRedactionsForRequest :many
SELECT id, request_id, source_file_id, output_file_id, regions, status, reviewed_by, created_at FROM dsar.redactions
WHERE request_id = $1 ORDER BY created_at DESC;
