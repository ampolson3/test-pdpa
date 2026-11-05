-- +goose Up
-- DPA-10 (DPA register & expiry alerts): notifies role LEGAL renewal_notice_days before an agreement's own
-- end date. Operational text, not legal wording sent to a counterparty, so no DRAFT marker (rule 8) — same
-- reasoning as DSAR-07/DSAR-08's own reminder templates.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'agreement.renewal_reminder', 'in_app', 'th', 'สัญญาใกล้หมดอายุ', 'ข้อตกลง {{.agreement_no}} "{{.title}}" จะหมดอายุวันที่ {{.effective_to}} (เหลือ {{.days_left}} วัน)', '["agreement_no", "title", "effective_to", "days_left"]'),
    (NULL, 'agreement.renewal_reminder', 'in_app', 'en', 'Agreement nearing expiry', 'Agreement {{.agreement_no}} "{{.title}}" expires on {{.effective_to}} ({{.days_left}} days left)', '["agreement_no", "title", "effective_to", "days_left"]'),
    (NULL, 'agreement.renewal_reminder', 'email', 'th', 'ข้อตกลง {{.agreement_no}} ใกล้หมดอายุ', 'ข้อตกลง {{.agreement_no}} "{{.title}}" จะหมดอายุวันที่ {{.effective_to}} เหลือเวลา {{.days_left}} วัน กรุณาพิจารณาต่ออายุหรือยุติตามความเหมาะสม', '["agreement_no", "title", "effective_to", "days_left"]'),
    (NULL, 'agreement.renewal_reminder', 'email', 'en', 'Agreement {{.agreement_no}} is nearing expiry', 'Agreement {{.agreement_no}} "{{.title}}" expires on {{.effective_to}}, {{.days_left}} days from now. Please review for renewal or termination.', '["agreement_no", "title", "effective_to", "days_left"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'agreement.renewal_reminder');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'agreement.renewal_reminder';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
