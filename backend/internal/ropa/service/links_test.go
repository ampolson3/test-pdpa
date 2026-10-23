package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// countConsentTasks is TestNotifyConsentPurposesForReview's own read-back of the acceptance criterion's
// dedup guarantee: a dpo.tasks row per consent purpose, never more than one.
func countConsentTasks(t *testing.T, ctx context.Context, purposeID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`SELECT count(*) FROM dpo.tasks WHERE source_type = 'consent' AND source_id = $1`, purposeID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestNotifyConsentPurposesForReview is ROPA-19's own "Purpose" consumer: every one of an activity's
// purposes that cites a real consent.purposes row opens one dpo.tasks review job, and a repeat call (the
// same reasoning a second RoPA approval would trigger) opens nothing more for an already-flagged purpose.
func TestNotifyConsentPurposesForReview(t *testing.T) {
	e := setup(t, "ropalinks")
	e.svc.Dpo = &dposervice.Service{Audit: audit.New()}

	var activityID, purposeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "MKT", UnitType: "department"})
		if err != nil {
			return err
		}
		basis := lawfulBasisCode(t, ctx, e.org)
		purposeID = insertConsentPurpose(t, ctx, le.ID, basis)
		a, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ROPA19-01", Name: "ทดสอบความเชื่อมโยง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		_, err = e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a.ID, PurposeText: "การตลาด", LawfulBasisCode: basis, ConsentPurposeID: &purposeID})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.NotifyConsentPurposesForReview(ctx, activityID, "RoPA activity changed"); err != nil {
			return err
		}
		if n := countConsentTasks(t, ctx, purposeID); n != 1 {
			t.Errorf("after first notify: got %d tasks, want 1", n)
		}
		// A repeat call (the same thing a second approval would trigger) must not duplicate the task.
		if err := e.svc.NotifyConsentPurposesForReview(ctx, activityID, "RoPA activity changed"); err != nil {
			return err
		}
		if n := countConsentTasks(t, ctx, purposeID); n != 1 {
			t.Errorf("after repeat notify: got %d tasks, want still 1", n)
		}
		return nil
	})
}

// TestNotifyConsentPurposesForReview_NilDpoIsHarmless mirrors every other nil-means-skip cross-module field
// this service already has (Events/Collab) — a context that never wired Dpo (e.g. cmd/api) must not error.
func TestNotifyConsentPurposesForReview_NilDpoIsHarmless(t *testing.T) {
	e := setup(t, "ropalinksnil")

	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "MKT", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ROPA19-02", Name: "ไม่มี Dpo", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.NotifyConsentPurposesForReview(ctx, activityID, "RoPA activity changed")
	})
}

// TestGetActivityLinks is ROPA-19's own read side: the activity's own linkage tab lists every consent
// purpose one of its purposes cites, and (with Notice wired) every notice composed from it.
func TestGetActivityLinks(t *testing.T) {
	e := setup(t, "ropalinksread")

	var activityID, purposeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "MKT", UnitType: "department"})
		if err != nil {
			return err
		}
		basis := lawfulBasisCode(t, ctx, e.org)
		purposeID = insertConsentPurpose(t, ctx, le.ID, basis)
		a, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ROPA19-03", Name: "ทดสอบอ่านความเชื่อมโยง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		_, err = e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a.ID, PurposeText: "การตลาด", LawfulBasisCode: basis, ConsentPurposeID: &purposeID})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		links, err := e.svc.GetActivityLinks(ctx, activityID)
		if err != nil {
			return err
		}
		if len(links.ConsentPurposes) != 1 || links.ConsentPurposes[0].ConsentID != purposeID {
			t.Errorf("ConsentPurposes = %+v, want exactly the one linked purpose", links.ConsentPurposes)
		}
		if links.Notices != nil {
			t.Errorf("Notices = %v, want nil with no Notice interface wired", links.Notices)
		}
		return nil
	})

	// An unknown (or cross-tenant) activity id is ErrNotFound before either cross-module read ever runs.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.GetActivityLinks(ctx, uuid.New()); err == nil {
			t.Error("GetActivityLinks for an unknown activity: want an error")
		}
		return nil
	})
}

// TestGetActivityLinks_TwoTenantIsolation proves RLS, not just the activity-visibility check, actually
// scopes the read: tenant B's own activity id must be invisible from tenant A's transaction.
func TestGetActivityLinks_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ropalinksisoA")
	b := setup(t, "ropalinksisoB")

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
		act, err := a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ISO-01", Name: "A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetActivityLinks(ctx, activityID); err == nil {
			t.Error("GetActivityLinks across tenants: want an error, got none")
		}
		return nil
	})
}
