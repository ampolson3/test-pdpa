// PNG-13 (ลิงก์เอกสารอ้างอิงในประกาศ): notice.linked_documents was already fully specified in the baseline
// migrations (label, url, file_id, display_mode inline/popup/link) — unused until now, no new migration.
package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// LinkedDocumentType is the PLT-09 entity type of a linked document's own attached file (when file_id, not
// url, is the source) — one per linked-document row, the same per-row attachment shape PNG-04's own
// evidence file uses.
const LinkedDocumentType = "notice_linked_document"

var linkedDocumentDisplayModes = []string{"inline", "popup", "link"}

// LinkedDocument is one reference document linked from a notice (a retention schedule, a security policy,
// or anything else LEGAL wants to point to) — either a plain URL or the caller's own attached PLT-09 file,
// never both, shown per DisplayMode.
type LinkedDocument struct {
	ID          uuid.UUID
	NoticeID    uuid.UUID
	Label       string
	URL         *string
	FileID      *uuid.UUID
	DisplayMode string
	RowVersion  int32
	CreatedAt   time.Time
}

// LinkedDocumentInput is a new link: exactly one of URL/FileID.
type LinkedDocumentInput struct {
	Label       string
	URL         *string
	FileID      *uuid.UUID
	DisplayMode string
}

func toLinkedDocument(r noticestore.ListLinkedDocumentsRow) LinkedDocument {
	return LinkedDocument{
		ID: r.ID, NoticeID: r.NoticeID, Label: r.Label, URL: r.Url, FileID: uuidPtr(r.FileID),
		DisplayMode: r.DisplayMode, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}

// ListLinkedDocuments is both the admin read side and PNG-06's own public-page read (no separate "Public"
// method needed — this service has no internal permission gate of its own, same as every other notice read;
// the HTTP layer's x-permission on the admin route is the only gate, and the public handler only calls this
// once PublicNotice has already proven the notice is published and the key names it).
func (s *Service) ListLinkedDocuments(ctx context.Context, noticeID uuid.UUID) ([]LinkedDocument, error) {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return nil, err
	}
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListLinkedDocuments(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	out := make([]LinkedDocument, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLinkedDocument(noticestore.ListLinkedDocumentsRow(r)))
	}
	return out, nil
}

// AddLinkedDocument links a reference document to a notice: a plain URL, or the caller's own clean,
// still-unattached PLT-09 upload (the same "caller's own clean upload" pattern PNG-04's evidence file and
// DSAR-04's agent document already use) — never both.
func (s *Service) AddLinkedDocument(ctx context.Context, noticeID uuid.UUID, in LinkedDocumentInput) (LinkedDocument, error) {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return LinkedDocument{}, err
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len([]rune(in.Label)) > 300 {
		return LinkedDocument{}, fmt.Errorf("%w: label", ErrInvalid)
	}
	if !slices.Contains(linkedDocumentDisplayModes, in.DisplayMode) {
		return LinkedDocument{}, fmt.Errorf("%w: display_mode", ErrInvalid)
	}
	hasURL := in.URL != nil && strings.TrimSpace(*in.URL) != ""
	hasFile := in.FileID != nil && *in.FileID != uuid.Nil
	if hasURL == hasFile {
		return LinkedDocument{}, fmt.Errorf("%w: exactly one of url or file_id is required", ErrInvalid)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return LinkedDocument{}, err
	}
	if hasFile {
		if s.Files == nil {
			return LinkedDocument{}, fmt.Errorf("%w: file_id", ErrInvalid)
		}
		if _, err := s.Files.Get(ctx, *in.FileID); err != nil {
			return LinkedDocument{}, fmt.Errorf("%w: file_id", ErrInvalid)
		}
		if err := s.Files.AttachSystem(ctx, *in.FileID, LinkedDocumentType, id); err != nil {
			return LinkedDocument{}, fmt.Errorf("%w: file_id", ErrInvalid)
		}
	}

	var url *string
	if hasURL {
		trimmed := strings.TrimSpace(*in.URL)
		url = &trimmed
	}
	row, err := noticestore.New(pdb.MustTxFromContext(ctx)).InsertLinkedDocument(ctx, noticestore.InsertLinkedDocumentParams{
		ID: id, NoticeID: noticeID, Label: in.Label, Url: url, FileID: pgUUID(in.FileID), DisplayMode: in.DisplayMode,
	})
	if err != nil {
		return LinkedDocument{}, err
	}
	out := toLinkedDocument(noticestore.ListLinkedDocumentsRow(row))
	if err := s.audit(ctx, "notice.linked_document.add", noticeID, nil, map[string]any{"label": out.Label, "display_mode": out.DisplayMode}); err != nil {
		return LinkedDocument{}, err
	}
	return out, nil
}

// RemoveLinkedDocument unlinks a reference document from a notice.
func (s *Service) RemoveLinkedDocument(ctx context.Context, noticeID, id uuid.UUID) error {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return err
	}
	n, err := noticestore.New(pdb.MustTxFromContext(ctx)).DeleteLinkedDocument(ctx, noticestore.DeleteLinkedDocumentParams{ID: id, NoticeID: noticeID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "notice.linked_document.remove", noticeID, nil, map[string]any{"linked_document_id": id})
}

// PublicLinkedDocumentURL resolves a linked document's own URL for the public page (PNG-13's own display):
// the stored url, or a short-lived signed link to the attached file — the same no-permission reasoning
// PublicVersionHTML/PublicVersionFile already use, since the caller already proved this notice is published.
func (s *Service) PublicLinkedDocumentURL(ctx context.Context, ld LinkedDocument) (string, error) {
	if ld.URL != nil {
		return *ld.URL, nil
	}
	if ld.FileID != nil && s.Files != nil {
		return s.Files.SignedURL(ctx, *ld.FileID)
	}
	return "", ErrInvalid
}
