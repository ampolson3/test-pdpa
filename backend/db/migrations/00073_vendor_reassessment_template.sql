-- +goose Up
-- VEN-10 (periodic re-assessment, ม.37(1)): vendor.vendors.tier/next_assessment_at were already set by
-- VEN-02's own RecordIntake — no schema change needed, only the operational reminder text (not legal
-- wording sent to a data subject, same reasoning DSAR-08/CON-04/PNG-15/ROPA-15's own templates already
-- use — no DRAFT marker, rule 8).
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'vendor.reassessment_due', 'in_app', 'th', 'ถึงรอบประเมินคู่ค้าซ้ำ', 'คู่ค้าระดับความเสี่ยง {{.tier}} ถึงรอบประเมินซ้ำตามกำหนด กรุณาส่งแบบประเมินให้คู่ค้าตอบใหม่', '["tier"]'),
    (NULL, 'vendor.reassessment_due', 'in_app', 'en', 'Vendor re-assessment due', 'A {{.tier}}-tier vendor is due for its periodic re-assessment — please send the assessment again', '["tier"]'),
    (NULL, 'vendor.reassessment_due', 'email', 'th', 'ถึงรอบประเมินคู่ค้าซ้ำ', 'คู่ค้าระดับความเสี่ยง {{.tier}} ถึงรอบประเมินซ้ำตามกำหนด กรุณาส่งแบบประเมินให้คู่ค้าตอบใหม่', '["tier"]'),
    (NULL, 'vendor.reassessment_due', 'email', 'en', 'Vendor re-assessment due ({{.tier}} tier)', 'A {{.tier}}-tier vendor is due for its periodic re-assessment. Please send the assessment again.', '["tier"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'vendor.reassessment_due');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'vendor.reassessment_due';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
