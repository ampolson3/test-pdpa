-- +goose Up
-- DSAR-07 (ม.30 วรรคสาม): internal staff reminder when a DSAR request has 7 days left before its 30-day
-- deadline. Operational text, not legal wording sent to a data subject, so no DRAFT marker (rule 8) — same
-- reasoning as PNG-04's notice.indirect_reminder/overdue templates.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'dsar.sla_reminder', 'in_app', 'th', 'ใกล้ครบกำหนดคำขอใช้สิทธิ', 'คำขอ {{.request_no}} เหลือ {{.days_left}} วันก่อนครบกำหนด 30 วัน (ม.30 วรรคสาม) — กำหนดคือ {{.due}}', '["request_no", "days_left", "due"]'),
    (NULL, 'dsar.sla_reminder', 'in_app', 'en', 'DSAR request due soon', 'Request {{.request_no}} has {{.days_left}} day(s) left of its 30-day window (ม.30 วรรคสาม) — due {{.due}}', '["request_no", "days_left", "due"]'),
    (NULL, 'dsar.sla_reminder', 'email', 'th', 'ใกล้ครบกำหนดคำขอใช้สิทธิ (ม.30 วรรคสาม)', 'คำขอใช้สิทธิเลขที่ {{.request_no}} เหลือ {{.days_left}} วันก่อนครบกำหนดดำเนินการภายใน 30 วัน กำหนดคือ {{.due}}', '["request_no", "days_left", "due"]'),
    (NULL, 'dsar.sla_reminder', 'email', 'en', 'DSAR request due soon (ม.30 วรรคสาม)', 'DSAR request {{.request_no}} has {{.days_left}} day(s) left before its 30-day deadline. Due by {{.due}}.', '["request_no", "days_left", "due"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'dsar.sla_reminder');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'dsar.sla_reminder';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
