-- name: ResolveTemplate :one
-- The tenant's own template wins over the global one (tenant_id NULL); RLS shows both.
SELECT id, tenant_id, code, channel, language, subject, body, variables
FROM platform.notification_templates
WHERE code = @code AND channel = @channel AND language = @language
ORDER BY tenant_id NULLS LAST
LIMIT 1;

-- name: ListTemplates :many
SELECT id, tenant_id, code, channel, language, subject, body, variables, row_version, updated_at
FROM platform.notification_templates
ORDER BY code, channel, language, tenant_id NULLS LAST;

-- name: GetTemplate :one
SELECT id, tenant_id, code, channel, language, subject, body, variables, row_version, updated_at
FROM platform.notification_templates WHERE id = @id;

-- name: InsertTemplate :one
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables, created_by)
VALUES (current_setting('app.tenant_id')::uuid, @code, @channel, @language, @subject, @body, @variables,
        NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING
RETURNING id, tenant_id, code, channel, language, subject, body, variables, row_version, updated_at;

-- name: UpdateTemplate :one
-- Optimistic lock: only the version the client read (If-Match) is updated. Global rows are read-only
-- for tenants (RLS tenant_write), so they never match here.
UPDATE platform.notification_templates
SET subject = @subject, body = @body, variables = @variables, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND row_version = @row_version AND tenant_id = current_setting('app.tenant_id')::uuid
RETURNING id, tenant_id, code, channel, language, subject, body, variables, row_version, updated_at;

-- name: DeleteTemplate :execrows
DELETE FROM platform.notification_templates
WHERE id = @id AND row_version = @row_version AND tenant_id = current_setting('app.tenant_id')::uuid;

-- name: InsertNotification :one
INSERT INTO platform.notifications (id, tenant_id, template_id, channel, recipient_user_id, recipient_address_enc,
                                    payload, status, entity_type, entity_id, created_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @template_id, @channel, @recipient_user_id, @recipient_address_enc,
        @payload, @status, @entity_type, @entity_id, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING created_at;

-- name: LockNotification :one
SELECT id, template_id, channel, recipient_user_id, recipient_address_enc, payload, status, attempts
FROM platform.notifications WHERE id = @id FOR UPDATE;

-- name: MarkNotificationSent :exec
UPDATE platform.notifications
SET status = 'sent', attempts = attempts + 1, sent_at = now(), provider_message_id = @provider_message_id,
    error = NULL, row_version = row_version + 1
WHERE id = @id;

-- name: MarkNotificationAttemptFailed :one
UPDATE platform.notifications
SET attempts = attempts + 1, error = @error, row_version = row_version + 1,
    status = CASE WHEN @final::bool THEN 'failed' ELSE status END
WHERE id = @id
RETURNING attempts;

-- name: GetNotification :one
SELECT n.id, n.channel, n.recipient_user_id, n.recipient_address_enc, n.status, n.attempts, n.provider_message_id,
       n.sent_at, n.error, n.entity_type, n.entity_id, n.created_at, n.updated_at, t.code AS template_code
FROM platform.notifications n LEFT JOIN platform.notification_templates t ON t.id = n.template_id
WHERE n.id = @id;

-- name: ListNotifications :many
-- Delivery-status page: newest first, keyset after (created_at, id) of the previous page.
SELECT n.id, n.channel, n.recipient_user_id, n.recipient_address_enc, n.status, n.attempts, n.provider_message_id,
       n.sent_at, n.error, n.entity_type, n.entity_id, n.created_at, n.updated_at, t.code AS template_code
FROM platform.notifications n LEFT JOIN platform.notification_templates t ON t.id = n.template_id
WHERE (sqlc.narg(status)::text IS NULL OR n.status = sqlc.narg(status)::text)
  AND (sqlc.narg(channel)::text IS NULL OR n.channel = sqlc.narg(channel)::text)
  AND (n.created_at, n.id) < (@before_created_at::timestamptz, @before_id::uuid)
ORDER BY n.created_at DESC, n.id DESC
LIMIT @page_size;

-- name: ListInbox :many
SELECT n.id, n.payload, n.status, n.created_at, t.code AS template_code, t.subject AS template_subject, t.body AS template_body
FROM platform.notifications n LEFT JOIN platform.notification_templates t ON t.id = n.template_id
WHERE n.channel = 'in_app' AND n.recipient_user_id = @user_id AND n.status IN ('sent', 'delivered')
ORDER BY n.created_at DESC
LIMIT @page_size;

-- name: CountUnread :one
SELECT count(*) FROM platform.notifications
WHERE channel = 'in_app' AND recipient_user_id = @user_id AND status = 'sent';

-- name: MarkRead :execrows
-- In-app: "delivered" = seen by its recipient (only the recipient can mark it).
UPDATE platform.notifications SET status = 'delivered', row_version = row_version + 1
WHERE id = @id AND channel = 'in_app' AND recipient_user_id = @user_id AND status = 'sent';
