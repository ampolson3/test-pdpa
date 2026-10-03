-- CON-11 (ม.20): guardian consent takes effect only once the guardian has verified via an IAM-05 OTP.

-- name: InsertGuardianApproval :one
INSERT INTO consent.guardian_approvals (id, tenant_id, minor_subject_id, guardian_subject_id, receipt_id,
    relationship, status, requested_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, 'requested', $6,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, minor_subject_id, guardian_subject_id, receipt_id, relationship, status, requested_at, approved_at, row_version;

-- name: GetGuardianApproval :one
SELECT ga.id, ga.minor_subject_id, ga.guardian_subject_id, ga.receipt_id, ga.relationship, ga.status,
    ga.requested_at, ga.approved_at, ga.row_version, r.channel
FROM consent.guardian_approvals ga
JOIN consent.consent_receipts r ON r.id = ga.receipt_id
WHERE ga.id = $1;

-- name: ApproveGuardianApproval :one
-- WHERE status = 'requested' is the idempotency guard: a redelivered/retried confirm is a harmless no-op (0 rows).
UPDATE consent.guardian_approvals SET status = 'approved', approved_at = now(), updated_at = now(), row_version = row_version + 1
WHERE id = $1 AND status = 'requested'
RETURNING id, minor_subject_id, guardian_subject_id, receipt_id, relationship, status, requested_at, approved_at, row_version;

-- name: ListPendingStatusForSubject :many
-- Every purpose still waiting on this minor's guardian, locked so a concurrent Record() can't race the confirm.
SELECT subject_id, purpose_id, status, purpose_version_id, preferences, expires_at
FROM consent.consent_status WHERE subject_id = $1 AND status = 'PENDING' FOR UPDATE;
