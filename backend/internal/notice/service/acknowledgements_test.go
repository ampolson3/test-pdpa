package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// publishedNotice creates and fully publishes one notice (submit → DPO approve → publish), returning it
// ready for PNG-09's own acknowledgement tests.
func publishedNotice(t *testing.T, e env, suffix string) noticeservice.Notice {
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
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศ PNG-09", Slug: "png09-" + suffix})
		return err
	})
	publishNotice(t, e, n, completeContent())
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.GetNotice(ctx, n.ID)
		return err
	})
	return n
}

// TestRecordPublicAcknowledgement_AnonymousByDefault is PNG-09's own public half: a visitor's
// acknowledgement is recorded against the current published version with no identity, and shows up in the
// admin report with the right channel and no user name.
func TestRecordPublicAcknowledgement_AnonymousByDefault(t *testing.T) {
	e := setup(t, "png09anon")
	n := publishedNotice(t, e, "anon")

	e.in(t, func(ctx context.Context) error {
		a, versionNo, err := e.svc.RecordPublicAcknowledgement(ctx, n, nil, "web", nil)
		if err != nil {
			t.Fatal(err)
		}
		if versionNo != 1 {
			t.Errorf("version_no = %d, want 1", versionNo)
		}
		if a.UserID != nil {
			t.Errorf("expected no user id on an anonymous acknowledgement, got %v", *a.UserID)
		}
		list, total, err := e.svc.ListAcknowledgements(ctx, n.ID, 1, nil, 50)
		if err != nil {
			return err
		}
		if total != 1 || len(list) != 1 {
			t.Fatalf("list/total = %+v/%d, want exactly 1", list, total)
		}
		if list[0].Channel != "web" || list[0].UserID != nil || list[0].UserName != "" {
			t.Errorf("got %+v, want channel=web and no identity", list[0])
		}
		return nil
	})

	// Unknown channel refused.
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.RecordPublicAcknowledgement(ctx, n, nil, "carrier-pigeon", nil); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("bad channel: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestRecordAcknowledgement_NamedUser is the staff/API half — a named user's acknowledgement is recorded
// and resolved by name in the report; an unknown user id is refused.
func TestRecordAcknowledgement_NamedUser(t *testing.T) {
	e := setup(t, "png09named")
	n := publishedNotice(t, e, "named")

	var employee uuid.UUID
	e.in(t, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, 'employee@test.example', 'An Employee', 'active') RETURNING id`,
		).Scan(&employee)
	})

	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.RecordAcknowledgement(ctx, n.ID, 1, employee, "api")
		if err != nil {
			return err
		}
		if a.UserID == nil || *a.UserID != employee {
			t.Errorf("user_id = %v, want %v", a.UserID, employee)
		}
		list, total, err := e.svc.ListAcknowledgements(ctx, n.ID, 1, nil, 50)
		if err != nil {
			return err
		}
		if total != 1 || list[0].UserName == "" {
			t.Errorf("expected the named acknowledgement's display name resolved, got %+v", list)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordAcknowledgement(ctx, n.ID, 1, uuid.New(), "api"); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("unknown user_id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestListAcknowledgements_TwoTenantIsolation: tenant B can never read tenant A's own acknowledgements.
func TestListAcknowledgements_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "png09isoa")
	e2 := setup(t, "png09isob")
	n := publishedNotice(t, e1, "isoa")

	e1.in(t, func(ctx context.Context) error {
		_, _, err := e1.svc.RecordPublicAcknowledgement(ctx, n, nil, "web", nil)
		return err
	})

	if err := e2.inAs(e2.tenant.UserID, noticePermissions, nil, func(ctx context.Context) error {
		_, _, err := e2.svc.ListAcknowledgements(ctx, n.ID, 1, nil, 50)
		return err
	}); !errors.Is(err, noticeservice.ErrNotFound) {
		t.Errorf("tenant B reading tenant A's notice: err = %v, want ErrNotFound", err)
	}
}
