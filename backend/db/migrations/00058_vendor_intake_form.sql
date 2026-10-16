-- +goose Up
-- VEN-02 (docs/decisions.md Q-33): the inherent-risk intake questionnaire that computes a vendor's tier.
-- No schema change: platform.form_definitions/form_versions (00002) already allow form_type = 'intake' —
-- internal/wiring.Forms's own comment already named it "reserved but unregistered" — and vendor.intakes/
-- vendor.vendors.tier (00014) are already fully specified, unused until now. The scoring bands are keyed
-- exactly low/medium/high/critical so forms.Result.Band maps straight onto vendor.intakes.tier_result and
-- vendor.vendors.tier with no separate translation step (the same CHECK-matching move DPIA-01's own
-- screening bands used for assess.assessments.status).
--
-- Follows the same single-statement-per-INSERT workaround DPIA-01 (migration 00047) documented: a combined
-- WITH ... INSERT ... RETURNING ... UPDATE ... FROM CTE chain never sees a sibling CTE's own just-inserted
-- row under one query snapshot, so this uses fixed UUIDs and three separate top-level statements instead.
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;

INSERT INTO platform.form_definitions (id, tenant_id, code, name, form_type, status)
VALUES ('00000000-0000-4000-8000-000000580001', NULL, 'vendor_intake', 'แบบประเมินความเสี่ยงเบื้องต้นของคู่ค้า', 'intake', 'published');

INSERT INTO platform.form_versions (id, tenant_id, form_id, version_no, schema, scoring, languages, published_at)
VALUES ('00000000-0000-4000-8000-000000580002', NULL, '00000000-0000-4000-8000-000000580001', 1, $j${
        "sections": [
            {
                "key": "inherent_risk",
                "title": {"th": "ความเสี่ยงเบื้องต้น", "en": "Inherent risk"},
                "questions": [
                    {"key": "data_volume", "type": "single_choice", "required": true,
                     "label": {"th": "ปริมาณข้อมูลส่วนบุคคลที่คู่ค้าจะเข้าถึงหรือประมวลผล", "en": "Volume of personal data the vendor will access or process"},
                     "options": [
                        {"value": "small", "label": {"th": "น้อย (< 1,000 รายการ)", "en": "Small (< 1,000 records)"}, "score": 0},
                        {"value": "medium", "label": {"th": "ปานกลาง (1,000–100,000 รายการ)", "en": "Medium (1,000–100,000 records)"}, "score": 1},
                        {"value": "large", "label": {"th": "มาก (100,000–1,000,000 รายการ)", "en": "Large (100,000–1,000,000 records)"}, "score": 2},
                        {"value": "very_large", "label": {"th": "มากที่สุด (> 1,000,000 รายการ)", "en": "Very large (> 1,000,000 records)"}, "score": 3}
                     ]},
                    {"key": "sensitive_data", "type": "yes_no", "required": true,
                     "label": {"th": "คู่ค้าจะเข้าถึงข้อมูลส่วนบุคคลที่มีความอ่อนไหวตามมาตรา 26 หรือไม่", "en": "Will the vendor access sensitive personal data under section 26?"},
                     "options": [{"value": "yes", "score": 2}, {"value": "no", "score": 0}]},
                    {"key": "system_access_level", "type": "single_choice", "required": true,
                     "label": {"th": "ระดับการเข้าถึงระบบของคู่ค้า", "en": "Vendor's level of system access"},
                     "options": [
                        {"value": "none", "label": {"th": "ไม่มีการเข้าถึงระบบโดยตรง", "en": "No direct system access"}, "score": 0},
                        {"value": "read_only", "label": {"th": "อ่านข้อมูลอย่างเดียว", "en": "Read-only"}, "score": 1},
                        {"value": "read_write", "label": {"th": "อ่านและเขียนข้อมูล", "en": "Read and write"}, "score": 2},
                        {"value": "admin", "label": {"th": "สิทธิผู้ดูแลระบบ", "en": "Administrative access"}, "score": 3}
                     ]},
                    {"key": "cross_border_transfer", "type": "yes_no", "required": true,
                     "label": {"th": "การให้บริการมีการโอนข้อมูลส่วนบุคคลไปยังต่างประเทศหรือไม่", "en": "Does the engagement involve transferring personal data abroad?"},
                     "options": [{"value": "yes", "score": 2}, {"value": "no", "score": 0}]}
                ]
            }
        ]
    }$j$::jsonb, $j${
        "bands": [
            {"key": "low", "label": {"th": "ต่ำ", "en": "Low"}, "min": 0, "max": 2},
            {"key": "medium", "label": {"th": "ปานกลาง", "en": "Medium"}, "min": 3, "max": 5},
            {"key": "high", "label": {"th": "สูง", "en": "High"}, "min": 6, "max": 8},
            {"key": "critical", "label": {"th": "สูงมาก", "en": "Critical"}, "min": 9}
        ]
    }$j$::jsonb, ARRAY['th', 'en'], now());

UPDATE platform.form_definitions SET current_version_id = '00000000-0000-4000-8000-000000580002'
WHERE id = '00000000-0000-4000-8000-000000580001';

ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.form_definitions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions NO FORCE ROW LEVEL SECURITY;
UPDATE platform.form_definitions SET current_version_id = NULL WHERE id = '00000000-0000-4000-8000-000000580001';
DELETE FROM platform.form_versions WHERE id = '00000000-0000-4000-8000-000000580002';
DELETE FROM platform.form_definitions WHERE id = '00000000-0000-4000-8000-000000580001';
ALTER TABLE platform.form_definitions FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.form_versions FORCE ROW LEVEL SECURITY;
