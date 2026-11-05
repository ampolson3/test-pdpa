-- PNG-09: notice.acknowledgements — who acknowledged which published notice version, through which
-- channel, when. Append-only (no update/delete query exists here on purpose — rule 4's own evidence
-- pattern); REVOKE UPDATE/DELETE/TRUNCATE is in deploy/db/10-grants.sql.

-- name: InsertAcknowledgement :one
INSERT INTO notice.acknowledgements (id, tenant_id, notice_version_id, user_id, channel, receipt_id, ip)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6)
RETURNING id, notice_version_id, subject_id, user_id, channel, receipt_id, occurred_at, ip;

-- name: ListAcknowledgementsByVersion :many
SELECT id, notice_version_id, subject_id, user_id, channel, receipt_id, occurred_at, ip
FROM notice.acknowledgements
WHERE notice_version_id = $1
  AND ($2::timestamptz IS NULL OR occurred_at < $2)
ORDER BY occurred_at DESC, id DESC
LIMIT $3;

-- name: CountAcknowledgementsByVersion :one
SELECT count(*) FROM notice.acknowledgements WHERE notice_version_id = $1;
