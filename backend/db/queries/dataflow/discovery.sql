-- name: ListActiveClassifiers :many
-- Every classifier visible to this tenant (global defaults + the tenant's own, if any ever exist) —
-- DFG-09's own classification pass runs every active one against every sampled value.
SELECT id, code, name, classifier_type, pattern, data_category_id
FROM dataflow.classifiers
WHERE is_active = true
ORDER BY code;

-- name: InsertDiscoveryScan :one
INSERT INTO dataflow.discovery_scans (tenant_id, connector_id, asset_id, status, started_at)
VALUES (current_setting('app.tenant_id')::uuid, @connector_id, sqlc.narg(asset_id), 'running', now())
RETURNING id, connector_id, asset_id, status, started_at, finished_at, objects_scanned, findings_count, row_version, created_at;

-- name: FinishDiscoveryScan :one
UPDATE dataflow.discovery_scans
SET status = @status, finished_at = now(), objects_scanned = @objects_scanned, findings_count = @findings_count,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1
RETURNING id, connector_id, asset_id, status, started_at, finished_at, objects_scanned, findings_count, row_version, created_at;

-- name: GetDiscoveryScan :one
SELECT id, connector_id, asset_id, status, started_at, finished_at, objects_scanned, findings_count, row_version, created_at
FROM dataflow.discovery_scans
WHERE id = $1;

-- name: ListDiscoveryScans :many
-- Newest first; keyset cursor on (created_at, id).
SELECT id, connector_id, asset_id, status, started_at, finished_at, objects_scanned, findings_count, row_version, created_at
FROM dataflow.discovery_scans
WHERE (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: InsertDiscoveryFinding :one
INSERT INTO dataflow.discovery_findings (tenant_id, scan_id, object_path, classifier_id, match_ratio, sample_count, suggested_category_id)
VALUES (current_setting('app.tenant_id')::uuid, @scan_id, @object_path, @classifier_id, @match_ratio, @sample_count, sqlc.narg(suggested_category_id))
RETURNING id, scan_id, object_path, classifier_id, match_ratio, sample_count, suggested_category_id, status, reviewed_by, row_version, created_at;

-- name: ListDiscoveryFindingsForScan :many
SELECT f.id, f.scan_id, f.object_path, f.classifier_id, c.code AS classifier_code, c.name AS classifier_name,
    f.match_ratio, f.sample_count, f.suggested_category_id, f.status, f.reviewed_by, f.row_version, f.created_at
FROM dataflow.discovery_findings f
JOIN dataflow.classifiers c ON c.id = f.classifier_id
WHERE f.scan_id = $1
ORDER BY f.match_ratio DESC, f.created_at;

-- name: GetDiscoveryFinding :one
SELECT id, scan_id, object_path, classifier_id, match_ratio, sample_count, suggested_category_id, status, reviewed_by, row_version, created_at
FROM dataflow.discovery_findings
WHERE id = $1;

-- name: ReviewDiscoveryFinding :one
UPDATE dataflow.discovery_findings
SET status = @status, reviewed_by = NULLIF(current_setting('app.user_id', true), '')::uuid,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = $1 AND row_version = $2
RETURNING id, scan_id, object_path, classifier_id, match_ratio, sample_count, suggested_category_id, status, reviewed_by, row_version, created_at;
