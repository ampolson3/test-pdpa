-- +goose Up
-- PLT-06 form & assessment engine.
-- * One editable draft version per form (published_at IS NULL); published versions are immutable.
-- * A submission can be an in-progress response ('draft') whose sections are assigned to different people
--   (platform.form_section_assignments); it becomes 'submitted' (validated and scored) once.
ALTER TABLE platform.form_submissions ADD COLUMN status text NOT NULL DEFAULT 'submitted' CHECK (status IN ('draft', 'submitted'));
ALTER TABLE platform.form_submissions ALTER COLUMN submitted_at DROP NOT NULL;
ALTER TABLE platform.form_submissions ADD COLUMN result jsonb;
CREATE UNIQUE INDEX ux_platform_form_versions_draft ON platform.form_versions (form_id) WHERE published_at IS NULL;
CREATE UNIQUE INDEX ux_platform_form_versions_no ON platform.form_versions (form_id, version_no);

CREATE TABLE platform.form_section_assignments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    submission_id uuid NOT NULL,
    section_key varchar(60) NOT NULL,
    assignee_user_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    row_version int NOT NULL DEFAULT 1,
    CONSTRAINT pk_platform_form_section_assignments PRIMARY KEY (id),
    CONSTRAINT uq_form_section_assignments UNIQUE (tenant_id, submission_id, section_key),
    CONSTRAINT fk_form_section_assignments_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id),
    CONSTRAINT fk_form_section_assignments_submission FOREIGN KEY (submission_id) REFERENCES platform.form_submissions (id) ON DELETE CASCADE,
    CONSTRAINT fk_form_section_assignments_assignee FOREIGN KEY (assignee_user_id) REFERENCES iam.users (id)
);
CREATE INDEX ix_platform_form_section_assignments_assignee ON platform.form_section_assignments (tenant_id, assignee_user_id) WHERE status = 'open';
ALTER TABLE platform.form_section_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.form_section_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.form_section_assignments
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER trg_form_section_assignments_updated BEFORE UPDATE ON platform.form_section_assignments
    FOR EACH ROW EXECUTE FUNCTION platform.set_updated_at();

ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
INSERT INTO platform.notification_templates (tenant_id, code, channel, language, subject, body, variables) VALUES
    (NULL, 'form.section_assigned', 'in_app', 'th', 'แบบฟอร์มรอคุณตอบ: {{.form}}', 'คุณได้รับมอบหมายให้ตอบส่วน "{{.section}}" ของ {{.form}}', '["form", "section"]'),
    (NULL, 'form.section_assigned', 'in_app', 'en', 'Form waiting for you: {{.form}}', 'You were asked to answer "{{.section}}" of {{.form}}', '["form", "section"]')
ON CONFLICT ON CONSTRAINT uq_notification_templates_code_channel_language DO NOTHING;
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY;
DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'form.section_assigned');
DELETE FROM platform.notification_templates WHERE tenant_id IS NULL AND code = 'form.section_assigned';
ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS platform.form_section_assignments;
DROP INDEX IF EXISTS platform.ux_platform_form_versions_no;
DROP INDEX IF EXISTS platform.ux_platform_form_versions_draft;
ALTER TABLE platform.form_submissions DROP COLUMN IF EXISTS result;
UPDATE platform.form_submissions SET submitted_at = coalesce(submitted_at, created_at);
ALTER TABLE platform.form_submissions ALTER COLUMN submitted_at SET NOT NULL;
ALTER TABLE platform.form_submissions DROP COLUMN IF EXISTS status;
