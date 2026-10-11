-- name: ListActivityTransfers :many
SELECT id, activity_id, recipient_id, country_code, transfer_basis, safeguards, tia_assessment_id, row_version, created_at
FROM ropa.activity_transfers
WHERE activity_id = $1
ORDER BY created_at;

-- name: GetActivityTransfer :one
SELECT id, activity_id, recipient_id, country_code, transfer_basis, safeguards, tia_assessment_id, row_version, created_at
FROM ropa.activity_transfers
WHERE id = $1 AND activity_id = $2;

-- name: InsertActivityTransfer :one
INSERT INTO ropa.activity_transfers (id, tenant_id, activity_id, recipient_id, country_code, transfer_basis,
    safeguards, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, activity_id, recipient_id, country_code, transfer_basis, safeguards, tia_assessment_id, row_version, created_at;

-- name: DeleteActivityTransfer :execrows
DELETE FROM ropa.activity_transfers WHERE id = $1 AND activity_id = $2;

-- name: SetTransferTIA :one
-- DPX-06: links a cross-border transfer to the assess.assessments row its TIA was just recorded into.
UPDATE ropa.activity_transfers
SET tia_assessment_id = @tia_assessment_id, updated_at = now(),
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND activity_id = @activity_id
RETURNING id, activity_id, recipient_id, country_code, transfer_basis, safeguards, tia_assessment_id, row_version, created_at;
