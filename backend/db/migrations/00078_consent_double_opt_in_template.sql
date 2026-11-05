-- +goose Up
-- CON-23 (double opt-in): the e-mailed/SMS confirmation link itself — operational text, not legal wording
-- sent about a decision already in effect (the decision has no effect until this link is confirmed), so no
-- DRAFT marker, the same reasoning IAM-05/DSAR-07/PNG-04's own operational reminder templates already use.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'consent.double_opt_in', 'sms', 'th', NULL, 'กรุณายืนยันความยินยอมของคุณที่ {{.link}} ลิงก์นี้จะหมดอายุภายใน 48 ชั่วโมง', '["link"]'),
    (NULL, 'consent.double_opt_in', 'sms', 'en', NULL, 'Please confirm your consent at {{.link}}. This link expires in 48 hours.', '["link"]'),
    (NULL, 'consent.double_opt_in', 'email', 'th', 'กรุณายืนยันความยินยอมของคุณ', 'ความยินยอมของคุณจะมีผลหลังยืนยันเท่านั้น กรุณากดลิงก์เพื่อยืนยัน: {{.link}} ลิงก์นี้จะหมดอายุภายใน 48 ชั่วโมง', '["link"]'),
    (NULL, 'consent.double_opt_in', 'email', 'en', 'Please confirm your consent', 'Your consent takes effect only once confirmed. Please click the link to confirm: {{.link}}. This link expires in 48 hours.', '["link"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'consent.double_opt_in');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'consent.double_opt_in';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
