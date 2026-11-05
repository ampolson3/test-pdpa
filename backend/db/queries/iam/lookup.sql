-- name: SearchActiveUsers :many
-- @mention picker: active users whose name starts with the query (case-insensitive). Names only — no e-mail.
SELECT id, display_name FROM iam.users
WHERE status = 'active' AND display_name ILIKE @prefix || '%'
ORDER BY display_name
LIMIT 10;

-- name: ActiveUserNames :many
SELECT id, display_name FROM iam.users WHERE id = ANY (@ids::uuid[]) AND status = 'active';

-- name: GroupIDsOfUser :many
SELECT group_id FROM iam.group_members WHERE user_id = @user_id;

-- name: ActiveGroupMembers :many
SELECT m.user_id FROM iam.group_members m JOIN iam.users u ON u.id = m.user_id
WHERE m.group_id = @group_id AND u.status = 'active'
ORDER BY m.user_id;

-- name: GroupNames :many
SELECT id, name FROM iam.groups WHERE id = ANY (@ids::uuid[]);

-- name: SearchGroups :many
SELECT id, name FROM iam.groups WHERE name ILIKE @prefix || '%' ORDER BY name LIMIT 10;

-- name: UserNamesAnyStatus :many
-- Names of users whatever their status (audit trails name people who have since left).
SELECT id, display_name FROM iam.users WHERE id = ANY (@ids::uuid[]);

-- name: AllActiveUsers :many
-- Every active user of the tenant, with name — DPX-02's own "ผู้ที่ยังไม่รับทราบ" report needs the full
-- roster to compare against who has attested, not just one role's own holders.
SELECT id, display_name FROM iam.users WHERE status = 'active' ORDER BY display_name;

-- name: GetPrimaryOrgUnit :one
-- A user's own primary org unit (DPX-01: resolving which org-unit-targeted training assignments apply
-- to the calling employee).
SELECT primary_org_unit_id FROM iam.users WHERE id = @id AND status = 'active';

-- name: ActiveUsersWithRole :many
-- Active users holding a role tenant-wide, directly or through a group (who to alert about a breach, BRE-07).
SELECT DISTINCT u.id FROM iam.users u
JOIN iam.role_assignments ra ON ra.user_id = u.id
    OR ra.group_id IN (SELECT gm.group_id FROM iam.group_members gm WHERE gm.user_id = u.id)
JOIN iam.roles r ON r.id = ra.role_id
WHERE r.code = @role_code AND u.status = 'active'
  AND ra.valid_from <= now() AND (ra.valid_to IS NULL OR ra.valid_to > now())
ORDER BY u.id;
