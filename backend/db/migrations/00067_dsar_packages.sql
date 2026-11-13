-- +goose Up
-- DSAR-14 (ส่งข้อมูลให้เจ้าของข้อมูลอย่างปลอดภัย, ม.37(1)): dsar.packages was already fully specified in
-- the baseline migrations (00012_dsar.sql) — file_id, format, token_hash, password_protected, expires_at,
-- download_count, last_downloaded_at, purged_at — unused until now, the same "table seeded, feature not
-- built yet" gap this codebase has closed for DSAR-01/BRE-01's own public surfaces. The one schema change
-- this feature needs: the public download link is resolved the same way every other /public/v1 surface
-- resolves its tenant — platform.public_keys, before the tenant transaction opens — but its entity_type
-- CHECK constraint (00002_platform.sql) doesn't yet name this entity.
ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form', 'dsar_package'));

-- +goose Down
ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form'));
