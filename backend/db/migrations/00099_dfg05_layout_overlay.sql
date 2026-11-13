-- +goose Up
-- DFG-05 (แก้ไขแผนผังแบบลากวาง / drag & drop editor): dataflow.layouts already exists (migration 00009),
-- unused until now — a per-user overlay of node positions + per-node annotation text on top of the live
-- graph DFG-01/03/04/06 already compute, keyed by (tenant, view_type, scope_id, user_id). Its own
-- view_type CHECK constraint only ever anticipated 'legal_entity' for DFG-06's own company view, but
-- DFG-06 (built later) actually stores that view under the wire-facing literal "company"
-- (dataflowservice.ViewCompany) — widen the CHECK to accept it, and add the unique index this feature's
-- own upsert needs (NULLS NOT DISTINCT so a tenant-wide, no-scope layout and an anonymous-session layout
-- both still upsert correctly instead of inserting a duplicate row every save).

ALTER TABLE dataflow.layouts DROP CONSTRAINT layouts_view_type_check;
ALTER TABLE dataflow.layouts ADD CONSTRAINT layouts_view_type_check
    CHECK (view_type IN ('org_unit', 'system', 'legal_entity', 'company', 'data_category', 'lineage', 'world'));

CREATE UNIQUE INDEX uq_dataflow_layouts_scope_user ON dataflow.layouts (tenant_id, view_type, scope_id, user_id) NULLS NOT DISTINCT;

-- +goose Down
DROP INDEX IF EXISTS dataflow.uq_dataflow_layouts_scope_user;

ALTER TABLE dataflow.layouts DROP CONSTRAINT layouts_view_type_check;
ALTER TABLE dataflow.layouts ADD CONSTRAINT layouts_view_type_check
    CHECK (view_type IN ('org_unit', 'system', 'legal_entity', 'data_category', 'lineage', 'world'));
