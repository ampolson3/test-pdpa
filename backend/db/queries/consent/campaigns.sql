-- CON-24 (ส่งคำขอความยินยอมแบบ Bulk): consent.campaigns/consent.campaign_recipients were already fully
-- specified in the baseline migration (00005_consent.sql) — unused until now. No new migration beyond
-- widening platform.public_keys.entity_type (00095).

-- name: InsertCampaign :one
INSERT INTO consent.campaigns (id, tenant_id, name, purpose_ids, audience_query, channel, template_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @name, @purpose_ids, @audience_query, @channel, @template_id,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetCampaign :one
SELECT * FROM consent.campaigns WHERE id = $1;

-- name: ListCampaigns :many
SELECT * FROM consent.campaigns ORDER BY created_at DESC;

-- name: SetCampaignStatus :execrows
UPDATE consent.campaigns SET status = @status, sent_count = @sent_count, response_count = @response_count,
    row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: InsertCampaignRecipient :one
-- No unique constraint names (campaign_id, subject_id) — the caller (CreateCampaign) checks
-- GetCampaignRecipientByCampaignAndSubject first and skips the insert for an already-present pair, so this
-- never needs to race a real constraint.
INSERT INTO consent.campaign_recipients (id, tenant_id, campaign_id, subject_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @campaign_id, @subject_id,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetCampaignRecipientByCampaignAndSubject :one
SELECT * FROM consent.campaign_recipients WHERE campaign_id = $1 AND subject_id = $2;

-- name: ListCampaignRecipients :many
SELECT * FROM consent.campaign_recipients WHERE campaign_id = $1 ORDER BY created_at;

-- name: CountCampaignRecipientsByStatus :many
SELECT status, count(*) AS n FROM consent.campaign_recipients WHERE campaign_id = $1 GROUP BY status;

-- name: SetCampaignRecipientSent :execrows
-- A fresh token per (re)send — the previous link, if any, stops matching the moment this overwrites its hash.
UPDATE consent.campaign_recipients SET token_hash = @token_hash, status = 'sent', sent_at = @sent_at,
    row_version = row_version + 1, updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status != 'responded';

-- name: LockCampaignRecipientByTokenHash :one
SELECT * FROM consent.campaign_recipients WHERE tenant_id = current_setting('app.tenant_id')::uuid AND campaign_id = @campaign_id
    AND token_hash = @token_hash FOR UPDATE;

-- name: MarkCampaignRecipientOpened :execrows
UPDATE consent.campaign_recipients SET status = 'opened', row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND status IN ('sent', 'queued');

-- name: FindIdentifierForSubject :one
-- Picks the subject's own identifier of the given kind to address a notification to (CON-24's send/resend) —
-- any matching row; a subject with several of the same kind just uses whichever sorts first.
SELECT value_enc FROM consent.subject_identifiers WHERE subject_id = @subject_id AND identifier_type = @identifier_type ORDER BY is_primary DESC, created_at LIMIT 1;

-- name: MarkCampaignRecipientResponded :execrows
UPDATE consent.campaign_recipients SET status = 'responded', responded_at = @responded_at, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND status != 'responded';
