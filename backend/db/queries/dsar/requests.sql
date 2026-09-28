-- name: LockRequestNumbering :exec
SELECT pg_advisory_xact_lock(hashtext('dsar.request_no:' || current_setting('app.tenant_id') || ':' || @year::text));

-- name: CountRequestsInYear :one
SELECT count(*)::int FROM dsar.requests WHERE request_no LIKE @prefix::text || '%';

-- name: InsertRequest :one
INSERT INTO dsar.requests (id, tenant_id, request_no, request_type_id, legal_entity_id, channel, on_behalf,
    details, requester_name_enc, requester_contact_enc, requester_blind_index, status, received_at, due_at,
    created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'received', $11, $12,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, request_no, request_type_id, legal_entity_id, channel, on_behalf, details, status, received_at,
    due_at, verified_at, closed_at, outcome, rejection_reason_code, assignee_user_id, row_version, updated_at;

-- name: GetRequestRequesterName :one
SELECT requester_name_enc FROM dsar.requests WHERE id = $1;

-- name: GetRequest :one
SELECT id, request_no, request_type_id, legal_entity_id, channel, on_behalf, details, status, received_at,
    due_at, verified_at, closed_at, outcome, rejection_reason_code, assignee_user_id, row_version, updated_at
FROM dsar.requests WHERE id = $1;

-- name: ListRequests :many
-- DSAR-17: request_no is matched as a case-insensitive substring (ม.39(7) "ค้นหาด้วยเลขคำขอ"); an email or
-- other identifier is matched by exact blind index (never decrypted to search — rule 3), computed by the
-- caller before this query runs.
SELECT id, request_no, request_type_id, legal_entity_id, channel, on_behalf, details, status, received_at,
    due_at, verified_at, closed_at, outcome, rejection_reason_code, assignee_user_id, row_version, updated_at
FROM dsar.requests
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(request_no)::text IS NULL OR request_no ILIKE '%' || sqlc.narg(request_no)::text || '%')
  AND (sqlc.narg(blind_index)::bytea IS NULL OR requester_blind_index = sqlc.narg(blind_index)::bytea)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (received_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY received_at DESC, id DESC
LIMIT @lim;

-- name: UpdateRequestStatus :one
UPDATE dsar.requests SET status = $2, outcome = $3, rejection_reason_code = $4,
    closed_at = CASE WHEN $2 IN ('completed', 'rejected', 'withdrawn') THEN now() ELSE closed_at END,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $5
RETURNING id, request_no, request_type_id, legal_entity_id, channel, on_behalf, details, status, received_at,
    due_at, verified_at, closed_at, outcome, rejection_reason_code, assignee_user_id, row_version, updated_at;

-- name: UpdateRequestAssignee :one
-- DSAR-07: who is notified as "the responsible person" alongside role DPO when the SLA reminder fires.
UPDATE dsar.requests SET assignee_user_id = $2,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $3
RETURNING id, request_no, request_type_id, legal_entity_id, channel, on_behalf, details, status, received_at,
    due_at, verified_at, closed_at, outcome, rejection_reason_code, assignee_user_id, row_version, updated_at;
