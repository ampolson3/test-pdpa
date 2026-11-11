-- +goose Up
-- DFG-09 (Data discovery): dataflow.classifiers already had a tenant_id-nullable "global default" shape
-- (the same org.countries/ORG-07 master-data pattern) since the baseline migration, unused until now.
-- Seeds the platform's own Thai classifiers — the module doc's own literal "ตรวจจับข้อมูลแบบไทย (เลขบัตร
-- 13 หลัก เบอร์โทรไทย ชื่อภาษาไทย)". thai_national_id is a real mod-11 checksum (internal/org/service's
-- own ValidThaiID, reused here); thai_phone is a plain regex; thai_name is a deliberately simplified
-- Thai-script heuristic (no real name dictionary/ML model is available in this environment — the same
-- "practical mechanism instead of unreachable infra" move CON-04's own chromedp crawler already made),
-- flagged for later replacement with a real dictionary/ML classifier. Global rows (tenant_id NULL) need
-- RLS force lifted inside this migration, as in 00019/00031/00040 (restored before commit).

ALTER TABLE dataflow.classifiers NO FORCE ROW LEVEL SECURITY;

INSERT INTO dataflow.classifiers (tenant_id, code, name, classifier_type, pattern, is_active) VALUES
    (NULL, 'thai_national_id', 'เลขบัตรประชาชน / เลขทะเบียนนิติบุคคล (13 หลัก)', 'checksum', '^[0-9]{13}$', true),
    (NULL, 'thai_phone', 'เบอร์โทรศัพท์ไทย', 'regex', '^(0[689][0-9]{8}|\+66[689][0-9]{8})$', true),
    (NULL, 'thai_name', 'ชื่อ-นามสกุลภาษาไทย (heuristic — ร่าง, ยังไม่มีพจนานุกรมจริง)', 'dictionary', NULL, true);

ALTER TABLE dataflow.classifiers FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE dataflow.classifiers NO FORCE ROW LEVEL SECURITY;
DELETE FROM dataflow.classifiers WHERE tenant_id IS NULL AND code IN ('thai_national_id', 'thai_phone', 'thai_name');
ALTER TABLE dataflow.classifiers FORCE ROW LEVEL SECURITY;
