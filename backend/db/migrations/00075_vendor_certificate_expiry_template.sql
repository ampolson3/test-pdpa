-- +goose Up
-- VEN-06 (evidence & certificates): vendor.certificates was already fully specified in the baseline
-- migrations, unused until now — no schema change needed, only the operational reminder text (not legal
-- wording sent to a data subject, the same reasoning DSAR-08/CON-04/PNG-15/ROPA-15/VEN-10's own templates
-- already use — no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'vendor.certificate_expiry', 'in_app', 'th', 'ใบรับรองคู่ค้าใกล้หมดอายุ', 'ใบรับรอง {{.cert_type}} ของคู่ค้าจะหมดอายุวันที่ {{.valid_to}} กรุณาติดตามขอใบรับรองใหม่', '["cert_type", "valid_to"]'),
    (NULL, 'vendor.certificate_expiry', 'in_app', 'en', 'Vendor certificate expiring soon', 'The {{.cert_type}} certificate expires on {{.valid_to}} — please follow up for a renewed one', '["cert_type", "valid_to"]'),
    (NULL, 'vendor.certificate_expiry', 'email', 'th', 'ใบรับรองคู่ค้าใกล้หมดอายุ', 'ใบรับรอง {{.cert_type}} ของคู่ค้าจะหมดอายุวันที่ {{.valid_to}} กรุณาติดตามขอใบรับรองใหม่', '["cert_type", "valid_to"]'),
    (NULL, 'vendor.certificate_expiry', 'email', 'en', 'Vendor certificate expiring soon ({{.cert_type}})', 'The {{.cert_type}} certificate expires on {{.valid_to}}. Please follow up with the vendor for a renewed certificate.', '["cert_type", "valid_to"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'vendor.certificate_expiry');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'vendor.certificate_expiry';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
