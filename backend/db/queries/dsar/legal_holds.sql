-- name: ListActiveLegalHoldsForAsset :many
-- DPX-05's legal-hold check: any active hold naming this exact asset blocks that asset's disposal jobs.
-- Scoped to asset_id only (not also matching by data_category_id) — an engineering simplification, not a
-- docs/decisions.md question: a hold recorded against a data_category_id with no asset_id is DSAR-11's own
-- exemption-check concern, not this feature's literal acceptance criterion.
SELECT id, asset_id, law_ref
FROM dsar.legal_holds
WHERE asset_id = $1 AND is_active = true;
