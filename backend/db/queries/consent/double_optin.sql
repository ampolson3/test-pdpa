-- CON-23: consent.double_optin_requests was already fully specified in the baseline migration (one row
-- per PENDING transaction, its own confirmation token hash and expiry) — no new migration.

-- name: InsertDoubleOptInRequest :one
INSERT INTO consent.double_optin_requests (id, tenant_id, transaction_id, channel, token_hash, sent_at, expires_at,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @transaction_id, @channel, @token_hash, @sent_at, @expires_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, tenant_id, transaction_id, channel, token_hash, sent_at, expires_at, confirmed_at, status, row_version;

-- name: LockDoubleOptInRequestByTokenHash :one
SELECT id, tenant_id, transaction_id, channel, token_hash, sent_at, expires_at, confirmed_at, status, row_version
FROM consent.double_optin_requests WHERE token_hash = $1 FOR UPDATE;

-- name: ConfirmDoubleOptInRequest :execrows
UPDATE consent.double_optin_requests SET status = 'confirmed', confirmed_at = @confirmed_at, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status = 'sent';

-- name: ExpireDoubleOptInRequest :execrows
UPDATE consent.double_optin_requests SET status = 'expired', row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status = 'sent';

-- name: ListDueDoubleOptInRequests :many
-- CON-23's sweep: every still-"sent" request whose confirmation window has passed, per tenant.
SELECT id, transaction_id FROM consent.double_optin_requests WHERE status = 'sent' AND expires_at <= now();

-- name: ListActiveTenants :many
-- The same "sweep lists tenants, per-tenant job reads its own rows" pattern CON-04/PNG-15/ROPA-15 already
-- established (CLAUDE.md rule 1). platform.tenants has no RLS and is already readable by pdpa_app.
SELECT id FROM platform.tenants WHERE status IN ('trial', 'active') ORDER BY id;

-- name: GetTransaction :one
SELECT id, occurred_at, receipt_id, subject_id, purpose_id, purpose_version_id, transaction_type, preferences,
    reason_code, expires_at, source
FROM consent.consent_transactions WHERE id = $1;
