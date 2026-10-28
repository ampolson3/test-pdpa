-- ROPA-20: ropa.sme_exemption_checks was already fully specified in the baseline migrations (migration
-- 00008, FK'd in 00018) — one row per screening round against a legal entity, citing the PLT-06 form
-- submission it was computed from. No new migration for this table.

-- name: InsertSmeExemptionCheck :one
INSERT INTO ropa.sme_exemption_checks (id, tenant_id, legal_entity_id, form_submission_id, result, basis, assessed_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @legal_entity_id, @form_submission_id, @result, @basis, @assessed_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, legal_entity_id, form_submission_id, result, basis, assessed_at, row_version, created_at;

-- name: GetSmeExemptionCheck :one
SELECT id, legal_entity_id, form_submission_id, result, basis, assessed_at, row_version, created_at
FROM ropa.sme_exemption_checks WHERE id = @id;

-- name: ListSmeExemptionChecksByLegalEntity :many
SELECT id, legal_entity_id, form_submission_id, result, basis, assessed_at, row_version, created_at
FROM ropa.sme_exemption_checks WHERE legal_entity_id = @legal_entity_id ORDER BY assessed_at DESC;
