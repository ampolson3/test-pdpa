-- +goose Up
-- CON-12/09/10/15/13: purposes versioned through PLT-08, collection points with a public key and a s.19 publish
-- checklist, receipts hash-chained per data subject.
-- * purposes.data_category_codes: the org.data_categories the purpose uses (a sensitive one makes the purpose need
--   explicit consent, s.26). consent.data_elements stays for the field-level mapping RoPA brings later.
-- * collection_points.public_key: the platform.public_keys key issued on publish (revoked on retire).
ALTER TABLE consent.purposes ADD COLUMN data_category_codes text[] NOT NULL DEFAULT '{}';
ALTER TABLE consent.purposes ADD COLUMN description_th text;
ALTER TABLE consent.purposes ADD COLUMN description_en text;
-- CON-10: the specific statement shown with a sensitive purpose's own checkbox (s.26 explicit consent)
ALTER TABLE consent.purpose_versions ADD COLUMN explicit_text_th text;
ALTER TABLE consent.purpose_versions ADD COLUMN explicit_text_en text;

ALTER TABLE consent.collection_points ADD COLUMN public_key varchar(64);
ALTER TABLE consent.collection_points ADD COLUMN allowed_origins text[] NOT NULL DEFAULT '{}';
ALTER TABLE consent.collection_points ADD COLUMN publish_checklist jsonb;
ALTER TABLE consent.collection_points ADD COLUMN published_at timestamptz;
ALTER TABLE consent.collection_points ADD COLUMN published_by uuid;
ALTER TABLE consent.collection_points ADD CONSTRAINT fk_collection_points_published_by FOREIGN KEY (published_by) REFERENCES iam.users (id);

-- the chain head of a subject: newest receipt first
CREATE INDEX ix_consent_receipts_subject_chain ON consent.consent_receipts (tenant_id, subject_id, occurred_at DESC, id DESC);
-- a subject's history on the partitioned consent_transactions: migration 00035 (Go, rule 13)

ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'consent.receipt', 'email', 'th', 'หลักฐานการให้ความยินยอม {{.receipt_no}}',
     'บันทึกการตัดสินใจของคุณแล้ว เลขที่หลักฐาน {{.receipt_no}} ({{.occurred_at}})\n\n{{.decisions}}\n\nคุณเปลี่ยนใจหรือถอนความยินยอมได้ทุกเมื่อผ่านช่องทางขององค์กร', '["receipt_no", "occurred_at", "decisions"]'),
    (NULL, 'consent.receipt', 'email', 'en', 'Your consent receipt {{.receipt_no}}',
     'We recorded your choices. Receipt {{.receipt_no}} ({{.occurred_at}})\n\n{{.decisions}}\n\nYou can change your mind or withdraw consent at any time through the organization''s channels.', '["receipt_no", "occurred_at", "decisions"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'consent.receipt');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'consent.receipt';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS consent.ix_consent_receipts_subject_chain;
ALTER TABLE consent.collection_points DROP CONSTRAINT IF EXISTS fk_collection_points_published_by;
ALTER TABLE consent.collection_points DROP COLUMN IF EXISTS published_by;
ALTER TABLE consent.collection_points DROP COLUMN IF EXISTS published_at;
ALTER TABLE consent.collection_points DROP COLUMN IF EXISTS publish_checklist;
ALTER TABLE consent.collection_points DROP COLUMN IF EXISTS allowed_origins;
ALTER TABLE consent.collection_points DROP COLUMN IF EXISTS public_key;
ALTER TABLE consent.purpose_versions DROP COLUMN IF EXISTS explicit_text_en;
ALTER TABLE consent.purpose_versions DROP COLUMN IF EXISTS explicit_text_th;
ALTER TABLE consent.purposes DROP COLUMN IF EXISTS description_en;
ALTER TABLE consent.purposes DROP COLUMN IF EXISTS description_th;
ALTER TABLE consent.purposes DROP COLUMN IF EXISTS data_category_codes;
