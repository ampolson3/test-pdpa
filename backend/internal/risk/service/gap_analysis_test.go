package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// fakeNotice is RRA-04's own test double for the Notice interface — no real notice/service construction
// needed just to flip whether an activity is "covered" or not.
type fakeNotice struct{ covered bool }

func (f fakeNotice) ActivityHasNotice(ctx context.Context, activityID uuid.UUID) (bool, error) {
	return f.covered, nil
}

func gapSetup(t *testing.T, suffix string, covered bool) scoreEnv {
	t.Helper()
	e := scoreSetup(t, suffix)
	e.svc.Notice = fakeNotice{covered: covered}
	t.Cleanup(func() {
		owner := dbtest.OwnerPool(t)
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.gap_findings`)
			return nil
		})
	})
	return e
}

func sensitiveCategoryID(t *testing.T, ctx context.Context, org *orgservice.Service) uuid.UUID {
	t.Helper()
	items, err := org.ListMaster(ctx, orgservice.KindDataCategories)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.IsSensitive && it.ID != nil {
			return *it.ID
		}
	}
	t.Fatal("expected a sensitive default data category to be seeded (ORG-07)")
	return uuid.Nil
}

func nonConsentLawfulBasis(t *testing.T, ctx context.Context, org *orgservice.Service) string {
	t.Helper()
	items, err := org.ListMaster(ctx, orgservice.KindLawfulBases)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if !it.RequiresConsent {
			return it.Code
		}
	}
	t.Fatal("expected at least one non-consent lawful basis to be seeded (ORG-07)")
	return ""
}

func subjectTypeID(t *testing.T, ctx context.Context, org *orgservice.Service) uuid.UUID {
	t.Helper()
	items, err := org.ListMaster(ctx, orgservice.KindSubjectTypes)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ID == nil {
		t.Fatal("expected data subject types to be seeded (ORG-07)")
	}
	return *items[0].ID
}

// TestAnalyzeActivity_DetectsEveryGapType is RRA-04's own acceptance criterion directly: an activity
// deliberately missing everything the five seeded rules check for gets exactly five open findings, one
// per rule code — "ช่องว่างทุกประเภทใน rule ถูกตรวจพบในชุดข้อมูลทดสอบ".
func TestAnalyzeActivity_DetectsEveryGapType(t *testing.T) {
	e := gapSetup(t, "rra04every", false)
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-01",
			Name: "กิจกรรมทดสอบช่องว่าง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID

		// Sensitive data with no retention rule and no consenting purpose (no_retention, sensitive_no_consent).
		catID := sensitiveCategoryID(t, ctx, e.org)
		subID := subjectTypeID(t, ctx, e.org)
		if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activityID, DataCategoryID: catID, SubjectTypeID: subID, Source: "direct"}); err != nil {
			return err
		}

		// A foreign recipient with no logged transfer (transfer_no_basis).
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูลต่างประเทศ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activityID, PartyID: party.ID, RecipientRole: "processor", DisclosureBasis: "สัญญา"}); err != nil {
			return err
		}
		// no_lawful_basis: deliberately never call AddActivityPurpose.
		// no_notice_coverage: gapSetup(..., false) above.
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		findings, err := e.svc.AnalyzeActivity(ctx, activityID)
		if err != nil {
			return err
		}
		if len(findings) != 5 {
			t.Fatalf("expected all 5 seeded rules to find a gap, got %d: %+v", len(findings), findings)
		}
		seen := map[string]bool{}
		for _, f := range findings {
			if f.Status != "open" {
				t.Errorf("finding %s status = %q, want open", f.RuleCode, f.Status)
			}
			seen[f.RuleCode] = true
		}
		for _, code := range []string{"no_lawful_basis", "no_retention", "no_notice_coverage", "transfer_no_basis", "sensitive_no_consent"} {
			if !seen[code] {
				t.Errorf("expected rule %q to fire, it did not", code)
			}
		}
		return nil
	})
}

// TestAnalyzeActivity_ResolvesWhenFixed proves a gap that no longer applies is cleared automatically on
// the next analysis — re-running always reflects the activity's live state.
func TestAnalyzeActivity_ResolvesWhenFixed(t *testing.T) {
	e := gapSetup(t, "rra04fix", true) // notice already covers it, so only no_lawful_basis/no_retention apply
	var activityID uuid.UUID
	var basis string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-02",
			Name: "กิจกรรมทดสอบแก้ไขช่องว่าง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		basis = nonConsentLawfulBasis(t, ctx, e.org)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		findings, err := e.svc.AnalyzeActivity(ctx, activityID)
		if err != nil {
			return err
		}
		if len(findings) != 2 {
			t.Fatalf("expected no_lawful_basis + no_retention only, got %d: %+v", len(findings), findings)
		}
		return nil
	})

	// Fix the lawful-basis gap by adding a purpose; no_retention is still missing.
	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activityID, PurposeText: "ทดสอบ", LawfulBasisCode: basis})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		findings, err := e.svc.AnalyzeActivity(ctx, activityID)
		if err != nil {
			return err
		}
		if len(findings) != 1 || findings[0].RuleCode != "no_retention" {
			t.Fatalf("expected only no_retention to remain open, got %+v", findings)
		}
		all, err := e.svc.ListGapFindingsForActivity(ctx, activityID)
		if err != nil {
			return err
		}
		var resolvedCount int
		for _, f := range all {
			if f.RuleCode == "no_lawful_basis" {
				if f.Status != "resolved" {
					t.Errorf("no_lawful_basis finding status = %q, want resolved", f.Status)
				}
				resolvedCount++
			}
		}
		if resolvedCount != 1 {
			t.Errorf("expected exactly one no_lawful_basis finding (resolved), got %d", resolvedCount)
		}
		return nil
	})
}

// TestAnalyzeActivity_UnknownActivityRefused mirrors RRA-01's own ActivityVisible check.
func TestAnalyzeActivity_UnknownActivityRefused(t *testing.T) {
	e := gapSetup(t, "rra04bad", true)
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AnalyzeActivity(ctx, uuid.New()); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("unknown activity: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestAnalyzeActivity_TwoTenantIsolation proves a cross-tenant activity id is refused the same way
// RRA-01's own Score already is.
func TestAnalyzeActivity_TwoTenantIsolation(t *testing.T) {
	a := gapSetup(t, "rra04iso1", true)
	b := gapSetup(t, "rra04iso2", true)
	var activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-ISO-01",
			Name: "กิจกรรมของเอ", Role: "controller"}, 0)
		activityID = act.ID
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.AnalyzeActivity(ctx, activityID); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("tenant B analyzing tenant A's activity: %v, want ErrNotFound", err)
		}
		return nil
	})
}
