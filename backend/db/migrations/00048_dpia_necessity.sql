-- +goose Up
-- DPIA-05 (docs/decisions.md Q-27): a global necessity/proportionality checklist — 4 yes/no questions
-- paraphrasing ม.22 (data minimization), ม.24/26 (appropriate lawful basis) and this feature's own
-- "less invasive alternative" description, seeded as a DRAFT pending legal review (same move as Q-20/Q-25/Q-26).
-- No schema change: platform.form_definitions/form_versions (00002) and assess.templates (00011) already
-- exist; the "assessment" PLT-06 form type is already registered (internal/wiring.Forms).
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

-- Fixed ids, separate statements — see migration 00047's own note on why a combined WITH/CTE chain silently
-- leaves current_version_id NULL.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000480001', NULL, 'dpia_necessity', 'ประเมินความจำเป็นและความได้สัดส่วน', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000480002', NULL, '00000000-0000-4000-8000-000000480001', 1, $j${
        "sections": [
            {
                "key": "necessity",
                "title": {"th": "ความจำเป็นและความได้สัดส่วน (ม.22, ม.24, ม.26)", "en": "Necessity and proportionality (ss.22, 24, 26)"},
                "questions": [
                    {
                        "key": "minimal_data",
                        "type": "yes_no",
                        "label": {"th": "เก็บข้อมูลส่วนบุคคลเท่าที่จำเป็นตามวัตถุประสงค์เท่านั้น (ม.22)", "en": "Collects only the personal data necessary for the stated purpose (s.22)"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "purpose_specific",
                        "type": "yes_no",
                        "label": {"th": "วัตถุประสงค์ของกิจกรรมชัดเจนและเจาะจง ไม่กว้างเกินความจำเป็น", "en": "The activity's purpose is specific and not broader than necessary"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "lawful_basis_appropriate",
                        "type": "yes_no",
                        "label": {"th": "ฐานทางกฎหมายที่เลือกใช้ (ม.24/26) เหมาะสมกับลักษณะข้อมูลและกิจกรรม", "en": "The chosen lawful basis (ss.24/26) fits the data and the activity"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "less_invasive_considered",
                        "type": "yes_no",
                        "label": {"th": "ได้พิจารณาทางเลือกที่กระทบสิทธิของเจ้าของข้อมูลน้อยกว่าแล้วก่อนเลือกวิธีนี้", "en": "A less invasive alternative was considered before choosing this approach"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000480002'
WHERE id = '00000000-0000-4000-8000-000000480001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'dpia', 'dpia_necessity', 'ประเมินความจำเป็นและความได้สัดส่วน', '00000000-0000-4000-8000-000000480001', 1, ARRAY['ม.22', 'ม.24', 'ม.26'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'dpia' AND code = 'dpia_necessity';
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'dpia_necessity';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_necessity');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_necessity';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
