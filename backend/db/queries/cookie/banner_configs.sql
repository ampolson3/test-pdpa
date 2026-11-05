-- CON-01: cookie.banner_configs — versioned per domain (uq_banner_configs_domain_id_version_no); draft →
-- published → archived. Publishing one version archives every other published version of the same domain
-- in the same statement, so PublicBanner always has at most one published row to pick.

-- name: NextBannerVersionNo :one
SELECT coalesce(max(version_no), 0) + 1 FROM cookie.banner_configs WHERE domain_id = $1;

-- name: InsertBannerConfig :one
INSERT INTO cookie.banner_configs (id, tenant_id, domain_id, version_no, layout, theme, texts, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain_id, version_no, status, layout, theme, texts, published_at, row_version, updated_at;

-- name: GetBannerConfig :one
SELECT id, domain_id, version_no, status, layout, theme, texts, published_at, row_version, updated_at
FROM cookie.banner_configs WHERE id = $1;

-- name: LockBannerConfig :one
SELECT id, domain_id, version_no, status, layout, theme, texts, published_at, row_version, updated_at
FROM cookie.banner_configs WHERE id = $1 FOR UPDATE;

-- name: ListBannerConfigs :many
SELECT id, domain_id, version_no, status, layout, theme, texts, published_at, row_version, updated_at
FROM cookie.banner_configs WHERE domain_id = $1
ORDER BY version_no DESC;

-- name: UpdateBannerConfigDraft :execrows
UPDATE cookie.banner_configs SET layout = $2, theme = $3, texts = $4, row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND status = 'draft' AND row_version = $5;

-- name: ArchivePublishedBannerConfigs :exec
UPDATE cookie.banner_configs SET status = 'archived', row_version = row_version + 1, updated_at = now()
WHERE domain_id = $1 AND status = 'published';

-- name: PublishBannerConfig :execrows
UPDATE cookie.banner_configs SET status = 'published', published_at = now(),
    published_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND status = 'draft' AND row_version = $2;

-- name: GetPublishedBannerConfig :one
SELECT id, domain_id, version_no, status, layout, theme, texts, published_at, row_version, updated_at
FROM cookie.banner_configs WHERE domain_id = $1 AND status = 'published'
ORDER BY version_no DESC LIMIT 1;
