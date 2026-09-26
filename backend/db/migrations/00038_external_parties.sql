-- ORG-06: merging a duplicate external party keeps one record ("record เดียวที่ทุกโมดูลอ้างถึง") by
-- marking the loser inactive and pointing it at the survivor, instead of deleting it (every other
-- module's FK to org.external_parties must keep resolving even for a party merged away).

-- +goose Up
ALTER TABLE org.external_parties ADD COLUMN merged_into_id uuid REFERENCES org.external_parties (id);
ALTER TABLE org.external_parties ADD CONSTRAINT ck_external_parties_merge_not_self CHECK (merged_into_id IS NULL OR merged_into_id <> id);
CREATE INDEX ix_org_external_parties_merged_into_id ON org.external_parties (tenant_id, merged_into_id) WHERE merged_into_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS org.ix_org_external_parties_merged_into_id;
ALTER TABLE org.external_parties DROP CONSTRAINT IF EXISTS ck_external_parties_merge_not_self;
ALTER TABLE org.external_parties DROP COLUMN IF EXISTS merged_into_id;
