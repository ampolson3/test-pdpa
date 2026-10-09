-- DPA-02 agreement engine (shared with DSA): create an agreement from a vendor + RoPA activities, wrapping a
-- PLT-16 document (document_id).

-- name: LockAgreementNumbering :exec
SELECT pg_advisory_xact_lock(hashtext('agreement.agreement_no:' || current_setting('app.tenant_id') || ':' || @prefix::text));

-- name: CountAgreementsWithPrefix :one
SELECT count(*)::int FROM agreement.agreements WHERE agreement_no LIKE @prefix::text || '%';

-- name: InsertAgreement :one
INSERT INTO agreement.agreements (id, tenant_id, agreement_type, agreement_no, title, our_role, counterparty_id,
    vendor_id, template_id, document_id, auto_renew, renewal_notice_days, effective_from, created_by, updated_by)
VALUES (@id, NULLIF(current_setting('app.tenant_id', true), '')::uuid, @agreement_type, @agreement_no, @title, @our_role,
    @counterparty_id, @vendor_id, @template_id, @document_id, @auto_renew, @renewal_notice_days, @effective_from,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: InsertAgreementParty :one
INSERT INTO agreement.parties (id, tenant_id, agreement_id, party_id, party_role, created_by, updated_by)
VALUES (@id, NULLIF(current_setting('app.tenant_id', true), '')::uuid, @agreement_id, @party_id, @party_role,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: InsertAgreementActivity :exec
INSERT INTO agreement.agreement_activities (tenant_id, agreement_id, activity_id)
VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, @agreement_id, @activity_id);

-- name: GetAgreement :one
SELECT * FROM agreement.agreements WHERE id = $1;

-- name: GetAgreementByDocumentID :one
-- DPA-03's own pre-submit gate resolves the agreement wrapping the PLT-16 document being submitted.
SELECT * FROM agreement.agreements WHERE document_id = $1;

-- name: ListAgreementPartiesForAgreement :many
SELECT * FROM agreement.parties WHERE agreement_id = $1 ORDER BY created_at;

-- name: ListAgreementActivityIDs :many
SELECT activity_id FROM agreement.agreement_activities WHERE agreement_id = $1 ORDER BY activity_id;

-- name: ListActivityIDsForAgreements :many
-- DPA-11: the activities linked to each of a page of agreements, in one query — used by ListAgreements so
-- a vendor's own agreement list (filtered by vendor_id) always shows its linked activities too, the same
-- way GetAgreement already does for one agreement.
SELECT agreement_id, activity_id FROM agreement.agreement_activities WHERE agreement_id = ANY(@agreement_ids::uuid[]) ORDER BY agreement_id, activity_id;

-- name: ListAgreements :many
-- Newest first; keyset cursor on (created_at, id).
SELECT * FROM agreement.agreements
WHERE (sqlc.narg(agreement_type)::text IS NULL OR agreement_type = sqlc.narg(agreement_type))
    AND (sqlc.narg(vendor_id)::uuid IS NULL OR vendor_id = sqlc.narg(vendor_id))
    AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: UpdateAgreementSchedule :one
-- DPA-10: the registry's own start/end dates and renewal settings — the only mutable fields this pass
-- exposes on an agreement (status transitions belong to DPA-06/07/08/09, not built yet).
UPDATE agreement.agreements
SET effective_from = @effective_from, effective_to = @effective_to, auto_renew = @auto_renew,
    renewal_notice_days = @renewal_notice_days, row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND row_version = @row_version
RETURNING *;
