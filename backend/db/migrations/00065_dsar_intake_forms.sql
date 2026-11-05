-- +goose Up
-- DSAR-01 (เว็บฟอร์มขอใช้สิทธิสำหรับเจ้าของข้อมูล): the public, loginless web form a data subject uses to
-- submit a DSAR request directly, without an admin ever typing it in — the counterpart DSAR-02 already built
-- for staff-recorded channels (phone/branch/letter/LINE/email). dsar.requests/dsar.request_types (DSAR-13) and
-- the fixed-field CreateRequestInput shape are already exactly what a submission needs; what's missing is a
-- publishable, tenant-scoped "form instance" a public key can point at — the same role notice.notices plays
-- for PLT-16 documents and consent.collection_points plays for consent purposes. Deliberately not built on
-- PLT-06's generic form engine: DSAR intake fields are fixed by CreateRequestInput, not a configurable
-- question schema, so a dynamic form definition would duplicate structure this table already names directly.
CREATE TABLE dsar.intake_forms (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    public_key varchar(64),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_dsar_intake_forms PRIMARY KEY (id),
    CONSTRAINT fk_dsar_intake_forms_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id),
    CONSTRAINT fk_dsar_intake_forms_legal_entity FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id)
);
CREATE INDEX ix_dsar_intake_forms_tenant_id ON dsar.intake_forms (tenant_id);

ALTER TABLE dsar.intake_forms ENABLE ROW LEVEL SECURITY;
ALTER TABLE dsar.intake_forms FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dsar.intake_forms
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

COMMENT ON TABLE dsar.intake_forms IS 'DSAR-01: เว็บฟอร์มสาธารณะสำหรับเจ้าของข้อมูลยื่นคำขอใช้สิทธิเอง — เมื่อ publish แล้วจะมี public_key (platform.public_keys)';

-- Confirmation message sent right after a public submission (DSAR-01's own acceptance criterion: the
-- requester gets their request number immediately — here, also by the channel they gave at intake).
-- Operational text, no DRAFT marker (rule 8 is about legal/notice wording, not a plain receipt notice) —
-- same reasoning DSAR-08's own dsar.subtask_assigned template already used.
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'dsar.request_received', 'email', 'th', 'ได้รับคำขอใช้สิทธิของท่านแล้ว เลขที่ {{.request_no}}',
        'เรียนท่านผู้ยื่นคำขอ\n\nบริษัทได้รับคำขอใช้สิทธิของท่านแล้ว\nเลขที่คำขอ: {{.request_no}}\nประเภท: {{.request_type_name}}\n\nกรุณาเก็บเลขที่คำขอนี้ไว้สำหรับติดตามสถานะ บริษัทจะดำเนินการและติดต่อท่านกลับภายในระยะเวลาที่กฎหมายกำหนด',
        '["request_no", "request_type_name"]'),
    (NULL, 'dsar.request_received', 'email', 'en', 'We received your request, no. {{.request_no}}',
        'Dear requester,\n\nWe have received your data subject rights request.\nRequest no.: {{.request_no}}\nType: {{.request_type_name}}\n\nPlease keep this request number to track its status. We will process it and contact you within the legally required timeframe.',
        '["request_no", "request_type_name"]'),
    (NULL, 'dsar.request_received', 'sms', 'th', NULL,
        'ได้รับคำขอใช้สิทธิของท่านแล้ว เลขที่ {{.request_no}} กรุณาเก็บเลขที่นี้ไว้ติดตามสถานะ', '["request_no"]'),
    (NULL, 'dsar.request_received', 'sms', 'en', NULL,
        'We received your request no. {{.request_no}}. Keep this number to track its status.', '["request_no"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates
    WHERE tenant_id IS NULL AND code = 'dsar.request_received');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'dsar.request_received';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
DROP TABLE dsar.intake_forms;
