package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestSuggest_EveryItemCitesALegalArticle is RTG-06's own acceptance criterion directly: every suggested
// item (purpose, data, retention) from a real RTG-01 template carries a rationale that names a real legal
// article — the lawful basis's own section_ref for a purpose (not the template's own generic "มาตรา 24/26"
// pair), ม.26/39(2) for data depending on sensitivity, and ม.39(3) for retention — and Suggest never writes
// anything (it only reads the template and this tenant's own org.* master data).
func TestSuggest_EveryItemCitesALegalArticle(t *testing.T) {
	e := setup(t, "rtg06suggest")
	var sugg ropaservice.TemplateSuggestions
	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		var err error
		sugg, err = e.svc.Suggest(ctx, tplID)
		return err
	})

	if len(sugg.Purposes) != 1 || len(sugg.Data) != 4 || len(sugg.Retention) != 1 {
		t.Fatalf("expected the template's own 1 purpose / 4 data / 1 retention item, got %d/%d/%d",
			len(sugg.Purposes), len(sugg.Data), len(sugg.Retention))
	}

	p := sugg.Purposes[0]
	if p.LawfulBasisCode != "CONTRACT" || p.LawfulBasisNameTh == "" {
		t.Errorf("purpose suggestion missing lawful basis: %+v", p)
	}
	if !strings.Contains(p.Rationale, "ม.") {
		t.Errorf("purpose rationale does not cite a legal article: %q", p.Rationale)
	}
	if strings.Contains(p.Rationale, "24/26") {
		t.Errorf("purpose rationale should cite the lawful basis's own specific article, not the template's generic pair: %q", p.Rationale)
	}

	for _, d := range sugg.Data {
		if d.DataCategoryName == "" || d.SubjectTypeName == "" {
			t.Errorf("data suggestion missing resolved names: %+v", d)
		}
		wantRef := "ม.39(2)"
		if d.IsSensitive {
			wantRef = "ม.26"
		}
		if !strings.Contains(d.Rationale, wantRef) {
			t.Errorf("data rationale %q does not cite %s (is_sensitive=%v)", d.Rationale, wantRef, d.IsSensitive)
		}
	}

	r := sugg.Retention[0]
	if !strings.Contains(r.Rationale, "39(3)") || !strings.Contains(r.Rationale, r.RetentionBasis) {
		t.Errorf("retention rationale missing article or basis text: %+v", r)
	}

	// Suggest is read-only: nothing it touched left any new row behind for this tenant.
	e.in(t, func(ctx context.Context) error {
		activities, _, err := e.svc.ListActivities(ctx, ropaservice.ActivityFilter{})
		if err != nil {
			return err
		}
		if len(activities) != 0 {
			t.Errorf("Suggest must not write anything; found %d activities", len(activities))
		}
		return nil
	})
}

// TestSuggest_UnknownTemplateRefused mirrors CreateActivityFromTemplate's own unknown-id handling.
func TestSuggest_UnknownTemplateRefused(t *testing.T) {
	e := setup(t, "rtg06suggestbad")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Suggest(ctx, uuid.New()); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown template: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestApplySuggestedItems_OnlySelectedItemsAreWritten is the other half of RTG-06's acceptance criterion:
// suggestions are never saved until the user confirms them, one at a time — selecting only the purpose and
// one of the four data items writes exactly those two rows and nothing else from the template.
func TestApplySuggestedItems_OnlySelectedItemsAreWritten(t *testing.T) {
	e := setup(t, "rtg06apply")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})

	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: unit.LegalEntityID, OrgUnitID: unit.ID, Code: "HR-SUG-01",
			Name: "กิจกรรมเปล่าสำหรับทดสอบ", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		updated, err := e.svc.ApplySuggestedItems(ctx, a.ID, ropaservice.ApplySuggestedItemsInput{
			TemplateID: tplID, Purposes: []int{0}, Data: []int{1},
		})
		if err != nil {
			return err
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, updated.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 1 {
			t.Errorf("expected exactly the one selected purpose, got %d", len(purposes))
		}
		data, err := e.svc.ListActivityData(ctx, updated.ID)
		if err != nil {
			return err
		}
		if len(data) != 1 {
			t.Errorf("expected exactly the one selected data item (index 1), got %d", len(data))
		}
		retention, err := e.svc.ListRetentionRules(ctx, updated.ID)
		if err != nil {
			return err
		}
		if len(retention) != 0 {
			t.Errorf("retention was never selected, expected none, got %d", len(retention))
		}
		return nil
	})
}

// TestApplySuggestedItems_OutOfRangeIndexRefused proves a bad index is rejected before anything is
// written, rather than silently ignored or panicking.
func TestApplySuggestedItems_OutOfRangeIndexRefused(t *testing.T) {
	e := setup(t, "rtg06applybad")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: unit.LegalEntityID, OrgUnitID: unit.ID, Code: "HR-SUG-02",
			Name: "กิจกรรมเปล่าสำหรับทดสอบ", Role: "controller"}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		if _, err := e.svc.ApplySuggestedItems(ctx, a.ID, ropaservice.ApplySuggestedItemsInput{TemplateID: tplID, Purposes: []int{99}}); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("out-of-range purpose index: %v, want ErrInvalid", err)
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 0 {
			t.Errorf("nothing should have been written, got %d purposes", len(purposes))
		}
		return nil
	})
}

// TestApplySuggestedItems_TwoTenantIsolation proves a cross-tenant activity id is refused the same way
// GetActivity already refuses one.
func TestApplySuggestedItems_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "rtg06applyiso1")
	b := setup(t, "rtg06applyiso2")
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
		created, err = a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: unit.LegalEntityID, OrgUnitID: unit.ID, Code: "HR-ISO-02",
			Name: "กิจกรรมของเอ", Role: "controller"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		if _, err := b.svc.ApplySuggestedItems(ctx, created.ID, ropaservice.ApplySuggestedItemsInput{TemplateID: tplID, Purposes: []int{0}}); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B applying to tenant A's activity: %v, want ErrNotFound", err)
		}
		return nil
	})
}
