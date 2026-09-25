-- +goose NO TRANSACTION
-- PLT-11: indexes for the outbox dispatcher (internal/platform/events).
-- ix_platform_outbox_events_unpublished: the dispatcher reads a tenant's unpublished events oldest first
-- (FOR UPDATE SKIP LOCKED); partial, so it stays small however many published events accumulate.
-- uq_platform_webhook_deliveries_subscription_event: one delivery per (subscription, event), so a re-run
-- dispatch never creates a second delivery for the same event (ON CONFLICT DO NOTHING).
-- CONCURRENTLY because both tables grow with every domain event (CLAUDE.md rule 13).

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS ix_platform_outbox_events_unpublished
    ON platform.outbox_events (tenant_id, occurred_at, id) WHERE published_at IS NULL;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_platform_webhook_deliveries_subscription_event
    ON platform.webhook_deliveries (tenant_id, subscription_id, event_id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS platform.uq_platform_webhook_deliveries_subscription_event;
DROP INDEX CONCURRENTLY IF EXISTS platform.ix_platform_outbox_events_unpublished;
