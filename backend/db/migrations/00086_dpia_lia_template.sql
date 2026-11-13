-- +goose Up
-- DPIA-16 (ม.24(5), docs/decisions.md Q-41): a global legitimate-interest-assessment (LIA) template — the
-- standard 3-step LIA test (purpose / necessity / balancing against the data subject's own rights), seeded
-- as a DRAFT pending legal review (same move as Q-20/Q-25/Q-26/Q-27/Q-37). No schema change: assess.templates'
-- own assessment_type CHECK already lists 'lia' (migration 00011), and the "assessment" PLT-06 form type is
-- already registered (internal/wiring.Forms).
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

-- Fixed ids, separate statements — see migration 00047's own note on why a combined WITH/CTE chain silently
-- leaves current_version_id NULL.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000860001', NULL, 'dpia_lia', 'ประเมินฐานประโยชน์โดยชอบด้วยกฎหมาย (LIA)', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000860002', NULL, '00000000-0000-4000-8000-000000860001', 1, $j${
        "sections": [
            {
                "key": "purpose",
                "title": {"th": "1. วัตถุประสงค์ (Purpose test)", "en": "1. Purpose test"},
                "questions": [
                    {
                        "key": "purpose_legitimate",
                        "type": "yes_no",
                        "label": {"th": "มีผลประโยชน์ที่ชอบด้วยกฎหมายของผู้ควบคุมข้อมูลหรือบุคคลที่สามที่ระบุได้ชัดเจน (ม.24(5))", "en": "A specific, identifiable legitimate interest of the controller or a third party exists (s.24(5))"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            },
            {
                "key": "necessity",
                "title": {"th": "2. ความจำเป็น (Necessity test)", "en": "2. Necessity test"},
                "questions": [
                    {
                        "key": "necessary_for_purpose",
                        "type": "yes_no",
                        "label": {"th": "การประมวลผลเป็นวิธีที่จำเป็นต่อการบรรลุผลประโยชน์นั้น และไม่มีวิธีอื่นที่กระทบสิทธิน้อยกว่า", "en": "The processing is necessary to achieve that interest, with no less intrusive alternative"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            },
            {
                "key": "balancing",
                "title": {"th": "3. การชั่งน้ำหนัก (Balancing test)", "en": "3. Balancing test"},
                "questions": [
                    {
                        "key": "not_overridden_by_rights",
                        "type": "yes_no",
                        "label": {"th": "ผลประโยชน์ดังกล่าวไม่ถูกลดทอนด้วยสิทธิและเสรีภาพขั้นพื้นฐานของเจ้าของข้อมูล โดยคำนึงถึงความคาดหวังตามสมควรของเจ้าของข้อมูล", "en": "The interest is not overridden by the data subject's own rights and freedoms, considering their reasonable expectations"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "safeguards_in_place",
                        "type": "yes_no",
                        "label": {"th": "มีมาตรการป้องกันเพิ่มเติม (เช่น การลดข้อมูล, pseudonymization) เพื่อลดผลกระทบต่อเจ้าของข้อมูล", "en": "Additional safeguards (e.g. data minimization, pseudonymization) are in place to reduce the impact on the data subject"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000860002'
WHERE id = '00000000-0000-4000-8000-000000860001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'lia', 'dpia_lia', 'ประเมินฐานประโยชน์โดยชอบด้วยกฎหมาย (LIA)', '00000000-0000-4000-8000-000000860001', 1, ARRAY['ม.24(5)'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'lia' AND code = 'dpia_lia';
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'dpia_lia';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_lia');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_lia';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
