-- DPIA-13: breach.incident_assets was already fully specified in the baseline migrations, completely unused
-- until this feature's own link from an incident to the RoPA activity it affected (rule 1 FK visibility
-- checked in the service, not here). ON CONFLICT DO NOTHING against migration 00074's own idempotency index
-- makes a redelivered breach.activity_linked-triggering call (or a plain double-click) a harmless no-op.

-- name: InsertIncidentActivity :one
INSERT INTO breach.incident_assets (id, tenant_id, incident_id, activity_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @activity_id,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT ON CONSTRAINT uq_breach_incident_assets_incident_activity DO NOTHING
RETURNING id, incident_id, activity_id, created_at;

-- name: ListIncidentActivities :many
SELECT id, incident_id, activity_id, created_at
FROM breach.incident_assets WHERE incident_id = @incident_id AND activity_id IS NOT NULL ORDER BY created_at;
