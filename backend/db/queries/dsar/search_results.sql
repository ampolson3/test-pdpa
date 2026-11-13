-- name: InsertSearchResult :one
-- DSAR-09: one row per connector searched in a single cross-system search run.
INSERT INTO dsar.search_results (tenant_id, request_id, connector_id, found, record_count, summary_enc, searched_at)
VALUES (current_setting('app.tenant_id')::uuid, @request_id, @connector_id, @found, sqlc.narg(record_count), sqlc.narg(summary_enc), now())
RETURNING id, tenant_id, request_id, connector_id, found, record_count, searched_at, purged_at;

-- name: ListSearchResultsForRequest :many
-- Joined to platform.connectors for the connector's own name/type — a plain read-only convenience join
-- across schemas, the same pattern ROPA-01's own inventory query already uses for org.data_categories.
-- Never selects summary_enc (rule 3: the decrypted contents are never surfaced on this list).
SELECT sr.id, sr.connector_id, c.name AS connector_name, c.connector_type, sr.found, sr.record_count,
    sr.searched_at, sr.purged_at
FROM dsar.search_results sr
JOIN platform.connectors c ON c.id = sr.connector_id
WHERE sr.request_id = $1
ORDER BY sr.searched_at DESC;

-- name: PurgeSearchResultsForRequest :exec
-- DSAR-09's own acceptance criterion: "ผลถูกลบเมื่อปิดคำขอ" — the finding itself (found, record_count,
-- the encrypted summary) is wiped the moment the request closes; the row stays only as a trail of which
-- connectors were searched and that their result was later purged, the same "row kept, content gone"
-- pattern PLT-09 already uses for an infected file.
UPDATE dsar.search_results
SET found = false, record_count = NULL, summary_enc = NULL, result_file_id = NULL, purged_at = now()
WHERE request_id = $1 AND purged_at IS NULL;
