-- PNG-15: the same "sweep lists tenants, per-tenant job reads its own rows" pattern CON-04's own
-- cookie.scan_schedule_sweep already established (CLAUDE.md rule 1). platform.tenants has no RLS and is
-- already readable by pdpa_app.

-- name: ListActiveTenants :many
SELECT id FROM platform.tenants WHERE status IN ('trial', 'active') ORDER BY id;
