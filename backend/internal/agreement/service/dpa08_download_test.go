package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestDownloadLog_RecordsEveryExport is DPA-08's own acceptance criterion: every export of a document
// (dpa/dsa agreements today, every PLT-16 doc type in general) logs who downloaded which version, in
// which format and language, through the one shared docs.Service.Export path.
func TestDownloadLog_RecordsEveryExport(t *testing.T) {
	e := setup(t, "dpa08download")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil)

	// Nothing downloaded yet.
	e.in(t, func(ctx context.Context) error {
		log, err := e.svc.Docs.DownloadLog(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		if len(log) != 0 {
			t.Fatalf("expected no downloads yet, got %+v", log)
		}
		return nil
	})

	// Export the still-draft agreement document as html, then as docx.
	e.in(t, func(ctx context.Context) error {
		_, _, err := e.svc.Docs.Export(ctx, a.DocumentID, nil, "th", "html")
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, _, err := e.svc.Docs.Export(ctx, a.DocumentID, nil, "th", "docx")
		return err
	})

	e.in(t, func(ctx context.Context) error {
		log, err := e.svc.Docs.DownloadLog(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		if len(log) != 2 {
			t.Fatalf("expected 2 recorded downloads, got %d: %+v", len(log), log)
		}
		// newest first
		if log[0].Format != "docx" || log[1].Format != "html" {
			t.Fatalf("expected newest-first ordering, got %+v", log)
		}
		for _, entry := range log {
			if entry.Language != "th" || entry.VersionNo != 1 || entry.UserID != e.tenant.UserID {
				t.Fatalf("unexpected download entry %+v", entry)
			}
		}
		return nil
	})

	// A failed export (unknown language) logs nothing.
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.Docs.Export(ctx, a.DocumentID, nil, "fr", "html"); err == nil {
			t.Fatal("expected an error for an unsupported language")
		}
		log, err := e.svc.Docs.DownloadLog(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		if len(log) != 2 {
			t.Fatalf("a failed export must not be logged, got %d entries", len(log))
		}
		return nil
	})
}

// TestDownloadLog_TwoTenantIsolation proves a tenant cannot read another tenant's own download history —
// DownloadLog goes through the same Get(ctx, id) RLS-scoped lookup every other docs.Service read uses.
func TestDownloadLog_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "dpa08isoA")
	e2 := setup(t, "dpa08isoB")

	var leID, vendorID uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e1)
		return nil
	})
	a := createAgreement(t, e1, leID, vendorID, nil)
	e1.in(t, func(ctx context.Context) error {
		_, _, err := e1.svc.Docs.Export(ctx, a.DocumentID, nil, "th", "html")
		return err
	})

	if err := e2.inAs(e2.tenant.UserID, permissions, nil, func(ctx context.Context) error {
		if _, err := e2.svc.Docs.DownloadLog(ctx, a.DocumentID); err == nil {
			t.Fatal("expected tenant B to be refused access to tenant A's document")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
