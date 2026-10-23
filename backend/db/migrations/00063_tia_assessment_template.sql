-- +goose Up
-- DPX-06 (docs/decisions.md Q-36): a ready-to-use transfer impact assessment (TIA) questionnaire for
-- cross-border transfers (ม.28-29; ประกาศ สคส. ตามมาตรา 28 และ 29 พ.ศ. 2566). No schema change:
-- assess.templates' own assessment_type CHECK constraint already includes 'tia' (migration 00011), and
-- ropa.activity_transfers.tia_assessment_id -> assess.assessments already has its FK (migration 00018) —
-- both sitting unused until now, the exact same "reserved but unused" pattern VEN-04 (Q-31) found for
-- assessment_type = 'vendor'. The generic template library (DPIA-03's CRUD on assess.templates) needs no
-- new code to serve this either.
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000630001', NULL, 'transfer_tia', 'แบบประเมินผลกระทบการโอนข้อมูลไปต่างประเทศ (TIA)', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000630002', NULL, '00000000-0000-4000-8000-000000630001', 1, $j${
        "sections": [
            {
                "key": "transfer_impact",
                "title": {"th": "การประเมินผลกระทบการโอนข้อมูลไปต่างประเทศ (ม.28-29)", "en": "Cross-border transfer impact assessment (ม.28-29)"},
                "questions": [
                    {"key": "host_country_adequate", "type": "yes_no", "required": true,
                     "label": {"th": "ประเทศปลายทางมีมาตรฐานการคุ้มครองข้อมูลที่เพียงพอตามประกาศคณะกรรมการหรือไม่", "en": "Does the destination country provide an adequate level of data protection per the Board's own notification?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]},
                    {"key": "has_binding_safeguard", "type": "yes_no", "required": true,
                     "label": {"th": "มีมาตรการคุ้มครองที่มีผลผูกพัน เช่น BCR หรือข้อสัญญามาตรฐาน (Standard Contractual Clauses) หรือไม่", "en": "Is a binding safeguard in place, e.g. binding corporate rules or standard contractual clauses?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]},
                    {"key": "has_certification", "type": "yes_no", "required": true,
                     "label": {"th": "ผู้รับข้อมูลปลายทางมีใบรับรองการคุ้มครองข้อมูลที่คณะกรรมการรับรองหรือไม่", "en": "Does the recipient hold a certification the Board recognizes?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]},
                    {"key": "government_access_risk", "type": "yes_no", "required": true,
                     "label": {"th": "กฎหมายของประเทศปลายทางให้อำนาจรัฐเข้าถึงข้อมูลได้โดยไม่มีการตรวจสอบที่เหมาะสมหรือไม่", "en": "Does the destination country's own law allow government access without adequate oversight?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "onward_transfer_risk", "type": "yes_no", "required": true,
                     "label": {"th": "ผู้รับข้อมูลปลายทางสามารถโอนข้อมูลต่อไปยังประเทศที่สามได้โดยไม่มีมาตรการควบคุมหรือไม่", "en": "Can the recipient transfer the data onward to a third country without equivalent controls?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "data_subject_rights_enforceable", "type": "yes_no", "required": true,
                     "label": {"th": "เจ้าของข้อมูลใช้สิทธิและเยียวยาตามกฎหมายของประเทศปลายทางได้จริงหรือไม่", "en": "Can data subjects realistically exercise their rights and seek remedy under the destination country's law?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]}
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000630002'
WHERE id = '00000000-0000-4000-8000-000000630001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'tia', 'transfer_tia', 'แบบประเมินผลกระทบการโอนข้อมูลไปต่างประเทศ (TIA) (ร่าง — รอฝ่ายกฎหมายตรวจ)', '00000000-0000-4000-8000-000000630001', 1,
        ARRAY['ม.28', 'ม.29', 'ประกาศ สคส. ตามมาตรา 28 และ 29 พ.ศ. 2566'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'tia' AND code = 'transfer_tia';
UPDATE platform.form_definitions SET current_version_id = NULL
    WHERE tenant_id IS NULL AND code = 'transfer_tia';
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (
    SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'transfer_tia');
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code = 'transfer_tia';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
