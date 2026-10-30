-- +goose Up
-- DSA-05 (ข้อกำหนดที่ต้องมี): the same "คลัง control" DPA-03's own migration 00057 already built —
-- platform.clause_library + agreement.mandatory_rules, both already agreement_type-agnostic (every read
-- path in internal/agreement/service/clauses.go keys off the agreement's own agreement_type, never
-- hard-codes "dpa") — seeds the nine ม.27/28-29/37(2)/37(4) clause topics this feature's own module-doc
-- description names, each flagged DRAFT (rule 8) and marked is_mandatory. agreement.mandatory_rules then
-- names which of these codes a "dsa" agreement must attach before its document can be submitted for
-- approval (the acceptance criterion) — cross-border transfer is again the only conditional rule
-- (condition {"requires_transfer": true}), exactly mirroring DPA-03's own cross_border_transfer rule.
ALTER TABLE platform.clause_library NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.clause_library (id, tenant_id, code, category, title, body_th, body_en, legal_ref, applies_to, is_mandatory, version_no, status)
VALUES
  ('00000000-0000-7000-8000-0000000d5a01'::uuid, NULL, 'dsa.purpose_limitation', 'dsa',
   'วัตถุประสงค์ที่ใช้ได้ / Permitted purpose',
   $j${"title": "วัตถุประสงค์ที่ใช้ได้", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้รับข้อมูลตกลงใช้ข้อมูลส่วนบุคคลที่ได้รับเฉพาะตามวัตถุประสงค์ที่ระบุไว้ในข้อตกลงนี้เท่านั้น ตามมาตรา 27"}]}]}}$j$::jsonb,
   $j${"title": "Permitted purpose", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Receiving Party agrees to use the received personal data only for the purpose stated in this agreement, under section 27."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a02'::uuid, NULL, 'dsa.no_excess_use', 'dsa',
   'ห้ามใช้หรือเปิดเผยเกินวัตถุประสงค์ / No use or disclosure beyond purpose',
   $j${"title": "ห้ามใช้หรือเปิดเผยเกินวัตถุประสงค์", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้รับข้อมูลต้องไม่ใช้หรือเปิดเผยข้อมูลส่วนบุคคลเกินกว่าวัตถุประสงค์ที่แจ้งไว้ ตามมาตรา 27"}]}]}}$j$::jsonb,
   $j${"title": "No use or disclosure beyond purpose", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Receiving Party shall not use or disclose the personal data beyond the notified purpose, under section 27."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a03'::uuid, NULL, 'dsa.security_measures', 'dsa',
   'มาตรการความปลอดภัย / Security measures',
   $j${"title": "มาตรการความปลอดภัย", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ทั้งสองฝ่ายต้องจัดให้มีมาตรการรักษาความมั่นคงปลอดภัยที่เหมาะสมตามมาตรา 37(2) และประกาศมาตรการความปลอดภัย พ.ศ. 2565"}]}]}}$j$::jsonb,
   $j${"title": "Security measures", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] Both parties shall implement appropriate security measures under section 37(2) and the 2022 Security Measures Notification."}]}]}}$j$::jsonb,
   'ม.37(2)', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a04'::uuid, NULL, 'dsa.breach_notification', 'dsa',
   'การแจ้งเหตุละเมิดข้อมูล / Breach notification',
   $j${"title": "การแจ้งเหตุละเมิดข้อมูล", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ฝ่ายที่พบเหตุการละเมิดข้อมูลส่วนบุคคลที่เกี่ยวข้องกับข้อมูลตามข้อตกลงนี้ต้องแจ้งให้อีกฝ่ายทราบโดยไม่ชักช้า ตามมาตรา 37(4)"}]}]}}$j$::jsonb,
   $j${"title": "Breach notification", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] A party that discovers a personal data breach concerning data under this agreement shall notify the other party without delay, under section 37(4)."}]}]}}$j$::jsonb,
   'ม.37(4)', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a05'::uuid, NULL, 'dsa.dsar_support', 'dsa',
   'การรองรับคำขอใช้สิทธิ / Data subject rights support',
   $j${"title": "การรองรับคำขอใช้สิทธิ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ทั้งสองฝ่ายต้องให้ความช่วยเหลือซึ่งกันและกันในการตอบสนองต่อคำขอใช้สิทธิของเจ้าของข้อมูลส่วนบุคคลที่เกี่ยวข้องกับข้อมูลตามข้อตกลงนี้ ตามมาตรา 27"}]}]}}$j$::jsonb,
   $j${"title": "Data subject rights support", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] Both parties shall assist each other in responding to data subject rights requests concerning data under this agreement, under section 27."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a06'::uuid, NULL, 'dsa.retention_and_destruction', 'dsa',
   'ระยะเวลาเก็บและการทำลาย / Retention period and destruction',
   $j${"title": "ระยะเวลาเก็บและการทำลาย", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้รับข้อมูลต้องเก็บรักษาข้อมูลส่วนบุคคลไม่เกินระยะเวลาที่จำเป็นตามวัตถุประสงค์ และทำลายข้อมูลเมื่อครบกำหนด ตามมาตรา 27"}]}]}}$j$::jsonb,
   $j${"title": "Retention period and destruction", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Receiving Party shall retain personal data no longer than necessary for the stated purpose and destroy it once that period ends, under section 27."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a07'::uuid, NULL, 'dsa.onward_disclosure', 'dsa',
   'การส่งต่อ / Onward disclosure',
   $j${"title": "การส่งต่อ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] ผู้รับข้อมูลจะเปิดเผยข้อมูลส่วนบุคคลต่อไปยังบุคคลที่สามได้เฉพาะเมื่อได้รับความยินยอมล่วงหน้าจากผู้เปิดเผยข้อมูลและมีฐานทางกฎหมายรองรับ ตามมาตรา 27"}]}]}}$j$::jsonb,
   $j${"title": "Onward disclosure", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] The Receiving Party may further disclose the personal data to a third party only with the Disclosing Party's prior consent and a valid legal basis, under section 27."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a08'::uuid, NULL, 'dsa.cross_border_transfer', 'dsa',
   'การโอนข้อมูลไปต่างประเทศ / Cross-border transfer',
   $j${"title": "การโอนข้อมูลไปต่างประเทศ", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] หากมีการโอนข้อมูลส่วนบุคคลไปต่างประเทศภายใต้ข้อตกลงนี้ ผู้รับข้อมูลต้องปฏิบัติตามมาตรา 28 และ 29 และจัดให้มีมาตรการคุ้มครองที่เหมาะสม"}]}]}}$j$::jsonb,
   $j${"title": "Cross-border transfer of personal data", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] Where personal data is transferred abroad under this agreement, the Receiving Party shall comply with sections 28 and 29 and implement appropriate safeguards."}]}]}}$j$::jsonb,
   'ม.28-29', '{dsa}', true, 1, 'published'),
  ('00000000-0000-7000-8000-0000000d5a09'::uuid, NULL, 'dsa.termination', 'dsa',
   'การสิ้นสุดข้อตกลง / Termination',
   $j${"title": "การสิ้นสุดข้อตกลง", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[ร่าง — ต้องได้รับการทบทวนจากฝ่ายกฎหมายก่อนใช้งานจริง] เมื่อสิ้นสุดข้อตกลงนี้ ผู้รับข้อมูลต้องยุติการใช้ข้อมูลส่วนบุคคลและคืนหรือทำลายข้อมูลตามที่ตกลงกัน เว้นแต่กฎหมายกำหนดให้ต้องเก็บไว้ต่อไป"}]}]}}$j$::jsonb,
   $j${"title": "Termination", "doc": {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "[DRAFT — must be reviewed by Legal before real use] On termination of this agreement, the Receiving Party shall stop using the personal data and return or destroy it as agreed, unless retention is required by law."}]}]}}$j$::jsonb,
   'ม.27', '{dsa}', true, 1, 'published');

