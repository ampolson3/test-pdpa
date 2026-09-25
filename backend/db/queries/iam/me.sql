-- name: GetUserByID :one
SELECT id, tenant_id, display_name, email, locale, mfa_required, primary_org_unit_id
FROM iam.users
WHERE id = $1;

-- name: GetTenantByID :one
SELECT id, code, name
FROM platform.tenants
WHERE id = $1;

-- name: ListEffectiveRoleAssignments :many
-- Every role currently granted to the user directly or through a group, whether tenant-wide or
-- scoped to a legal entity / org unit (docs/security/permissions.md).
SELECT ra.role_id, r.code AS role_code, ra.scope_type, ra.legal_entity_id, ra.org_unit_id, ra.include_descendants
FROM iam.role_assignments ra
JOIN iam.roles r ON r.id = ra.role_id
WHERE (
        ra.user_id = $1
        OR ra.group_id IN (SELECT group_id FROM iam.group_members WHERE user_id = $1)
      )
  AND ra.valid_from <= now()
  AND (ra.valid_to IS NULL OR ra.valid_to > now());

-- name: ListPermissionsForRoles :many
SELECT DISTINCT permission_code
FROM iam.role_permissions
WHERE role_id = ANY(sqlc.arg(role_ids)::uuid[]);
