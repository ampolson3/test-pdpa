-- CON-01: cookie.categories — tenant-authored cookie categories (necessary / analytics / marketing / ...),
-- shown as toggles in the banner's own settings panel. No seeded defaults (ORG-20's own "no seeded holiday
-- data" precedent) — the DPO creates them for their own domains.

-- name: InsertCategory :one
INSERT INTO cookie.categories (id, tenant_id, code, names, descriptions, is_required, display_order, purpose_id, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, code, names, descriptions, is_required, display_order, purpose_id, row_version, updated_at;

-- name: ListCategories :many
SELECT id, code, names, descriptions, is_required, display_order, purpose_id, row_version, updated_at
FROM cookie.categories
ORDER BY display_order, code;

-- name: GetCategory :one
SELECT id, code, names, descriptions, is_required, display_order, purpose_id, row_version, updated_at
FROM cookie.categories WHERE id = $1;

-- name: DeleteCategory :execrows
DELETE FROM cookie.categories WHERE id = $1;
