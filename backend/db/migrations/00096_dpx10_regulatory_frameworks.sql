-- DPX-10 เชื่อมโยงกฎหมายอื่นที่เกี่ยวข้อง (Related laws mapping). A small, global catalog of the other Thai
-- laws/notices this module's own description names (cybersecurity act, computer crime act's 90-day traffic
-- log duty, BOT/OIC/SEC sector notices) — each turned into an ordinary PLT-06 checklist, the module doc's
-- own "เป็น checklist" literally, never a bespoke per-law engine. gov.regulatory_frameworks is global
-- (tenant_id NULL, no RLS) the same way gov.regulatory_updates already is (00017_gov.sql) — a reference
-- list every tenant sees, maintained by migration/SUPER, not a tenant's own CRUD; gov.framework_assessments
-- is the thin, tenant-scoped run record, mirroring dpo.security_assessments' own shape (score/band + a
-- form_submission_id) and DPO-09's own Assess pattern. Seeded content is a DRAFT pending legal review
-- (docs/decisions.md Q-46) — paraphrased from this feature's own module-doc description, not copied from
-- any official notice text.

-- +goose Up
ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security', 'checklist', 'sme_exemption', 'framework'));

CREATE TABLE gov.regulatory_frameworks (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    code varchar(60) NOT NULL,
    name_th text NOT NULL,
    name_en text NOT NULL,
    legal_refs text[] NOT NULL DEFAULT '{}',
    description_th text,
    description_en text,
    checklist_form_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_regulatory_frameworks PRIMARY KEY (id),
    CONSTRAINT uq_gov_regulatory_frameworks_code UNIQUE (code)
);
ALTER TABLE gov.regulatory_frameworks ADD CONSTRAINT fk_regulatory_frameworks_checklist_form FOREIGN KEY (checklist_form_id) REFERENCES platform.form_definitions (id);
CREATE TRIGGER trg_regulatory_frameworks_updated BEFORE UPDATE ON gov.regulatory_frameworks FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
COMMENT ON TABLE gov.regulatory_frameworks IS 'กฎหมาย/ประกาศอื่นที่เกี่ยวข้อง พร้อม checklist (DPX-10)';

CREATE TABLE gov.framework_assessments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    framework_id uuid NOT NULL,
    form_submission_id uuid NOT NULL,
    score numeric(6, 2) NOT NULL,
    result text NOT NULL,
    factors jsonb NOT NULL,
    assessed_by uuid,
    assessed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_gov_framework_assessments PRIMARY KEY (id)
);
ALTER TABLE gov.framework_assessments ADD CONSTRAINT fk_framework_assessments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE gov.framework_assessments ADD CONSTRAINT fk_framework_assessments_framework FOREIGN KEY (framework_id) REFERENCES gov.regulatory_frameworks (id);
ALTER TABLE gov.framework_assessments ADD CONSTRAINT fk_framework_assessments_form_submission FOREIGN KEY (form_submission_id) REFERENCES platform.form_submissions (id);
ALTER TABLE gov.framework_assessments ADD CONSTRAINT fk_framework_assessments_assessed_by FOREIGN KEY (assessed_by) REFERENCES iam.users (id);
CREATE INDEX ix_gov_framework_assessments_framework ON gov.framework_assessments (tenant_id, framework_id, assessed_at DESC);
CREATE TRIGGER trg_framework_assessments_updated BEFORE UPDATE ON gov.framework_assessments FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();
COMMENT ON TABLE gov.framework_assessments IS 'ผลการตอบ checklist ของกฎหมายอื่น ต่อรอบ (DPX-10)';

