-- name: ListControls :many
-- ROPA-09: the security-measures catalog (ม.37(1)) — global rows (tenant_id NULL) plus this tenant's own,
-- exactly the ORG-07 master-data visibility pattern. RLS's tenant_read policy already scopes this.
SELECT id, tenant_id, code, name, category, description, framework_refs, row_version
FROM risk.controls
ORDER BY category, name;

-- name: GetControl :one
SELECT id, tenant_id, code, name, category, description, framework_refs, row_version
FROM risk.controls
WHERE id = $1;
