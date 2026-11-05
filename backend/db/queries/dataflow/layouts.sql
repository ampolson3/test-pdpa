-- name: GetLayout :one
-- DFG-05: the caller's own saved drag-and-drop overlay for one (view_type, scope_id) diagram. Scoped by
-- user_id too (NULLS NOT DISTINCT on the matching unique index, migration 00099) — each person arranges
-- their own copy of the diagram.
SELECT id, layout, row_version, updated_at
FROM dataflow.layouts
WHERE view_type = sqlc.arg(view_type)::text
    AND (scope_id = sqlc.narg(scope_id)::uuid OR (scope_id IS NULL AND sqlc.narg(scope_id)::uuid IS NULL))
    AND (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
        OR (user_id IS NULL AND NULLIF(current_setting('app.user_id', true), '')::uuid IS NULL));

-- name: UpsertLayout :one
-- A full replace of the overlay — never a merge, per DFG-05's own "ตำแหน่งที่จัดเองยังอยู่หลังข้อมูลอัปเดต"
-- acceptance criterion read literally as "the saved overlay itself persists," not a field-by-field patch.
INSERT INTO dataflow.layouts (tenant_id, view_type, scope_id, layout, user_id, created_by, updated_by)
VALUES (
    current_setting('app.tenant_id')::uuid, sqlc.arg(view_type)::text, sqlc.narg(scope_id)::uuid, sqlc.arg(layout)::jsonb,
    NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid
)
ON CONFLICT (tenant_id, view_type, scope_id, user_id)
DO UPDATE SET layout = EXCLUDED.layout, updated_by = EXCLUDED.updated_by, updated_at = now(), row_version = dataflow.layouts.row_version + 1
WHERE dataflow.layouts.row_version = sqlc.arg(expected_row_version)::int
RETURNING id, layout, row_version, updated_at;

-- name: DeleteLayout :exec
-- DFG-05: "คืนค่าเดิม" — discard the caller's own saved overlay; a harmless no-op if nothing was saved.
DELETE FROM dataflow.layouts
WHERE view_type = sqlc.arg(view_type)::text
    AND (scope_id = sqlc.narg(scope_id)::uuid OR (scope_id IS NULL AND sqlc.narg(scope_id)::uuid IS NULL))
    AND (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
        OR (user_id IS NULL AND NULLIF(current_setting('app.user_id', true), '')::uuid IS NULL));
