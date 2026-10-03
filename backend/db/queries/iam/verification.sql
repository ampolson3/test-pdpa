-- name: InsertSubjectVerification :one
-- IAM-05: an OTP verification attempt. otp_hash is SHA-256 of the plaintext code (never stored or logged,
-- rule 3); identifier_blind_index is the caller's own crypto.Keyring.BlindIndex of the raw identifier — the
-- identifier itself is never persisted here.
INSERT INTO iam.subject_verifications (id, tenant_id, subject_id, identifier_blind_index, purpose, method,
    otp_hash, status, assurance_level, expires_at)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'pending', $7, $8)
RETURNING id, subject_id, purpose, method, attempts, status, assurance_level, expires_at, verified_at, row_version;

-- name: GetSubjectVerification :one
SELECT id, subject_id, purpose, method, otp_hash, attempts, status, assurance_level, expires_at, verified_at, row_version
FROM iam.subject_verifications WHERE id = $1;

-- name: UpdateSubjectVerificationStatus :one
-- Every VerifyOTP call writes here, success or failure — attempts always increments; status only moves to
-- verified/failed/expired on the deciding call (a later attempt against an already-decided row is refused
-- before this runs, in the service).
UPDATE iam.subject_verifications SET status = $2, attempts = attempts + 1,
    verified_at = CASE WHEN $2 = 'verified' THEN now() ELSE verified_at END,
    updated_at = now(), row_version = row_version + 1
WHERE id = $1 AND row_version = $3
RETURNING id, subject_id, purpose, method, attempts, status, assurance_level, expires_at, verified_at, row_version;
