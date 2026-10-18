-- +goose Up
-- VEN-03 (sub-processor management, ม.40): vendor.sub_processors was already fully specified in the
-- baseline migrations, unused until now — only the operational alert text (not legal wording sent to a
-- data subject, the same reasoning DSAR-08/CON-04/PNG-15/ROPA-15/VEN-10/VEN-06's own templates already
-- use — no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'vendor.sub_processor_added', 'in_app', 'th', 'มีผู้ประมวลผลช่วงรายใหม่รออนุมัติ', 'คู่ค้าได้เพิ่มผู้ประมวลผลช่วงสำหรับงาน "{{.service}}" กรุณาพิจารณาอนุมัติก่อนใช้งาน', '["service"]'),
    (NULL, 'vendor.sub_processor_added', 'in_app', 'en', 'A new sub-processor is awaiting approval', 'The vendor added a sub-processor for "{{.service}}" — please review it before it is used', '["service"]'),
    (NULL, 'vendor.sub_processor_added', 'email', 'th', 'มีผู้ประมวลผลช่วงรายใหม่รออนุมัติ', 'คู่ค้าได้เพิ่มผู้ประมวลผลช่วงสำหรับงาน "{{.service}}" กรุณาพิจารณาอนุมัติก่อนใช้งาน', '["service"]'),
    (NULL, 'vendor.sub_processor_added', 'email', 'en', 'A new sub-processor is awaiting approval ({{.service}})', 'The vendor added a sub-processor for "{{.service}}". Please review and decide before it is used.', '["service"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'vendor.sub_processor_added');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'vendor.sub_processor_added';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
