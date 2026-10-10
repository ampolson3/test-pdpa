-- name: ListActiveGapRules :many
-- RRA-04: every active rule (global defaults plus this tenant's own) — the same ORG-07 master-data
-- visibility pattern risk.controls already uses.
SELECT id, tenant_id, code, name, severity, legal_ref, is_active, row_version
FROM risk.gap_rules
WHERE is_active
ORDER BY code;

-- name: GetOpenGapFinding :one
SELECT id, tenant_id, rule_id, activity_id, status, detected_at, resolved_at, task_id, row_version
FROM risk.gap_findings
WHERE rule_id = $1 AND activity_id = $2 AND status = 'open';

-- name: InsertGapFinding :one
INSERT INTO risk.gap_findings (tenant_id, rule_id, activity_id)
VALUES (current_setting('app.tenant_id')::uuid, $1, $2)
RETURNING id, tenant_id, rule_id, activity_id, status, detected_at, resolved_at, task_id, row_version;

-- name: ResolveGapFinding :one
UPDATE risk.gap_findings
SET status = 'resolved', resolved_at = now(), row_version = row_version + 1
WHERE id = $1 AND status = 'open'
RETURNING id, tenant_id, rule_id, activity_id, status, detected_at, resolved_at, task_id, row_version;

-- name: ListGapFindingsByActivity :many
SELECT id, tenant_id, rule_id, activity_id, status, detected_at, resolved_at, task_id, row_version
FROM risk.gap_findings
WHERE activity_id = $1
ORDER BY detected_at DESC;

-- name: ListOpenGapFindings :many
-- RRA-04's own list for the gap register (every open finding, newest first) — RRA-07 will add a
-- task_id filter and status transitions on top of this same table.
SELECT id, tenant_id, rule_id, activity_id, status, detected_at, resolved_at, task_id, row_version
FROM risk.gap_findings
WHERE status = 'open'
ORDER BY detected_at DESC;
