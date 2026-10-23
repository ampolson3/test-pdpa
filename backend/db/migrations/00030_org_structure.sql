-- +goose Up
-- ORG-01 / ORG-04: a juristic registration number identifies one legal entity per tenant; org-unit subtree
-- queries (ltree <@ / @>) use a GiST index.
CREATE UNIQUE INDEX ux_org_legal_entities_registration_no ON org.legal_entities (tenant_id, registration_no) WHERE registration_no IS NOT NULL;
CREATE INDEX ix_org_org_units_path_gist ON org.org_units USING gist (path);

-- +goose Down
DROP INDEX IF EXISTS org.ix_org_org_units_path_gist;
DROP INDEX IF EXISTS org.ux_org_legal_entities_registration_no;
