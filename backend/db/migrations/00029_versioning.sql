-- +goose Up
-- PLT-08 versioning & approval: one open version (draft / in_review / approved) and one published version
-- per record at a time; pending approvals looked up by role for the approval inbox; the in-app templates
-- telling the requester about a decision (tenants may override, PLT-04). Global template rows need RLS
-- force lifted inside this migration, as in 00019 (restored before commit).
CREATE UNIQUE INDEX ux_platform_record_versions_open ON platform.record_versions (tenant_id, entity_type, entity_id)
    WHERE status IN ('draft', 'in_review', 'approved');
CREATE UNIQUE INDEX ux_platform_record_versions_published ON platform.record_versions (tenant_id, entity_type, entity_id)
    WHERE status = 'published';
CREATE INDEX ix_platform_approvals_pending_role ON platform.approvals (tenant_id, approver_role) WHERE decision = 'pending';

ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'approval.approved', 'in_app', 'th', 'อนุมัติแล้ว: {{.title}}', '{{.title}} เวอร์ชัน {{.version}} ได้รับอนุมัติครบทุกขั้นแล้ว (ล่าสุดโดย {{.approver}})', '["title", "version", "approver"]'),
    (NULL, 'approval.approved', 'in_app', 'en', 'Approved: {{.title}}', '{{.title}} version {{.version}} passed every approval step (last by {{.approver}})', '["title", "version", "approver"]'),
    (NULL, 'approval.returned', 'in_app', 'th', 'ส่งกลับแก้ไข: {{.title}}', '{{.approver}} ส่ง {{.title}} เวอร์ชัน {{.version}} กลับให้แก้ไข: {{.reason}}', '["title", "version", "approver", "reason"]'),
    (NULL, 'approval.returned', 'in_app', 'en', 'Returned: {{.title}}', '{{.approver}} returned {{.title}} version {{.version}} for changes: {{.reason}}', '["title", "version", "approver", "reason"]'),
    (NULL, 'approval.rejected', 'in_app', 'th', 'ไม่อนุมัติ: {{.title}}', '{{.approver}} ไม่อนุมัติ {{.title}} เวอร์ชัน {{.version}}: {{.reason}}', '["title", "version", "approver", "reason"]'),
    (NULL, 'approval.rejected', 'in_app', 'en', 'Rejected: {{.title}}', '{{.approver}} rejected {{.title}} version {{.version}}: {{.reason}}', '["title", "version", "approver", "reason"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (
    SELECT id FROM platform.notification_templates WHERE tenant_id IS NULL AND code LIKE 'approval.%');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code LIKE 'approval.%';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS platform.ix_platform_approvals_pending_role;
DROP INDEX IF EXISTS platform.ux_platform_record_versions_published;
DROP INDEX IF EXISTS platform.ux_platform_record_versions_open;
