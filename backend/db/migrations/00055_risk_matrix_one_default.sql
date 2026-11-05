-- +goose Up
-- RRA-02 (ตั้งค่า risk matrix): risk.risk_matrices was already fully specified in the baseline migrations
-- (is_default boolean, no further constraint) — a tenant can have several matrices (e.g. drafts under
-- review) but only one may ever be "the" default any later feature (RRA-01, DPIA-06) resolves against when
-- no matrix id is given explicitly, the same "BusinessCalendar(ctx, id|nil)" pattern ORG-20's own default
-- calendar already established. A partial unique index enforces that invariant at the database level, not
-- just in application code, so two concurrent "set as default" requests can never both win.
CREATE UNIQUE INDEX ux_risk_matrices_one_default ON risk.risk_matrices (tenant_id) WHERE is_default;

-- +goose Down
DROP INDEX risk.ux_risk_matrices_one_default;
