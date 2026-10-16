-- +goose Up
-- PNG-07 (ม.21): a notice publish carries two flags the DPO sets right before publishing — is this a material
-- change, and does it change a purpose? Staged on notice.notices itself (not a new input on PLT-08's generic
-- publish endpoint, which knows nothing about notice-specific fields) and consumed/reset by
-- notice.Service.OnDocumentPublished in the same transaction that writes the notice_versions row.
ALTER TABLE notice.notices ADD COLUMN pending_is_material_change boolean NOT NULL DEFAULT false;
ALTER TABLE notice.notices ADD COLUMN pending_changes_purpose boolean NOT NULL DEFAULT false;

-- A purpose-changing publish opens a dpo.tasks job ("สร้างงานขอความยินยอมใหม่อัตโนมัติ") so a human goes and
-- authors the actual re-consent purpose text in CON (rule 8: legal wording is never auto-generated) — the same
-- "widen the CHECK for a new, clearly-fitting source" move DPO-09 made for platform.form_definitions.form_type.
ALTER TABLE dpo.tasks DROP CONSTRAINT tasks_source_type_check;
ALTER TABLE dpo.tasks ADD CONSTRAINT tasks_source_type_check
    CHECK (source_type IN ('ropa_gap', 'dpia', 'audit', 'breach', 'risk', 'vendor', 'agreement', 'manual', 'consent'));

-- Operational alert to role DPO when a published notice is a material change (decisions.md Q-29: no generic,
-- addressable "data subject" audience exists yet — no portal account model, no acknowledgement list (PNG-09,
-- not built) — so, like BRE-07/PNG-04 before their own real routing existed, this reaches the responsible human
-- rather than guessing a data-subject delivery channel).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'notice.material_change', 'in_app', 'th', 'ประกาศเปลี่ยนแปลงสาระสำคัญ', 'ประกาศ "{{.notice_title}}" เผยแพร่ฉบับใหม่ที่เปลี่ยนสาระสำคัญ (เวอร์ชัน {{.version_no}}) — โปรดพิจารณาแจ้งเจ้าของข้อมูลที่เกี่ยวข้อง', '["notice_title", "version_no"]'),
    (NULL, 'notice.material_change', 'in_app', 'en', 'Notice changed materially', 'Notice "{{.notice_title}}" published a new version with a material change (v{{.version_no}}) — consider notifying affected data subjects', '["notice_title", "version_no"]'),
    (NULL, 'notice.material_change', 'email', 'th', 'ประกาศเปลี่ยนแปลงสาระสำคัญ', 'ประกาศ "{{.notice_title}}" เผยแพร่ฉบับใหม่ที่เปลี่ยนสาระสำคัญ (เวอร์ชัน {{.version_no}}) โปรดพิจารณาแจ้งเจ้าของข้อมูลที่เกี่ยวข้องตาม ม.21', '["notice_title", "version_no"]'),
    (NULL, 'notice.material_change', 'email', 'en', 'Notice changed materially', 'Notice "{{.notice_title}}" published a new version with a material change (v{{.version_no}}). Please consider notifying affected data subjects (ม.21).', '["notice_title", "version_no"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'notice.material_change');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'notice.material_change';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

ALTER TABLE dpo.tasks DROP CONSTRAINT tasks_source_type_check;
ALTER TABLE dpo.tasks ADD CONSTRAINT tasks_source_type_check
    CHECK (source_type IN ('ropa_gap', 'dpia', 'audit', 'breach', 'risk', 'vendor', 'agreement', 'manual'));

ALTER TABLE notice.notices DROP COLUMN pending_changes_purpose;
ALTER TABLE notice.notices DROP COLUMN pending_is_material_change;
