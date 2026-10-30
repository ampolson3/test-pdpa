-- RRA-05 per-activity checklist.

-- name: InsertActivityChecklist :one
INSERT INTO risk.activity_checklists (id, tenant_id, activity_id, form_submission_id, score, result, factors,
    completed_by, completed_at, created_by, updated_by)
VALUES (@id, current_setting('app.tenant_id')::uuid, @activity_id, @form_submission_id, @score, @result, @factors,
    @completed_by, @completed_at, @completed_by, @completed_by)
RETURNING id, activity_id, form_submission_id, score, result, factors, completed_by, completed_at, row_version;

-- name: GetActivityChecklist :one
SELECT id, activity_id, form_submission_id, score, result, factors, completed_by, completed_at, row_version
FROM risk.activity_checklists
WHERE id = $1;

-- name: ListActivityChecklists :many
-- Newest first, for one activity.
SELECT id, activity_id, form_submission_id, score, result, factors, completed_by, completed_at, row_version
FROM risk.activity_checklists
WHERE activity_id = @activity_id
ORDER BY completed_at DESC, id DESC;
