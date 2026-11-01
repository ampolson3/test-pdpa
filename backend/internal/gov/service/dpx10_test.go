package service_test

import (
	"context"
	"testing"

	govservice "pdpa-platform/internal/gov/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/wiring"
)

// dpx10Setup wires govEnv's own svc (govSetup, retention_test.go) with a real forms.Service: DPX-10's
// checklists (migration 00096) are already seeded global, published 'framework'-type forms — no local
// fixture form to create or publish, the same move DPX-03's own dpx03Setup already makes for its global
// "org_maturity" template.
func dpx10Setup(t *testing.T, suffix string) govEnv {
	t.Helper()
	e := govSetup(t, suffix)
	e.svc.Forms = wiring.Forms(nil, audit.New())
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM gov.framework_assessments`)
			return nil
		})
	})
	return e
}

var dpx10Perms = []string{"dpx.framework.read", "dpx.framework.create", "dpx.framework.update", "dpx.framework.publish", "dpx.framework.execute"}

func (e govEnv) dpx10In(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: dpx10Perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// TestListFrameworks_ReturnsSeededCatalog is the acceptance criterion's own first half directly: the
// catalog migration 00096 seeded is there to pick from, each already naming a ready-to-use checklist.
func TestListFrameworks_ReturnsSeededCatalog(t *testing.T) {
	e := dpx10Setup(t, "dpx10list")
	var list []govservice.Framework
	e.dpx10In(t, func(ctx context.Context) error {
		var err error
		list, err = e.svc.ListFrameworks(ctx)
		return err
	})
	if len(list) < 4 {
		t.Fatalf("expected at least the 4 seeded frameworks, got %d", len(list))
	}
	var cyber *govservice.Framework
	for i, f := range list {
		if f.Code == "cybersecurity_act" {
			cyber = &list[i]
		}
	}
	if cyber == nil {
		t.Fatalf("cybersecurity_act not in catalog: %+v", list)
	}
	if cyber.ChecklistFormID == nil {
		t.Fatal("cybersecurity_act has no checklist_form_id — not ready to use")
	}
	if cyber.NameTh == "" || cyber.NameEn == "" {
		t.Fatalf("expected bilingual names, got %+v", cyber)
	}
}

// TestAssessFramework_ScoresAndRecords is the acceptance criterion's own second half: picking a framework
// (computer_crime_act) and answering its checklist records a real run, scored by the form engine.
func TestAssessFramework_ScoresAndRecords(t *testing.T) {
	e := dpx10Setup(t, "dpx10assess")
	var a govservice.FrameworkAssessment
	e.dpx10In(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.AssessFramework(ctx, "computer_crime_act", forms.Answers{"cca_log_90days": "yes", "cca_log_integrity": "no"})
		return err
	})
	if a.Score != 1 {
		t.Fatalf("expected score 1 (one yes out of two), got %v", a.Score)
	}
	if len(a.Factors) != 2 {
		t.Fatalf("expected 2 factors, got %d: %+v", len(a.Factors), a.Factors)
	}

	// re-reading the run and listing this framework's own runs both see it.
	var got govservice.FrameworkAssessment
	e.dpx10In(t, func(ctx context.Context) error {
		var err error
		got, err = e.svc.GetFrameworkAssessment(ctx, a.ID)
		return err
	})
	if got.ID != a.ID || got.FrameworkID != a.FrameworkID {
		t.Fatalf("GetFrameworkAssessment mismatch: %+v vs %+v", got, a)
	}

	code := "computer_crime_act"
	var list []govservice.FrameworkAssessment
	e.dpx10In(t, func(ctx context.Context) error {
		var err error
		list, err = e.svc.ListFrameworkAssessments(ctx, &code)
		return err
	})
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("expected exactly this one run listed for the framework, got %+v", list)
	}

	// a different, unrelated framework's own list stays empty.
	other := "bot_notices"
	var otherList []govservice.FrameworkAssessment
	e.dpx10In(t, func(ctx context.Context) error {
		var err error
		otherList, err = e.svc.ListFrameworkAssessments(ctx, &other)
		return err
	})
	if len(otherList) != 0 {
		t.Fatalf("expected no runs for bot_notices, got %+v", otherList)
	}
}

// TestAssessFramework_UnknownCodeRefused: an unknown framework code has no checklist to answer.
func TestAssessFramework_UnknownCodeRefused(t *testing.T) {
	e := dpx10Setup(t, "dpx10unknown")
	e.dpx10In(t, func(ctx context.Context) error {
		_, err := e.svc.AssessFramework(ctx, "no_such_framework", forms.Answers{})
		if err != govservice.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		return nil
	})
}

// TestAssessFramework_RequiresPermission: a caller without dpx.framework.execute cannot answer.
func TestAssessFramework_RequiresPermission(t *testing.T) {
	e := dpx10Setup(t, "dpx10noperm")
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{"dpx.framework.read"}})
		_, err := e.svc.AssessFramework(ctx, "cybersecurity_act", forms.Answers{"csc_critical_infra": "yes"})
		if err != govservice.ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestFrameworkAssessments_TwoTenantIsolation: two tenants' own assessment runs never cross.
func TestFrameworkAssessments_TwoTenantIsolation(t *testing.T) {
	e1 := dpx10Setup(t, "dpx10isoa")
	e2 := dpx10Setup(t, "dpx10isob")

	var a1 govservice.FrameworkAssessment
	e1.dpx10In(t, func(ctx context.Context) error {
		var err error
		a1, err = e1.svc.AssessFramework(ctx, "cybersecurity_act", forms.Answers{"csc_critical_infra": "yes", "csc_incident_plan": "yes", "csc_risk_assessment": "yes"})
		return err
	})

	e2.dpx10In(t, func(ctx context.Context) error {
		_, err := e2.svc.GetFrameworkAssessment(ctx, a1.ID)
		if err != govservice.ErrNotFound {
			t.Fatalf("expected tenant B to never see tenant A's own run, got %v", err)
		}
		return nil
	})

	code := "cybersecurity_act"
	var listB []govservice.FrameworkAssessment
	e2.dpx10In(t, func(ctx context.Context) error {
		var err error
		listB, err = e2.svc.ListFrameworkAssessments(ctx, &code)
		return err
	})
	if len(listB) != 0 {
		t.Fatalf("expected tenant B's own list to be empty, got %+v", listB)
	}
}
