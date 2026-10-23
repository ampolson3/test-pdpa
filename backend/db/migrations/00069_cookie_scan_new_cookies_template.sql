-- +goose Up
-- CON-04 (automated cookie scanner): notifies IT/DPO when a scan round finds cookies the tenant's own
-- catalogue didn't have yet. Operational text, not legal wording sent to a data subject, so no DRAFT
-- marker (rule 8) — same reasoning as DSAR-08's own dsar.subtask_assigned template.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'cookie.scan_new_cookies', 'in_app', 'th', 'พบคุกกี้ใหม่จากการสแกน', 'การสแกนคุกกี้ของ {{.domain}} พบคุกกี้ใหม่ {{.new_cookie_count}} รายการที่ยังไม่ถูกจัดหมวดในระบบ', '["domain", "new_cookie_count"]'),
    (NULL, 'cookie.scan_new_cookies', 'in_app', 'en', 'New cookies found by scan', 'The cookie scan of {{.domain}} found {{.new_cookie_count}} new cookie(s) not yet catalogued', '["domain", "new_cookie_count"]'),
    (NULL, 'cookie.scan_new_cookies', 'email', 'th', 'พบคุกกี้ใหม่จากการสแกน {{.domain}}', 'การสแกนคุกกี้อัตโนมัติของ {{.domain}} พบคุกกี้ใหม่ {{.new_cookie_count}} รายการที่ยังไม่ถูกจัดหมวด กรุณาตรวจสอบและจัดหมวดหมู่ในระบบ', '["domain", "new_cookie_count"]'),
    (NULL, 'cookie.scan_new_cookies', 'email', 'en', 'New cookies found scanning {{.domain}}', 'The automated cookie scan of {{.domain}} found {{.new_cookie_count}} new cookie(s) not yet catalogued. Please review and classify them.', '["domain", "new_cookie_count"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'cookie.scan_new_cookies');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'cookie.scan_new_cookies';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
