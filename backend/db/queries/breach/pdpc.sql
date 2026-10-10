-- BRE-08/09 PDPC notification rounds (initial / supplementary / final), each pointing at a published PLT-16
-- document version and keeping the evidence of that round's filing.

-- name: NextPDPCSequenceNo :one
SELECT (coalesce(max(sequence_no), 0) + 1)::smallint FROM breach.pdpc_notifications WHERE incident_id = @incident_id;

-- name: InsertPDPCNotification :one
INSERT INTO breach.pdpc_notifications (id, tenant_id, incident_id, sequence_no, notification_type, document_version_id,
    submitted_at, submission_ref, is_late, late_reason, evidence_file_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @sequence_no, @notification_type, @document_version_id,
    @submitted_at, sqlc.narg(submission_ref), @is_late, sqlc.narg(late_reason), sqlc.narg(evidence_file_id), @actor, @actor)
RETURNING id;

-- name: GetPDPCNotification :one
SELECT id, incident_id, sequence_no, notification_type, document_version_id, approved_by, submitted_at, submission_ref,
    is_late, late_reason, evidence_file_id, created_by, row_version, created_at
FROM breach.pdpc_notifications WHERE id = @id;

-- name: LockPDPCNotification :one
SELECT id, incident_id, sequence_no, notification_type, document_version_id, approved_by, submitted_at, submission_ref,
    is_late, late_reason, evidence_file_id, created_by, row_version, created_at
FROM breach.pdpc_notifications WHERE id = @id FOR UPDATE;

-- name: ListPDPCNotifications :many
SELECT id, incident_id, sequence_no, notification_type, document_version_id, approved_by, submitted_at, submission_ref,
    is_late, late_reason, evidence_file_id, created_by, row_version, created_at
FROM breach.pdpc_notifications WHERE incident_id = @incident_id ORDER BY sequence_no;

-- name: ApprovePDPCNotification :execrows
UPDATE breach.pdpc_notifications SET approved_by = @actor, updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version AND approved_by IS NULL;

-- name: CountConfirmedPDPCNotifications :one
SELECT count(*)::int FROM breach.pdpc_notifications WHERE incident_id = @incident_id AND approved_by IS NOT NULL;
