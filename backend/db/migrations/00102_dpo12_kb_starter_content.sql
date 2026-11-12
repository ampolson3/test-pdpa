-- +goose Up
-- DPO-12 (Knowledge base): dpo.kb_articles already carries a tenant_id-nullable "global" shape (the
-- ORG-07 master-data pattern). Seeds a small starter set of platform-wide articles so a fresh tenant's
-- knowledge base is not empty. Wording is a plain-language DRAFT paraphrase pending legal review
-- (docs/decisions.md Q-48, rule 8) — every title carries the "[ร่าง]" marker. Global rows need RLS
-- force lifted inside this migration, as in 00019/00031/00040.

ALTER TABLE dpo.kb_articles NO FORCE ROW LEVEL SECURITY;

INSERT INTO dpo.kb_articles (tenant_id, category, title, body, language, tags, source, status, version_no, published_at) VALUES
    (NULL, 'pdpa_basics', '[ร่าง] พ.ร.บ.คุ้มครองข้อมูลส่วนบุคคล พ.ศ. 2562 คืออะไร',
     '{"text": "พ.ร.บ.คุ้มครองข้อมูลส่วนบุคคล พ.ศ. 2562 (PDPA) กำหนดหลักเกณฑ์การเก็บ ใช้ และเปิดเผยข้อมูลส่วนบุคคล เพื่อคุ้มครองสิทธิของเจ้าของข้อมูล องค์กรที่เป็นผู้ควบคุมหรือผู้ประมวลผลข้อมูลต้องมีฐานทางกฎหมาย แจ้งวัตถุประสงค์ และรักษาความมั่นคงปลอดภัยของข้อมูล"}'::jsonb,
     'th', ARRAY['pdpa','พื้นฐาน'], 'law', 'published', 1, now()),
    (NULL, 'rights', '[ร่าง] เจ้าของข้อมูลมีสิทธิอะไรบ้าง',
     '{"text": "เจ้าของข้อมูลมีสิทธิขอเข้าถึงและรับสำเนา ขอให้โอนย้ายข้อมูล คัดค้านการประมวลผล ขอให้ลบหรือทำลาย ขอให้ระงับการใช้ ขอให้แก้ไขข้อมูลให้ถูกต้อง และถอนความยินยอมได้ตลอดเวลา องค์กรต้องตอบคำขอภายใน 30 วันนับแต่ได้รับคำขอ"}'::jsonb,
     'th', ARRAY['สิทธิ','dsar'], 'law', 'published', 1, now()),
    (NULL, 'consent', '[ร่าง] ความยินยอมที่สมบูรณ์ต้องมีอะไรบ้าง',
     '{"text": "ความยินยอมต้องขอก่อนหรือขณะเก็บข้อมูล แยกจากข้อความอื่น ระบุวัตถุประสงค์ชัดเจน ใช้ภาษาที่เข้าใจง่าย และเจ้าของข้อมูลต้องถอนความยินยอมได้ง่ายเท่ากับตอนให้ ห้ามติ๊กช่องยินยอมไว้ล่วงหน้า"}'::jsonb,
     'th', ARRAY['ความยินยอม','consent'], 'law', 'published', 1, now()),
    (NULL, 'breach', '[ร่าง] เกิดเหตุละเมิดข้อมูลส่วนบุคคลต้องทำอย่างไร',
     '{"text": "เมื่อทราบเหตุละเมิดที่มีความเสี่ยงต่อสิทธิและเสรีภาพของบุคคล ต้องแจ้งสำนักงานคณะกรรมการคุ้มครองข้อมูลส่วนบุคคลโดยไม่ชักช้าภายใน 72 ชั่วโมงนับแต่ทราบเหตุ หากมีความเสี่ยงสูงต้องแจ้งเจ้าของข้อมูลพร้อมแนวทางเยียวยาด้วย"}'::jsonb,
     'th', ARRAY['ละเมิดข้อมูล','72 ชั่วโมง'], 'law', 'published', 1, now()),
    (NULL, 'faq', '[ร่าง] RoPA คืออะไรและต้องจัดทำเมื่อไร',
     '{"question": "RoPA คืออะไร", "answer": "RoPA คือบันทึกรายการกิจกรรมการประมวลผลข้อมูลส่วนบุคคลตามมาตรา 39 ผู้ควบคุมข้อมูลต้องบันทึกอย่างน้อยรายการข้อมูลที่เก็บ วัตถุประสงค์ ผู้รับข้อมูล ระยะเวลาเก็บรักษา และมาตรการรักษาความปลอดภัย เว้นแต่เข้าข้อยกเว้นกิจการขนาดเล็ก"}'::jsonb,
     'th', ARRAY['ropa','ม.39'], 'faq', 'published', 1, now()),
    (NULL, 'faq', '[ร่าง] DPIA ต้องทำเมื่อไร',
     '{"question": "เมื่อไรต้องทำ DPIA", "answer": "ควรทำเมื่อกิจกรรมมีความเสี่ยงสูงต่อสิทธิของบุคคล เช่น ประมวลผลข้อมูลอ่อนไหวจำนวนมาก ติดตามพฤติกรรมอย่างเป็นระบบ ตัดสินใจอัตโนมัติ หรือใช้เทคโนโลยีใหม่ ระบบคัดกรองในเมนู DPIA ช่วยประเมินเบื้องต้น"}'::jsonb,
     'th', ARRAY['dpia','ความเสี่ยง'], 'faq', 'published', 1, now());

ALTER TABLE dpo.kb_articles FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE dpo.kb_articles NO FORCE ROW LEVEL SECURITY;
DELETE FROM dpo.kb_articles WHERE tenant_id IS NULL AND title LIKE '[ร่าง]%';
ALTER TABLE dpo.kb_articles FORCE ROW LEVEL SECURITY;
