-- DPX-02: gov.policies/gov.policy_attestations were already fully specified in the baseline migrations
-- (00017_gov.sql) — a published PLT-16 "policy" document DPO/LEGAL tracks for mandatory employee
-- attestation, with evidence (timestamp + IP) per attestation. No queries existed for either table.

-- name: InsertPolicy :one
INSERT INTO gov.policies (tenant_id, title, document_id, version_no, requires_attestation, published_at, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, @title::text, @document_id::uuid, @version_no::int, @requires_attestation::bool, now(),
        sqlc.narg(actor)::uuid, sqlc.narg(actor)::uuid)
ON CONFLICT ON CONSTRAINT uq_policies_document_id_version_no DO UPDATE SET title = EXCLUDED.title
RETURNING *;

-- name: GetPolicy :one
SELECT * FROM gov.policies WHERE id = $1;

-- name: ListPolicies :many
SELECT * FROM gov.policies ORDER BY published_at DESC NULLS LAST, created_at DESC;

-- name: InsertPolicyAttestation :one
INSERT INTO gov.policy_attestations (tenant_id, policy_id, policy_version_no, user_id, ip)
VALUES (current_setting('app.tenant_id')::uuid, @policy_id::uuid, @policy_version_no::int, @user_id::uuid, sqlc.narg(ip)::inet)
RETURNING *;

-- name: GetPolicyAttestationForUser :one
SELECT * FROM gov.policy_attestations WHERE policy_id = @policy_id::uuid AND user_id = @user_id::uuid;

-- name: ListPolicyAttestations :many
SELECT * FROM gov.policy_attestations WHERE policy_id = $1 ORDER BY attested_at;
