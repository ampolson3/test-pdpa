-- name: ListAppointments :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, legal_entity_id, dpo_type, user_id, external_name, external_company, contact_email, contact_phone,
    appointed_at, appointment_file_id, pdpc_notified_at, pdpc_evidence_file_id, ended_at, row_version, created_at, updated_at
FROM dpo.appointments
WHERE (sqlc.narg(legal_entity_id)::uuid IS NULL OR legal_entity_id = sqlc.narg(legal_entity_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetAppointment :one
SELECT id, legal_entity_id, dpo_type, user_id, external_name, external_company, contact_email, contact_phone,
    appointed_at, appointment_file_id, pdpc_notified_at, pdpc_evidence_file_id, ended_at, row_version, created_at, updated_at
FROM dpo.appointments
WHERE id = $1;

-- name: CurrentAppointment :one
-- DPO-01's merge-field source: the legal entity's active appointment (not yet ended), most recently appointed.
SELECT id, legal_entity_id, dpo_type, user_id, external_name, external_company, contact_email, contact_phone,
    appointed_at, appointment_file_id, pdpc_notified_at, pdpc_evidence_file_id, ended_at, row_version, created_at, updated_at
FROM dpo.appointments
WHERE legal_entity_id = $1 AND ended_at IS NULL
ORDER BY appointed_at DESC, created_at DESC
LIMIT 1;

-- name: InsertAppointment :one
INSERT INTO dpo.appointments (id, tenant_id, legal_entity_id, dpo_type, user_id, external_name, external_company,
    contact_email, contact_phone, appointed_at, appointment_file_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, dpo_type, user_id, external_name, external_company, contact_email, contact_phone,
    appointed_at, appointment_file_id, pdpc_notified_at, pdpc_evidence_file_id, ended_at, row_version, created_at, updated_at;

-- name: UpdateAppointment :one
UPDATE dpo.appointments
SET legal_entity_id = $3, dpo_type = $4, user_id = $5, external_name = $6, external_company = $7,
    contact_email = $8, contact_phone = $9, appointed_at = $10, appointment_file_id = $11,
    pdpc_notified_at = $12, pdpc_evidence_file_id = $13, ended_at = $14, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, legal_entity_id, dpo_type, user_id, external_name, external_company, contact_email, contact_phone,
    appointed_at, appointment_file_id, pdpc_notified_at, pdpc_evidence_file_id, ended_at, row_version, created_at, updated_at;