ALTER TABLE gov.framework_assessments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON gov.framework_assessments
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- permission family: dpx.framework (read-mostly — the catalog itself is global/seeded, so create/update/
-- publish on the underlying form are never exercised by a real tenant screen, same reasoning ROPA-20's own
-- sme_exemption note already gives; execute is the only action a real screen uses, to answer a checklist)
INSERT INTO iam.permissions (code, module, resource, action, description, is_sensitive) VALUES
    ('dpx.framework.read', 'dpx', 'framework', 'R', 'กฎหมายอื่นที่เกี่ยวข้องและ checklist — ดู', false),
    ('dpx.framework.create', 'dpx', 'framework', 'C', 'กฎหมายอื่นที่เกี่ยวข้องและ checklist — สร้าง', false),
    ('dpx.framework.update', 'dpx', 'framework', 'U', 'กฎหมายอื่นที่เกี่ยวข้องและ checklist — แก้ไข', false),
    ('dpx.framework.publish', 'dpx', 'framework', 'P', 'กฎหมายอื่นที่เกี่ยวข้องและ checklist — เผยแพร่', false),
    ('dpx.framework.execute', 'dpx', 'framework', 'X', 'กฎหมายอื่นที่เกี่ยวข้องและ checklist — ตอบ', false);

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;

INSERT INTO iam.role_permissions (role_id, tenant_id, permission_code)
SELECT r.id, NULL, p.code
FROM iam.roles r
CROSS JOIN (VALUES
    ('DPO', 'dpx.framework.read'), ('DPO', 'dpx.framework.create'), ('DPO', 'dpx.framework.update'),
    ('DPO', 'dpx.framework.publish'), ('DPO', 'dpx.framework.execute'),
    ('PRIVACY', 'dpx.framework.read'), ('PRIVACY', 'dpx.framework.create'), ('PRIVACY', 'dpx.framework.update'),
    ('PRIVACY', 'dpx.framework.execute'),
    ('LEGAL', 'dpx.framework.read'), ('LEGAL', 'dpx.framework.execute'),
    ('IT', 'dpx.framework.read'), ('IT', 'dpx.framework.execute'),
    ('SEC', 'dpx.framework.read'), ('SEC', 'dpx.framework.execute'),
    ('OWNER', 'dpx.framework.read'),
    ('AUDIT', 'dpx.framework.read'),
    ('EXEC', 'dpx.framework.read')
) AS p(role_code, code)
WHERE r.tenant_id IS NULL AND r.code = p.role_code;

ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;

