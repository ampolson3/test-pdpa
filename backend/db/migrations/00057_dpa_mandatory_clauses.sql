-- +goose Up
-- DPA-03 (ข้อกำหนดที่ต้องมี): the "คลัง control" for this feature is PLT-16's own clause library
-- (platform.clause_library, already built by PLT-16/DPA-01) — seeds the nine ม.40(1)-(3)/37(2)/28-29 clause
-- topics the module doc's own description names, each flagged DRAFT (rule 8 — legal wording stays flagged
-- until Legal reviews it, the same "seed a draft pending review" move ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01/
-- VEN-04/DPA-01 already made) and marked is_mandatory so DPA-01's own clone-and-edit flow surfaces them.
-- agreement.mandatory_rules then names which of these codes a "dpa" agreement must attach before its
-- document can be submitted for approval (the acceptance criterion) — the cross-border transfer rule is the
-- only conditional one (condition {"requires_transfer": true}): every other rule always applies.
ALTER TABLE platform.clause_library NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.clause_library (id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory, version_no, status)
VALUES
  ('00000000-0000-7000-8000-0000000d0a01'::uuid, NULL, 'dpa.processing_on_instructions', 'dpa',
   'ประมวลผลตามคำสั่งเท่านั้น / Processing only on instructions',
   $j${"title": "ประมวลผลตามคำสั่งเท่านั้น", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลตกลงประมวลผลข้อมูลส่วนบุคคลเฉพาะตามคำสั่งที่เป็นลายลักษณ์อักษรของผู้ควบคุมข้อมูลส่วนบุคคลเท่านั้น ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Processing only on instructions", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor shall process personal data only on the Controller's written instructions, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a02'::uuid, NULL, 'dpa.confidentiality', 'dpa',
   'การรักษาความลับ / Confidentiality',
   $j${"title": "การรักษาความลับ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลต้องรักษาความลับของข้อมูลส่วนบุคคลและกำหนดให้บุคคลที่เกี่ยวข้องรักษาความลับเช่นเดียวกัน ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Confidentiality", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor shall keep personal data confidential and require anyone involved in processing to do the same, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a03'::uuid, NULL, 'dpa.security_measures', 'dpa',
   'มาตรการความปลอดภัย / Security measures',
   $j${"title": "มาตรการความปลอดภัย", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลต้องจัดให้มีมาตรการรักษาความมั่นคงปลอดภัยที่เหมาะสมตามมาตรา 37(2) และประกาศมาตรการความปลอดภัย พ.ศ. 2565"}]}]}}$j$::jsonb,
   $j${"title": "Security measures", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor shall implement appropriate security measures under section 37(2) and the 2022 Security Measures Notification."}]}]}}$j$::jsonb,
   'ม.37(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a04'::uuid, NULL, 'dpa.breach_notification', 'dpa',
   'การแจ้งเหตุละเมิดข้อมูล / Breach notification',
   $j${"title": "การแจ้งเหตุละเมิดข้อมูล", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลต้องแจ้งเหตุการละเมิดข้อมูลส่วนบุคคลให้ผู้ควบคุมข้อมูลส่วนบุคคลทราบโดยไม่ชักช้า ตามมาตรา 37(4)"}]}]}}$j$::jsonb,
   $j${"title": "Breach notification", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor shall notify the Controller of a personal data breach without delay, under section 37(4)."}]}]}}$j$::jsonb,
   'ม.37(4)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a05'::uuid, NULL, 'dpa.sub_processors', 'dpa',
   'ผู้ประมวลผลช่วง / Sub-processors',
   $j${"title": "ผู้ประมวลผลช่วง", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลจะมอบหมายให้ผู้ประมวลผลข้อมูลส่วนบุคคลช่วงดำเนินการแทนได้เฉพาะเมื่อได้รับความยินยอมล่วงหน้าจากผู้ควบคุมข้อมูลส่วนบุคคล ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Sub-processors", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor may only engage a sub-processor with the Controller's prior consent, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a06'::uuid, NULL, 'dpa.dsar_assistance', 'dpa',
   'ความช่วยเหลือในการตอบคำขอใช้สิทธิ / DSAR assistance',
   $j${"title": "ความช่วยเหลือในการตอบคำขอใช้สิทธิ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ประมวลผลข้อมูลส่วนบุคคลต้องให้ความช่วยเหลือผู้ควบคุมข้อมูลส่วนบุคคลในการตอบสนองต่อคำขอใช้สิทธิของเจ้าของข้อมูลส่วนบุคคล ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Assistance with data subject rights requests", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Processor shall assist the Controller in responding to data subject rights requests, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a07'::uuid, NULL, 'dpa.return_or_destroy', 'dpa',
   'การคืนหรือทำลายข้อมูลเมื่อสิ้นสุด / Return or destruction on termination',
   $j${"title": "การคืนหรือทำลายข้อมูลเมื่อสิ้นสุด", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] เมื่อสิ้นสุดข้อตกลงนี้ ผู้ประมวลผลข้อมูลส่วนบุคคลต้องคืนหรือทำลายข้อมูลส่วนบุคคลทั้งหมด ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Return or destruction of personal data on termination", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] On termination of this agreement, the Processor shall return or destroy all personal data, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a08'::uuid, NULL, 'dpa.audit_rights', 'dpa',
   'สิทธิในการตรวจสอบของผู้ควบคุม / Controller audit rights',
   $j${"title": "สิทธิในการตรวจสอบของผู้ควบคุม", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้ควบคุมข้อมูลส่วนบุคคลมีสิทธิตรวจสอบการปฏิบัติตามข้อตกลงนี้ของผู้ประมวลผลข้อมูลส่วนบุคคลตามความเหมาะสม ตามมาตรา 40(2)"}]}]}}$j$::jsonb,
   $j${"title": "Controller's audit rights", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Controller may audit the Processor's compliance with this agreement as reasonably appropriate, under section 40(2)."}]}]}}$j$::jsonb,
   'ม.40(2)', '{dpa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d0a09'::uuid, NULL, 'dpa.cross_border_transfer', 'dpa',
   'การโอนข้อมูลไปต่างประเทศ / Cross-border transfer',
   $j${"title": "การโอนข้อมูลไปต่างประเทศ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] หากมีการโอนข้อมูลส่วนบุคคลไปต่างประเทศ ผู้ประมวลผลข้อมูลส่วนบุคคลต้องปฏิบัติตามมาตรา 28 และ 29 และจัดให้มีมาตรการคุ้มครองที่เหมาะสม"}]}]}}$j$::jsonb,
   $j${"title": "Cross-border transfer of personal data", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] Where personal data is transferred abroad, the Processor shall comply with sections 28 and 29 and implement appropriate safeguards."}]}]}}$j$::jsonb,
   'ม.28-29', '{dpa}', true, 1, 'published');

INSERT INTO agreement.mandatory_rules (agreement_type, clause_code, legal_ref, condition)
VALUES
  ('dpa', 'dpa.processing_on_instructions', 'ม.40(2)', NULL),
  ('dpa', 'dpa.confidentiality', 'ม.40(2)', NULL),
  ('dpa', 'dpa.security_measures', 'ม.37(2)', NULL),
  ('dpa', 'dpa.breach_notification', 'ม.37(4)', NULL),
  ('dpa', 'dpa.sub_processors', 'ม.40(2)', NULL),
  ('dpa', 'dpa.dsar_assistance', 'ม.40(2)', NULL),
  ('dpa', 'dpa.return_or_destroy', 'ม.40(2)', NULL),
  ('dpa', 'dpa.audit_rights', 'ม.40(2)', NULL),
  ('dpa', 'dpa.cross_border_transfer', 'ม.28-29', '{"requires_transfer": true}'::jsonb);

ALTER TABLE platform.clause_library FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.clause_library NO FORCE ROW LEVEL SECURITY;
DELETE FROM agreement.mandatory_rules WHERE agreement_type = 'dpa' AND clause_code LIKE 'dpa.%';
DELETE FROM platform.clause_library WHERE tenant_id IS NULL AND code LIKE 'dpa.%';
ALTER TABLE platform.clause_library FORCE ROW LEVEL SECURITY;
