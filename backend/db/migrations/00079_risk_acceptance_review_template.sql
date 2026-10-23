-- +goose Up
-- RRA-08 (ยอมรับความเสี่ยงพร้อมผู้อนุมัติ): risk.acceptances was already fully specified in the baseline
-- migrations, unused until now — no schema change needed, only the operational reminder text fired exactly
-- at an acceptance's own expires_at (not legal wording sent to a data subject, the same reasoning DSAR-08/
-- CON-04/PNG-15/ROPA-15/VEN-10/VEN-06's own templates already use — no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'risk.acceptance_review', 'in_app', 'th', 'การยอมรับความเสี่ยงหมดอายุแล้ว', 'การยอมรับความเสี่ยง "{{.risk_title}}" หมดอายุแล้วเมื่อ {{.expires_at}} กรุณาทบทวนและพิจารณาความเสี่ยงอีกครั้ง', '["risk_title", "expires_at"]'),
    (NULL, 'risk.acceptance_review', 'in_app', 'en', 'Risk acceptance has expired', 'The acceptance of risk "{{.risk_title}}" expired on {{.expires_at}} — please review it again', '["risk_title", "expires_at"]'),
    (NULL, 'risk.acceptance_review', 'email', 'th', 'การยอมรับความเสี่ยงหมดอายุแล้ว', 'การยอมรับความเสี่ยง "{{.risk_title}}" หมดอายุแล้วเมื่อ {{.expires_at}} กรุณาทบทวนและพิจารณาความเสี่ยงอีกครั้ง', '["risk_title", "expires_at"]'),
    (NULL, 'risk.acceptance_review', 'email', 'en', 'Risk acceptance has expired ({{.risk_title}})', 'The acceptance of risk "{{.risk_title}}" expired on {{.expires_at}}. Please review this risk again.', '["risk_title", "expires_at"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'risk.acceptance_review');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'risk.acceptance_review';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
