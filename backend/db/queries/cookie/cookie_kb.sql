-- CON-04: cookie.cookie_kb — the global, unauthenticated knowledge base of well-known third-party cookie
-- name patterns (migration 00068) used to auto-suggest a category for a freshly scanned cookie. No tenant_id
-- column (deliberately global, like org.countries) — RLS doesn't apply here.

-- name: ListCookieKB :many
SELECT id, name_pattern, provider, category_code, description_th, description_en FROM cookie.cookie_kb ORDER BY name_pattern;
