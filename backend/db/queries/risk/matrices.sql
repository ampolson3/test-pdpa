-- RRA-02 (ตั้งค่า risk matrix): CRUD on risk.risk_matrices, already fully specified in the baseline
-- migrations. One default per tenant is enforced by a partial unique index (migration 00055); unsetting
-- every other default happens in the same statement as setting the new one, in one UPDATE, so there is
-- never a moment with two defaults or none during a swap.

-- name: ListRiskMatrices :many
SELECT * FROM risk.risk_matrices ORDER BY is_default DESC, name;

-- name: GetRiskMatrix :one
SELECT * FROM risk.risk_matrices WHERE id = $1;

-- name: GetDefaultRiskMatrix :one
SELECT * FROM risk.risk_matrices WHERE is_default LIMIT 1;

-- name: InsertRiskMatrix :one
INSERT INTO risk.risk_matrices (tenant_id, name, likelihood_levels, impact_levels, thresholds, is_default, created_by, updated_by)
VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, @name, @likelihood_levels, @impact_levels, @thresholds, @is_default,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: UpdateRiskMatrix :one
UPDATE risk.risk_matrices
SET name = @name, likelihood_levels = @likelihood_levels, impact_levels = @impact_levels, thresholds = @thresholds,
    is_default = @is_default, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING *;

-- name: ClearOtherDefaultRiskMatrices :exec
UPDATE risk.risk_matrices SET is_default = false, row_version = row_version + 1 WHERE is_default AND id != @id;

-- name: DeleteRiskMatrix :execrows
DELETE FROM risk.risk_matrices WHERE id = $1 AND row_version = $2;
