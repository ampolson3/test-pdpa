-- +goose Up
-- DPX-03 (ม.37(1), docs/decisions.md Q-45): a global organization-wide PDPA readiness/maturity
-- questionnaire — 5 dimensions (governance & DPO, RoPA/data inventory, consent & notices, data subject
-- rights, breach & security) × 2 yes_no questions each, seeded as a DRAFT pending legal review (same move
-- as Q-20/Q-25/Q-26/Q-27/Q-37/Q-41/Q-44). No schema change: assess.templates' own assessment_type CHECK
-- already lists 'maturity' (migration 00011), and the "assessment" PLT-06 form type is already registered
-- (internal/wiring.Forms).
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

-- Fixed ids, separate statements — see migration 00047's own note on why a combined WITH/CTE chain silently
-- leaves current_version_id NULL.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000930001', NULL, 'org_maturity', 'ประเมินความพร้อมองค์กรด้าน PDPA', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000930002', NULL, '00000000-0000-4000-8000-000000930001', 1, $j${
        "sections": [
            {
                "key": "governance",
                "title": {"th": "1. การกำกับดูแลและ DPO", "en": "1. Governance & DPO"},
                "questions": [
                    {"key": "gov_dpo_appointed", "type": "yes_no", "label": {"th": "แต่งตั้ง DPO แล้วและมีช่องทางติดต่อที่เผยแพร่", "en": "A DPO is appointed with a published contact channel"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "gov_policy_published", "type": "yes_no", "label": {"th": "มีนโยบายคุ้มครองข้อมูลส่วนบุคคลภายในที่พนักงานรับทราบ", "en": "An internal data protection policy exists and employees have attested to it"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            },
            {
                "key": "ropa",
                "title": {"th": "2. RoPA และบัญชีข้อมูล", "en": "2. RoPA & data inventory"},
                "questions": [
                    {"key": "ropa_maintained", "type": "yes_no", "label": {"th": "มีบันทึกรายการกิจกรรมประมวลผลข้อมูลส่วนบุคคล (RoPA) ที่เป็นปัจจุบัน", "en": "A current record of processing activities (RoPA) is maintained"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "ropa_reviewed_annually", "type": "yes_no", "label": {"th": "ทบทวน RoPA อย่างน้อยปีละครั้งหรือเมื่อกิจกรรมเปลี่ยน", "en": "RoPA is reviewed at least annually or when activities change"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            },
            {
                "key": "consent",
                "title": {"th": "3. ความยินยอมและประกาศ", "en": "3. Consent & notices"},
                "questions": [
                    {"key": "notices_published", "type": "yes_no", "label": {"th": "เผยแพร่ประกาศความเป็นส่วนตัวครบตาม ม.23 สำหรับทุกช่องทางเก็บข้อมูล", "en": "Privacy notices covering every ม.23 topic are published for every collection channel"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "consent_records_kept", "type": "yes_no", "label": {"th": "เก็บหลักฐานความยินยอมและรองรับการถอนความยินยอมได้", "en": "Consent evidence is retained and withdrawal is supported"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            },
            {
                "key": "dsar",
                "title": {"th": "4. สิทธิของเจ้าของข้อมูล", "en": "4. Data subject rights"},
                "questions": [
                    {"key": "dsar_channel_exists", "type": "yes_no", "label": {"th": "มีช่องทางรับคำขอใช้สิทธิที่ชัดเจนและมีการยืนยันตัวตนผู้ยื่นคำขอ", "en": "A clear DSAR intake channel exists with requester identity verification"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "dsar_sla_tracked", "type": "yes_no", "label": {"th": "ติดตามกำหนดเวลาตอบคำขอภายใน 30 วันอย่างเป็นระบบ", "en": "The 30-day DSAR deadline is tracked systematically"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            },
            {
                "key": "breach_security",
                "title": {"th": "5. การแจ้งเหตุละเมิดและความปลอดภัย", "en": "5. Breach notification & security"},
                "questions": [
                    {"key": "breach_plan_exists", "type": "yes_no", "label": {"th": "มีแผนตอบสนองต่อเหตุละเมิดและกำหนดเวลาแจ้ง สคส. ภายใน 72 ชั่วโมง", "en": "A breach response plan exists with a 72-hour PDPC notification clock"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "security_measures_reviewed", "type": "yes_no", "label": {"th": "ทบทวนมาตรการความปลอดภัยตามประกาศมาตรการความปลอดภัยอย่างน้อยปีละครั้ง", "en": "Security measures are reviewed against the security-measures notice at least annually"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000930002'
WHERE id = '00000000-0000-4000-8000-000000930001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'maturity', 'org_maturity', 'ประเมินความพร้อมองค์กรด้าน PDPA', '00000000-0000-4000-8000-000000930001', 1, ARRAY['ม.37(1)'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'maturity' AND code = 'org_maturity';
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'org_maturity';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'org_maturity');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'org_maturity';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
