-- +goose Up
-- RRA-04 (วิเคราะห์ช่องว่างทางกฎหมายอัตโนมัติ; ม.23, ม.24, ม.26, ม.28-29, ม.39): risk.gap_rules/
-- risk.gap_findings (baseline migration 00010) were already fully specified for exactly this feature —
-- gap_findings even already carries a task_id FK to dpo.tasks, anticipating RRA-07's remediation-task
-- linkage. This migration seeds the five gap types the module doc's own description names, as a global
-- (tenant_id NULL) catalog the same "ORG-07 master data" visibility pattern risk.controls (ROPA-09,
-- decisions.md Q-25) already uses. `expression` is reserved for a future generic rule interpreter — none
-- exists yet, so it is an empty placeholder and the actual check per code is a small Go switch in
-- internal/risk/service/gap_analysis.go (gapPresent); adding a new rule later means a new row here plus a
-- new case there, matching "เพิ่ม rule ได้" (rules are extensible) without inventing an expression
-- language this feature's own acceptance criterion never asked for. Global rows need RLS force lifted
-- inside this migration, as in 00019/00031/00040 (restored before commit).

ALTER TABLE risk.gap_rules NO FORCE ROW LEVEL SECURITY;

INSERT INTO risk.gap_rules (code, name, expression, severity, legal_ref) VALUES
    ('no_lawful_basis', 'กิจกรรมไม่มีวัตถุประสงค์หรือฐานทางกฎหมาย', '{}'::jsonb, 'high', 'ม.24, ม.39(1)'),
    ('no_retention', 'กิจกรรมไม่มีระยะเวลาเก็บรักษาหรือวิธีทำลายข้อมูล', '{}'::jsonb, 'medium', 'ม.39(3)'),
    ('no_notice_coverage', 'กิจกรรมยังไม่มีประกาศความเป็นส่วนตัวครอบคลุม', '{}'::jsonb, 'high', 'ม.23'),
    ('transfer_no_basis', 'มีการโอนข้อมูลไปต่างประเทศโดยไม่มีฐานการโอนที่บันทึกไว้', '{}'::jsonb, 'high', 'ม.28-29'),
    ('sensitive_no_consent', 'มีข้อมูลอ่อนไหวโดยไม่มีหลักฐานความยินยอมโดยชัดแจ้ง', '{}'::jsonb, 'high', 'ม.26');

ALTER TABLE risk.gap_rules FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE risk.gap_rules NO FORCE ROW LEVEL SECURITY;
DELETE FROM risk.gap_rules WHERE tenant_id IS NULL AND code IN (
    'no_lawful_basis', 'no_retention', 'no_notice_coverage', 'transfer_no_basis', 'sensitive_no_consent'
);
ALTER TABLE risk.gap_rules FORCE ROW LEVEL SECURITY;
