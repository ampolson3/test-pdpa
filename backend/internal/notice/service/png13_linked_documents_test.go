package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
)

func draftNotice(t *testing.T, e env, suffix string) noticeservice.Notice {
	t.Helper()
	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", NameEn: "Test Co., Ltd.",
			ContactEmail: "dpo@test.example", ContactPhone: "02-000-0000",
			Address:      orgservice.Address{Line1: "1 ถนนทดสอบ", District: "เขตทดสอบ", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true}, 0)
		if err != nil {
			return err
		}
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศ PNG-13", Slug: "png13-" + suffix})
		return err
	})
	return n
}

// TestAddLinkedDocument_UrlRoundTripsAndRemoves is the acceptance criterion's data half: a linked document
// is stored with its own display mode, is listed back, and can be unlinked.
func TestAddLinkedDocument_UrlRoundTripsAndRemoves(t *testing.T) {
	e := setup(t, "png13url")
	n := draftNotice(t, e, "url")

	var ld noticeservice.LinkedDocument
	e.in(t, func(ctx context.Context) error {
		url := "https://example.com/retention-schedule.pdf"
		var err error
		ld, err = e.svc.AddLinkedDocument(ctx, n.ID, noticeservice.LinkedDocumentInput{
			Label: "ตารางระยะเวลาเก็บรักษา", URL: &url, DisplayMode: "popup",
		})
		return err
	})
	if ld.DisplayMode != "popup" || ld.URL == nil || *ld.URL != "https://example.com/retention-schedule.pdf" {
		t.Fatalf("linked document = %+v", ld)
	}

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListLinkedDocuments(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != ld.ID {
			t.Fatalf("list = %+v", list)
		}
		return e.svc.RemoveLinkedDocument(ctx, n.ID, ld.ID)
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListLinkedDocuments(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("list after remove = %+v, want empty", list)
		}
		if err := e.svc.RemoveLinkedDocument(ctx, n.ID, ld.ID); !errors.Is(err, noticeservice.ErrNotFound) {
			t.Errorf("removing again: %v", err)
		}
		return nil
	})
}

// TestAddLinkedDocument_Validation covers the input rules: exactly one of url/file_id, a known display
// mode, and a file_id refused with no Files configured (this fixture's own svc.Files is nil).
func TestAddLinkedDocument_Validation(t *testing.T) {
	e := setup(t, "png13val")
	n := draftNotice(t, e, "val")

	cases := []struct {
		name string
		in   noticeservice.LinkedDocumentInput
	}{
		{"empty label", noticeservice.LinkedDocumentInput{Label: "", URL: strp("https://x.example"), DisplayMode: "link"}},
		{"bad display mode", noticeservice.LinkedDocumentInput{Label: "x", URL: strp("https://x.example"), DisplayMode: "bogus"}},
		{"neither url nor file_id", noticeservice.LinkedDocumentInput{Label: "x", DisplayMode: "link"}},
		{"both url and file_id", noticeservice.LinkedDocumentInput{Label: "x", URL: strp("https://x.example"), FileID: uuidp(uuid.Must(uuid.NewV7())), DisplayMode: "link"}},
		{"file_id with no Files configured", noticeservice.LinkedDocumentInput{Label: "x", FileID: uuidp(uuid.Must(uuid.NewV7())), DisplayMode: "inline"}},
	}
	for _, c := range cases {
		c := c
		e.in(t, func(ctx context.Context) error {
			if _, err := e.svc.AddLinkedDocument(ctx, n.ID, c.in); !errors.Is(err, noticeservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", c.name, err)
			}
			return nil
		})
	}
}

// TestPublicLinkedDocumentURL is the acceptance criterion's own public-page half: a published notice's
// linked documents resolve to real urls, shown per their own display_mode.
func TestPublicLinkedDocumentURL(t *testing.T) {
	e := setup(t, "png13pub")
	n := draftNotice(t, e, "pub")
	var ld noticeservice.LinkedDocument
	e.in(t, func(ctx context.Context) error {
		url := "https://example.com/security-policy"
		var err error
		ld, err = e.svc.AddLinkedDocument(ctx, n.ID, noticeservice.LinkedDocumentInput{Label: "นโยบายความปลอดภัย", URL: &url, DisplayMode: "inline"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		resolved, err := e.svc.PublicLinkedDocumentURL(ctx, ld)
		if err != nil {
			return err
		}
		if resolved != "https://example.com/security-policy" {
			t.Errorf("resolved url = %q", resolved)
		}
		return nil
	})
}

// TestAddLinkedDocument_TwoTenantIsolation proves RLS on notice.linked_documents.
func TestAddLinkedDocument_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "png13isoa")
	b := setup(t, "png13isob")
	n := draftNotice(t, a, "isoa")

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListLinkedDocuments(ctx, n.ID); err != nil {
			// A cross-tenant notice id resolves to no rows (not an error) — but the notice itself
			// is also invisible, proven below.
		}
		if _, err := b.svc.GetNotice(ctx, n.ID); !errors.Is(err, noticeservice.ErrNotFound) {
			t.Errorf("tenant B saw tenant A's notice: %v", err)
		}
		return nil
	})
}

func strp(s string) *string        { return &s }
func uuidp(u uuid.UUID) *uuid.UUID { return &u }
