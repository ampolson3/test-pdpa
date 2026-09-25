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
