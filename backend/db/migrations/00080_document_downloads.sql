-- +goose Up
-- DPA-08 (เวอร์ชันและประวัติการดาวน์โหลด): built generically on PLT-16, the same reasoning PNG-06's own
-- version tracking already used — every document type (dpa/dsa agreements today, notices/letters later)
-- already shares one Export path (docs.Service.Export), so the download log lives here, not in the
-- agreement schema, and every sibling doc type gets the same acceptance criterion for free. Append-only
-- (no row_version/updated_at), the same shape ropa.activity_versions already established for an evidence
-- log nothing should ever edit after the fact.
CREATE TABLE platform.document_downloads (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    document_id uuid NOT NULL,
    version_no int NOT NULL,
    format text NOT NULL CHECK (format IN ('pdf', 'docx', 'html')),
    language text NOT NULL CHECK (language IN ('th', 'en')),
    user_id uuid NOT NULL,
    downloaded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_platform_document_downloads PRIMARY KEY (id)
);

ALTER TABLE platform.document_downloads ADD CONSTRAINT fk_document_downloads_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id);
ALTER TABLE platform.document_downloads ADD CONSTRAINT fk_document_downloads_document FOREIGN KEY (document_id) REFERENCES platform.documents (id);
ALTER TABLE platform.document_downloads ADD CONSTRAINT fk_document_downloads_user FOREIGN KEY (user_id) REFERENCES iam.users (id);

CREATE INDEX ix_platform_document_downloads_document ON platform.document_downloads (tenant_id, document_id, downloaded_at DESC);

ALTER TABLE platform.document_downloads ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.document_downloads
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down
DROP TABLE IF EXISTS platform.document_downloads;
