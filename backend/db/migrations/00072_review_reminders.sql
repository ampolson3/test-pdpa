-- +goose Up
-- PNG-15/ROPA-15 (periodic review reminder): notice.notices already has review_cycle_months + next_review_at
-- (unused until now) — ropa.processing_activities has next_review_at but no cycle column of its own, so this
-- adds one, the same default (12 months) notice.notices already uses.
ALTER TABLE ropa.processing_activities ADD COLUMN review_cycle_months smallint NOT NULL DEFAULT 12;

-- +goose StatementBegin
COMMENT ON COLUMN ropa.processing_activities.review_cycle_months IS 'เดือนระหว่างรอบทบทวน (ROPA-15)';
-- +goose StatementEnd

-- Operational reminder text (not legal wording sent to a data subject), same reasoning as DSAR-08/CON-04's
-- own operational templates — no DRAFT marker (rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'notice.review_due', 'in_app', 'th', 'ถึงรอบทบทวนประกาศ', 'ประกาศ "{{.title}}" ถึงรอบทบทวนตามกำหนด ({{.cycle_months}} เดือน) กรุณาตรวจสอบและอัปเดตหากจำเป็น', '["title", "cycle_months"]'),
    (NULL, 'notice.review_due', 'in_app', 'en', 'Notice review due', 'Notice "{{.title}}" is due for its periodic review ({{.cycle_months}} months) — please check it and update if needed', '["title", "cycle_months"]'),
    (NULL, 'notice.review_due', 'email', 'th', 'ถึงรอบทบทวนประกาศ: {{.title}}', 'ประกาศ "{{.title}}" ถึงรอบทบทวนตามกำหนด ({{.cycle_months}} เดือน) กรุณาตรวจสอบและอัปเดตหากจำเป็น', '["title", "cycle_months"]'),
    (NULL, 'notice.review_due', 'email', 'en', 'Notice review due: {{.title}}', 'Notice "{{.title}}" is due for its periodic review ({{.cycle_months}} months). Please check it and update if needed.', '["title", "cycle_months"]'),
    (NULL, 'ropa.review_due', 'in_app', 'th', 'ถึงรอบทบทวนกิจกรรม RoPA', 'กิจกรรม "{{.name}}" ({{.code}}) ถึงรอบทบทวนตามกำหนด ({{.cycle_months}} เดือน) กรุณาตรวจสอบและอัปเดตหากจำเป็น', '["name", "code", "cycle_months"]'),
    (NULL, 'ropa.review_due', 'in_app', 'en', 'RoPA activity review due', 'Activity "{{.name}}" ({{.code}}) is due for its periodic review ({{.cycle_months}} months) — please check it and update if needed', '["name", "code", "cycle_months"]'),
    (NULL, 'ropa.review_due', 'email', 'th', 'ถึงรอบทบทวนกิจกรรม: {{.name}}', 'กิจกรรม "{{.name}}" ({{.code}}) ถึงรอบทบทวนตามกำหนด ({{.cycle_months}} เดือน) กรุณาตรวจสอบและอัปเดตหากจำเป็น', '["name", "code", "cycle_months"]'),
    (NULL, 'ropa.review_due', 'email', 'en', 'RoPA activity review due: {{.name}}', 'Activity "{{.name}}" ({{.code}}) is due for its periodic review ({{.cycle_months}} months). Please check it and update if needed.', '["name", "code", "cycle_months"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('notice.review_due', 'ropa.review_due'));
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code IN ('notice.review_due', 'ropa.review_due');
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

ALTER TABLE ropa.processing_activities DROP COLUMN review_cycle_months;
