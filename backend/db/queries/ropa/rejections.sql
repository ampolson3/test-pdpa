-- name: InsertActivityRejection :many
INSERT INTO ropa.activity_rejections (tenant_id, activity_id, dsar_request_id, reason_code, rejected_at, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT (activity_id, dsar_request_id) DO NOTHING
RETURNING id, activity_id, dsar_request_id, reason_code, rejected_at, row_version;

-- name: ListActivityRejections :many
SELECT id, activity_id, dsar_request_id, reason_code, rejected_at, row_version
FROM ropa.activity_rejections WHERE activity_id = $1 ORDER BY rejected_at DESC;
