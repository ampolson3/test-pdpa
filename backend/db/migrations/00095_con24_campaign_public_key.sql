-- +goose Up
-- CON-24 (ส่งคำขอความยินยอมแบบ Bulk, แนวปฏิบัติตลาด): consent.campaigns/consent.campaign_recipients were
-- already fully specified in the baseline migrations (00005_consent.sql) — name, purpose_ids, audience_query,
-- channel, template_id, status, sent_count, response_count on the campaign; subject_id, token_hash, status,
-- sent_at, responded_at per recipient — unused until now, the same "table seeded, feature not built yet" gap
-- this codebase has closed for several prior public surfaces. The recipient's own link is resolved the same
-- way every other /public/v1 surface resolves its tenant — platform.public_keys, before the tenant
-- transaction opens — but its entity_type CHECK constraint (00002_platform.sql, widened once already by
-- 00067_dsar_packages.sql) doesn't yet name this entity.
ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form', 'dsar_package', 'consent_campaign'));

-- +goose Down
ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form', 'dsar_package'));
