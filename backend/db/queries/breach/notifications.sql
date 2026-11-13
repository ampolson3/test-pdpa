-- BRE-10 data-subject notification batches.

-- name: InsertSubjectNotification :one
INSERT INTO breach.subject_notifications (id, tenant_id, incident_id, channel, template_code, variables, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @channel, @template_code, @variables, @actor, @actor)
RETURNING id;

-- name: GetSubjectNotification :one
SELECT id, incident_id, channel, template_code, variables, total_recipients, sent_count, failed_count, status, started_at,
    completed_at, created_by, approved_by, approved_at, row_version, created_at
FROM breach.subject_notifications WHERE id = @id;

-- name: LockSubjectNotification :one
SELECT id, incident_id, channel, template_code, variables, total_recipients, sent_count, failed_count, status, started_at,
    completed_at, created_by, approved_by, approved_at, row_version, created_at
FROM breach.subject_notifications WHERE id = @id FOR UPDATE;

-- name: ListSubjectNotifications :many
SELECT id, incident_id, channel, template_code, variables, total_recipients, sent_count, failed_count, status, started_at,
    completed_at, created_by, approved_by, approved_at, row_version, created_at
FROM breach.subject_notifications WHERE incident_id = @incident_id ORDER BY created_at, id;

-- name: UpdateSubjectNotificationDraft :execrows
UPDATE breach.subject_notifications SET variables = @variables, template_code = @template_code, updated_at = now(),
    updated_by = @actor, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version AND status = 'draft';

-- name: SetRecipientTotal :exec
UPDATE breach.subject_notifications SET total_recipients = @total, updated_at = now(), row_version = row_version + 1 WHERE id = @id;

-- name: StartSubjectNotification :exec
UPDATE breach.subject_notifications SET status = 'sending', started_at = now(), approved_by = @actor, approved_at = now(),
    updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id;

-- name: AddSubjectNotificationProgress :exec
UPDATE breach.subject_notifications SET sent_count = sent_count + @sent, failed_count = failed_count + @failed, updated_at = now(),
    row_version = row_version + 1
WHERE id = @id;

-- name: FinishSubjectNotification :exec
UPDATE breach.subject_notifications SET status = @status, completed_at = now(), updated_at = now(), row_version = row_version + 1
WHERE id = @id;

-- name: DeleteQueuedRecipients :exec
DELETE FROM breach.notification_recipients WHERE subject_notification_id = @id;

-- name: CountRecipients :one
SELECT count(*)::int FROM breach.notification_recipients WHERE subject_notification_id = @id;

-- name: NextQueuedRecipients :many
SELECT id, address_enc, language FROM breach.notification_recipients
WHERE subject_notification_id = @id AND status = 'queued' ORDER BY line_no, id LIMIT @lim FOR UPDATE SKIP LOCKED;

-- name: MarkRecipientSent :exec
UPDATE breach.notification_recipients SET status = 'sent', sent_at = now(), notification_id = @notification_id WHERE id = @id;

-- name: MarkRecipientFailed :exec
UPDATE breach.notification_recipients SET status = 'failed', error = @error WHERE id = @id;

-- name: ListRecipients :many
SELECT id, address_enc, language, status, sent_at, error, notification_id, line_no FROM breach.notification_recipients
WHERE subject_notification_id = @id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(after_line)::int IS NULL OR line_no > sqlc.narg(after_line))
ORDER BY line_no, id LIMIT @lim;

-- name: InsertRecipients :exec
-- A batch in one statement (COPY isn't allowed on a table with row-level security).
INSERT INTO breach.notification_recipients (id, tenant_id, subject_notification_id, address_enc, language, line_no)
SELECT unnest(@ids::uuid[]), current_setting('app.tenant_id')::uuid, @subject_notification_id::uuid, unnest(@addresses::bytea[]),
    unnest(@languages::text[]), unnest(@line_nos::int[]);

-- name: RecipientNotificationIDs :many
SELECT notification_id FROM breach.notification_recipients WHERE subject_notification_id = @id AND notification_id IS NOT NULL;
