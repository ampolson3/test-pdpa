package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// smeSetup wires a real Forms engine with the "sme_exemption" type registered exactly the way
// internal/wiring/forms.go already reserves it (Respond: "ropa.activity.create").
func smeSetup(t *testing.T, suffix string) env {
	t.Helper()
	e := setup(t, suffix)
	e.svc.Forms = &forms.Service{Audit: e.svc.Audit}
	e.svc.Forms.Register("sme_exemption", forms.Policy{Read: "ropa.activity.read", Create: "ropa.activity.create",
		Update: "ropa.activity.update", Publish: "ropa.activity.approve", Respond: "ropa.activity.create"})
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.sme_exemption_checks`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	return e
}

func (e env) inSme(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{
			"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve",
			"org.structure.read", "org.structure.update",
		}}))
	}); err != nil {
		t.Fatal(err)
	}
}

// publishedSmeExemptionForm publishes a form of the "sme_exemption" type whose own question keys match
// the fixed set this feature's service reads (the same ones the real seeded migration 00090 form uses).
func publishedSmeExemptionForm(t *testing.T, ctx context.Context, e env) {
	t.Helper()
	yn := []forms.Option{{Value: "yes"}, {Value: "no"}}
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{
		{Key: "controller", Title: forms.Text{"th": "ผู้ควบคุม"}, Questions: []forms.Question{
			{Key: "is_sme_or_community", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			{Key: "not_large_scale", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			{Key: "no_sensitive_data", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			{Key: "no_high_risk_profiling", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
		}},
		{Key: "processor", Title: forms.Text{"th": "ผู้ประมวลผล"}, Questions: []forms.Question{
			{Key: "acts_only_as_processor", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			{Key: "no_subcontracted_high_risk", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
		}},
	}}}
	form, err := e.svc.Forms.CreateForm(ctx, "sme_exemption", "SME exemption screening", "sme_exemption", d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Forms.Publish(ctx, form.ID, form.LatestVersion); err != nil {
		t.Fatal(err)
	}
}

func seedLegalEntity(t *testing.T, ctx context.Context, e env) orgservice.LegalEntity {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return le
}

func allYesAnswers() forms.Answers {
	return forms.Answers{
		"is_sme_or_community": "yes", "not_large_scale": "yes", "no_sensitive_data": "yes", "no_high_risk_profiling": "yes",
		"acts_only_as_processor": "yes", "no_subcontracted_high_risk": "yes",
	}
}

// TestAssessSmeExemption_AcceptanceCriterion is the acceptance criterion directly: the result names
// whether the legal entity is exempt and cites the relevant notice(s) as the basis.
func TestAssessSmeExemption_AcceptanceCriterion(t *testing.T) {
	e := smeSetup(t, "ropa20accept")
	var le orgservice.LegalEntity
	e.inSme(t, func(ctx context.Context) error {
		le = seedLegalEntity(t, ctx, e)
		publishedSmeExemptionForm(t, ctx, e)
		return nil
	})

	var exempt ropaservice.SmeExemptionCheck
	e.inSme(t, func(ctx context.Context) error {
		var err error
		exempt, err = e.svc.AssessSmeExemption(ctx, le.ID, false, allYesAnswers())
		return err
	})
	if exempt.Result != "exempt" || exempt.Basis == "" {
		t.Fatalf("expected exempt with a basis, got %+v", exempt)
	}

	var notExempt ropaservice.SmeExemptionCheck
	e.inSme(t, func(ctx context.Context) error {
		answers := allYesAnswers()
		answers["not_large_scale"] = "no"
		var err error
		notExempt, err = e.svc.AssessSmeExemption(ctx, le.ID, false, answers)
		return err
	})
	if notExempt.Result != "not_exempt" || notExempt.Basis == "" {
		t.Fatalf("expected not_exempt with a basis, got %+v", notExempt)
	}
}

// TestAssessSmeExemption_ProcessorPartial covers the processor half: qualifying for only one of the two
// notices (controller 2565 / processor 2567) when the entity is a processor reads as partial, not exempt.
func TestAssessSmeExemption_ProcessorPartial(t *testing.T) {
	e := smeSetup(t, "ropa20partial")
	var le orgservice.LegalEntity
	e.inSme(t, func(ctx context.Context) error {
		le = seedLegalEntity(t, ctx, e)
		publishedSmeExemptionForm(t, ctx, e)
		return nil
	})

	var partial ropaservice.SmeExemptionCheck
	e.inSme(t, func(ctx context.Context) error {
		answers := allYesAnswers()
		answers["acts_only_as_processor"] = "no"
		var err error
		partial, err = e.svc.AssessSmeExemption(ctx, le.ID, true, answers)
		return err
	})
	if partial.Result != "partial" {
		t.Fatalf("expected partial, got %+v", partial)
	}

	var bothExempt ropaservice.SmeExemptionCheck
	e.inSme(t, func(ctx context.Context) error {
		var err error
		bothExempt, err = e.svc.AssessSmeExemption(ctx, le.ID, true, allYesAnswers())
		return err
	})
	if bothExempt.Result != "exempt" {
		t.Fatalf("expected exempt when both sections qualify, got %+v", bothExempt)
	}

	e.inSme(t, func(ctx context.Context) error {
		list, err := e.svc.ListSmeExemptionChecks(ctx, le.ID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("expected 2 rounds recorded, got %d", len(list))
		}
		return nil
	})
}

// TestAssessSmeExemption_UnknownLegalEntityRefused and no published form are both refused distinctly.
func TestAssessSmeExemption_UnknownLegalEntityRefused(t *testing.T) {
	e := smeSetup(t, "ropa20unknown")
	e.inSme(t, func(ctx context.Context) error {
		publishedSmeExemptionForm(t, ctx, e)
		_, err := e.svc.AssessSmeExemption(ctx, uuid.New(), false, allYesAnswers())
		if !errors.Is(err, ropaservice.ErrInvalid) {
			t.Fatalf("expected ErrInvalid for an unknown legal entity, got %v", err)
		}
		return nil
	})
}

func TestAssessSmeExemption_NoPublishedFormRefused(t *testing.T) {
	e := smeSetup(t, "ropa20noform")
	var le orgservice.LegalEntity
	e.inSme(t, func(ctx context.Context) error {
		le = seedLegalEntity(t, ctx, e)
		_, err := e.svc.AssessSmeExemption(ctx, le.ID, false, allYesAnswers())
		if !errors.Is(err, ropaservice.ErrBadSmeExemptionForm) {
			t.Fatalf("expected ErrBadSmeExemptionForm with no published form, got %v", err)
		}
		return nil
	})
}

// TestSmeExemptionResult_PureFunction covers the business rule directly, without a database.
func TestSmeExemptionResult_PureFunction(t *testing.T) {
	cases := []struct {
		name        string
		isProcessor bool
		answers     forms.Answers
		want        string
	}{
		{"controller all yes, not processor", false, allYesAnswers(), "exempt"},
		{"controller one no, not processor", false, forms.Answers{"is_sme_or_community": "yes", "not_large_scale": "no",
			"no_sensitive_data": "yes", "no_high_risk_profiling": "yes"}, "not_exempt"},
		{"both sections yes, is processor", true, allYesAnswers(), "exempt"},
		{"controller yes only, is processor", true, forms.Answers{"is_sme_or_community": "yes", "not_large_scale": "yes",
			"no_sensitive_data": "yes", "no_high_risk_profiling": "yes", "acts_only_as_processor": "no", "no_subcontracted_high_risk": "yes"}, "partial"},
		{"processor yes only, is processor", true, forms.Answers{"is_sme_or_community": "no", "not_large_scale": "yes",
			"no_sensitive_data": "yes", "no_high_risk_profiling": "yes", "acts_only_as_processor": "yes", "no_subcontracted_high_risk": "yes"}, "partial"},
		{"neither, is processor", true, forms.Answers{"is_sme_or_community": "no", "not_large_scale": "no",
			"no_sensitive_data": "no", "no_high_risk_profiling": "no", "acts_only_as_processor": "no", "no_subcontracted_high_risk": "no"}, "not_exempt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, basis := ropaservice.SmeExemptionResult(c.isProcessor, c.answers)
			if got != c.want {
				t.Errorf("got %q, want %q (basis: %s)", got, c.want, basis)
			}
			if basis == "" {
				t.Error("expected a non-empty basis")
			}
		})
	}
}
