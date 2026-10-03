-- ORG-01 legal entities · ORG-04 org units.

-- name: ListLegalEntities :many
SELECT id, parent_id, name_th, name_en, registration_no, tax_id, address, contact_email, contact_phone, logo_file_id,
       is_controller, is_processor, status, row_version, updated_at
FROM org.legal_entities ORDER BY status, name_th;

-- name: GetLegalEntity :one
SELECT id, parent_id, name_th, name_en, registration_no, tax_id, address, contact_email, contact_phone, logo_file_id,
       is_controller, is_processor, status, row_version, updated_at
FROM org.legal_entities WHERE id = $1;

-- name: InsertLegalEntity :one
INSERT INTO org.legal_entities (id, tenant_id, parent_id, name_th, name_en, registration_no, tax_id, address, contact_email, contact_phone,
                                logo_file_id, is_controller, is_processor, status, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, parent_id, name_th, name_en, registration_no, tax_id, address, contact_email, contact_phone, logo_file_id,
          is_controller, is_processor, status, row_version, updated_at;

-- name: UpdateLegalEntity :one
UPDATE org.legal_entities
SET parent_id = $3, name_th = $4, name_en = $5, registration_no = $6, tax_id = $7, address = $8, contact_email = $9, contact_phone = $10,
    logo_file_id = $11, is_controller = $12, is_processor = $13, status = $14, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, parent_id, name_th, name_en, registration_no, tax_id, address, contact_email, contact_phone, logo_file_id,
          is_controller, is_processor, status, row_version, updated_at;

-- name: LegalEntityAncestors :many
-- The chain of parents of a legal entity (to refuse cycles).
WITH RECURSIVE up AS (
    SELECT l.id, l.parent_id FROM org.legal_entities l WHERE l.id = $1
    UNION ALL
    SELECT p.id, p.parent_id FROM org.legal_entities p JOIN up ON p.id = up.parent_id
)
SELECT id FROM up;

-- name: ListOrgUnits :many
SELECT id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at
FROM org.org_units
WHERE (sqlc.narg(legal_entity_id)::uuid IS NULL OR legal_entity_id = sqlc.narg(legal_entity_id)::uuid)
  AND (@include_closed::boolean OR status = 'active')
ORDER BY path;

-- name: GetOrgUnit :one
SELECT id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at
FROM org.org_units WHERE id = $1;

-- name: LockOrgUnit :one
SELECT id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at
FROM org.org_units WHERE id = $1 FOR UPDATE;

-- name: InsertOrgUnit :one
INSERT INTO org.org_units (id, tenant_id, legal_entity_id, parent_id, path, code, name_th, name_en, unit_type, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, (@path::text)::ltree, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at;

-- name: UpdateOrgUnit :one
UPDATE org.org_units
SET code = $3, name_th = $4, name_en = $5, unit_type = $6, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at;

-- name: MoveOrgSubtree :execrows
-- Re-roots the subtree at old_path under new_prefix (the unit's new path) in one statement.
UPDATE org.org_units
SET path = CASE WHEN path = (@old_path::text)::ltree THEN (@new_path::text)::ltree
                ELSE (@new_path::text)::ltree || subpath(path, nlevel((@old_path::text)::ltree)) END,
    row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE path <@ (@old_path::text)::ltree;

-- name: SetOrgUnitParent :exec
UPDATE org.org_units SET parent_id = $2 WHERE id = $1;

-- name: CountActiveChildren :one
SELECT count(*)::int FROM org.org_units WHERE parent_id = $1 AND status = 'active';

-- name: CloseOrgUnit :one
UPDATE org.org_units
SET status = 'closed', closed_at = now(), row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2 AND status = 'active'
RETURNING id, legal_entity_id, parent_id, path::text AS path, nlevel(path)::int AS depth, code, name_th, name_en, unit_type, status, closed_at, row_version, updated_at;

-- name: UnitWithin :one
-- Whether unit_id is scope_id itself or (when descendants count) below it — evaluated on the current tree.
SELECT EXISTS (
    SELECT 1 FROM org.org_units u, org.org_units s
    WHERE u.id = @unit_id AND s.id = @scope_id
      AND (u.id = s.id OR (@include_descendants::boolean AND u.path <@ s.path))
)::boolean;
