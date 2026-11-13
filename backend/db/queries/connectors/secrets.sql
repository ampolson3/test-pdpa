-- name: UpsertConnectorSecret :exec
INSERT INTO platform.connector_secrets (id, tenant_id, secret_ref, value_enc, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT (tenant_id, secret_ref) DO UPDATE SET
    value_enc = EXCLUDED.value_enc,
    updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = platform.connector_secrets.row_version + 1;

-- name: GetConnectorSecret :one
SELECT value_enc FROM platform.connector_secrets WHERE secret_ref = $1;

-- name: DeleteConnectorSecret :exec
DELETE FROM platform.connector_secrets WHERE secret_ref = $1;