INSERT INTO agreement.mandatory_rules (agreement_type, clause_code, legal_ref, condition)
VALUES
  ('dsa', 'dsa.purpose_limitation', 'ม.27', NULL),
  ('dsa', 'dsa.no_excess_use', 'ม.27', NULL),
  ('dsa', 'dsa.security_measures', 'ม.37(2)', NULL),
  ('dsa', 'dsa.breach_notification', 'ม.37(4)', NULL),
  ('dsa', 'dsa.dsar_support', 'ม.27', NULL),
  ('dsa', 'dsa.retention_and_destruction', 'ม.27', NULL),
  ('dsa', 'dsa.onward_disclosure', 'ม.27', NULL),
  ('dsa', 'dsa.cross_border_transfer', 'ม.28-29', '{"requires_transfer": true}'::jsonb),
  ('dsa', 'dsa.termination', 'ม.27', NULL);

ALTER TABLE platform.clause_library FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.clause_library NO FORCE ROW LEVEL SECURITY;
DELETE FROM agreement.mandatory_rules WHERE agreement_type = 'dsa' AND clause_code LIKE 'dsa.%';
DELETE FROM platform.clause_library WHERE tenant_id IS NULL AND code LIKE 'dsa.%';
ALTER TABLE platform.clause_library FORCE ROW LEVEL SECURITY;
