package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// templateIDByCode looks up one of RTG-01's seeded global activity templates by its code (the same
// lookup the admin UI's picker does via GetActivityTemplate, just skipping the HTTP layer).
func templateIDByCode(t *testing.T, ctx context.Context, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT id FROM ropa.activity_templates WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatalf("templateIDByCode(%q): %v", code, err)
	}
	return id
}

// TestCreateActivityFromTemplate_FillsEveryM39Topic is ROPA-05's own acceptance criterion: creating an
// activity from an RTG-01 template gives it defaults covering every ม.39 topic the template carries
// (purposes, data, retention, security controls — recipients are deliberately left for the user, see
// CreateActivityFromTemplate's own doc comment), and the activity's completeness score reflects that
// immediately, with no further edits.
func TestCreateActivityFromTemplate_FillsEveryM39Topic(t *testing.T) {
	e := setup(t, "ropafromtpl")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		a, err := e.svc.CreateActivityFromTemplate(ctx, tplID, unit.LegalEntityID, unit.ID, "HR-TPL-01", nil)
		if err != nil {
			t.Fatalf("CreateActivityFromTemplate: %v", err)
		}
		if a.Name == "" || a.Code != "HR-TPL-01" || a.Role != "controller" {
			t.Errorf("activity core fields not set from template: %+v", a)
		}
		if len(a.MissingItems) != 0 {
			t.Errorf("expected no missing ม.39 items right after creation, got %v", a.MissingItems)
		}
		if a.Completeness != 100 {
			t.Errorf("expected 100%% completeness right after creation, got %d (%v)", a.Completeness, a.MissingItems)
		}

		purposes, err := e.svc.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(purposes) == 0 || purposes[0].LawfulBasisCode != "CONTRACT" {
			t.Errorf("expected the template's purpose to be copied, got %+v", purposes)
		}

		data, err := e.svc.ListActivityData(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(data) != 4 {
			t.Errorf("expected the template's 4 data rows to be copied, got %d", len(data))
		}

		retention, err := e.svc.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(retention) != 1 || retention[0].DisposalMethod != "delete" {
			t.Errorf("expected the template's retention rule to be copied, got %+v", retention)
		}

		controls, err := e.svc.ListActivityControls(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(controls) != 2 {
			t.Errorf("expected the template's 2 security controls to be linked, got %d", len(controls))
		}

		if a.RightsAndAccess == "" {
			t.Errorf("expected a default rights_and_access note, got empty")
		}
		return nil
	})
}

// TestCreateActivityFromTemplate_UnknownTemplateRefused proves an unknown or cross-tenant template id
// is refused as a plain validation error, not a 500 — GetActivityTemplate's own ErrNotFound (templates
// are global, so every tenant sees the same ones; a bogus id is the only way to trigger this).
func TestCreateActivityFromTemplate_UnknownTemplateRefused(t *testing.T) {
	e := setup(t, "ropafromtplbad")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CreateActivityFromTemplate(ctx, uuid.New(), unit.LegalEntityID, unit.ID, "HR-BAD-01", nil); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown template: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestCreateActivityFromTemplate_TwoTenantIsolation proves the resulting activity is only visible to
// the tenant that created it — the templates themselves stay global by design (ORG-07's own pattern).
func TestCreateActivityFromTemplate_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ropafromtpliso1")
	b := setup(t, "ropafromtpliso2")
	var unit orgservice.OrgUnit
	var created ropaservice.Activity
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		created, err = a.svc.CreateActivityFromTemplate(ctx, tplID, unit.LegalEntityID, unit.ID, "HR-ISO-01", nil)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetActivity(ctx, created.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's template-created activity: %v", err)
		}
		return nil
	})
}
