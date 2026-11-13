-- +goose Up
-- ROPA-09 (ม.37(1); ประกาศมาตรการความปลอดภัย พ.ศ. 2565): every processing activity must reference the
-- security measures applied to it. risk.controls (baseline migration 00010, comment: "คลังมาตรการ / control
-- (ประกาศมาตรการความปลอดภัย, ISO)") is the catalog ropa.activity_controls.control_id already points at — no
-- RRA feature owns seeding it yet (RRA-06 is P2 and only links controls to *risks*, not this catalog), so this
-- migration seeds a DRAFT global catalog now, the same "seed a draft + flag for legal review" move ORG-07 made
-- for its master data (docs/decisions.md Q-20; this one is Q-25). Global rows (tenant_id NULL) need RLS force
-- lifted inside this migration, as in 00019/00031 (restored before commit).

ALTER TABLE risk.controls NO FORCE ROW LEVEL SECURITY;

INSERT INTO risk.controls (code, name, category, description, framework_refs) VALUES
    ('ORG_POLICY', 'นโยบายและมาตรการคุ้มครองข้อมูลส่วนบุคคลระดับองค์กร', 'organizational',
        'กำหนด นำไปปฏิบัติ และทบทวนนโยบายคุ้มครองข้อมูลส่วนบุคคลเป็นลายลักษณ์อักษร', ARRAY['ม.37(1)']),
    ('ORG_TRAINING', 'การฝึกอบรมและสร้างความตระหนักรู้แก่พนักงาน', 'organizational',
        'อบรมพนักงานและผู้ที่เกี่ยวข้องให้ตระหนักถึงหน้าที่ตาม พ.ร.บ. คุ้มครองข้อมูลส่วนบุคคล', ARRAY['ม.37(1)']),
    ('ORG_INCIDENT_PLAN', 'แผนรองรับเหตุการละเมิดข้อมูลส่วนบุคคล', 'organizational',
        'กำหนดขั้นตอนตรวจจับ รายงาน และตอบสนองต่อเหตุการละเมิดข้อมูลส่วนบุคคล', ARRAY['ม.37(1)', 'ม.37(4)']),
    ('TECH_ENCRYPTION', 'การเข้ารหัสข้อมูลส่วนบุคคล', 'technical',
        'เข้ารหัสข้อมูลส่วนบุคคลทั้งขณะจัดเก็บและระหว่างส่งผ่านเครือข่าย', ARRAY['ม.37(1)']),
    ('TECH_ACCESS_LOG', 'การบันทึกและติดตามการเข้าถึงข้อมูล (audit log)', 'technical',
        'บันทึกการเข้าถึง แก้ไข และลบข้อมูลส่วนบุคคลเพื่อตรวจสอบย้อนหลังได้', ARRAY['ม.37(1)']),
    ('TECH_BACKUP', 'การสำรองและกู้คืนข้อมูล', 'technical',
        'สำรองข้อมูลส่วนบุคคลสม่ำเสมอและทดสอบการกู้คืนเพื่อป้องกันการสูญหาย', ARRAY['ม.37(1)']),
    ('TECH_MALWARE', 'การป้องกันมัลแวร์และการโจมตีทางไซเบอร์', 'technical',
        'ติดตั้งและปรับปรุงระบบป้องกันมัลแวร์ ไฟร์วอลล์ และแพตช์ความปลอดภัยอย่างสม่ำเสมอ', ARRAY['ม.37(1)']),
    ('PHYS_SITE_ACCESS', 'การควบคุมการเข้า-ออกสถานที่จัดเก็บข้อมูล', 'physical',
        'จำกัดการเข้าถึงสถานที่จัดเก็บเซิร์ฟเวอร์และเอกสารที่มีข้อมูลส่วนบุคคล', ARRAY['ม.37(1)']),
    ('PHYS_DOC_DISPOSAL', 'การทำลายเอกสาร/สื่อบันทึกข้อมูลอย่างปลอดภัย', 'physical',
        'ทำลายเอกสารและสื่อบันทึกข้อมูลส่วนบุคคลด้วยวิธีที่ไม่สามารถกู้คืนได้เมื่อหมดความจำเป็น', ARRAY['ม.37(1)']),
    ('AC_ROLE_BASED', 'การกำหนดสิทธิ์การเข้าถึงตามหน้าที่ (role-based access control)', 'access_control',
        'จำกัดสิทธิ์การเข้าถึงข้อมูลส่วนบุคคลเฉพาะผู้ที่มีหน้าที่เกี่ยวข้องตามหลัก need-to-know', ARRAY['ม.37(1)']),
    ('AC_AUTHENTICATION', 'การยืนยันตัวตนก่อนเข้าถึงระบบ', 'access_control',
        'บังคับใช้รหัสผ่านที่รัดกุมและการยืนยันตัวตนหลายปัจจัยสำหรับระบบที่เข้าถึงข้อมูลส่วนบุคคล', ARRAY['ม.37(1)']),
    ('AC_REVIEW', 'การทบทวนสิทธิ์การเข้าถึงเป็นระยะ', 'access_control',
        'ทบทวนและเพิกถอนสิทธิ์การเข้าถึงข้อมูลส่วนบุคคลเมื่อพนักงานเปลี่ยนหน้าที่หรือพ้นสภาพ', ARRAY['ม.37(1)']);

ALTER TABLE risk.controls FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE risk.controls NO FORCE ROW LEVEL SECURITY;
DELETE FROM risk.controls WHERE tenant_id IS NULL AND code IN (
    'ORG_POLICY', 'ORG_TRAINING', 'ORG_INCIDENT_PLAN', 'TECH_ENCRYPTION', 'TECH_ACCESS_LOG', 'TECH_BACKUP',
    'TECH_MALWARE', 'PHYS_SITE_ACCESS', 'PHYS_DOC_DISPOSAL', 'AC_ROLE_BASED', 'AC_AUTHENTICATION', 'AC_REVIEW'
);
ALTER TABLE risk.controls FORCE ROW LEVEL SECURITY;
