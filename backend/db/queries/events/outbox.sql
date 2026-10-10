-- name: InsertOutboxEvent :one
INSERT INTO platform.outbox_events (id, tenant_id, aggregate_type, aggregate_id, event_type, payload, occurred_at, created_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING tenant_id;

-- name: LockUnpublishedEvents :many
-- Oldest first; rows another dispatcher holds are skipped, not waited on. skip_ids excludes rows this
-- dispatcher already locked and left unpublished (failed, or behind a failed event of the same aggregate).
SELECT id, tenant_id, aggregate_type, aggregate_id, event_type, payload, occurred_at, attempts
FROM platform.outbox_events
WHERE published_at IS NULL AND NOT (id = ANY (@skip_ids::uuid[]))
ORDER BY occurred_at, id
LIMIT @batch_size
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE platform.outbox_events
SET published_at = now(), attempts = attempts + 1, row_version = row_version + 1
WHERE id = $1;

-- name: MarkEventFailed :one
UPDATE platform.outbox_events
SET attempts = attempts + 1, row_version = row_version + 1
WHERE id = $1
RETURNING attempts;

-- name: CreateWebhookDeliveries :execrows
-- One pending delivery per active subscription of the event's type (ST-07 [*] → pending).
INSERT INTO platform.webhook_deliveries (tenant_id, subscription_id, event_id, status, next_retry_at)
SELECT s.tenant_id, s.id, @event_id::uuid, 'pending', now()
FROM platform.webhook_subscriptions s
WHERE s.status = 'active' AND @event_type::text = ANY (s.event_types)
ON CONFLICT (tenant_id, subscription_id, event_id) DO NOTHING;

-- name: ListDispatchableTenants :many
-- platform.tenants is global (no RLS) and readable by pdpa_app.
SELECT id FROM platform.tenants WHERE status IN ('trial', 'active') ORDER BY id;
