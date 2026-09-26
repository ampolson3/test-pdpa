-- name: ListActivities :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, row_version, created_at, updated_at
FROM ropa.processing_activities
WHERE (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q) || '%' OR code ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetActivity :one
SELECT id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, row_version, created_at, updated_at
FROM ropa.processing_activities
WHERE id = $1;

-- name: InsertActivity :one
INSERT INTO ropa.processing_activities (id, tenant_id, legal_entity_id, org_unit_id, code, name, description, role,
    controller_party_id, owner_user_id, rights_and_access, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, row_version, created_at, updated_at;

-- name: UpdateActivity :one
UPDATE ropa.processing_activities
SET legal_entity_id = $3, org_unit_id = $4, code = $5, name = $6, description = $7, role = $8,
    controller_party_id = $9, owner_user_id = $10, rights_and_access = $11,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, row_version, created_at, updated_at;

-- name: SetActivityCompleteness :exec
UPDATE ropa.processing_activities SET completeness = $2 WHERE id = $1;

-- name: SubmitActivity :one
UPDATE ropa.processing_activities
SET status = 'pending_approval', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status IN ('draft', 'under_review')
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, row_version, created_at, updated_at;

-- name: ListActivityPurposes :many
SELECT id, activity_id, purpose_id, purpose_text, lawful_basis_code, consent_purpose_id, row_version, created_at
FROM ropa.activity_purposes
WHERE activity_id = $1
ORDER BY created_at;

-- name: InsertActivityPurpose :one
INSERT INTO ropa.activity_purposes (id, tenant_id, activity_id, purpose_id, purpose_text, lawful_basis_code,
    consent_purpose_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, activity_id, purpose_id, purpose_text, lawful_basis_code, consent_purpose_id, row_version, created_at;

-- name: DeleteActivityPurpose :execrows
DELETE FROM ropa.activity_purposes WHERE id = $1 AND activity_id = $2;

-- name: ListActivityData :many
SELECT ad.id, ad.activity_id, ad.data_category_id, ad.subject_type_id, ad.source, ad.source_party_id,
    ad.is_sensitive, ad.volume_band, ad.row_version, ad.created_at
FROM ropa.activity_data ad
WHERE ad.activity_id = $1
ORDER BY ad.created_at;

-- name: InsertActivityData :one
INSERT INTO ropa.activity_data (id, tenant_id, activity_id, data_category_id, subject_type_id, source,
    source_party_id, is_sensitive, volume_band, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, activity_id, data_category_id, subject_type_id, source, source_party_id, is_sensitive, volume_band, row_version, created_at;

-- name: DeleteActivityData :execrows
DELETE FROM ropa.activity_data WHERE id = $1 AND activity_id = $2;

-- name: ListRetentionRules :many
SELECT id, activity_id, data_category_id, retention_months, retention_basis, trigger_event, disposal_method,
    row_version, created_at
FROM ropa.retention_rules
WHERE activity_id = $1
ORDER BY created_at;

-- name: InsertRetentionRule :one
INSERT INTO ropa.retention_rules (id, tenant_id, activity_id, data_category_id, retention_months, retention_basis,
    trigger_event, disposal_method, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, activity_id, data_category_id, retention_months, retention_basis, trigger_event, disposal_method, row_version, created_at;

-- name: DeleteRetentionRule :execrows
DELETE FROM ropa.retention_rules WHERE id = $1 AND activity_id = $2;

-- name: ListActivityRecipients :many
SELECT id, activity_id, party_id, recipient_role, disclosure_basis, data_category_ids, row_version, created_at
FROM ropa.activity_recipients
WHERE activity_id = $1
ORDER BY created_at;

-- name: InsertActivityRecipient :one
INSERT INTO ropa.activity_recipients (id, tenant_id, activity_id, party_id, recipient_role, disclosure_basis,
    data_category_ids, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, activity_id, party_id, recipient_role, disclosure_basis, data_category_ids, row_version, created_at;

-- name: DeleteActivityRecipient :execrows
DELETE FROM ropa.activity_recipients WHERE id = $1 AND activity_id = $2;
