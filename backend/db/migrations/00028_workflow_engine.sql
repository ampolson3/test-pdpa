-- +goose Up
-- PLT-05 workflow & SLA engine.
-- 1. admin.workflow.{read,create,update}: manage the tenant's workflow definitions (ORGADMIN, SUPER; DPO reads).
-- 2. sla_timers.paused_at: a timer paused while its instance sits in a state marked pause_sla (decisions Q-06
--    default: DSAR pauses only in awaiting_info); resuming pushes due_at back by the paused time.
-- 3. Global in-app templates for task assignment, SLA reminders and breaches (tenants may override, PLT-04).
-- Global rows need RLS force lifted inside this migration, as in 00019 (restored before commit).

INSERT INTO iam.permissions (code, module, resource, action, description, is_sensitive) VALUES
    ('admin.workflow.create', 'admin', 'workflow', 'C', 'นิยาม workflow และ SLA — สร้าง', false),
    ('admin.workflow.read', 'admin', 'workflow', 'R', 'นิยาม workflow และ SLA — ดู', false),
    ('admin.workflow.update', 'admin', 'workflow', 'U', 'นิยาม workflow และ SLA — แก้ไข', false);

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, p.code
FROM iam.roles r
JOIN (VALUES ('ORGADMIN', 'admin.workflow.create'), ('ORGADMIN', 'admin.workflow.read'), ('ORGADMIN', 'admin.workflow.update'),
             ('SUPER', 'admin.workflow.create'), ('SUPER', 'admin.workflow.read'), ('SUPER', 'admin.workflow.update'),
             ('DPO', 'admin.workflow.read')) AS p(role, code) ON p.role = r.code
WHERE r.tenant_id IS NULL;

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

ALTER TABLE platform.sla_timers ADD COLUMN paused_at timestamptz;

-- My-tasks lookups by assignee only cover open work.
CREATE INDEX ix_platform_workflow_tasks_open_user ON platform.workflow_tasks (tenant_id, assignee_user_id) WHERE status IN ('open', 'in_progress');
CREATE INDEX ix_platform_workflow_tasks_open_group ON platform.workflow_tasks (tenant_id, assignee_group_id) WHERE status IN ('open', 'in_progress');

ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'workflow.task_assigned', 'in_app', 'th', 'งานใหม่: {{.title}}', 'คุณได้รับมอบหมายงาน "{{.title}}" ({{.workflow}}) ครบกำหนด {{.due}}', '["title", "workflow", "due"]'),
    (NULL, 'workflow.task_assigned', 'in_app', 'en', 'New task: {{.title}}', 'You were assigned "{{.title}}" ({{.workflow}}), due {{.due}}', '["title", "workflow", "due"]'),
    (NULL, 'workflow.sla_reminder', 'in_app', 'th', 'ใกล้ครบกำหนด: {{.workflow}}', '{{.workflow}} ขั้น "{{.state}}" จะครบกำหนด {{.due}}', '["workflow", "state", "due"]'),
    (NULL, 'workflow.sla_reminder', 'in_app', 'en', 'Due soon: {{.workflow}}', '{{.workflow}} at "{{.state}}" is due {{.due}}', '["workflow", "state", "due"]'),
    (NULL, 'workflow.sla_breached', 'in_app', 'th', 'เกินกำหนด: {{.workflow}}', '{{.workflow}} ขั้น "{{.state}}" เกินกำหนดแล้ว (ครบกำหนด {{.due}})', '["workflow", "state", "due"]'),
    (NULL, 'workflow.sla_breached', 'in_app', 'en', 'Overdue: {{.workflow}}', '{{.workflow}} at "{{.state}}" is overdue (was due {{.due}})', '["workflow", "state", "due"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;

ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (
    SELECT id FROM platform.notification_templates WHERE tenant_id IS NULL AND code LIKE 'workflow.%');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code LIKE 'workflow.%';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS platform.ix_platform_workflow_tasks_open_group;
DROP INDEX IF EXISTS platform.ix_platform_workflow_tasks_open_user;
ALTER TABLE platform.sla_timers DROP COLUMN IF EXISTS paused_at;

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;
DELETE FROM iam.role_permissions WHERE permission_code LIKE 'admin.workflow.%';
ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;
DELETE FROM iam.permissions WHERE code LIKE 'admin.workflow.%';
