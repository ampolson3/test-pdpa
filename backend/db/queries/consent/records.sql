-- CON-15 recording: data subjects, receipts (hash chain per subject), transactions (append-only), status projection.

-- name: FindSubjectByIdentifier :one
SELECT subject_id FROM consent.subject_identifiers WHERE identifier_type = $1 AND blind_index = $2;

-- name: InsertSubject :exec
INSERT INTO consent.data_subjects (id, tenant_id, subject_key, last_activity_at, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, now(),
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: TouchSubject :exec
UPDATE consent.data_subjects SET last_activity_at = now() WHERE id = $1;

-- name: GetSubject :one
SELECT id, subject_key, is_minor, legal_capacity, last_activity_at, created_at FROM consent.data_subjects WHERE id = $1;

-- name: InsertIdentifier :exec
INSERT INTO consent.subject_identifiers (tenant_id, subject_id, identifier_type, value_enc, blind_index, is_primary, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: ListIdentifiers :many
SELECT subject_id, identifier_type, value_enc, is_primary, verified_at FROM consent.subject_identifiers
WHERE subject_id = ANY (@subject_ids::uuid[]) ORDER BY subject_id, is_primary DESC, identifier_type;

-- name: RecentSubjects :many
SELECT id, subject_key, last_activity_at, created_at FROM consent.data_subjects
ORDER BY last_activity_at DESC NULLS LAST, id DESC LIMIT @page_size;

-- name: LockSubjectChain :exec
-- Serialises receipts of one data subject so each links to the previous one (the chain must not fork).
SELECT pg_advisory_xact_lock(hashtextextended(concat('consent.receipts:', sqlc.arg(subject_id)::uuid), 0));

-- name: LastReceipt :one
SELECT hash, occurred_at FROM consent.consent_receipts WHERE subject_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1;

-- name: InsertReceipt :exec
INSERT INTO consent.consent_receipts (id, tenant_id, receipt_no, subject_id, collection_point_id, channel, captured_by_user_id,
    form_submission_id, ip, user_agent, language, occurred_at, prev_hash, hash)
VALUES (@id, current_setting('app.tenant_id')::uuid, @receipt_no, @subject_id, @collection_point_id, @channel, @captured_by_user_id,
    @form_submission_id, @ip, @user_agent, @language, @occurred_at, NULLIF(@prev_hash::text, ''), @hash);

-- name: InsertTransaction :exec
INSERT INTO consent.consent_transactions (id, tenant_id, occurred_at, receipt_id, subject_id, purpose_id, purpose_version_id,
    transaction_type, preferences, reason_code, expires_at, source, idempotency_key)
VALUES (@id, current_setting('app.tenant_id')::uuid, @occurred_at, @receipt_id, @subject_id, @purpose_id, @purpose_version_id,
    @transaction_type, @preferences, @reason_code, @expires_at, @source, @idempotency_key);

-- name: LockStatus :one
SELECT status, purpose_version_id, preferences, expires_at FROM consent.consent_status
WHERE subject_id = $1 AND purpose_id = $2 FOR UPDATE;

-- name: UpsertStatus :exec
INSERT INTO consent.consent_status (subject_id, purpose_id, tenant_id, status, purpose_version_id, last_transaction_id, preferences, expires_at, updated_at)
VALUES (@subject_id, @purpose_id, current_setting('app.tenant_id')::uuid, @status, @purpose_version_id, @last_transaction_id, @preferences, @expires_at, now())
ON CONFLICT (subject_id, purpose_id) DO UPDATE
SET status = EXCLUDED.status, purpose_version_id = EXCLUDED.purpose_version_id, last_transaction_id = EXCLUDED.last_transaction_id,
    preferences = EXCLUDED.preferences, expires_at = EXCLUDED.expires_at, updated_at = now();

-- name: ListSubjectStatus :many
SELECT s.purpose_id, s.status, s.purpose_version_id, s.preferences, s.expires_at, s.updated_at,
       p.code, p.name_th, p.name_en, p.is_sensitive, v.version_no, cv.version_no AS current_version_no, cv.requires_reconsent AS current_requires_reconsent,
       cv.change_type AS current_change_type
FROM consent.consent_status s
JOIN consent.purposes p ON p.id = s.purpose_id
JOIN consent.purpose_versions v ON v.id = s.purpose_version_id
LEFT JOIN consent.purpose_versions cv ON cv.id = p.current_version_id
WHERE s.subject_id = $1
ORDER BY p.code;

-- name: ListSubjectTransactions :many
SELECT t.id, t.occurred_at, t.transaction_type, t.preferences, t.reason_code, t.expires_at, t.source,
       p.code AS purpose_code, p.name_th AS purpose_name_th, p.name_en AS purpose_name_en, v.version_no,
       r.receipt_no, r.channel, cp.name AS collection_point_name, r.captured_by_user_id
FROM consent.consent_transactions t
JOIN consent.purposes p ON p.id = t.purpose_id
JOIN consent.purpose_versions v ON v.id = t.purpose_version_id
JOIN consent.consent_receipts r ON r.id = t.receipt_id
JOIN consent.collection_points cp ON cp.id = r.collection_point_id
WHERE t.subject_id = $1
ORDER BY t.occurred_at DESC, t.id DESC
LIMIT 500;

-- name: ListSubjectReceipts :many
-- A subject's chain, oldest first, for verification.
SELECT id, receipt_no, subject_id, collection_point_id, channel, captured_by_user_id, form_submission_id, ip, user_agent, language,
       occurred_at, coalesce(prev_hash, '')::text AS prev_hash, hash
FROM consent.consent_receipts WHERE subject_id = $1 ORDER BY occurred_at, id;

-- name: ListReceiptTransactions :many
SELECT id, receipt_id, purpose_id, purpose_version_id, transaction_type, preferences, reason_code, expires_at, source, occurred_at
FROM consent.consent_transactions WHERE receipt_id = ANY (@receipt_ids::uuid[]) ORDER BY receipt_id, id;

-- name: GetReceiptByNo :one
SELECT id, subject_id FROM consent.consent_receipts WHERE receipt_no = $1;
