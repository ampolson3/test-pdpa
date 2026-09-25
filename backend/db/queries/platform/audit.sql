-- name: GetLatestAuditHash :one
-- The hash of the most recent audit_log row for this tenant, or "" if this is the first entry.
-- The caller must hold this within the same transaction that inserts the next row (append-only
-- chain, CLAUDE.md rule 4): concurrent writers for one tenant would otherwise race on prev_hash.
SELECT COALESCE(
    (SELECT hash FROM platform.audit_log WHERE tenant_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1),
    ''
)::text AS hash;

-- name: InsertAuditLog :one
INSERT INTO platform.audit_log (
    tenant_id, actor_type, actor_id, action, entity_type, entity_id, before, after, ip, user_agent, prev_hash, hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12
)
RETURNING id, occurred_at;
