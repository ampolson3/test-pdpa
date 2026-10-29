-- DPX-03: gov.audits/gov.audit_findings were already fully specified in the baseline migrations
-- (00017_gov.sql, FK'd in 00018: legal_entity_id -> org.legal_entities, assessment_id -> assess.
-- assessments, owner_user_id -> iam.users, task_id -> dpo.tasks, evidence_file_id -> platform.files) —
-- no migration needed.

-- name: InsertAudit :one
INSERT INTO gov.audits (id, tenant_id, audit_type, legal_entity_id, assessment_id, period, status, overall_score, completed_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @audit_type, @legal_entity_id, @assessment_id, @period, @status, @overall_score, @completed_at,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, audit_type, legal_entity_id, assessment_id, period, status, overall_score, completed_at, row_version, created_at, updated_at;

-- name: GetAudit :one
SELECT id, audit_type, legal_entity_id, assessment_id, period, status, overall_score, completed_at, row_version, created_at, updated_at
FROM gov.audits WHERE id = @id;

-- name: ListAudits :many
SELECT id, audit_type, legal_entity_id, assessment_id, period, status, overall_score, completed_at, row_version, created_at, updated_at
FROM gov.audits
WHERE (sqlc.narg(legal_entity_id)::uuid IS NULL OR legal_entity_id = sqlc.narg(legal_entity_id))
ORDER BY created_at DESC;

-- name: DeleteAudit :execrows
DELETE FROM gov.audits WHERE id = @id;

-- name: InsertAuditFinding :one
INSERT INTO gov.audit_findings (id, tenant_id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @audit_id, @title, @severity, sqlc.narg(requirement_ref)::text, sqlc.narg(recommendation)::text,
    sqlc.narg(owner_user_id)::uuid, sqlc.narg(due_at)::date,
    NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, status, task_id, evidence_file_id, row_version, created_at, updated_at;

-- name: GetAuditFinding :one
SELECT id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, status, task_id, evidence_file_id, row_version, created_at, updated_at
FROM gov.audit_findings WHERE id = @id;

-- name: ListAuditFindings :many
SELECT id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, status, task_id, evidence_file_id, row_version, created_at, updated_at
FROM gov.audit_findings WHERE audit_id = @audit_id ORDER BY
    CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, created_at;

-- name: UpdateAuditFinding :one
UPDATE gov.audit_findings
SET title = @title, severity = @severity, requirement_ref = sqlc.narg(requirement_ref)::text, recommendation = sqlc.narg(recommendation)::text,
    owner_user_id = sqlc.narg(owner_user_id)::uuid, due_at = sqlc.narg(due_at)::date,
    updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, status, task_id, evidence_file_id, row_version, created_at, updated_at;

-- name: SetAuditFindingStatus :one
UPDATE gov.audit_findings
SET status = @status, updated_at = now(), updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version
RETURNING id, audit_id, title, severity, requirement_ref, recommendation, owner_user_id, due_at, status, task_id, evidence_file_id, row_version, created_at, updated_at;
