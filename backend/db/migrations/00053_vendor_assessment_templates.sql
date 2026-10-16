-- +goose Up
-- VEN-04 (docs/decisions.md Q-31): at least 3 ready-to-use vendor assessment templates — a PDPA compliance
-- questionnaire (ม.40), an information security assessment (mapped to ISO/IEC 27001/27701 control areas,
-- seeded as a draft pending legal/security review, the same move as DPIA-01 Q-26/ROPA-09 Q-25/RTG-01 Q-28).
-- No schema change: platform.form_definitions/form_versions (00002) and assess.templates (00011) already
-- exist, and assessment_type = 'vendor' is already in assess.templates' own CHECK constraint, unused until
-- now — the generic template library (DPIA-03's CRUD on assess.templates, already assessment_type-agnostic)
-- needs no new code to serve these.
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;

-- 1) PDPA compliance questionnaire (ม.40).
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000530001', NULL, 'vendor_pdpa', 'แบบประเมินการปฏิบัติตาม PDPA ของคู่ค้า', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000530002', NULL, '00000000-0000-4000-8000-000000530001', 1, $j${
        "sections": [
            {
                "key": "pdpa_compliance",
                "title": {"th": "การปฏิบัติตาม PDPA", "en": "PDPA compliance"},
                "questions": [
                    {"key": "has_dpo", "type": "yes_no", "required": true,
                     "label": {"th": "คู่ค้ามีเจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล (DPO) หรือผู้รับผิดชอบหรือไม่", "en": "Does the vendor have a DPO or a designated responsible person?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_retention_policy", "type": "yes_no", "required": true,
                     "label": {"th": "มีนโยบายกำหนดระยะเวลาเก็บรักษาและทำลายข้อมูลส่วนบุคคลที่ชัดเจนหรือไม่", "en": "Is there a clear retention and disposal policy for personal data?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_breach_process", "type": "yes_no", "required": true,
                     "label": {"th": "มีกระบวนการแจ้งเหตุละเมิดข้อมูลส่วนบุคคลต่อผู้ว่าจ้างหรือไม่", "en": "Is there a process to notify the controller of a personal data breach?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_subprocessor_list", "type": "yes_no", "required": true,
                     "label": {"th": "มีรายชื่อผู้ประมวลผลข้อมูลช่วง (sub-processor) ที่เปิดเผยได้หรือไม่", "en": "Can the vendor disclose a list of its own sub-processors?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_dpa_signed", "type": "yes_no", "required": true,
                     "label": {"th": "ยินดีลงนามข้อตกลงประมวลผลข้อมูลส่วนบุคคล (DPA) ตามมาตรา 40 หรือไม่", "en": "Willing to sign a data processing agreement (DPA) under section 40?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000530002'
WHERE id = '00000000-0000-4000-8000-000000530001';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'vendor', 'vendor_pdpa', 'แบบประเมินการปฏิบัติตาม PDPA ของคู่ค้า (ร่าง — รอฝ่ายกฎหมายตรวจ)', '00000000-0000-4000-8000-000000530001', 1, ARRAY['ม.40'], 'published');

-- 2) Information security assessment (ISO/IEC 27001/27701 control areas).
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000530003', NULL, 'vendor_security', 'แบบประเมินความปลอดภัยสารสนเทศของคู่ค้า', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000530004', NULL, '00000000-0000-4000-8000-000000530003', 1, $j${
        "sections": [
            {
                "key": "information_security",
                "title": {"th": "ความปลอดภัยสารสนเทศ (ISO/IEC 27001 / 27701)", "en": "Information security (ISO/IEC 27001 / 27701)"},
                "questions": [
                    {"key": "has_isms_cert", "type": "yes_no", "required": true,
                     "label": {"th": "มีการรับรองมาตรฐานความปลอดภัยสารสนเทศ เช่น ISO/IEC 27001 หรือไม่", "en": "Certified to an information security standard such as ISO/IEC 27001?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_pims_cert", "type": "yes_no", "required": true,
                     "label": {"th": "มีการรับรองมาตรฐานการจัดการข้อมูลส่วนบุคคล เช่น ISO/IEC 27701 หรือไม่", "en": "Certified to a privacy information management standard such as ISO/IEC 27701?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_access_control", "type": "yes_no", "required": true,
                     "label": {"th": "มีการควบคุมการเข้าถึงข้อมูลตามหลัก least privilege หรือไม่", "en": "Is access to data controlled on a least-privilege basis?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_encryption", "type": "yes_no", "required": true,
                     "label": {"th": "มีการเข้ารหัสข้อมูลส่วนบุคคลทั้งระหว่างส่งและจัดเก็บหรือไม่", "en": "Is personal data encrypted both in transit and at rest?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_incident_response", "type": "yes_no", "required": true,
                     "label": {"th": "มีแผนตอบสนองต่อเหตุการณ์ด้านความปลอดภัยสารสนเทศหรือไม่", "en": "Is there an information security incident response plan?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "has_security_training", "type": "yes_no", "required": true,
                     "label": {"th": "มีการอบรมด้านความปลอดภัยสารสนเทศให้พนักงานเป็นประจำหรือไม่", "en": "Do staff receive regular information security training?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000530004'
WHERE id = '00000000-0000-4000-8000-000000530003';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'vendor', 'vendor_security', 'แบบประเมินความปลอดภัยสารสนเทศของคู่ค้า (ร่าง — รอฝ่ายกฎหมายตรวจ)', '00000000-0000-4000-8000-000000530003', 1,
        ARRAY['ม.37(1)', 'ISO/IEC 27001', 'ISO/IEC 27701'], 'published');

-- 3) Cross-border data transfer assessment.
INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000530005', NULL, 'vendor_transfer', 'แบบประเมินการโอนข้อมูลไปต่างประเทศของคู่ค้า', 'assessment', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000530006', NULL, '00000000-0000-4000-8000-000000530005', 1, $j${
        "sections": [
            {
                "key": "cross_border_transfer",
                "title": {"th": "การโอนข้อมูลไปต่างประเทศ", "en": "Cross-border data transfer"},
                "questions": [
                    {"key": "stores_data_abroad", "type": "yes_no", "required": true,
                     "label": {"th": "จัดเก็บหรือประมวลผลข้อมูลส่วนบุคคลนอกประเทศไทยหรือไม่", "en": "Does the vendor store or process personal data outside Thailand?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
                    {"key": "host_country_adequate", "type": "yes_no", "required": true,
                     "label": {"th": "ประเทศที่จัดเก็บข้อมูลมีระดับการคุ้มครองข้อมูลที่เพียงพอตามประกาศคณะกรรมการหรือไม่", "en": "Does the host country provide an adequate level of data protection per the Board's own notification?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]},
                    {"key": "has_scc_or_safeguard", "type": "yes_no", "required": true,
                     "label": {"th": "มีมาตรการคุ้มครองที่เหมาะสม เช่น Standard Contractual Clauses หรือข้อตกลงที่มีผลผูกพันหรือไม่", "en": "Are suitable safeguards in place, e.g. Standard Contractual Clauses or binding corporate rules?"},
                     "options": [{"value": "yes", "score": 0}, {"value": "no", "score": 1}]},
                    {"key": "subprocessor_abroad", "type": "yes_no", "required": true,
                     "label": {"th": "ผู้ประมวลผลข้อมูลช่วง (sub-processor) ของคู่ค้าตั้งอยู่ต่างประเทศหรือไม่", "en": "Are any of the vendor's own sub-processors located abroad?"},
                     "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
                ]
            }
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000530006'
WHERE id = '00000000-0000-4000-8000-000000530005';

INSERT INTO assess.templates (tenant_id, assessment_type, code, name, form_id, version_no, legal_refs, status)
VALUES (NULL, 'vendor', 'vendor_transfer', 'แบบประเมินการโอนข้อมูลไปต่างประเทศของคู่ค้า (ร่าง — รอฝ่ายกฎหมายตรวจ)', '00000000-0000-4000-8000-000000530005', 1,
        ARRAY['ม.28', 'ม.29'], 'published');

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM assess.templates WHERE tenant_id IS NULL AND assessment_type = 'vendor'
    AND code IN ('vendor_pdpa', 'vendor_security', 'vendor_transfer');
UPDATE platform.form_definitions SET current_version_id = NULL
    WHERE tenant_id IS NULL AND code IN ('vendor_pdpa', 'vendor_security', 'vendor_transfer');
DELETE FROM platform.form_versions WHERE tenant_id IS NULL AND form_id IN (
    SELECT id FROM platform.form_definitions WHERE tenant_id IS NULL AND code IN ('vendor_pdpa', 'vendor_security', 'vendor_transfer'));
DELETE FROM platform.form_definitions WHERE tenant_id IS NULL AND code IN ('vendor_pdpa', 'vendor_security', 'vendor_transfer');
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE assess.templates FORCE ROW LEVEL SECURITY;
