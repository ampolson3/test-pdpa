-- ORG-07 master data: global rows (tenant_id NULL, read-only for tenants) and the tenant's own.

-- name: ListDataCategories :many
SELECT id, tenant_id, code, name_th, name_en, is_sensitive, sensitive_type, parent_id, row_version
FROM org.data_categories ORDER BY (tenant_id IS NOT NULL), is_sensitive, code;

-- name: GetDataCategory :one
SELECT id, tenant_id, code, name_th, name_en, is_sensitive, sensitive_type, parent_id, row_version
FROM org.data_categories WHERE id = $1;

-- name: InsertDataCategory :one
INSERT INTO org.data_categories (id, tenant_id, code, name_th, name_en, is_sensitive, sensitive_type, parent_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, code, name_th, name_en, is_sensitive, sensitive_type, parent_id, row_version;

-- name: UpdateDataCategory :one
UPDATE org.data_categories
SET name_th = $3, name_en = $4, is_sensitive = $5, sensitive_type = $6, parent_id = $7, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL
RETURNING id, tenant_id, code, name_th, name_en, is_sensitive, sensitive_type, parent_id, row_version;

-- name: DeleteDataCategory :execrows
DELETE FROM org.data_categories WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL;

-- name: ListDataSubjectTypes :many
SELECT id, tenant_id, code, name_th, name_en, is_vulnerable, row_version
FROM org.data_subject_types ORDER BY (tenant_id IS NOT NULL), code;

-- name: GetDataSubjectType :one
SELECT id, tenant_id, code, name_th, name_en, is_vulnerable, row_version FROM org.data_subject_types WHERE id = $1;

-- name: InsertDataSubjectType :one
INSERT INTO org.data_subject_types (id, tenant_id, code, name_th, name_en, is_vulnerable, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, code, name_th, name_en, is_vulnerable, row_version;

-- name: UpdateDataSubjectType :one
UPDATE org.data_subject_types
SET name_th = $3, name_en = $4, is_vulnerable = $5, row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL
RETURNING id, tenant_id, code, name_th, name_en, is_vulnerable, row_version;

-- name: DeleteDataSubjectType :execrows
DELETE FROM org.data_subject_types WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL;

-- name: ListProcessingPurposes :many
SELECT id, tenant_id, code, name_th, name_en, category, row_version
FROM org.processing_purposes ORDER BY (tenant_id IS NOT NULL), code;

-- name: GetProcessingPurpose :one
SELECT id, tenant_id, code, name_th, name_en, category, row_version FROM org.processing_purposes WHERE id = $1;

-- name: InsertProcessingPurpose :one
INSERT INTO org.processing_purposes (id, tenant_id, code, name_th, name_en, category, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, code, name_th, name_en, category, row_version;

-- name: UpdateProcessingPurpose :one
UPDATE org.processing_purposes
SET name_th = $3, name_en = $4, category = $5, row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL
RETURNING id, tenant_id, code, name_th, name_en, category, row_version;

-- name: DeleteProcessingPurpose :execrows
DELETE FROM org.processing_purposes WHERE id = $1 AND row_version = $2 AND tenant_id IS NOT NULL;

-- name: GlobalCodeExists :one
-- Whether a global row of the kind already uses the code (tenant rows may not shadow defaults).
SELECT CASE sqlc.arg(kind)::text
    WHEN 'data_categories' THEN EXISTS (SELECT 1 FROM org.data_categories WHERE tenant_id IS NULL AND code = sqlc.arg(code)::text)
    WHEN 'data_subject_types' THEN EXISTS (SELECT 1 FROM org.data_subject_types WHERE tenant_id IS NULL AND code = sqlc.arg(code)::text)
    ELSE EXISTS (SELECT 1 FROM org.processing_purposes WHERE tenant_id IS NULL AND code = sqlc.arg(code)::text)
END::boolean;

-- name: ListLawfulBases :many
SELECT code, section_ref, name_th, name_en, for_sensitive, requires_consent, requires_lia FROM org.lawful_bases ORDER BY for_sensitive, section_ref, code;

-- name: ListCountries :many
SELECT code, name_th, name_en, adequacy_status, region FROM org.countries ORDER BY code;
