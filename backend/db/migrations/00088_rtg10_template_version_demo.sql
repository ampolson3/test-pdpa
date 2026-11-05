-- +goose Up
-- RTG-10 (docs/decisions.md Q-43): "เวอร์ชัน template และแจ้งเมื่อกฎหมายเปลี่ยน" — a template's own version
-- history is simply a second ropa.activity_templates row sharing the same (template_set_id, code) with a
-- higher version_no, the identical mechanism RTG-01/02/03/07's own migrations already use to seed v1 rows
-- in the first place (rule 8: this is reviewed legal/business content, so issuing a new version is a
-- migration a legal reviewer signs off on, not a live admin action — see internal/ropa/service/
-- template_review.go's own package doc). This migration is RTG-10's own demonstration: no official new
-- PDPC announcement motivates this specific content, so it stands for a hypothetical one (an added
-- encryption-at-rest control for applicant data) flagged DRAFT pending legal review, the same "seed a
-- draft + flag it" move ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01/PNG-11/RTG-02/RTG-03/RTG-07 already made —
-- its only real purpose is to exercise the acceptance criterion end to end against real seeded data:
-- every activity any tenant already created from v1 of "recruitment_job_posting" (RTG-01, migration
-- 00049) now shows up in GET /admin/v1/ropa/activities/outdated-templates and gets one dpo.tasks review
-- task the next time ropa.template_review_check runs for that tenant.
--
-- Everything else about v1's own content stays unchanged except the one genuinely new item
-- (TECH_ENCRYPTION) — the unique index UNIQUE NULLS NOT DISTINCT (tenant_id, template_set_id, code,
-- version_no) already in the baseline migration (00008) is what makes version_no=2 a real, distinct row
-- rather than a conflict with v1's own version_no=1.
ALTER TABLE ropa.activity_templates NO FORCE ROW LEVEL SECURITY;

INSERT INTO ropa.activity_templates (template_set_id, code, name_th, name_en, job_category, role, defaults, rationale, legal_refs, version_no) VALUES
    ('00000000-0000-7000-8000-000000000001', 'recruitment_job_posting', 'ประกาศรับสมัครงานและคัดกรองใบสมัคร', 'Job posting and application screening', 'recruitment', 'controller',
     '{"description_th": "เผยแพร่ตำแหน่งงานว่างและคัดกรองคุณสมบัติผู้สมัครเบื้องต้น", "description_en": "Publish openings and screen applicant qualifications", "purposes": [{"purpose_text_th": "สรรหาและคัดเลือกบุคลากร", "purpose_text_en": "Recruit and select personnel", "lawful_basis_code": "CONTRACT"}], "data": [{"data_category_code": "identity", "subject_type_code": "job_applicant", "source": "direct"}, {"data_category_code": "contact", "subject_type_code": "job_applicant", "source": "direct"}, {"data_category_code": "education", "subject_type_code": "job_applicant", "source": "direct"}, {"data_category_code": "employment", "subject_type_code": "job_applicant", "source": "direct"}], "recipients": [{"recipient_role": "processor", "note_th": "ผู้ให้บริการระบบสรรหาบุคลากร (ATS) ภายนอก"}], "retention": [{"retention_months": 12, "retention_basis_th": "เก็บไว้พิจารณาตำแหน่งอื่นที่เหมาะสมภายในระยะเวลาที่แจ้งผู้สมัคร", "trigger_event_th": "วันที่ปิดรับสมัครหรือสิ้นสุดกระบวนการ", "disposal_method": "delete"}], "security_controls": ["AC_ROLE_BASED", "TECH_ACCESS_LOG", "TECH_ENCRYPTION"]}'::jsonb,
     '{"purpose_th": "ฐานทางกฎหมายที่เลือกสอดคล้องกับลักษณะความสัมพันธ์และวัตถุประสงค์ของกิจกรรม \"ประกาศรับสมัครงานและคัดกรองใบสมัคร\" ตามมาตรา 24/26", "retention_th": "ระยะเวลาเก็บรักษาอ้างอิงแนวปฏิบัติทั่วไปของกิจกรรมประเภทนี้ โปรดปรับตามนโยบายและข้อกำหนดกฎหมายเฉพาะของแต่ละองค์กร", "security_th": "เวอร์ชันนี้เพิ่มมาตรการเข้ารหัสข้อมูลผู้สมัคร (TECH_ENCRYPTION) ตามแนวปฏิบัติที่ปรับปรุงใหม่", "note_th": "ค่าตั้งต้นนี้เป็นร่างอ้างอิงทั่วไป ยังไม่ได้รับการตรวจสอบจากฝ่ายกฎหมาย โปรดทบทวนก่อนใช้งานจริง"}'::jsonb,
     ARRAY['ม.39(1)','ม.39(4)'], 2);

ALTER TABLE ropa.activity_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
DELETE FROM ropa.activity_templates
WHERE template_set_id = '00000000-0000-7000-8000-000000000001' AND code = 'recruitment_job_posting' AND version_no = 2;
