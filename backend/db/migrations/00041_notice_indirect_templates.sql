-- +goose Up
-- PNG-04 (ม.25): internal staff reminders for the 30-day indirect-collection notice deadline. Operational text,
-- not legal wording sent to a data subject (that's the method/evidence the staff member records themselves), so
-- no DRAFT marker is needed (CLAUDE.md rule 8 is about text shown to a data subject or the PDPC).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'notice.indirect_reminder', 'in_app', 'th', 'ใกล้ครบกำหนดแจ้งเจ้าของข้อมูล', 'เหลือ {{.days_left}} วันก่อนครบกำหนด 30 วัน (ม.25) — กำหนดแจ้งคือ {{.due}}', '["days_left", "due"]'),
    (NULL, 'notice.indirect_reminder', 'in_app', 'en', 'Indirect-collection notice due soon', '{{.days_left}} day(s) left of the 30-day window (ม.25) — due {{.due}}', '["days_left", "due"]'),
    (NULL, 'notice.indirect_reminder', 'email', 'th', 'ใกล้ครบกำหนดแจ้งเจ้าของข้อมูล (ม.25)', 'รายการเก็บข้อมูลจากแหล่งอื่นเหลือ {{.days_left}} วันก่อนครบกำหนดแจ้งเจ้าของข้อมูลภายใน 30 วัน กำหนดแจ้งคือ {{.due}}', '["days_left", "due"]'),
    (NULL, 'notice.indirect_reminder', 'email', 'en', 'Indirect-collection notice due soon (ม.25)', 'An indirect-collection record has {{.days_left}} day(s) left before its 30-day notice deadline. Due by {{.due}}.', '["days_left", "due"]'),
    (NULL, 'notice.indirect_overdue', 'in_app', 'th', 'เกินกำหนด 30 วันแล้ว', 'ยังไม่ได้บันทึกหลักฐานการแจ้งเจ้าของข้อมูล (ม.25) — กำหนดแจ้งคือ {{.due}}', '["due"]'),
    (NULL, 'notice.indirect_overdue', 'in_app', 'en', 'Past the 30-day deadline', 'No evidence of notice has been recorded yet (ม.25) — was due {{.due}}', '["due"]'),
    (NULL, 'notice.indirect_overdue', 'email', 'th', 'เกินกำหนดแจ้งเจ้าของข้อมูล 30 วัน (ม.25)', 'รายการเก็บข้อมูลจากแหล่งอื่นเกินกำหนดแจ้งเจ้าของข้อมูลแล้ว (กำหนดแจ้งคือ {{.due}}) กรุณาบันทึกวิธีแจ้งและหลักฐานโดยเร็ว', '["due"]'),
    (NULL, 'notice.indirect_overdue', 'email', 'en', 'Past the 30-day indirect-collection deadline (ม.25)', 'An indirect-collection record is past its notice deadline (was due {{.due}}). Please record the method and evidence as soon as possible.', '["due"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('notice.indirect_reminder', 'notice.indirect_overdue'));
DELETE FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('notice.indirect_reminder', 'notice.indirect_overdue');
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
