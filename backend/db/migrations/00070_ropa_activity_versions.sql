-- +goose Up
-- ROPA-13 (เวอร์ชันและการอนุมัติ, ม.39): ropa.processing_activities.status already declares the full
-- ST-05 graph (draft/pending_approval/active/under_review/ended, docs/states/ST-05.md) but ROPA-03 only
-- ever built the draft -> pending_approval edge (SubmitActivity) — this feature adds the rest
-- (pending_approval -> active/draft, active <-> under_review, active -> ended) plus the one new table the
-- acceptance criterion needs: an append-only snapshot taken every time a round is approved, so "an
-- approved activity can only be edited through a new version" has real history to show and compare,
-- mirroring consent.consent_transactions' own append-only shape (no row_version/updated_at — this table is
-- never updated once written, only ever inserted into).
CREATE TABLE ropa.activity_versions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    version_no int NOT NULL,
    snapshot jsonb NOT NULL,
    approved_by uuid,
    approved_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_ropa_activity_versions PRIMARY KEY (id),
    CONSTRAINT uq_ropa_activity_versions_activity_version UNIQUE (tenant_id, activity_id, version_no)
);

ALTER TABLE ropa.activity_versions ADD CONSTRAINT fk_activity_versions_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE ropa.activity_versions ADD CONSTRAINT fk_activity_versions_activity FOREIGN KEY (activity_id) REFERENCES ropa.processing_activities (id);
ALTER TABLE ropa.activity_versions ADD CONSTRAINT fk_activity_versions_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);

CREATE INDEX ix_ropa_activity_versions_activity ON ropa.activity_versions (tenant_id, activity_id, version_no DESC);

ALTER TABLE ropa.activity_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ropa.activity_versions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down
DROP TABLE IF EXISTS ropa.activity_versions;
