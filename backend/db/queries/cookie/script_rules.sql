-- CON-02: cookie.script_rules — which scripts belong to which category, so the SDK can enable/disable them
-- the moment a visitor's preference changes (the acceptance criterion's own "มีผลทันทีกับสคริปต์ในหมวดนั้น").
-- pattern is matched against a <script> tag's own src or data-pdpa-src attribute by the SDK, substring/
-- regex, tenant's own choice — this table only stores the declared pattern + category + action.

-- name: InsertScriptRule :one
INSERT INTO cookie.script_rules (id, tenant_id, domain_id, pattern, category_id, action, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, domain_id, pattern, category_id, action, row_version, updated_at;

-- name: ListScriptRulesByDomain :many
SELECT id, domain_id, pattern, category_id, action, row_version, updated_at
FROM cookie.script_rules
WHERE domain_id = $1
ORDER BY pattern;

-- name: ListScriptRulesWithCategoryCode :many
-- The public, SDK-facing read: every rule for a domain with its category's own code (not id) — the wire
-- shape the SDK already knows categories by.
SELECT sr.pattern, c.code AS category_code, sr.action
FROM cookie.script_rules sr
JOIN cookie.categories c ON c.id = sr.category_id
WHERE sr.domain_id = $1
ORDER BY sr.pattern;

-- name: DeleteScriptRule :execrows
DELETE FROM cookie.script_rules WHERE id = $1;
