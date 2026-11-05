-- name: ListActivityControls :many
SELECT activity_id, control_id, description
FROM ropa.activity_controls
WHERE activity_id = $1
ORDER BY control_id;

-- name: InsertActivityControl :one
INSERT INTO ropa.activity_controls (activity_id, control_id, tenant_id, description)
VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3)
RETURNING activity_id, control_id, description;

-- name: DeleteActivityControl :execrows
DELETE FROM ropa.activity_controls WHERE activity_id = $1 AND control_id = $2;
