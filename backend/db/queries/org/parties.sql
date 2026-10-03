-- name: ListExternalParties :many
-- Newest first; keyset cursor on (created_at, id). Merged-away records are hidden unless requested.
SELECT id, party_type, name_th, name_en, registration_no, country_code, contact, website, dedupe_key, status,
    merged_into_id, row_version, created_at, updated_at
FROM org.external_parties
WHERE (sqlc.narg(party_type)::text IS NULL OR party_type = sqlc.narg(party_type))
  AND (sqlc.narg(country_code)::text IS NULL OR country_code = sqlc.narg(country_code))
  AND (sqlc.narg(q)::text IS NULL OR name_th ILIKE '%' || sqlc.narg(q) || '%' OR name_en ILIKE '%' || sqlc.narg(q) || '%'
       OR registration_no ILIKE '%' || sqlc.narg(q) || '%')
  AND (@include_merged::bool OR merged_into_id IS NULL)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: GetExternalParty :one
SELECT id, party_type, name_th, name_en, registration_no, country_code, contact, website, dedupe_key, status,
    merged_into_id, row_version, created_at, updated_at
FROM org.external_parties
WHERE id = $1;

-- name: InsertExternalParty :one
INSERT INTO org.external_parties (id, tenant_id, party_type, name_th, name_en, registration_no, country_code, contact, website, dedupe_key,
    created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, party_type, name_th, name_en, registration_no, country_code, contact, website, dedupe_key, status,
    merged_into_id, row_version, created_at, updated_at;

-- name: UpdateExternalParty :one
UPDATE org.external_parties
SET party_type = $3, name_th = $4, name_en = $5, registration_no = $6, country_code = $7, contact = $8, website = $9,
    dedupe_key = $10, status = $11, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, party_type, name_th, name_en, registration_no, country_code, contact, website, dedupe_key, status,
    merged_into_id, row_version, created_at, updated_at;

-- name: ListDuplicateExternalParties :many
-- Every active, unmerged party whose dedupe_key is shared by more than one such party — the FE groups
-- them by dedupe_key to offer a merge.
SELECT id, party_type, name_th, name_en, country_code, dedupe_key
FROM org.external_parties p
WHERE status = 'active' AND merged_into_id IS NULL AND dedupe_key IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM org.external_parties o
    WHERE o.dedupe_key = p.dedupe_key AND o.status = 'active' AND o.merged_into_id IS NULL AND o.id <> p.id
  )
ORDER BY dedupe_key, name_th;

-- name: MergeExternalParty :execrows
UPDATE org.external_parties
SET status = 'inactive', merged_into_id = $2, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    row_version = row_version + 1
WHERE id = $1 AND row_version = $3 AND merged_into_id IS NULL;
