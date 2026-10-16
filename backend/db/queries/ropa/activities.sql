-- name: ListActivities :many
-- Newest first; keyset cursor on (created_at, id). ROPA-14's own acceptance criterion: with no explicit
-- status filter, an ended activity is left out of this "current" list — the second clause of the status
-- condition. Searching retroactively is just passing status=ended explicitly, which the first clause of
-- the same condition already allows through for any status value.
SELECT id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at
FROM ropa.processing_activities
WHERE (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id))
  AND (status = sqlc.narg(status) OR (sqlc.narg(status)::text IS NULL AND status <> 'ended'))
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q) || '%' OR code ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetActivity :one
SELECT id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at
FROM ropa.processing_activities
WHERE id = $1;

-- name: EndActivity :one
-- ST-05 "active -> ended" edge (ROPA-14's own acceptance criterion): reason is required by the service,
-- not the column (kept nullable so a row ended before this feature's own validation existed would still
-- read back fine) — history (every child table row) stays exactly as it was, only status/ended_at/
-- end_reason change.
UPDATE ropa.processing_activities
SET status = 'ended', ended_at = now(), end_reason = @end_reason,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'active'
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: ListProcessorActivities :many
-- ROPA-04: every processor-role activity, for the export — code order, not paginated (a tenant's
-- processor RoPA is meant to be printed/filed as one document, same as ORG-19's audit CSV export).
SELECT id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at
FROM ropa.processing_activities
WHERE role = 'processor'
ORDER BY code;

-- name: InsertActivity :one
INSERT INTO ropa.processing_activities (id, tenant_id, legal_entity_id, org_unit_id, code, name, description, role,
    controller_party_id, owner_user_id, rights_and_access, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: UpdateActivity :one
UPDATE ropa.processing_activities
SET legal_entity_id = $3, org_unit_id = $4, code = $5, name = $6, description = $7, role = $8,
    controller_party_id = $9, owner_user_id = $10, rights_and_access = $11,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: SetActivityCompleteness :exec
UPDATE ropa.processing_activities SET completeness = $2 WHERE id = $1;

-- name: SubmitActivity :one
UPDATE ropa.processing_activities
SET status = 'pending_approval', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status IN ('draft', 'under_review')
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: ApproveActivity :one
-- ROPA-13 (ST-05): pending_approval -> active. approved_by/approved_at were already in the baseline
-- schema, unused until now.
UPDATE ropa.processing_activities
SET status = 'active', approved_by = NULLIF(current_setting('app.user_id', true), '')::uuid, approved_at = now(),
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'pending_approval'
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: ReturnActivity :one
-- ROPA-13 (ST-05): pending_approval -> draft ("ส่งกลับ"), reason kept only in the audit row (the table
-- itself has no reason column, same as every other field this migration didn't add one for).
UPDATE ropa.processing_activities
SET status = 'draft', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'pending_approval'
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: RequestActivityReview :one
-- ROPA-13 (ST-05): active -> under_review ("ครบรอบ / เปลี่ยนแปลง") — the only way to open an active
-- activity for editing again; SaveActivity itself refuses direct edits outside draft/under_review.
UPDATE ropa.processing_activities
SET status = 'under_review', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'active'
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

-- name: ConfirmActivityNoChange :one
-- ROPA-13 (ST-05): under_review -> active ("ยืนยันไม่เปลี่ยน") — a review round closed with nothing to
-- re-approve, so no new ropa.activity_versions row is taken for it.
UPDATE ropa.processing_activities
SET status = 'active', updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2 AND status = 'under_review'
RETURNING id, legal_entity_id, org_unit_id, code, name, description, role, controller_party_id, owner_user_id,
    status, completeness, rights_and_access, ended_at, end_reason, row_version, created_at, updated_at;

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

-- name: GetRetentionRule :one
SELECT id, activity_id, data_category_id, retention_months, retention_basis, trigger_event, disposal_method,
    row_version, created_at
FROM ropa.retention_rules
WHERE id = $1;

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
