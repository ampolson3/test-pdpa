-- +goose Up
-- PLT-07: platform (global, tenant_id NULL) in-app templates for comment notifications. Tenants can override
-- them with their own template of the same code, channel and language (PLT-04). Global rows need RLS force
-- lifted inside this migration, as in 00019 (restored before commit).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'collab.mention', 'in_app', 'th', '{{.author}} กล่าวถึงคุณ', '{{.author}} กล่าวถึงคุณในความเห็น: {{.excerpt}}', '["author", "excerpt"]'),
    (NULL, 'collab.mention', 'in_app', 'en', '{{.author}} mentioned you', '{{.author}} mentioned you in a comment: {{.excerpt}}', '["author", "excerpt"]'),
    (NULL, 'collab.reply', 'in_app', 'th', '{{.author}} ตอบความเห็นของคุณ', '{{.author}} ตอบว่า: {{.excerpt}}', '["author", "excerpt"]'),
    (NULL, 'collab.reply', 'in_app', 'en', '{{.author}} replied to your comment', '{{.author}} replied: {{.excerpt}}', '["author", "excerpt"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;

ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (
    SELECT id FROM platform.notification_templates WHERE tenant_id IS NULL AND code IN ('collab.mention', 'collab.reply'));
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code IN ('collab.mention', 'collab.reply');
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
