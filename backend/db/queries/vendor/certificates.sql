-- VEN-06 (evidence & certificates — vendor.certificates, already fully specified in the baseline migrations).

-- name: InsertCertificate :one
INSERT INTO vendor.certificates (id, tenant_id, vendor_id, cert_type, issuer, file_id, valid_from, valid_to,
    created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, vendor_id, cert_type, issuer, file_id, valid_from, valid_to, verified_by, row_version, created_at, updated_at;

-- name: ListCertificates :many
SELECT id, vendor_id, cert_type, issuer, file_id, valid_from, valid_to, verified_by, row_version, created_at, updated_at
FROM vendor.certificates
WHERE vendor_id = $1
ORDER BY valid_to NULLS LAST, created_at DESC;

-- name: GetCertificate :one
SELECT id, vendor_id, cert_type, issuer, file_id, valid_from, valid_to, verified_by, row_version, created_at, updated_at
FROM vendor.certificates
WHERE id = $1;

-- name: DeleteCertificate :execrows
DELETE FROM vendor.certificates WHERE id = $1 AND vendor_id = $2;
