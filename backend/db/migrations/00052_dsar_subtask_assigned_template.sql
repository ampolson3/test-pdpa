-- +goose Up
-- DSAR-08 (workflow & subtasks): notifies a subtask's assignee when they are given one. Operational text, not
-- legal wording sent to a data subject, so no DRAFT marker (rule 8) — same reasoning as DSAR-07's own
-- dsar.sla_reminder template.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'dsar.subtask_assigned', 'in_app', 'th', 'ได้รับมอบหมายงานย่อย', 'คุณได้รับมอบหมายงานย่อย "{{.action}}" ของคำขอใช้สิทธิ {{.request_no}}', '["request_no", "action"]'),
    (NULL, 'dsar.subtask_assigned', 'in_app', 'en', 'Subtask assigned', 'You were assigned the "{{.action}}" subtask of DSAR request {{.request_no}}', '["request_no", "action"]'),
    (NULL, 'dsar.subtask_assigned', 'email', 'th', 'ได้รับมอบหมายงานย่อยของคำขอใช้สิทธิ {{.request_no}}', 'คุณได้รับมอบหมายงานย่อย "{{.action}}" ของคำขอใช้สิทธิเลขที่ {{.request_no}} กรุณาดำเนินการและปิดงานเมื่อเสร็จสิ้น', '["request_no", "action"]'),
    (NULL, 'dsar.subtask_assigned', 'email', 'en', 'Subtask assigned on DSAR request {{.request_no}}', 'You were assigned the "{{.action}}" subtask of DSAR request {{.request_no}}. Please complete it and mark it done.', '["request_no", "action"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'dsar.subtask_assigned');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'dsar.subtask_assigned';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
