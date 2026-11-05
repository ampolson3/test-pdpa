-- name: ListActiveLegalHoldsForAsset :many
-- DPX-05's legal-hold check: any active hold naming this exact asset blocks that asset's disposal jobs.
-- Scoped to asset_id only (not also matching by data_category_id) — an engineering simplification, not a
-- docs/decisions.md question: a hold recorded against a data_category_id with no asset_id is DSAR-10's own
-- exemption-check concern (below), not this feature's literal acceptance criterion.
SELECT id, asset_id, law_ref
FROM dsar.legal_holds
WHERE asset_id = $1 AND is_active = true;

-- name: ListActiveLegalHoldsForAssetsOrCategories :many
-- DSAR-10's own exemption check: every active hold naming any of the given assets directly, OR any of the
-- given data categories (resolved by the caller from each asset's own ROPA-01 data inventory) with no
-- asset_id of its own — the broader match DPX-05's own query above deliberately left out of scope.
SELECT id, data_category_id, asset_id, law_ref, hold_rule
FROM dsar.legal_holds
WHERE is_active = true
  AND (
    asset_id = ANY(@asset_ids::uuid[])
    OR (asset_id IS NULL AND data_category_id = ANY(@data_category_ids::uuid[]))
  );

-- name: CreateLegalHold :one
INSERT INTO dsar.legal_holds (tenant_id, data_category_id, asset_id, law_ref, hold_rule)
VALUES (current_setting('app.tenant_id')::uuid, sqlc.narg(data_category_id), sqlc.narg(asset_id), @law_ref, @hold_rule)
RETURNING id, tenant_id, data_category_id, asset_id, law_ref, hold_rule, is_active, created_at, updated_at, row_version;

-- name: ListLegalHolds :many
SELECT id, tenant_id, data_category_id, asset_id, law_ref, hold_rule, is_active, created_at, updated_at, row_version
FROM dsar.legal_holds
ORDER BY created_at DESC;

-- name: GetLegalHold :one
SELECT id, tenant_id, data_category_id, asset_id, law_ref, hold_rule, is_active, created_at, updated_at, row_version
FROM dsar.legal_holds
WHERE id = $1;

-- name: DeactivateLegalHold :one
UPDATE dsar.legal_holds
SET is_active = false
WHERE id = $1 AND row_version = $2
RETURNING id, tenant_id, data_category_id, asset_id, law_ref, hold_rule, is_active, created_at, updated_at, row_version;

-- name: InsertExemptionCheck :one
INSERT INTO dsar.exemption_checks (tenant_id, request_id, legal_hold_id, result, reason)
VALUES (current_setting('app.tenant_id')::uuid, @request_id, sqlc.narg(legal_hold_id), @result, @reason)
RETURNING id, tenant_id, request_id, legal_hold_id, result, reason, created_at;

-- name: ListExemptionChecksForRequest :many
SELECT id, tenant_id, request_id, legal_hold_id, result, reason, created_at
FROM dsar.exemption_checks
WHERE request_id = $1
ORDER BY created_at DESC;

-- name: CountExemptionChecksForRequest :one
SELECT count(*)::int FROM dsar.exemption_checks WHERE request_id = $1;
