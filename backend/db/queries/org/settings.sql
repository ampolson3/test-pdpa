-- name: GetOrgSettings :one
SELECT tenant_id, default_language, date_era, branding, default_calendar_id, row_version, updated_at
FROM org.org_settings
WHERE tenant_id = current_setting('app.tenant_id')::uuid;

-- name: UpsertOrgSettings :one
INSERT INTO org.org_settings (tenant_id, default_language, date_era, branding, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT (tenant_id) DO UPDATE
SET default_language = EXCLUDED.default_language, date_era = EXCLUDED.date_era, branding = EXCLUDED.branding,
    updated_by = EXCLUDED.updated_by, row_version = org.org_settings.row_version + 1
WHERE org.org_settings.row_version = $4
RETURNING tenant_id, default_language, date_era, branding, default_calendar_id, row_version, updated_at;
