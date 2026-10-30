-- RRA-01 (ประเมินความเสี่ยงรายกิจกรรม): each computation is a new row (risk.activity_scores is already
-- fully specified in the baseline migrations) — a plain history trail, never updated in place.

-- name: InsertActivityScore :one
INSERT INTO risk.activity_scores (tenant_id, activity_id, matrix_id, likelihood, impact, inherent_score, level, factor_breakdown)
VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, @activity_id, @matrix_id, @likelihood, @impact, @inherent_score, @level, @factor_breakdown)
RETURNING *;

-- name: GetLatestActivityScore :one
SELECT * FROM risk.activity_scores WHERE activity_id = $1 ORDER BY computed_at DESC LIMIT 1;

-- name: ListLatestHighRiskScores :many
-- RRA-11: every activity's own *current* score (latest computation, not any stale earlier one) that reads
-- high/very_high right now — "กิจกรรมเสี่ยงสูงแสดงเด่นทั้งในรายการและแผนผัง".
SELECT * FROM (
    SELECT DISTINCT ON (activity_id) *
    FROM risk.activity_scores
    ORDER BY activity_id, computed_at DESC
) latest
WHERE level IN ('high', 'very_high')
ORDER BY inherent_score DESC;
