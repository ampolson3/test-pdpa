-- +goose Up
-- ROPA-20 (ประกาศ สคส. ยกเว้นกิจการขนาดเล็ก พ.ศ. 2565 และ พ.ศ. 2567, docs/decisions.md Q-44): a global SME
-- exemption screening form — ropa.sme_exemption_checks was already fully specified in the baseline
-- migrations (migration 00008, result/basis/legal_entity_id/form_submission_id, FK'd in 00018), unused
-- until now. Widens platform.form_definitions.form_type to add 'sme_exemption' (the same "widen the CHECK
-- for a new, clearly-fitting source" move DPO-09/RRA-05 already made) and seeds a DRAFT-pending-legal-review
-- screening form (same move as Q-20/Q-25/Q-26/Q-27/Q-37/Q-41): a controller-exemption section (2565) and a
-- processor-exemption section (2567), each yes_no — every "yes" in a section means that section's own
-- exemption applies.
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security', 'checklist', 'sme_exemption'));

ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;

-- Fixed ids, separate statements — see migration 00047's own note on why a combined WITH/CTE chain silently
-- leaves current_version_id NULL.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000900001', NULL, 'sme_exemption', 'ตรวจสิทธิ์ยกเว้นกิจการขนาดเล็ก (RoPA)', 'sme_exemption', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000900002', NULL, '00000000-0000-4000-8000-000000900001', 1, $j${
        "sections": [
            {
                "key": "controller",
                "title": {"th": "1. การยกเว้นสำหรับผู้ควบคุมข้อมูลขนาดเล็ก (ประกาศ พ.ศ. 2565)", "en": "1. Small-business controller exemption (2022 notice)"},
                "questions": [
                    {
                        "key": "is_sme_or_community",
                        "type": "yes_no",
                        "label": {"th": "เป็นวิสาหกิจขนาดกลางและขนาดย่อม (SME) หรือวิสาหกิจชุมชนตามที่กฎหมายกำหนด", "en": "Qualifies as an SME or community enterprise as legally defined"},
                        "required": true
                    },
                    {
                        "key": "not_large_scale",
                        "type": "yes_no",
                        "label": {"th": "ไม่มีการประมวลผลข้อมูลส่วนบุคคลในปริมาณมากอย่างเป็นระบบ (เช่น ฐานข้อมูลลูกค้า/ลูกจ้างขนาดใหญ่)", "en": "No systematic, large-scale processing of personal data (e.g. a large customer/employee database)"},
                        "required": true
                    },
                    {
                        "key": "no_sensitive_data",
                        "type": "yes_no",
                        "label": {"th": "ไม่มีการประมวลผลข้อมูลส่วนบุคคลที่มีความละเอียดอ่อนตาม ม.26 อย่างมีนัยสำคัญ", "en": "No significant processing of sensitive personal data under s.26"},
                        "required": true
                    },
                    {
                        "key": "no_high_risk_profiling",
                        "type": "yes_no",
                        "label": {"th": "ไม่มีการใช้ข้อมูลเพื่อการวิเคราะห์พฤติกรรม การทำโปรไฟล์ หรือการตัดสินใจอัตโนมัติที่มีความเสี่ยงสูง", "en": "No profiling, behavioral analysis or high-risk automated decision-making"},
                        "required": true
                    }
                ]
            },
            {
                "key": "processor",
                "title": {"th": "2. การยกเว้นสำหรับผู้ประมวลผลข้อมูลขนาดเล็ก (ประกาศ พ.ศ. 2567)", "en": "2. Small-business processor exemption (2024 notice)"},
                "questions": [
                    {
                        "key": "acts_only_as_processor",
                        "type": "yes_no",
                        "label": {"th": "ดำเนินการเป็นเพียงผู้ประมวลผลข้อมูลส่วนบุคคลตามคำสั่งของผู้ควบคุมข้อมูลเท่านั้น ไม่ได้เป็นผู้ควบคุมข้อมูลเอง", "en": "Acts only as a processor under a controller's instructions, not as a controller itself"},
                        "required": true
                    },
                    {
                        "key": "no_subcontracted_high_risk",
                        "type": "yes_no",
                        "label": {"th": "ไม่มีการมอบหมายงานประมวลผลที่มีความเสี่ยงสูงให้ผู้ประมวลผลข้อมูลช่วง", "en": "No high-risk processing subcontracted to a sub-processor"},
                        "required": true
                    }
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000900002'
WHERE id = '00000000-0000-4000-8000-000000900001';

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'sme_exemption';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'sme_exemption');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'sme_exemption';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security', 'checklist'));
