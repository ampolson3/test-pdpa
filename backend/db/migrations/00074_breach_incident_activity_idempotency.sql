-- +goose Up
-- DPIA-13: breach.incident_assets was already fully specified in the baseline migrations (00013_breach.sql)
-- but completely unused until this feature's own LinkActivity — this is the one new constraint it needs.
-- The outbox event this link publishes (breach.activity_linked) is delivered at-least-once (PLT-11), and a
-- redelivery of the same incident+activity pair must not create a second row (ROPA-10's own migration 00044
-- established this exact idempotency pattern for the same reason). Partial, since asset_id-only rows (not
-- yet owned by any feature) must stay unconstrained.
CREATE UNIQUE INDEX uq_breach_incident_assets_incident_activity ON breach.incident_assets (incident_id, activity_id)
    WHERE activity_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS breach.uq_breach_incident_assets_incident_activity;
