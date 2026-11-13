-- DSAR-04 (ม.20, ผู้รับมอบอำนาจ/ผู้ใช้อำนาจปกครอง/ผู้อนุบาล/ผู้พิทักษ์) — dsar.agents, already fully
-- specified in the baseline migrations: authority_file_id is NOT NULL, so a row can never exist without an
-- authority-proof document attached.

-- name: InsertAgent :one
INSERT INTO dsar.agents (id, tenant_id, request_id, agent_name_enc, authority_type, authority_file_id,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @request_id, @agent_name_enc, @authority_type,
    @authority_file_id, NULLIF(current_setting('app.user_id', true), '')::uuid,
    NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_id, authority_type, authority_file_id, verified_at, created_at, row_version;

-- name: GetAgent :one
SELECT id, request_id, authority_type, authority_file_id, verified_at, created_at, row_version
FROM dsar.agents
WHERE id = $1;

-- name: ListAgentsForRequest :many
SELECT id, request_id, authority_type, authority_file_id, verified_at, created_at, row_version
FROM dsar.agents
WHERE request_id = $1
ORDER BY created_at;

-- name: CountAgentsForRequest :one
SELECT count(*) FROM dsar.agents WHERE request_id = $1;

-- name: VerifyAgent :one
-- Staff confirm the authority document themselves (no trusted registry to check a power of attorney or
-- guardianship document against, the same human-judgement pattern DSAR-06's own id_document path uses) —
-- guarded to a still-unverified row so a redelivery or double-click is a harmless no-op.
UPDATE dsar.agents
SET verified_at = now(), row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = @id AND verified_at IS NULL
RETURNING id, request_id, authority_type, authority_file_id, verified_at, created_at, row_version;
