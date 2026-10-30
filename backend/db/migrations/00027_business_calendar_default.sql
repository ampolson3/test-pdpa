-- ORG-20: at most one default business calendar per tenant (the one SLA timers use when a record
-- names no calendar), and holidays looked up by date range within a calendar.

-- +goose Up
CREATE UNIQUE INDEX ux_org_business_calendars_default ON org.business_calendars (tenant_id) WHERE is_default;
CREATE UNIQUE INDEX ux_org_business_calendars_name ON org.business_calendars (tenant_id, lower(name));

-- +goose Down
DROP INDEX IF EXISTS org.ux_org_business_calendars_name;
DROP INDEX IF EXISTS org.ux_org_business_calendars_default;
