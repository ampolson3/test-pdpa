-- BRE-02 / 13 breach register.

-- name: LockIncidentNumbering :exec
SELECT pg_advisory_xact_lock(hashtext('breach.incident_no:' || current_setting('app.tenant_id') || ':' || @year::text));

-- name: CountIncidentsInYear :one
SELECT count(*)::int FROM breach.incidents WHERE incident_no LIKE @prefix::text || '%';

-- name: InsertIncident :one
INSERT INTO breach.incidents (id, tenant_id, incident_no, legal_entity_id, reported_via, reporter_user_id, title, description,
    breach_types, incident_type, occurred_at, aware_at, contained_at, affected_subjects, affected_categories, pdpc_due_at,
    is_drill, owner_user_id, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_no, @legal_entity_id, @reported_via, @reporter_user_id, @title,
    @description, @breach_types, @incident_type, @occurred_at, @aware_at, @contained_at, @affected_subjects, @affected_categories,
    @pdpc_due_at, @is_drill, @owner_user_id, @reporter_user_id, @reporter_user_id)
RETURNING id;

-- name: GetIncident :one
SELECT id, incident_no, legal_entity_id, reported_via, reporter_user_id, processor_party_id, title, description, breach_types,
    incident_type, occurred_at, aware_at, contained_at, affected_subjects, affected_categories, risk_level, decision,
    decision_reason, decided_by, pdpc_due_at, late_reason, is_drill, status, owner_user_id, close_reason, created_at,
    updated_at, row_version
FROM breach.incidents WHERE id = @id;

-- name: LockIncident :one
SELECT id, incident_no, legal_entity_id, reported_via, reporter_user_id, processor_party_id, title, description, breach_types,
    incident_type, occurred_at, aware_at, contained_at, affected_subjects, affected_categories, risk_level, decision,
    decision_reason, decided_by, pdpc_due_at, late_reason, is_drill, status, owner_user_id, close_reason, created_at,
    updated_at, row_version
FROM breach.incidents WHERE id = @id FOR UPDATE;

-- name: ListIncidents :many
-- Newest awareness first; keyset cursor on (aware_at, id). reporter limits to incidents one person reported
-- (people who may only report see their own).
SELECT id, incident_no, legal_entity_id, reported_via, reporter_user_id, processor_party_id, title, description, breach_types,
    incident_type, occurred_at, aware_at, contained_at, affected_subjects, affected_categories, risk_level, decision,
    decision_reason, decided_by, pdpc_due_at, late_reason, is_drill, status, owner_user_id, close_reason, created_at,
    updated_at, row_version
FROM breach.incidents
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(reporter)::uuid IS NULL OR reporter_user_id = sqlc.narg(reporter))
  AND (sqlc.narg(owner)::uuid IS NULL OR owner_user_id = sqlc.narg(owner))
  AND (sqlc.narg(risk)::text IS NULL OR risk_level = sqlc.narg(risk))
  AND (sqlc.narg(q)::text IS NULL OR incident_no ILIKE '%' || sqlc.narg(q) || '%' OR title ILIKE '%' || sqlc.narg(q) || '%'
       OR description ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR aware_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR aware_at < sqlc.narg(to_at))
  AND (NOT @open_only::bool OR status <> 'closed')
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (aware_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY aware_at DESC, id DESC
LIMIT @lim;

-- name: UpdateIncidentFacts :execrows
UPDATE breach.incidents SET title = @title, description = @description, breach_types = @breach_types, incident_type = @incident_type,
    occurred_at = @occurred_at, aware_at = @aware_at, pdpc_due_at = @pdpc_due_at, contained_at = @contained_at,
    affected_subjects = @affected_subjects, affected_categories = @affected_categories, owner_user_id = @owner_user_id,
    updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id AND row_version = @row_version;

-- name: SetIncidentStatus :exec
UPDATE breach.incidents SET status = @status, close_reason = COALESCE(sqlc.narg(close_reason), close_reason),
    owner_user_id = COALESCE(sqlc.narg(owner_user_id), owner_user_id),
    updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id;

-- name: SetIncidentRisk :exec
UPDATE breach.incidents SET risk_level = @risk_level, updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id;

-- name: SetIncidentDecision :exec
UPDATE breach.incidents SET decision = @decision, decision_reason = @decision_reason, decided_by = @actor, status = @status,
    updated_at = now(), updated_by = @actor, row_version = row_version + 1
WHERE id = @id;

-- name: InsertTimelineEvent :exec
INSERT INTO breach.timeline_events (id, tenant_id, incident_id, occurred_at, event_type, description, actor_id, is_auto)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @occurred_at, @event_type, @description, @actor_id, @is_auto);

-- name: ListTimeline :many
SELECT id, occurred_at, event_type, description, actor_id, is_auto FROM breach.timeline_events
WHERE incident_id = @incident_id ORDER BY occurred_at, id;

-- name: InsertAssessment :exec
INSERT INTO breach.assessments (id, tenant_id, incident_id, form_submission_id, score, risk_level, factors, assessed_by, assessed_at,
    created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @form_submission_id, @score, @risk_level, @factors, @assessed_by,
    @assessed_at, @assessed_by, @assessed_by);

-- name: ListAssessments :many
SELECT id, form_submission_id, score, risk_level, factors, assessed_by, assessed_at FROM breach.assessments
WHERE incident_id = @incident_id ORDER BY assessed_at DESC, id DESC;

-- name: InsertEvidence :exec
INSERT INTO breach.evidence (id, tenant_id, incident_id, file_id, description, collected_by, collected_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @incident_id, @file_id, @description, @collected_by, @collected_at,
    @collected_by, @collected_by);

-- name: ListEvidence :many
SELECT id, file_id, description, collected_by, collected_at FROM breach.evidence WHERE incident_id = @incident_id
ORDER BY collected_at, id;
