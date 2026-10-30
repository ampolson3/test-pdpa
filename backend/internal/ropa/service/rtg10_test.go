package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
)

// templateIDByCodeAndVersion is RTG-10's own lookup — unlike templateIDByCode (from_template_test.go),
// this feature's own migration (00088) means "recruitment_job_posting" now has two rows (v1 and v2) under
// the same code, so a plain code-only lookup would be ambiguous.
func templateIDByCodeAndVersion(t *testing.T, ctx context.Context, code string, versionNo int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`SELECT id FROM ropa.activity_templates WHERE code = $1 AND version_no = $2`, code, versionNo).Scan(&id); err != nil {
		t.Fatalf("templateIDByCodeAndVersion(%q, %d): %v", code, versionNo, err)
	}
	return id
}

func countReviewTasks(t *testing.T, ctx context.Context, activityID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`SELECT count(*) FROM dpo.tasks WHERE source_type = 'ropa_gap' AND source_id = $1`, activityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestListOutdatedTemplateActivities_AndCheckOpensOneTaskPerActivity is RTG-10's own acceptance
// criterion directly: an activity created from v1 of a template whose own migration (00088) later issued
// v2 shows up in ListOutdatedTemplateActivities, and the sweep opens exactly one deduped review task for
// it — a repeated check afterward opens nothing more.
func TestListOutdatedTemplateActivities_AndCheckOpensOneTaskPerActivity(t *testing.T) {
	e := setup(t, "ropartg10a")
	e.svc.Dpo = &dposervice.Service{Audit: audit.New()}

	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		v1 := templateIDByCodeAndVersion(t, ctx, "recruitment_job_posting", 1)
		a, err := e.svc.CreateActivityFromTemplate(ctx, v1, le.ID, unit.ID, "RTG10-01", nil)
		if err != nil {
			return err
		}
		activityID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		outdated, err := e.svc.ListOutdatedTemplateActivities(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, o := range outdated {
			if o.ActivityID == activityID {
				found = true
				if o.CurrentVersion != 1 || o.LatestVersion != 2 {
					t.Errorf("got current=%d latest=%d, want 1 and 2", o.CurrentVersion, o.LatestVersion)
				}
			}
		}
		if !found {
			t.Fatalf("expected activity %s in ListOutdatedTemplateActivities, got %+v", activityID, outdated)
		}

		if err := e.svc.CheckOutdatedTemplateActivities(ctx); err != nil {
			return err
		}
		if n := countReviewTasks(t, ctx, activityID); n != 1 {
			t.Errorf("after first check: got %d tasks, want 1", n)
		}
		// A repeat sweep tick must not duplicate the task.
		if err := e.svc.CheckOutdatedTemplateActivities(ctx); err != nil {
			return err
		}
		if n := countReviewTasks(t, ctx, activityID); n != 1 {
			t.Errorf("after repeat check: got %d tasks, want still 1", n)
		}
		return nil
	})
}

// TestListOutdatedTemplateActivities_CurrentVersionIsNotFlagged proves the other half directly: an
// activity created from the *latest* version of its own template never appears in the outdated list, and
// a sweep opens no task for it.
func TestListOutdatedTemplateActivities_CurrentVersionIsNotFlagged(t *testing.T) {
	e := setup(t, "ropartg10b")
	e.svc.Dpo = &dposervice.Service{Audit: audit.New()}

	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		v2 := templateIDByCodeAndVersion(t, ctx, "recruitment_job_posting", 2)
		a, err := e.svc.CreateActivityFromTemplate(ctx, v2, le.ID, unit.ID, "RTG10-02", nil)
		if err != nil {
			return err
		}
		activityID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		outdated, err := e.svc.ListOutdatedTemplateActivities(ctx)
		if err != nil {
			return err
		}
		for _, o := range outdated {
			if o.ActivityID == activityID {
				t.Fatalf("an activity created from the latest version must not be flagged, got %+v", o)
			}
		}
		if err := e.svc.CheckOutdatedTemplateActivities(ctx); err != nil {
			return err
		}
		if n := countReviewTasks(t, ctx, activityID); n != 0 {
			t.Errorf("got %d tasks for a current-version activity, want 0", n)
		}
		return nil
	})
}

// TestCheckOutdatedTemplateActivities_NilDpoIsHarmless mirrors every other nil-means-skip cross-module
// field this service already has (Events/Collab/ROPA-19's own Dpo) — a context that never wired Dpo
// (e.g. cmd/api) must not error.
func TestCheckOutdatedTemplateActivities_NilDpoIsHarmless(t *testing.T) {
	e := setup(t, "ropartg10c")

	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		v1 := templateIDByCodeAndVersion(t, ctx, "recruitment_job_posting", 1)
		_, err = e.svc.CreateActivityFromTemplate(ctx, v1, le.ID, unit.ID, "RTG10-03", nil)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.CheckOutdatedTemplateActivities(ctx)
	})
}

// TestListOutdatedTemplateActivities_TwoTenantIsolation proves RLS: tenant B never sees tenant A's own
// outdated activity.
func TestListOutdatedTemplateActivities_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ropartg10isoA")
	b := setup(t, "ropartg10isoB")

	var activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "A", NameTh: "A", UnitType: "department"})
		if err != nil {
			return err
		}
		v1 := templateIDByCodeAndVersion(t, ctx, "recruitment_job_posting", 1)
		act, err := a.svc.CreateActivityFromTemplate(ctx, v1, le.ID, unit.ID, "ISO-01", nil)
		if err != nil {
			return err
		}
		activityID = act.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		outdated, err := b.svc.ListOutdatedTemplateActivities(ctx)
		if err != nil {
			return err
		}
		for _, o := range outdated {
			if o.ActivityID == activityID {
				t.Error("tenant B must never see tenant A's own outdated activity")
			}
		}
		return nil
	})
}

// TestListTemplateVersions is the version-history read, "หน้าประวัติ template": both versions of
// recruitment_job_posting, newest first.
func TestListTemplateVersions(t *testing.T) {
	e := setup(t, "ropartg10d")

	e.in(t, func(ctx context.Context) error {
		v1 := templateIDByCodeAndVersion(t, ctx, "recruitment_job_posting", 1)
		tpl, err := e.templates.GetActivityTemplate(ctx, v1)
		if err != nil {
			return err
		}
		versions, err := e.templates.ListTemplateVersions(ctx, tpl.TemplateSetID, tpl.Code)
		if err != nil {
			return err
		}
		if len(versions) != 2 {
			t.Fatalf("got %d versions, want 2", len(versions))
		}
		if versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
			t.Errorf("versions = %+v, want [2, 1] (newest first)", versions)
		}
		latest, err := e.templates.LatestVersion(ctx, tpl.TemplateSetID, tpl.Code)
		if err != nil {
			return err
		}
		if latest.VersionNo != 2 {
			t.Errorf("LatestVersion = %d, want 2", latest.VersionNo)
		}
		return nil
	})
}
