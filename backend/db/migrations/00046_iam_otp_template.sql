-- +goose Up
-- IAM-05 (decisions.md D-01): the OTP itself, sent over SMS or e-mail to a data subject's own identifier
-- (never a stored recipient — rule 3). Not legal text (rule 8 is about notices/letters/PDPC forms/clauses),
-- so no DRAFT marker, same reasoning as PNG-04/DSAR-07's own operational reminder templates.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'iam.otp', 'sms', 'th', NULL, 'รหัสยืนยันตัวตนของคุณคือ {{.code}} หมดอายุใน 5 นาที ห้ามให้ผู้อื่น', '["code"]'),
    (NULL, 'iam.otp', 'sms', 'en', NULL, 'Your verification code is {{.code}}. It expires in 5 minutes. Do not share it.', '["code"]'),
    (NULL, 'iam.otp', 'email', 'th', 'รหัสยืนยันตัวตนของคุณ', 'รหัสยืนยันตัวตนของคุณคือ {{.code}} รหัสนี้จะหมดอายุใน 5 นาที กรุณาอย่าเปิดเผยรหัสนี้กับผู้อื่น', '["code"]'),
    (NULL, 'iam.otp', 'email', 'en', 'Your verification code', 'Your verification code is {{.code}}. It expires in 5 minutes. Please do not share it with anyone.', '["code"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'iam.otp');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'iam.otp';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
