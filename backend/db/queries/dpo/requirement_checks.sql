-- name: InsertRequirementCheck :one
INSERT INTO dpo.requirement_checks (id, tenant_id, legal_entity_id, assessment_id, result, basis, assessed_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, legal_entity_id, assessment_id, result, basis, assessed_at, created_at, created_by, updated_at, updated_by, row_version;

-- name: GetLatestRequirementCheckForLegalEntity :one
SELECT id, tenant_id, legal_entity_id, assessment_id, result, basis, assessed_at, created_at, created_by, updated_at, updated_by, row_version
FROM dpo.requirement_checks
WHERE legal_entity_id = $1
ORDER BY assessed_at DESC, created_at DESC
LIMIT 1;

-- name: ListRequirementChecks :many
SELECT id, tenant_id, legal_entity_id, assessment_id, result, basis, assessed_at, created_at, created_by, updated_at, updated_by, row_version
FROM dpo.requirement_checks
WHERE legal_entity_id = $1
ORDER BY assessed_at DESC, created_at DESC;
