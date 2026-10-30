-- +goose Up
-- ROPA-11 (data mapping questionnaire): operational text telling the respondent a questionnaire is
-- waiting for them — not legal/notice wording, the same reasoning DSAR-08/CON-04/PNG-15/ROPA-15/VEN-10/
-- VEN-06/VEN-03's own templates already use (no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'ropa.questionnaire_sent', 'in_app', 'th', 'มีแบบสอบถามข้อมูลส่วนบุคคลรอตอบ', 'ฝ่าย {{.org_unit}} ได้รับแบบสอบถามเก็บข้อมูลกิจกรรมประมวลผลข้อมูลส่วนบุคคล กรุณากรอกและส่งกลับ', '["org_unit"]'),
    (NULL, 'ropa.questionnaire_sent', 'in_app', 'en', 'A data mapping questionnaire is waiting for you', 'The {{.org_unit}} department has a new data mapping questionnaire to complete — please fill it in and submit', '["org_unit"]'),
    (NULL, 'ropa.questionnaire_sent', 'email', 'th', 'มีแบบสอบถามข้อมูลส่วนบุคคลรอตอบ', 'ฝ่าย {{.org_unit}} ได้รับแบบสอบถามเก็บข้อมูลกิจกรรมประมวลผลข้อมูลส่วนบุคคล กรุณากรอกและส่งกลับโดยเร็ว', '["org_unit"]'),
    (NULL, 'ropa.questionnaire_sent', 'email', 'en', 'A data mapping questionnaire is waiting for you ({{.org_unit}})', 'The {{.org_unit}} department has a new data mapping questionnaire. Please complete and submit it.', '["org_unit"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'ropa.questionnaire_sent');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'ropa.questionnaire_sent';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