-- seed: 4 frameworks, each a global published 'framework'-type checklist (DRAFT pending legal review)
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status) VALUES
    ('00000000-0000-4000-8000-000000960001', NULL, 'framework_cybersecurity_act', 'พ.ร.บ.การรักษาความมั่นคงปลอดภัยไซเบอร์ พ.ศ. 2562', 'framework', 'published'),
    ('00000000-0000-4000-8000-000000960003', NULL, 'framework_computer_crime_act', 'พ.ร.บ.คอมพิวเตอร์ (เก็บข้อมูลจราจร 90 วัน)', 'framework', 'published'),
    ('00000000-0000-4000-8000-000000960005', NULL, 'framework_bot_notices', 'ประกาศธนาคารแห่งประเทศไทย (ธปท.)', 'framework', 'published'),
    ('00000000-0000-4000-8000-000000960007', NULL, 'framework_sec_oic_notices', 'ประกาศ ก.ล.ต. / คปภ.', 'framework', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, languages, published_at) VALUES
    ('00000000-0000-4000-8000-000000960002', NULL, '00000000-0000-4000-8000-000000960001', 1, $j${
        "sections": [{"key": "cybersecurity", "title": {"th": "พ.ร.บ.ไซเบอร์ พ.ศ. 2562", "en": "Cybersecurity Act 2019"}, "questions": [
            {"key": "csc_critical_infra", "type": "yes_no", "label": {"th": "องค์กรเป็นโครงสร้างพื้นฐานสำคัญทางสารสนเทศ (CII) ตามประกาศ", "en": "The organization is designated critical information infrastructure (CII)"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
            {"key": "csc_incident_plan", "type": "yes_no", "label": {"th": "มีแผนรับมือภัยไซเบอร์และแจ้งเหตุต่อ สกมช. ตามกำหนด", "en": "A cyber-incident response plan exists and reports to NCSA on time"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
            {"key": "csc_risk_assessment", "type": "yes_no", "label": {"th": "มีการประเมินความเสี่ยงไซเบอร์เป็นประจำ", "en": "Cyber risk is assessed on a regular basis"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
        ]}]
    }$j$::jsonb, ARRAY['th', 'en'], now()),
    ('00000000-0000-4000-8000-000000960004', NULL, '00000000-0000-4000-8000-000000960003', 1, $j${
        "sections": [{"key": "computer_crime", "title": {"th": "พ.ร.บ.คอมพิวเตอร์", "en": "Computer Crime Act"}, "questions": [
            {"key": "cca_log_90days", "type": "yes_no", "label": {"th": "เก็บข้อมูลจราจรทางคอมพิวเตอร์ (traffic log) ไม่น้อยกว่า 90 วัน", "en": "Computer traffic logs are kept for at least 90 days"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
            {"key": "cca_log_integrity", "type": "yes_no", "label": {"th": "มีการป้องกันไม่ให้แก้ไขหรือลบ log ก่อนครบกำหนด", "en": "Logs are protected against tampering or early deletion"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
        ]}]
    }$j$::jsonb, ARRAY['th', 'en'], now()),
    ('00000000-0000-4000-8000-000000960006', NULL, '00000000-0000-4000-8000-000000960005', 1, $j${
        "sections": [{"key": "bot", "title": {"th": "ประกาศ ธปท.", "en": "BOT notices"}, "questions": [
            {"key": "bot_outsourcing", "type": "yes_no", "label": {"th": "การใช้บริการบุคคลภายนอก (outsourcing) ด้าน IT เป็นไปตามหลักเกณฑ์ ธปท.", "en": "IT outsourcing follows BOT's own requirements"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
            {"key": "bot_customer_data", "type": "yes_no", "label": {"th": "การคุ้มครองข้อมูลลูกค้าสถาบันการเงินเป็นไปตามประกาศ ธปท.", "en": "Financial customer data protection follows BOT notices"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
        ]}]
    }$j$::jsonb, ARRAY['th', 'en'], now()),
    ('00000000-0000-4000-8000-000000960008', NULL, '00000000-0000-4000-8000-000000960007', 1, $j${
        "sections": [{"key": "sec_oic", "title": {"th": "ประกาศ ก.ล.ต. / คปภ.", "en": "SEC / OIC notices"}, "questions": [
            {"key": "sec_disclosure", "type": "yes_no", "label": {"th": "การเปิดเผยข้อมูลต่อผู้ลงทุน/ผู้เอาประกันเป็นไปตามประกาศที่เกี่ยวข้อง", "en": "Disclosure to investors/policyholders follows the relevant notice"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]},
            {"key": "sec_outsourcing", "type": "yes_no", "label": {"th": "การใช้บริการบุคคลภายนอกเป็นไปตามหลักเกณฑ์ที่เกี่ยวข้อง", "en": "Outsourcing follows the relevant requirements"}, "required": true, "options": [{"value": "yes", "score": 1}, {"value": "no", "score": 0}]}
        ]}]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000960002' WHERE id = '00000000-0000-4000-8000-000000960001';
UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000960004' WHERE id = '00000000-0000-4000-8000-000000960003';
UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000960006' WHERE id = '00000000-0000-4000-8000-000000960005';
UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000960008' WHERE id = '00000000-0000-4000-8000-000000960007';

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;

INSERT INTO gov.regulatory_frameworks (code, name_th, name_en, legal_refs, description_th, description_en, checklist_form_id) VALUES
    ('cybersecurity_act', 'พ.ร.บ.การรักษาความมั่นคงปลอดภัยไซเบอร์ พ.ศ. 2562 (ร่าง — รอฝ่ายกฎหมายตรวจ)',
     'Cybersecurity Act 2019 (DRAFT — pending legal review)', ARRAY['พ.ร.บ.ไซเบอร์ พ.ศ. 2562'],
     'สำหรับองค์กรที่เป็นโครงสร้างพื้นฐานสำคัญทางสารสนเทศ (CII)', 'For organizations designated as critical information infrastructure (CII)',
     '00000000-0000-4000-8000-000000960001'),
    ('computer_crime_act', 'พ.ร.บ.คอมพิวเตอร์ – เก็บข้อมูลจราจร 90 วัน (ร่าง — รอฝ่ายกฎหมายตรวจ)',
     'Computer Crime Act — 90-day traffic log retention (DRAFT — pending legal review)', ARRAY['พ.ร.บ.คอมพิวเตอร์'],
     'หน้าที่เก็บรักษาข้อมูลจราจรทางคอมพิวเตอร์', 'The duty to retain computer traffic logs',
     '00000000-0000-4000-8000-000000960003'),
    ('bot_notices', 'ประกาศธนาคารแห่งประเทศไทย (ร่าง — รอฝ่ายกฎหมายตรวจ)', 'Bank of Thailand notices (DRAFT — pending legal review)',
     ARRAY['ประกาศ ธปท.'], 'สำหรับสถาบันการเงินและผู้ให้บริการที่อยู่ภายใต้การกำกับของ ธปท.',
     'For financial institutions and providers supervised by the Bank of Thailand', '00000000-0000-4000-8000-000000960005'),
    ('sec_oic_notices', 'ประกาศ ก.ล.ต. / คปภ. (ร่าง — รอฝ่ายกฎหมายตรวจ)', 'SEC / OIC notices (DRAFT — pending legal review)',
     ARRAY['ประกาศ ก.ล.ต.', 'ประกาศ คปภ.'], 'สำหรับบริษัทหลักทรัพย์ บริษัทประกัน และผู้ประกอบธุรกิจที่อยู่ภายใต้การกำกับ',
     'For securities firms, insurers and other businesses under SEC/OIC supervision', '00000000-0000-4000-8000-000000960007');

-- +goose Down
DELETE FROM gov.regulatory_frameworks WHERE code IN ('cybersecurity_act', 'computer_crime_act', 'bot_notices', 'sec_oic_notices');

ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
UPDATE platform.form_definitions SET current_version_id = NULL WHERE id IN (
    '00000000-0000-4000-8000-000000960001', '00000000-0000-4000-8000-000000960003',
    '00000000-0000-4000-8000-000000960005', '00000000-0000-4000-8000-000000960007');
DELETE FROM platform.form_versions WHERE id IN (
    '00000000-0000-4000-8000-000000960002', '00000000-0000-4000-8000-000000960004',
    '00000000-0000-4000-8000-000000960006', '00000000-0000-4000-8000-000000960008');
DELETE FROM platform.form_definitions WHERE id IN (
    '00000000-0000-4000-8000-000000960001', '00000000-0000-4000-8000-000000960003',
    '00000000-0000-4000-8000-000000960005', '00000000-0000-4000-8000-000000960007');
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;

ALTER TABLE iam.roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions NO FORCE ROW LEVEL SECURITY;
DELETE FROM iam.role_permissions WHERE permission_code LIKE 'dpx.framework.%';
ALTER TABLE iam.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE iam.role_permissions FORCE ROW LEVEL SECURITY;
DELETE FROM iam.permissions WHERE code LIKE 'dpx.framework.%';

DROP TABLE IF EXISTS gov.framework_assessments;
DROP TABLE IF EXISTS gov.regulatory_frameworks;

ALTER TABLE platform.form_definitions DROP CONSTRAINT form_definitions_form_type_check;
ALTER TABLE platform.form_definitions ADD CONSTRAINT form_definitions_form_type_check
    CHECK (form_type IN ('consent', 'dsar', 'breach', 'assessment', 'questionnaire', 'intake', 'quiz', 'security', 'checklist', 'sme_exemption'));
