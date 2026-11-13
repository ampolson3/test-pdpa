-- DPA-03 (ข้อกำหนดที่ต้องมี): attach platform.clause_library entries to an agreement and check them against
-- agreement.mandatory_rules before the document can be submitted for approval.

-- name: InsertAgreementClause :one
INSERT INTO agreement.clauses (id, tenant_id, agreement_id, clause_id, clause_version_no, position, is_mandatory, created_by, updated_by)
VALUES (@id, NULLIF(current_setting('app.tenant_id', true), '')::uuid, @agreement_id, @clause_id, @clause_version_no, @position,
    @is_mandatory, NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: ListAgreementClauses :many
SELECT c.id, c.agreement_id, c.clause_id, c.clause_version_no, c.position, c.is_mandatory, c.row_version, c.created_at,
    cl.code AS clause_code, cl.title AS clause_title, cl.legal_ref AS clause_legal_ref
FROM agreement.clauses c
LEFT JOIN platform.clause_library cl ON cl.id = c.clause_id
WHERE c.agreement_id = $1
ORDER BY c.position, c.created_at;

-- name: DeleteAgreementClause :execrows
DELETE FROM agreement.clauses WHERE id = $1 AND agreement_id = $2;

-- name: CountAgreementClauses :one
SELECT count(*)::int FROM agreement.clauses WHERE agreement_id = $1;

-- name: ListMandatoryRules :many
SELECT * FROM agreement.mandatory_rules WHERE agreement_type = $1 ORDER BY clause_code;

-- name: ListAgreementClauseCodes :many
-- Every clause_library code already attached to this agreement — used to compare against the tenant's own
-- mandatory_rules codes.
SELECT DISTINCT cl.code
FROM agreement.clauses c
JOIN platform.clause_library cl ON cl.id = c.clause_id
WHERE c.agreement_id = $1;
