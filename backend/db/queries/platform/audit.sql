-- name: LockAuditChain :exec
-- Serialises appends to one tenant's chain until the transaction ends, so two concurrent requests
-- can't both link to the same previous row (which would fork the chain). Taken at the end of the
-- request (the audit write is the last step before COMMIT), so it is held only briefly.
SELECT pg_advisory_xact_lock(hashtextextended(concat('platform.audit_log:', sqlc.arg(tenant_id)::uuid), 0));

-- name: NextAuditChainLink :one
-- The previous row's hash ("" for the first entry) and this row's occurred_at: the database clock,
-- but never earlier than the previous row, so chain order and (occurred_at, id) order agree.
SELECT coalesce(h.hash, '')::text AS prev_hash,
       greatest(clock_timestamp(), coalesce(h.occurred_at + interval '1 microsecond', clock_timestamp()))::timestamptz AS occurred_at
FROM (SELECT 1) AS one
LEFT JOIN LATERAL (
    SELECT hash, occurred_at FROM platform.audit_log
    WHERE tenant_id = @tenant_id::uuid
    ORDER BY occurred_at DESC, id DESC
    LIMIT 1
) AS h ON true;

-- name: InsertAuditLog :one
INSERT INTO platform.audit_log (
    tenant_id, occurred_at, actor_type, actor_id, action, entity_type, entity_id, before, after, ip, user_agent, prev_hash, hash
) VALUES (
    @tenant_id, @occurred_at, @actor_type, @actor_id, @action, @entity_type, @entity_id, @before, @after, @ip, @user_agent,
    NULLIF(@prev_hash::text, ''), @hash
)
RETURNING id;

-- name: ListAuditChainPage :many
-- One page of a tenant's chain in chain order, after (occurred_at, id) of the previous page's last row.
SELECT id, occurred_at, actor_type, actor_id, action, entity_type, entity_id, before, after, ip, user_agent,
       coalesce(prev_hash, '')::text AS prev_hash, hash
FROM platform.audit_log
WHERE tenant_id = @tenant_id::uuid
  AND (occurred_at, id) > (@after_occurred_at::timestamptz, @after_id::bigint)
ORDER BY occurred_at, id
LIMIT @page_size;

-- name: ListLiveTenants :many
-- platform.tenants is global (no RLS) and readable by pdpa_app.
SELECT id FROM platform.tenants WHERE status IN ('trial', 'active', 'suspended') ORDER BY id;

-- name: SearchAuditLog :many
-- ORG-19: the tenant's audit trail (RLS), newest first, before the cursor (occurred_at, id).
-- kind: 'changes' = business actions, 'requests' = per-request rows ("GET /admin/v1/…"), 'all'.
SELECT id, occurred_at, actor_type, actor_id, action, entity_type, entity_id, before, after, ip, user_agent
FROM platform.audit_log
WHERE (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id)::uuid)
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
  AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid)
  AND (sqlc.narg(action_prefix)::text IS NULL OR action LIKE sqlc.narg(action_prefix)::text || '%')
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR occurred_at < sqlc.narg(to_at)::timestamptz)
  AND (@kind::text = 'all' OR (@kind::text = 'requests') = (action ~ '^[A-Z]+ '))
  AND (occurred_at, id) < (@before_occurred_at::timestamptz, @before_id::bigint)
ORDER BY occurred_at DESC, id DESC
LIMIT @page_size;
