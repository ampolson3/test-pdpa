-- name: ListIndependenceDeclarations :many
-- DPO-03's own list, newest year first, for one appointment.
SELECT id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at
FROM dpo.independence_declarations
WHERE appointment_id = $1
ORDER BY year DESC;

-- name: GetIndependenceDeclaration :one
SELECT id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at
FROM dpo.independence_declarations
WHERE id = $1;

-- name: GetIndependenceDeclarationForYear :one
-- At most one declaration per appointment per year (DPO-03's own acceptance criterion: a signed
-- declaration must exist for every year) -- resolved by appointment + year, not a separate unique
-- index, since RecordDeclaration itself enforces "update the existing one" before ever inserting.
SELECT id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at
FROM dpo.independence_declarations
WHERE appointment_id = $1 AND year = $2;

-- name: InsertIndependenceDeclaration :one
INSERT INTO dpo.independence_declarations (id, tenant_id, appointment_id, year, other_duties, has_conflict,
    created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at;

-- name: UpdateIndependenceDeclaration :one
-- Editing an already-signed declaration is refused by the service layer, not here -- this query alone
-- would happily overwrite a signed one, same as every other sqlc UPDATE in this codebase leaves its
-- own business rules to the caller.
UPDATE dpo.independence_declarations
SET other_duties = $3, has_conflict = $4, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at;

-- name: SignIndependenceDeclaration :one
-- The acceptance criterion's own write: stamp signed_at (once -- the service checks it's still unsigned)
-- and optionally attach the caller's own clean, still-unattached PLT-09 upload as the signed evidence.
UPDATE dpo.independence_declarations
SET signed_at = $3, file_id = $4, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, appointment_id, year, other_duties, has_conflict, signed_at, file_id, row_version, created_at, updated_at;
