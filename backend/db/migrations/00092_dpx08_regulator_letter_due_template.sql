-- +goose Up
-- DPX-08 (regulator correspondence, gov.regulator_letters): operational text telling DPO/LEGAL a letter's
-- own due date is approaching — not legal/notice wording, the same reasoning DSAR-08/CON-04/PNG-15/
-- ROPA-15/VEN-10/VEN-06/VEN-03/ROPA-11's own templates already use (no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'gov.regulator_letter_due', 'in_app', 'th', 'ใกล้ครบกำหนดตอบหนังสือ สคส.', 'หนังสือเรื่อง "{{.subject}}" ครบกำหนดวันที่ {{.due_at}} กรุณาดำเนินการตอบกลับ', '["subject", "due_at"]'),
    (NULL, 'gov.regulator_letter_due', 'in_app', 'en', 'Regulator letter due soon', 'The letter "{{.subject}}" is due on {{.due_at}} — please prepare a response', '["subject", "due_at"]'),
    (NULL, 'gov.regulator_letter_due', 'email', 'th', 'ใกล้ครบกำหนดตอบหนังสือ สคส.', 'หนังสือเรื่อง "{{.subject}}" ครบกำหนดวันที่ {{.due_at}} กรุณาดำเนินการตอบกลับโดยเร็ว', '["subject", "due_at"]'),
    (NULL, 'gov.regulator_letter_due', 'email', 'en', 'Regulator letter due soon ({{.due_at}})', 'The letter "{{.subject}}" is due on {{.due_at}}. Please prepare and send a response.', '["subject", "due_at"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'gov.regulator_letter_due');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'gov.regulator_letter_due';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
