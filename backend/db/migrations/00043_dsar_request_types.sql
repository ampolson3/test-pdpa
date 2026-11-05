-- +goose Up
-- DSAR-13: response letter templates by right type + outcome. dsar.request_types already had name_th (baseline
-- migration 00012); this feature needs an English name too, since the letter is bilingual (module doc: "หนังสือ
-- ตอบ TH/EN") and nothing else in the baseline schema carries it.
ALTER TABLE dsar.request_types ADD COLUMN name_en text;

-- The 9 request-type rows themselves (one per dsar.requests.request_type_id CHECK code) are master data no
-- feature has owned seeding yet — same "seed the fixed catalog globally" move ORG-07 (Q-20) made, except these
-- codes are a closed set fixed by the DDL's own CHECK constraint, not something a tenant edits, so no draft/
-- legal-review flag is needed here (unlike ORG-07's paraphrased lawful bases).
ALTER TABLE dsar.request_types NO FORCE ROW LEVEL SECURITY;
INSERT INTO dsar.request_types (tenant_id, code, name_th, name_en, legal_ref, sla_days) VALUES
    (NULL, 'access', 'ขอเข้าถึงข้อมูลส่วนบุคคล', 'Access', 'ม.30', 30),
    (NULL, 'portability', 'ขอรับหรือโอนย้ายข้อมูลส่วนบุคคล', 'Data portability', 'ม.31', 30),
    (NULL, 'objection', 'คัดค้านการประมวลผลข้อมูลส่วนบุคคล', 'Objection', 'ม.32', 30),
    (NULL, 'erasure', 'ขอให้ลบหรือทำลายข้อมูลส่วนบุคคล', 'Erasure', 'ม.33', 30),
    (NULL, 'restriction', 'ขอให้ระงับการใช้ข้อมูลส่วนบุคคล', 'Restriction of processing', 'ม.34', 30),
    (NULL, 'rectification', 'ขอให้แก้ไขข้อมูลส่วนบุคคลให้ถูกต้อง', 'Rectification', 'ม.35', 30),
    (NULL, 'withdraw_consent', 'ขอถอนความยินยอม', 'Withdraw consent', 'ม.19', 30),
    (NULL, 'complaint', 'ร้องเรียน', 'Complaint', NULL, 30),
    (NULL, 'inquiry', 'สอบถาม', 'Inquiry', NULL, 30)
ON CONFLICT ON CONSTRAINT uq_request_types_code DO NOTHING;
ALTER TABLE dsar.request_types FORCE ROW LEVEL SECURITY;

-- Response letter DRAFT sample text (CLAUDE.md rule 8; docs/decisions.md Q-14's "seed a draft, flag for legal
-- review" pattern, same as PNG-03). Kept per *purpose* (result / rejection / request_info — the three outcomes
-- the module doc names: "ดำเนินการแล้ว / ปฏิเสธ / ขอข้อมูลเพิ่ม"), not per (type × purpose): the letter's
-- type-specific wording is produced at generation time by substituting the request type's own name into the
-- template body, so 3 bodies (not 27) cover every type/outcome combination without duplicating near-identical
-- text 9 times over — the type name is what actually differs, not the surrounding letter structure.
ALTER TABLE platform.templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.templates (tenant_id, template_type, code, name, language, industry, content, status) VALUES
    (NULL, 'dsar_response', 'result', 'ผลการพิจารณาคำขอ (ดำเนินการแล้ว)', 'th', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[ร่าง — ข้อความตัวอย่างนี้ต้องได้รับการทบทวนและอนุมัติจากฝ่ายกฎหมาย/DPO ก่อนส่งจริง]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"เรียน {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"อ้างถึงคำขอใช้สิทธิเลขที่ {{request_no}} ประเภท {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"บริษัทได้ดำเนินการตามคำขอของท่านเรียบร้อยแล้ว [โปรดระบุรายละเอียดผลการดำเนินการ]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"หากท่านมีข้อสงสัยเพิ่มเติม โปรดติดต่อ"}]}
        ]}'::jsonb, 'published'),
    (NULL, 'dsar_response', 'result', 'Response to your request (fulfilled)', 'en', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[DRAFT — this sample text must be reviewed and approved by Legal/DPO before real use]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Dear {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Re: your request no. {{request_no}}, type: {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"We have fulfilled your request. [Please state the details of what was done]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"If you have further questions, please contact"}]}
        ]}'::jsonb, 'published'),
    (NULL, 'dsar_response', 'rejection', 'ผลการพิจารณาคำขอ (ปฏิเสธ)', 'th', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[ร่าง — ข้อความตัวอย่างนี้ต้องได้รับการทบทวนและอนุมัติจากฝ่ายกฎหมาย/DPO ก่อนส่งจริง]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"เรียน {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"อ้างถึงคำขอใช้สิทธิเลขที่ {{request_no}} ประเภท {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"บริษัทขอเรียนแจ้งว่าไม่สามารถดำเนินการตามคำขอของท่านได้ เนื่องจาก {{rejection_reason}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"หากท่านมีข้อสงสัยเพิ่มเติม โปรดติดต่อ"}]}
        ]}'::jsonb, 'published'),
    (NULL, 'dsar_response', 'rejection', 'Response to your request (rejected)', 'en', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[DRAFT — this sample text must be reviewed and approved by Legal/DPO before real use]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Dear {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Re: your request no. {{request_no}}, type: {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"We are unable to fulfil your request because {{rejection_reason}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"If you have further questions, please contact"}]}
        ]}'::jsonb, 'published'),
    (NULL, 'dsar_response', 'request_info', 'ขอข้อมูลเพิ่มเติมสำหรับคำขอของท่าน', 'th', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[ร่าง — ข้อความตัวอย่างนี้ต้องได้รับการทบทวนและอนุมัติจากฝ่ายกฎหมาย/DPO ก่อนส่งจริง]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"เรียน {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"อ้างถึงคำขอใช้สิทธิเลขที่ {{request_no}} ประเภท {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"เพื่อดำเนินการตามคำขอของท่าน บริษัทขอข้อมูลเพิ่มเติมดังนี้ [โปรดระบุข้อมูลที่ต้องการ]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"หากท่านมีข้อสงสัยเพิ่มเติม โปรดติดต่อ"}]}
        ]}'::jsonb, 'published'),
    (NULL, 'dsar_response', 'request_info', 'We need more information for your request', 'en', NULL,
        '{"type":"doc","content":[
            {"type":"paragraph","content":[{"type":"text","text":"[DRAFT — this sample text must be reviewed and approved by Legal/DPO before real use]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Dear {{requester_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"Re: your request no. {{request_no}}, type: {{request_type_name}}"}]},
            {"type":"paragraph","content":[{"type":"text","text":"To process your request, we need the following additional information: [please state what is needed]"}]},
            {"type":"paragraph","content":[{"type":"text","text":"If you have further questions, please contact"}]}
        ]}'::jsonb, 'published')
ON CONFLICT ON CONSTRAINT uq_templates_template_type_code_language_version_no DO NOTHING;
ALTER TABLE platform.templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.templates WHERE tenant_id IS NULL AND template_type = 'dsar_response';
ALTER TABLE platform.templates FORCE ROW LEVEL SECURITY;

ALTER TABLE dsar.request_types NO FORCE ROW LEVEL SECURITY;
DELETE FROM dsar.request_types WHERE tenant_id IS NULL AND code IN
    ('access', 'portability', 'objection', 'erasure', 'restriction', 'rectification', 'withdraw_consent', 'complaint', 'inquiry');
ALTER TABLE dsar.request_types FORCE ROW LEVEL SECURITY;

ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS name_en;
