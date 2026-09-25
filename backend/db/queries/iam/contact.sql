-- name: GetUserContact :one
-- Where to reach an active user (notification recipient resolution, PLT-04).
SELECT email, phone, locale FROM iam.users WHERE id = @id AND status = 'active';
