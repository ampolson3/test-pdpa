package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
)

var formPerms = []string{"breach.form.create", "breach.form.read", "breach.form.update", "breach.form.delete", "breach.form.publish"}

func createReportForm(t *testing.T, f *breachtest.Fixture) breach.ReportForm {
	t.Helper()
	var out breach.ReportForm
	f.As(t, f.DPO, formPerms, func(ctx context.Context) error {
		var err error
		out, err = f.Svc.CreateReportForm(ctx, breach.CreateReportFormInput{LegalEntityID: f.EntityA, Name: "Public report form"})
		return err
	})
	return out
}

// Acceptance BRE-01: a loginless public submission creates an incident and the assigned team is notified at once —
// the same alert BRE-02's own admin-app intake already sends, reused through the shared create() core.
func TestCreatePublicReport_CreatesIncidentAndAlertsTeam(t *testing.T) {
	f := breachtest.Setup(t)
	form := createReportForm(t, f)

	// Publishing issues a public key.
	f.As(t, f.DPO, formPerms, func(ctx context.Context) error {
		var err error
		form, err = f.Svc.PublishReportForm(ctx, form.ID, form.RowVersion)
		return err
	})
	if form.Status != "active" || form.PublicKey == nil {
		t.Fatalf("published form = %+v", form)
	}

	var inc breach.Incident
	f.System(t, func(ctx context.Context) error {
		var err error
		inc, err = f.Svc.CreatePublicReport(ctx, form.ID, breach.PublicReport{
			Title: "พนักงานภายนอกพบไฟล์หลุด", Description: "พบไฟล์ข้อมูลลูกค้าเปิดเผยบนเว็บไซต์สาธารณะ",
			BreachTypes: []string{"confidentiality"}, ReporterContact: "finder@example.com",
		})
		return err
	})
	if inc.ID == uuid.Nil || inc.ReportedVia != "public_form" || inc.Status != breach.StatusReported {
		t.Fatalf("public report incident = %+v", inc)
	}
	if inc.LegalEntityID != f.EntityA {
		t.Errorf("legal entity = %v, want %v", inc.LegalEntityID, f.EntityA)
	}
	// The team (owner defaulting to nil -> no owner, plus every DPO) is alerted immediately — the acceptance criterion.
	if n := count(t, f, `SELECT count(DISTINCT recipient_user_id)::int FROM platform.notifications WHERE entity_id = $1 AND channel = 'in_app'`, inc.ID); n != 2 {
		t.Errorf("alerted %d people, want 2 DPOs (no owner for a public report)", n)
	}
	if n := count(t, f, `SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'breach.reported' AND aggregate_id = $1`, inc.ID); n != 1 {
		t.Errorf("breach.reported events: %d", n)
	}
	// The reporter's contact was encrypted, not stored in the clear.
	if n := count(t, f, `SELECT count(*)::int FROM breach.incidents WHERE id = $1 AND reporter_contact_enc IS NOT NULL`, inc.ID); n != 1 {
		t.Errorf("reporter_contact_enc not set")
	}
}

// A draft or retired form's key (or no key at all) never accepts a submission.
func TestCreatePublicReport_RefusesUnlessActive(t *testing.T) {
	f := breachtest.Setup(t)
	form := createReportForm(t, f)

	f.System(t, func(ctx context.Context) error {
		if _, err := f.Svc.CreatePublicReport(ctx, form.ID, breach.PublicReport{Title: "x", Description: "y", BreachTypes: []string{"confidentiality"}}); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("submission against a draft form: err = %v, want ErrNotFound", err)
		}
		return nil
	})

	f.As(t, f.DPO, formPerms, func(ctx context.Context) error {
		var err error
		form, err = f.Svc.PublishReportForm(ctx, form.ID, form.RowVersion)
		if err != nil {
			return err
		}
		form, err = f.Svc.RetireReportForm(ctx, form.ID, form.RowVersion)
		return err
	})
	f.System(t, func(ctx context.Context) error {
		if _, err := f.Svc.CreatePublicReport(ctx, form.ID, breach.PublicReport{Title: "x", Description: "y", BreachTypes: []string{"confidentiality"}}); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("submission against a retired form: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// Publishing twice keeps the same key; retiring stops it resolving (RetireReportForm's own behaviour, exercised
// here through the service the same way DSAR-01's own intake forms are).
func TestReportForm_PublishIsIdempotentOnTheKey(t *testing.T) {
	f := breachtest.Setup(t)
	form := createReportForm(t, f)
	var firstKey string
	f.As(t, f.DPO, formPerms, func(ctx context.Context) error {
		var err error
		form, err = f.Svc.PublishReportForm(ctx, form.ID, form.RowVersion)
		if err != nil {
			return err
		}
		firstKey = *form.PublicKey
		return nil
	})
	f.As(t, f.DPO, formPerms, func(ctx context.Context) error {
		var err error
		form, err = f.Svc.RetireReportForm(ctx, form.ID, form.RowVersion)
		if err != nil {
			return err
		}
		form, err = f.Svc.PublishReportForm(ctx, form.ID, form.RowVersion)
		return err
	})
	if form.PublicKey == nil || *form.PublicKey != firstKey {
		t.Errorf("re-publish key = %v, want unchanged %q", form.PublicKey, firstKey)
	}
}

// Two-tenant isolation: tenant B never sees tenant A's report forms.
func TestReportForms_TwoTenantIsolation(t *testing.T) {
	f := breachtest.Setup(t)
	form := createReportForm(t, f)
	f.Try(f.B.UserID, formPerms, func(ctx context.Context) error {
		if _, err := f.Svc.GetReportForm(ctx, form.ID); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("tenant B reading tenant A's form: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
