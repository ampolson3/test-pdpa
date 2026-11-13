package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
)

// withDpiaScreening wires e.svc.Dpia with a minimal dpiaservice.Service — ScreenSubject only needs Forms,
// the same global, tenant_id-NULL "dpo_requirement_check" template migration 00077 seeded, so no local
// fixture form needs to be created or published for this test. It also registers cleanup for the
// assess.assessments/assess.answers/dpo.requirement_checks rows this test's own AssessDpoRequirement calls
// write, which setup()'s own cleanup (appointments_test.go) doesn't know about.
func withDpiaScreening(t *testing.T, e env) env {
	t.Helper()
	e.svc.Dpia = &dpiaservice.Service{Forms: e.forms, Audit: audit.New()}
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.requirement_checks`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.answers`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessments`)
			return nil
		})
	})
	return e
}

func allNoAnswers() forms.Answers {
	return forms.Answers{
		"public_authority":           "no",
		"large_scale_processing":     "no",
		"regular_monitoring":         "no",
		"sensitive_or_criminal_core": "no",
	}
}

func TestAssessDpoRequirement_NotRequiredWhenNoCriteriaMet(t *testing.T) {
	e := withDpiaScreening(t, setup(t, "dporeq1"))
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		check, err := e.svc.AssessDpoRequirement(ctx, le.ID, allNoAnswers())
		if err != nil {
			return err
		}
		if check.Result != "not_required" {
			t.Errorf("got result %q, want not_required", check.Result)
		}
		if check.Basis == "" {
			t.Error("basis reason is empty")
		}
		if check.AssessmentID == uuid.Nil {
			t.Error("assessment_id not set")
		}
		return nil
	})
}

// TestAssessDpoRequirement_RequiredWhenAnyCriterionMet is the acceptance criterion directly: ม.41(2)'s own
// OR rule — a single flagged criterion already triggers "required", with a reason citing the notice.
func TestAssessDpoRequirement_RequiredWhenAnyCriterionMet(t *testing.T) {
	e := withDpiaScreening(t, setup(t, "dporeq2"))
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})

	answers := allNoAnswers()
	answers["large_scale_processing"] = "yes"
	e.in(t, func(ctx context.Context) error {
		check, err := e.svc.AssessDpoRequirement(ctx, le.ID, answers)
		if err != nil {
			return err
		}
		if check.Result != "required" {
			t.Errorf("got result %q, want required", check.Result)
		}
		if check.Basis == "" {
			t.Error("basis reason is empty")
		}
		return nil
	})

	// GetLatestRequirementCheck / ListRequirementChecks both see the round just recorded.
	e.in(t, func(ctx context.Context) error {
		latest, err := e.svc.GetLatestRequirementCheck(ctx, le.ID)
		if err != nil {
			return err
		}
		if latest.Result != "required" {
			t.Errorf("GetLatestRequirementCheck: got %q, want required", latest.Result)
		}
		list, err := e.svc.ListRequirementChecks(ctx, le.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("ListRequirementChecks: got %d rows, want 1", len(list))
		}
		return nil
	})
}

func TestAssessDpoRequirement_UnknownLegalEntityRefused(t *testing.T) {
	e := withDpiaScreening(t, setup(t, "dporeq3"))
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.AssessDpoRequirement(ctx, uuid.New(), allNoAnswers())
		if !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("got %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestGetLatestRequirementCheck_NoneYetIsNotFound(t *testing.T) {
	e := withDpiaScreening(t, setup(t, "dporeq4"))
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ซี จำกัด", IsController: true}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.GetLatestRequirementCheck(ctx, le.ID)
		if !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestAssessDpoRequirement_TwoTenantIsolation proves RLS alone keeps one tenant's requirement checks out of
// another's list — the same two-tenant isolation proof every new repository in this codebase carries (rule 1).
func TestAssessDpoRequirement_TwoTenantIsolation(t *testing.T) {
	e1 := withDpiaScreening(t, setup(t, "dporeq5a"))
	e2 := withDpiaScreening(t, setup(t, "dporeq5b"))

	var le1, le2 orgservice.LegalEntity
	e1.in(t, func(ctx context.Context) error {
		var err error
		le1, err = e1.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ดี จำกัด", IsController: true}, 0)
		return err
	})
	e2.in(t, func(ctx context.Context) error {
		var err error
		le2, err = e2.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท อี จำกัด", IsController: true}, 0)
		return err
	})

	e1.in(t, func(ctx context.Context) error {
		_, err := e1.svc.AssessDpoRequirement(ctx, le1.ID, allNoAnswers())
		return err
	})
	e2.in(t, func(ctx context.Context) error {
		_, err := e2.svc.AssessDpoRequirement(ctx, le2.ID, allNoAnswers())
		return err
	})

	// Tenant 2 cannot see tenant 1's legal entity (and so its own query against it fails as not-found via
	// RLS-hidden rows), and tenant 2's own list never includes tenant 1's round.
	e2.in(t, func(ctx context.Context) error {
		list, err := e2.svc.ListRequirementChecks(ctx, le1.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant 2 saw tenant 1's requirement checks: %+v", list)
		}
		return nil
	})
}
