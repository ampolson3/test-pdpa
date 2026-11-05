-- name: DeleteAnnexesByType :exec
DELETE FROM agreement.annexes WHERE agreement_id = @agreement_id AND annex_type = @annex_type;

-- name: InsertAnnex :one
INSERT INTO agreement.annexes (tenant_id, agreement_id, annex_type, file_id)
VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, @agreement_id, @annex_type, @file_id)
RETURNING id, agreement_id, annex_type, file_id, created_at, updated_at, row_version;

-- name: ListAnnexes :many
SELECT id, agreement_id, annex_type, file_id, created_at, updated_at, row_version
FROM agreement.annexes WHERE agreement_id = @agreement_id ORDER BY annex_type;
