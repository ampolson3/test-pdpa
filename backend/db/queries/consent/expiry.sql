-- CON-20: consent expiry and renewal.

-- name: ListDueExpirations :many
-- Every ACTIVE status past its own expires_at, with the collection point its original receipt was
-- attributed to (the EXPIRED transaction continues the same chain, attributed the same way).
SELECT cs.subject_id, cs.purpose_id, cs.purpose_version_id, p.code AS purpose_code, v.version_no,
       r.collection_point_id, cp.channel
FROM consent.consent_status cs
JOIN consent.purposes p ON p.id = cs.purpose_id
JOIN consent.purpose_versions v ON v.id = cs.purpose_version_id
JOIN consent.consent_transactions t ON t.id = cs.last_transaction_id
JOIN consent.consent_receipts r ON r.id = t.receipt_id
JOIN consent.collection_points cp ON cp.id = r.collection_point_id
WHERE cs.status = 'ACTIVE' AND cs.expires_at IS NOT NULL AND cs.expires_at <= now();

-- name: ListNearExpiry :many
-- ACTIVE statuses expiring within the given window, newest-due first — the admin UI's own "near expiry" list.
SELECT cs.subject_id, cs.purpose_id, p.code AS purpose_code, p.name_th, p.name_en, cs.expires_at
FROM consent.consent_status cs
JOIN consent.purposes p ON p.id = cs.purpose_id
WHERE cs.status = 'ACTIVE' AND cs.expires_at IS NOT NULL
  AND cs.expires_at <= now() + make_interval(days => sqlc.arg(within_days)::int)
ORDER BY cs.expires_at
LIMIT 200;
