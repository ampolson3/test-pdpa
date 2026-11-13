-- +goose Up
-- ROPA-10: `dsar.rejected` (PLT-11 outbox) is delivered at-least-once, so the subscriber that writes
-- ropa.activity_rejections must be idempotent by (activity, request) — a redelivery of the same rejection
-- must not create a second row. One request rejected against the same activity twice would also be a
-- logic error, so this is a real business constraint, not just a de-dup mechanism.
CREATE UNIQUE INDEX uq_ropa_activity_rejections_activity_request ON ropa.activity_rejections (activity_id, dsar_request_id);

-- +goose Down
DROP INDEX IF EXISTS ropa.uq_ropa_activity_rejections_activity_request;
