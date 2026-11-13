-- +goose Up
-- DPIA-07 (มาตรการลดความเสี่ยงและความเสี่ยงคงเหลือ): risk.risks already carries residual_likelihood/
-- residual_impact/residual_score, but no residual_level — unlike the inherent score, which has its own
-- `level` column (risk.risks.level) classified against the tenant's matrix (RRA-02) once at identification/
-- update time. Adding the matching residual column keeps the two symmetric: both are snapshots computed by
-- Classify at write time (DPIA-06's own inherent `level`, this feature's own residual_level), not live-only
-- values with nothing to show until the next edit.
ALTER TABLE risk.risks ADD COLUMN residual_level text CHECK (residual_level IN ('low', 'medium', 'high', 'very_high'));

-- +goose Down
ALTER TABLE risk.risks DROP COLUMN residual_level;
