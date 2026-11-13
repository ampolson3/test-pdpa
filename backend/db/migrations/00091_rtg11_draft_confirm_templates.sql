-- +goose Up
-- RTG-11 (มอบหมายร่าง RoPA ให้หน่วยงานยืนยัน): operational reminder/overdue text (not legal wording sent
-- to a data subject, the same reasoning DSAR-08/CON-04/PNG-15/ROPA-15/VEN-06/VEN-10's own templates already
-- use — no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'ropa.draft_confirm_reminder', 'in_app', 'th', 'กรุณายืนยันร่าง RoPA', 'ร่างกิจกรรม "{{.activity_name}}" ({{.activity_code}}) ยังไม่ได้รับการยืนยัน กรุณาตรวจสอบและส่งเพื่อขออนุมัติ', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_reminder', 'in_app', 'en', 'Please confirm the RoPA draft', 'The activity draft "{{.activity_name}}" ({{.activity_code}}) has not been confirmed yet — please review and submit it for approval', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_reminder', 'email', 'th', 'กรุณายืนยันร่าง RoPA', 'ร่างกิจกรรม "{{.activity_name}}" ({{.activity_code}}) ยังไม่ได้รับการยืนยัน กรุณาตรวจสอบและส่งเพื่อขออนุมัติ', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_reminder', 'email', 'en', 'Please confirm the RoPA draft ({{.activity_code}})', 'The activity draft "{{.activity_name}}" ({{.activity_code}}) has not been confirmed yet. Please review it and submit it for approval.', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_overdue', 'in_app', 'th', 'ร่าง RoPA เกินกำหนดยืนยัน', 'ร่างกิจกรรม "{{.activity_name}}" ({{.activity_code}}) ยังไม่ได้รับการยืนยันเกินกำหนดเวลาแล้ว กรุณาดำเนินการโดยเร็ว', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_overdue', 'in_app', 'en', 'RoPA draft confirmation overdue', 'The activity draft "{{.activity_name}}" ({{.activity_code}}) is overdue for confirmation — please act on it soon', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_overdue', 'email', 'th', 'ร่าง RoPA เกินกำหนดยืนยัน', 'ร่างกิจกรรม "{{.activity_name}}" ({{.activity_code}}) ยังไม่ได้รับการยืนยันเกินกำหนดเวลาแล้ว กรุณาดำเนินการโดยเร็ว', '["activity_name", "activity_code"]'),
    (NULL, 'ropa.draft_confirm_overdue', 'email', 'en', 'RoPA draft confirmation overdue ({{.activity_code}})', 'The activity draft "{{.activity_name}}" ({{.activity_code}}) is overdue for confirmation. Please act on it soon.', '["activity_name", "activity_code"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('ropa.draft_confirm_reminder', 'ropa.draft_confirm_overdue'));
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code IN ('ropa.draft_confirm_reminder', 'ropa.draft_confirm_overdue');
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
