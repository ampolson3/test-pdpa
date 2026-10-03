-- +goose Up
-- PNG-06 (ม.23): a published notice gets its own platform.public_keys key (entity_type='notice', already in
-- that table's CHECK constraint since the baseline) so the public page (/public/v1/notices/{key}) can resolve
-- the tenant before a transaction opens, the same way CON-09's collection_points.public_key already works.
-- notice.notice_versions (effective_from, document_version_id, checklist_result, public_url, ...) was already
-- fully specified in the baseline migrations for exactly this feature — no change needed there.
ALTER TABLE notice.notices ADD COLUMN public_key varchar(64);

-- +goose Down
ALTER TABLE notice.notices DROP COLUMN IF EXISTS public_key;
