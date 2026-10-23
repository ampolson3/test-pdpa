-- +goose Up
-- DPO-02 (docs/decisions.md Q-37): a global checklist form + assess.templates row, seeded as a DRAFT
-- pending legal review (same move as DPIA-01/02's own Q-26 — ORG-07/ROPA-09/PNG-03/DPX-06 all do the same).
-- The questions paraphrase ม.41(2) and the ประกาศ สคส. เรื่อง หลักเกณฑ์การแต่งตั้งเจ้าหน้าที่คุ้มครองข้อมูล
-- ส่วนบุคคล พ.ศ. 2566 from this feature's own module-doc description, not copied from the official notice.
-- No schema change: assess.templates' own assessment_type CHECK already lists 'dpo_check' (migration 00011),
-- unused until now — the same "the schema already anticipated this" pattern DPX-06/Q-36 found for 'tia'.
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000770001', NULL, 'dpo_requirement_check', 'ประเมินหน้าที่ต้องแต่งตั้ง DPO', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000770002', NULL, '00000000-0000-4000-8000-000000770001', 1, $j${
        "sections": [
            {
                "key": "criteria",
                "title": {"th": "เกณฑ์ตามมาตรา 41(2) และประกาศ สคส. พ.ศ. 2566", "en": "Criteria under section 41(2) and the 2023 PDPC notice"},
                "questions": [
                    {
                        "key": "public_authority",
                        "type": "yes_no",
                        "label": {"th": "องค์กรเป็นหน่วยงานของรัฐตามที่คณะกรรมการประกาศกำหนด", "en": "The organization is a government body designated by the committee"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "large_scale_processing",
                        "type": "yes_no",
                        "label": {"th": "มีการเก็บรวบรวม ใช้ หรือเปิดเผยข้อมูลส่วนบุคคลเป็นจำนวนมาก (ตั้งแต่ 100,000 รายขึ้นไป)", "en": "Collects, uses or discloses personal data at scale (100,000 or more records)"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "regular_monitoring",
                        "type": "yes_no",
                        "label": {"th": "กิจกรรมหลักขององค์กรมีการติดตามพฤติกรรมเจ้าของข้อมูลอย่างสม่ำเสมอหรือเป็นระบบ", "en": "A core activity involves regular, systematic monitoring of data subjects' behavior"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "sensitive_or_criminal_core",
                        "type": "yes_no",
                        "label": {"th": "กิจกรรมหลักขององค์กรเป็นการประมวลผลข้อมูลอ่อนไหวตามมาตรา 26 หรือข้อมูลประวัติอาชญากรรม", "en": "A core activity processes sensitive data (section 26) or criminal record data"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000770002'
WHERE id = '00000000-0000-4000-8000-000000770001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'dpo_check', 'dpo_requirement_check', 'ประเมินหน้าที่ต้องแต่งตั้ง DPO (ร่าง — รอฝ่ายกฎหมายตรวจ)',
    '00000000-0000-4000-8000-000000770001', 1, ARRAY['ม.41(2)', 'ประกาศ สคส. เรื่อง DPO พ.ศ. 2566'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'dpo_check' AND code = 'dpo_requirement_check';
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'dpo_requirement_check';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpo_requirement_check');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpo_requirement_check';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
