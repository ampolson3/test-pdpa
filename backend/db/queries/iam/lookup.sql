-- name: SearchActiveUsers :many
-- @mention picker: active users whose name starts with the query (case-insensitive). Names only — no e-mail.
SELECT id, display_name FROM iam.users
WHERE status = 'active' AND display_name ILIKE @prefix || '%'
ORDER BY display_name
LIMIT 10;

-- name: ActiveUserNames :many
SELECT id, display_name FROM iam.users WHERE id = ANY (@ids::uuid[]) AND status = 'active';
