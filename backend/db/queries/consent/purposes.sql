-- CON-12 purposes: the purposes row carries what is live; drafts and approval live in platform.record_versions (PLT-08).

-- name: ListPurposes :many
SELECT p.id, p.code, p.name_th, p.name_en, p.description_th, p.description_en, p.legal_entity_id, p.lawful_basis_code,
       p.is_sensitive, p.requires_explicit, p.min_age, p.lifespan_days, p.status, p.current_version_id, p.data_category_codes,
       p.row_version, p.updated_at, v.version_no AS current_version_no
FROM consent.purposes p LEFT JOIN consent.purpose_versions v ON v.id = p.current_version_id
ORDER BY p.code;

-- name: GetPurpose :one
SELECT p.id, p.code, p.name_th, p.name_en, p.description_th, p.description_en, p.legal_entity_id, p.lawful_basis_code,
       p.is_sensitive, p.requires_explicit, p.min_age, p.lifespan_days, p.status, p.current_version_id, p.data_category_codes,
       p.row_version, p.updated_at, v.version_no AS current_version_no
FROM consent.purposes p LEFT JOIN consent.purpose_versions v ON v.id = p.current_version_id
WHERE p.id = $1;

-- name: GetPurposeByCode :one
SELECT id FROM consent.purposes WHERE code = $1;

-- name: InsertPurpose :exec
INSERT INTO consent.purposes (id, tenant_id, code, name_th, name_en, legal_entity_id, lawful_basis_code, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, 'CONSENT',
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: LockPurpose :one
SELECT id, status, current_version_id FROM consent.purposes WHERE id = $1 FOR UPDATE;

-- name: ApplyPurpose :exec
-- A published version becomes the live purpose (versioning OnPublish).
UPDATE consent.purposes
SET name_th = @name_th, name_en = @name_en, description_th = @description_th, description_en = @description_en,
    lawful_basis_code = @lawful_basis_code, is_sensitive = @is_sensitive, requires_explicit = @requires_explicit,
    min_age = @min_age, lifespan_days = @lifespan_days, data_category_codes = @data_category_codes,
    status = 'active', current_version_id = @current_version_id, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: SetPurposeStatus :exec
UPDATE consent.purposes SET status = $2, row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1;

-- name: NextPurposeVersionNo :one
SELECT (coalesce(max(version_no), 0) + 1)::int FROM consent.purpose_versions WHERE purpose_id = $1;

-- name: InsertPurposeVersion :exec
INSERT INTO consent.purpose_versions (id, tenant_id, purpose_id, version_no, text_th, text_en, explicit_text_th, explicit_text_en, change_type, requires_reconsent, published_at, approved_by, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, now(), $10,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: ListPurposeVersions :many
SELECT id, purpose_id, version_no, text_th, text_en, explicit_text_th, explicit_text_en, change_type, requires_reconsent, published_at, approved_by
FROM consent.purpose_versions WHERE purpose_id = $1 ORDER BY version_no DESC;

-- name: GetPurposeVersion :one
SELECT id, purpose_id, version_no, text_th, text_en, explicit_text_th, explicit_text_en, change_type, requires_reconsent, published_at, approved_by
FROM consent.purpose_versions WHERE id = $1;

-- name: DeletePurposePreferences :exec
DELETE FROM consent.purpose_preferences WHERE purpose_id = $1;

-- name: InsertPurposePreference :exec
INSERT INTO consent.purpose_preferences (tenant_id, purpose_id, code, name_th, name_en, pref_type, options, display_order, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: ListPurposePreferences :many
SELECT purpose_id, code, name_th, name_en, pref_type, options, display_order
FROM consent.purpose_preferences WHERE purpose_id = ANY (@purpose_ids::uuid[]) ORDER BY purpose_id, display_order, code;

-- name: CountPurposeUse :one
-- Collection points still showing the purpose (retiring it would leave them broken).
SELECT count(*) FROM consent.collection_point_purposes cpp JOIN consent.collection_points cp ON cp.id = cpp.collection_point_id
WHERE cpp.purpose_id = $1 AND cp.status = 'active';
