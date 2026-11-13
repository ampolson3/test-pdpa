-- +goose NO TRANSACTION
-- PLT-04: indexes for the delivery-status page (newest first, optional status filter) and the in-app inbox
-- (a user's unread in-app notifications). CONCURRENTLY: platform.notifications grows with every message sent.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS ix_platform_notifications_created
    ON platform.notifications (tenant_id, created_at DESC, id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS ix_platform_notifications_inbox
    ON platform.notifications (tenant_id, recipient_user_id, created_at DESC) WHERE channel = 'in_app';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS platform.ix_platform_notifications_inbox;
DROP INDEX CONCURRENTLY IF EXISTS platform.ix_platform_notifications_created;
