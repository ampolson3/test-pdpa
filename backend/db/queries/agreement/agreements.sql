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

-- name: ListAgreementPartiesForAgreement :many
SELECT * FROM agreement.parties WHERE agreement_id = $1 ORDER BY created_at;

-- name: ListAgreementActivityIDs :many
SELECT activity_id FROM agreement.agreement_activities WHERE agreement_id = $1 ORDER BY activity_id;

-- name: ListAgreements :many
-- Newest first; keyset cursor on (created_at, id).
SELECT * FROM agreement.agreements
WHERE (sqlc.narg(agreement_type)::text IS NULL OR agreement_type = sqlc.narg(agreement_type))
    AND (sqlc.narg(vendor_id)::uuid IS NULL OR vendor_id = sqlc.narg(vendor_id))
    AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;
