-- +goose Up
-- BRE-02 / 07 / 10 / 12: what the breach register needs beyond the SA tables.
-- * incidents.owner_user_id: the person responsible for an incident (BRE-02 "every incident has a status and a clear
--   owner"); defaults to whoever records it. close_reason: why it was closed (not a breach, or lessons learned).
-- * subject_notifications.variables: the incident-specific text of a batch (summary, remedy, contact) filled into the
--   tenant's notification template; created_by already records the maker, approved_by / approved_at the checker.
-- * notification_recipients.notification_id / language / line_no: the platform.notifications message each recipient
--   was handed to (per-person delivery status, BRE-10), the recipient's language, and the CSV line it came from.
ALTER TABLE breach.incidents ADD COLUMN owner_user_id uuid;
ALTER TABLE breach.incidents ADD CONSTRAINT fk_incidents_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES iam.users (id);
ALTER TABLE breach.incidents ADD COLUMN close_reason text;
CREATE INDEX ix_breach_incidents_owner_user_id ON breach.incidents (tenant_id, owner_user_id);
CREATE INDEX ix_breach_incidents_status ON breach.incidents (tenant_id, status, aware_at DESC);
CREATE INDEX ix_breach_timeline_events_incident_time ON breach.timeline_events (tenant_id, incident_id, occurred_at);

ALTER TABLE breach.subject_notifications ADD COLUMN variables jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE breach.subject_notifications ADD COLUMN template_code varchar(80) NOT NULL DEFAULT 'breach.subject_notice';
ALTER TABLE breach.subject_notifications ADD COLUMN approved_by uuid;
ALTER TABLE breach.subject_notifications ADD CONSTRAINT fk_subject_notifications_approved_by FOREIGN KEY (approved_by) REFERENCES iam.users (id);
ALTER TABLE breach.subject_notifications ADD COLUMN approved_at timestamptz;

ALTER TABLE breach.notification_recipients ADD COLUMN notification_id uuid;
ALTER TABLE breach.notification_recipients ADD COLUMN language varchar(5) NOT NULL DEFAULT 'th';
ALTER TABLE breach.notification_recipients ADD COLUMN line_no int;
CREATE INDEX ix_breach_notification_recipients_queue ON breach.notification_recipients (tenant_id, subject_notification_id, status, line_no);

-- Platform templates (tenant_id NULL). Staff alerts are operational; the data-subject notice is legal wording, so it
-- is seeded as a marked DRAFT (CLAUDE.md rule 8, decisions Q-14) for the tenant's Legal/DPO to replace through the
-- notification-template editor before any real use.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'breach.reported', 'in_app', 'th', 'เหตุละเมิดใหม่ {{.incident_no}}', '{{.title}} · ต้องแจ้ง สคส. ภายใน {{.deadline}}', '["incident_no", "title", "deadline"]'),
    (NULL, 'breach.reported', 'in_app', 'en', 'New breach {{.incident_no}}', '{{.title}} · PDPC notice due by {{.deadline}}', '["incident_no", "title", "deadline"]'),
    (NULL, 'breach.reported', 'email', 'th', 'เหตุละเมิดใหม่ {{.incident_no}}', 'มีการบันทึกเหตุละเมิด {{.incident_no}}: {{.title}}' || chr(10) || 'ต้องแจ้ง สคส. ภายใน {{.deadline}} (72 ชั่วโมงนับแต่ทราบเหตุ)', '["incident_no", "title", "deadline"]'),
    (NULL, 'breach.reported', 'email', 'en', 'New breach {{.incident_no}}', 'Breach {{.incident_no}} was recorded: {{.title}}' || chr(10) || 'The PDPC notice is due by {{.deadline}} (72 hours from awareness).', '["incident_no", "title", "deadline"]'),
    (NULL, 'breach.deadline_reminder', 'in_app', 'th', '{{.incident_no}}: ผ่านไป {{.hours}} ชั่วโมง', 'เหลือเวลาแจ้ง สคส. ถึง {{.deadline}}', '["incident_no", "hours", "deadline"]'),
    (NULL, 'breach.deadline_reminder', 'in_app', 'en', '{{.incident_no}}: {{.hours}} hours elapsed', 'The PDPC notice is due by {{.deadline}}', '["incident_no", "hours", "deadline"]'),
    (NULL, 'breach.deadline_reminder', 'email', 'th', '{{.incident_no}}: ผ่านไป {{.hours}} ชั่วโมงนับแต่ทราบเหตุ', 'เหตุละเมิด {{.incident_no}} ยังไม่ได้แจ้ง สคส. กำหนดแจ้งคือ {{.deadline}}', '["incident_no", "hours", "deadline"]'),
    (NULL, 'breach.deadline_reminder', 'email', 'en', '{{.incident_no}}: {{.hours}} hours since awareness', 'Breach {{.incident_no}} has not been notified to the PDPC yet. The notice is due by {{.deadline}}.', '["incident_no", "hours", "deadline"]'),
    (NULL, 'breach.deadline_overdue', 'in_app', 'th', '{{.incident_no}} เกิน 72 ชั่วโมงแล้ว', 'ยังไม่ได้แจ้ง สคส. ต้องบันทึกเหตุผลความล่าช้า', '["incident_no"]'),
    (NULL, 'breach.deadline_overdue', 'in_app', 'en', '{{.incident_no}} is past 72 hours', 'Not yet notified to the PDPC: a reason for the delay is required.', '["incident_no"]'),
    (NULL, 'breach.deadline_overdue', 'email', 'th', '{{.incident_no}} เกิน 72 ชั่วโมงแล้ว', 'เหตุละเมิด {{.incident_no}} ยังไม่ได้แจ้ง สคส. ภายใน 72 ชั่วโมง ต้องบันทึกเหตุผลความล่าช้า', '["incident_no"]'),
    (NULL, 'breach.deadline_overdue', 'email', 'en', '{{.incident_no}} is past 72 hours', 'Breach {{.incident_no}} was not notified to the PDPC within 72 hours. A reason for the delay is required.', '["incident_no"]'),
    (NULL, 'breach.subject_notice', 'email', 'th', '[ร่าง — รอฝ่ายกฎหมายอนุมัติ] แจ้งเหตุละเมิดข้อมูลส่วนบุคคลจาก {{.organization}}', '[ร่าง — ข้อความนี้ต้องได้รับการอนุมัติจากฝ่ายกฎหมาย/DPO ก่อนใช้งานจริง]' || chr(10) || chr(10) || '{{.summary}}' || chr(10) || chr(10) || 'แนวทางเยียวยา: {{.remedy}}' || chr(10) || 'ติดต่อ: {{.contact}}', '["organization", "summary", "remedy", "contact"]'),
    (NULL, 'breach.subject_notice', 'email', 'en', '[DRAFT — pending legal approval] Personal data breach notice from {{.organization}}', '[DRAFT — this text must be approved by Legal/DPO before real use]' || chr(10) || chr(10) || '{{.summary}}' || chr(10) || chr(10) || 'Remedy: {{.remedy}}' || chr(10) || 'Contact: {{.contact}}', '["organization", "summary", "remedy", "contact"]'),
    (NULL, 'breach.subject_notice', 'sms', 'th', NULL, '[ร่าง] {{.organization}}: {{.summary}} ติดต่อ {{.contact}}', '["organization", "summary", "remedy", "contact"]'),
    (NULL, 'breach.subject_notice', 'sms', 'en', NULL, '[DRAFT] {{.organization}}: {{.summary}} Contact {{.contact}}', '["organization", "summary", "remedy", "contact"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('breach.reported', 'breach.deadline_reminder', 'breach.deadline_overdue', 'breach.subject_notice'));
DELETE FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code IN ('breach.reported', 'breach.deadline_reminder', 'breach.deadline_overdue', 'breach.subject_notice');
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS breach.ix_breach_notification_recipients_queue;
ALTER TABLE breach.notification_recipients DROP COLUMN IF EXISTS line_no;
ALTER TABLE breach.notification_recipients DROP COLUMN IF EXISTS language;
ALTER TABLE breach.notification_recipients DROP COLUMN IF EXISTS notification_id;
ALTER TABLE breach.subject_notifications DROP COLUMN IF EXISTS approved_at;
ALTER TABLE breach.subject_notifications DROP CONSTRAINT IF EXISTS fk_subject_notifications_approved_by;
ALTER TABLE breach.subject_notifications DROP COLUMN IF EXISTS approved_by;
ALTER TABLE breach.subject_notifications DROP COLUMN IF EXISTS template_code;
ALTER TABLE breach.subject_notifications DROP COLUMN IF EXISTS variables;
DROP INDEX IF EXISTS breach.ix_breach_timeline_events_incident_time;
DROP INDEX IF EXISTS breach.ix_breach_incidents_status;
DROP INDEX IF EXISTS breach.ix_breach_incidents_owner_user_id;
ALTER TABLE breach.incidents DROP COLUMN IF EXISTS close_reason;
ALTER TABLE breach.incidents DROP CONSTRAINT IF EXISTS fk_incidents_owner_user_id;
ALTER TABLE breach.incidents DROP COLUMN IF EXISTS owner_user_id;
