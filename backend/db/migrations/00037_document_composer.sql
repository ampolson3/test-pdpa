-- +goose Up
-- PLT-16 document composer: what the composer needs beyond the SA tables.
-- * documents.legal_entity_id: the legal entity whose details fill the org_* merge fields (ORG-01).
-- * document_versions: one published version is rendered in each of its languages, so the English PDF / DOCX get
--   their own columns next to the (Thai) pdf_file_id / docx_file_id; render_status tracks the docs.render job
--   (pending → done | failed) so the UI knows when the files exist.
-- * clause_library / templates: at most one draft per code (a new version is drafted after the last published one),
--   and indexes for the "latest version per code" listings.
ALTER TABLE platform.documents ADD COLUMN legal_entity_id uuid;
ALTER TABLE platform.documents ADD CONSTRAINT fk_documents_legal_entity_id FOREIGN KEY (legal_entity_id) REFERENCES org.legal_entities (id);
CREATE INDEX ix_platform_documents_legal_entity_id ON platform.documents (tenant_id, legal_entity_id);
CREATE INDEX ix_platform_documents_type_updated ON platform.documents (tenant_id, doc_type, updated_at DESC, id);

ALTER TABLE platform.document_versions ADD COLUMN pdf_en_file_id uuid;
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_pdf_en_file_id FOREIGN KEY (pdf_en_file_id) REFERENCES platform.files (id);
ALTER TABLE platform.document_versions ADD COLUMN docx_en_file_id uuid;
ALTER TABLE platform.document_versions ADD CONSTRAINT fk_document_versions_docx_en_file_id FOREIGN KEY (docx_en_file_id) REFERENCES platform.files (id);
ALTER TABLE platform.document_versions ADD COLUMN render_status text NOT NULL DEFAULT 'pending'
    CHECK (render_status IN ('pending', 'done', 'failed'));

CREATE UNIQUE INDEX uq_clause_library_one_draft ON platform.clause_library (tenant_id, code) NULLS NOT DISTINCT WHERE status = 'draft';
CREATE INDEX ix_platform_clause_library_code ON platform.clause_library (tenant_id, code, version_no DESC);
CREATE UNIQUE INDEX uq_templates_one_draft ON platform.templates (tenant_id, template_type, code) NULLS NOT DISTINCT WHERE status = 'draft';
CREATE INDEX ix_platform_templates_type_code ON platform.templates (tenant_id, template_type, code, version_no DESC);

-- +goose Down
DROP INDEX IF EXISTS platform.ix_platform_templates_type_code;
DROP INDEX IF EXISTS platform.uq_templates_one_draft;
DROP INDEX IF EXISTS platform.ix_platform_clause_library_code;
DROP INDEX IF EXISTS platform.uq_clause_library_one_draft;
ALTER TABLE platform.document_versions DROP COLUMN IF EXISTS render_status;
ALTER TABLE platform.document_versions DROP CONSTRAINT IF EXISTS fk_document_versions_docx_en_file_id;
ALTER TABLE platform.document_versions DROP COLUMN IF EXISTS docx_en_file_id;
ALTER TABLE platform.document_versions DROP CONSTRAINT IF EXISTS fk_document_versions_pdf_en_file_id;
ALTER TABLE platform.document_versions DROP COLUMN IF EXISTS pdf_en_file_id;
DROP INDEX IF EXISTS platform.ix_platform_documents_type_updated;
DROP INDEX IF EXISTS platform.ix_platform_documents_legal_entity_id;
ALTER TABLE platform.documents DROP CONSTRAINT IF EXISTS fk_documents_legal_entity_id;
ALTER TABLE platform.documents DROP COLUMN IF EXISTS legal_entity_id;
