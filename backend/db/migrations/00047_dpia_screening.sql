-- +goose Up
-- DPIA-01/02 (docs/decisions.md Q-26): a global screening form + assess.templates row, seeded as a DRAFT
-- pending legal review (same move as ORG-07 Q-20, ROPA-09 Q-25, PNG-03 Q-14) — the TDPG 4.0-P screening
-- questions are paraphrased from this feature's own module-doc description, not copied from an official
-- source. No schema change: platform.form_definitions/form_versions (00002) and assess.templates (00011)
-- already exist; the "assessment" PLT-06 form type is already registered (internal/wiring.Forms), unused
-- until now.
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

-- Fixed ids, not gen_random_uuid()/CTE-chained: a data-modifying WITH statement's primary UPDATE scans the
-- real table under the query's own snapshot, which does not see a row a sibling CTE inserted in the same
-- statement (confirmed by hand — the natural WITH def AS (INSERT ...), ver AS (INSERT ... RETURNING) UPDATE
-- ... FROM ver form left current_version_id NULL every time, 0 rows affected, no error). Separate statements
-- avoid it entirely.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000470001', NULL, 'dpia_screening', 'แบบคัดกรองความจำเป็นในการทำ DPIA', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000470002', NULL, '00000000-0000-4000-8000-000000470001', 1, $j${
        "sections": [
            {
                "key": "risk_factors",
                "title": {"th": "ปัจจัยเสี่ยงสูงตามแนวทาง TDPG 4.0-P", "en": "High-risk factors (TDPG 4.0-P)"},
                "questions": [
                    {
                        "key": "sensitive_data",
                        "type": "yes_no",
                        "label": {"th": "ประมวลผลข้อมูลส่วนบุคคลอ่อนไหวตามมาตรา 26 ในปริมาณมาก", "en": "Processes sensitive personal data (section 26) at scale"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "large_scale",
                        "type": "yes_no",
                        "label": {"th": "ประมวลผลข้อมูลส่วนบุคคลในปริมาณมาก (large-scale)", "en": "Processes personal data at large scale"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "monitoring",
                        "type": "yes_no",
                        "label": {"th": "มีการติดตามพฤติกรรมหรือสังเกตการณ์เจ้าของข้อมูลอย่างเป็นระบบ", "en": "Involves systematic monitoring or observation of data subjects"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "automated_decision",
                        "type": "yes_no",
                        "label": {"th": "มีการตัดสินใจอัตโนมัติที่มีผลกระทบอย่างมีนัยสำคัญต่อเจ้าของข้อมูล", "en": "Involves automated decision-making with significant effects on data subjects"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "new_tech",
                        "type": "yes_no",
                        "label": {"th": "ใช้เทคโนโลยีใหม่หรือ AI ที่ยังไม่มีการประเมินความเสี่ยงมาก่อน", "en": "Uses new technology or AI not previously risk-assessed"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    },
                    {
                        "key": "vulnerable_groups",
                        "type": "yes_no",
                        "label": {"th": "เกี่ยวข้องกับกลุ่มเปราะบาง เช่น เด็กหรือลูกจ้าง", "en": "Involves vulnerable groups, e.g. children or employees"},
                        "required": true,
                        "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]
                    }
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000470002'
WHERE id = '00000000-0000-4000-8000-000000470001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'dpia', 'dpia_screening', 'แบบคัดกรองความจำเป็นในการทำ DPIA (TDPG 4.0-P)', '00000000-0000-4000-8000-000000470001', 1, ARRAY['ม.37(1)', 'TDPG 4.0-P'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'dpia' AND code = 'dpia_screening';
UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NULL AND code = 'dpia_screening';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_screening');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'dpia_screening';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
