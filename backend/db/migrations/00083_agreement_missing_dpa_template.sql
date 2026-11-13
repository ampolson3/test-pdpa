-- +goose Up
-- DPA-12 (ม.40): the daily missing-DPA sweep's own alert — operational text, not legal wording sent to a
-- data subject, the same reasoning DSAR-08/CON-04/PNG-15/ROPA-15/VEN-06/VEN-10's own templates already use
-- (no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'agreement.missing_dpa', 'in_app', 'th', 'คู่ค้ายังไม่มีข้อตกลงประมวลผลข้อมูล (DPA)', 'คู่ค้า {{.vendor}} เป็นผู้ประมวลผลข้อมูลแต่ยังไม่มี DPA ที่มีผลบังคับ กรุณาติดตามจัดทำข้อตกลง', '["vendor"]'),
    (NULL, 'agreement.missing_dpa', 'in_app', 'en', 'Vendor has no active DPA', 'Vendor {{.vendor}} is a data processor with no agreement in effect — please follow up to put a DPA in place', '["vendor"]'),
    (NULL, 'agreement.missing_dpa', 'email', 'th', 'คู่ค้ายังไม่มีข้อตกลงประมวลผลข้อมูล (DPA)', 'คู่ค้า {{.vendor}} เป็นผู้ประมวลผลข้อมูลตามมาตรา 40 แต่ยังไม่มีข้อตกลงประมวลผลข้อมูล (DPA) ที่มีผลบังคับ กรุณาติดตามจัดทำข้อตกลงกับคู่ค้ารายนี้', '["vendor"]'),
    (NULL, 'agreement.missing_dpa', 'email', 'en', 'Vendor has no active DPA ({{.vendor}})', 'Vendor {{.vendor}} is a data processor under s.40 with no agreement currently in effect. Please follow up to put a DPA in place.', '["vendor"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'agreement.missing_dpa');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'agreement.missing_dpa';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
