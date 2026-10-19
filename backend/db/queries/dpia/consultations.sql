-- DPIA-11: stakeholder consultation log.

-- name: ListConsultations :many
SELECT id, assessment_id, stakeholder, stakeholder_type, consulted_at, summary, response, row_version, created_at
FROM assess.consultations
WHERE assessment_id = $1
ORDER BY consulted_at, created_at;

-- name: InsertConsultation :one
INSERT INTO assess.consultations (id, tenant_id, assessment_id, stakeholder, stakeholder_type, consulted_at,
    summary, response, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, assessment_id, stakeholder, stakeholder_type, consulted_at, summary, response, row_version, created_at;

-- name: DeleteConsultation :execrows
DELETE FROM assess.consultations WHERE id = $1 AND assessment_id = $2;
