-- name: GetRoleID :one
SELECT id FROM iam.roles WHERE (tenant_id IS NULL OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) AND code = @code;

-- name: InsertRoleAssignmentRequest :one
INSERT INTO iam.role_assignment_requests (id, tenant_id, target_user_id, role_id, requested_by)
VALUES (@id, NULLIF(current_setting('app.tenant_id', true), '')::uuid, @target_user_id, @role_id, @requested_by)
RETURNING id;

-- name: GetRoleAssignmentRequest :one
SELECT rr.id, rr.target_user_id, u.display_name AS target_user_name, rr.role_id, r.code AS role_code,
    rr.requested_by, rr.role_assignment_id, rr.applied_at, rr.created_at
FROM iam.role_assignment_requests rr
JOIN iam.users u ON u.id = rr.target_user_id
JOIN iam.roles r ON r.id = rr.role_id
WHERE rr.id = @id;

-- name: ListRoleAssignmentRequests :many
SELECT rr.id, rr.target_user_id, u.display_name AS target_user_name, rr.role_id, r.code AS role_code,
    rr.requested_by, req.display_name AS requested_by_name, rr.role_assignment_id, rr.applied_at, rr.created_at
FROM iam.role_assignment_requests rr
JOIN iam.users u ON u.id = rr.target_user_id
JOIN iam.roles r ON r.id = rr.role_id
JOIN iam.users req ON req.id = rr.requested_by
ORDER BY rr.created_at DESC
LIMIT 50;

-- name: ApplyRoleAssignmentRequest :exec
-- Inserts the real grant (the only moment it takes effect) and links the request back to it.
WITH ins AS (
    INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type, granted_by)
    VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, @target_user_id, @role_id, 'tenant', @granted_by)
    RETURNING id AS new_assignment_id
)
UPDATE iam.role_assignment_requests
SET role_assignment_id = (SELECT new_assignment_id FROM ins), applied_at = now()
WHERE iam.role_assignment_requests.id = @request_id;
