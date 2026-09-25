-- CON-09 collection points.

-- name: ListCollectionPoints :many
SELECT id, code, name, channel, legal_entity_id, form_id, verification_method, double_opt_in, age_gate, status,
       public_key, allowed_origins, publish_checklist, published_at, published_by, row_version, updated_at
FROM consent.collection_points ORDER BY status, code;

-- name: GetCollectionPoint :one
SELECT id, code, name, channel, legal_entity_id, form_id, verification_method, double_opt_in, age_gate, status,
       public_key, allowed_origins, publish_checklist, published_at, published_by, row_version, updated_at
FROM consent.collection_points WHERE id = $1;

-- name: GetCollectionPointByCode :one
SELECT id FROM consent.collection_points WHERE code = $1;

-- name: LockCollectionPoint :one
SELECT id, code, name, channel, legal_entity_id, form_id, verification_method, double_opt_in, age_gate, status,
       public_key, allowed_origins, publish_checklist, published_at, published_by, row_version, updated_at
FROM consent.collection_points WHERE id = $1 FOR UPDATE;

-- name: InsertCollectionPoint :exec
INSERT INTO consent.collection_points (id, tenant_id, code, name, channel, legal_entity_id, allowed_origins, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid);

-- name: UpdateCollectionPoint :execrows
UPDATE consent.collection_points
SET name = $3, channel = $4, legal_entity_id = $5, allowed_origins = $6, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2;

-- name: PublishCollectionPoint :exec
UPDATE consent.collection_points
SET status = 'active', public_key = coalesce(public_key, @public_key), publish_checklist = @checklist, published_at = now(),
    published_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id;

-- name: RetireCollectionPoint :exec
UPDATE consent.collection_points SET status = 'retired', row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1;

-- name: ListCollectionPointPurposes :many
SELECT cpp.collection_point_id, cpp.purpose_id, cpp.is_required, cpp.display_order,
       p.code, p.name_th, p.name_en, p.status, p.is_sensitive, p.requires_explicit, p.min_age, p.lifespan_days,
       p.current_version_id, v.version_no AS current_version_no, v.text_th, v.text_en, v.explicit_text_th, v.explicit_text_en
FROM consent.collection_point_purposes cpp
JOIN consent.purposes p ON p.id = cpp.purpose_id
LEFT JOIN consent.purpose_versions v ON v.id = p.current_version_id
WHERE cpp.collection_point_id = $1
ORDER BY cpp.display_order, p.code;

-- name: DeleteCollectionPointPurposes :exec
DELETE FROM consent.collection_point_purposes WHERE collection_point_id = $1;

-- name: InsertCollectionPointPurpose :exec
INSERT INTO consent.collection_point_purposes (collection_point_id, purpose_id, tenant_id, is_required, display_order)
VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4);
