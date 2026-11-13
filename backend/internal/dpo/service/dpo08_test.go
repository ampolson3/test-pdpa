package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestComplianceCalendarICS_RendersValidFeed is DPO-08's own "export iCal," a pure function needing no
// database: every item becomes one VEVENT with a stable UID, a DTSTART date and an escaped SUMMARY.
func TestComplianceCalendarICS_RendersValidFeed(t *testing.T) {
	due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	id := uuid.New()
	ics := dposervice.ComplianceCalendarICS([]dposervice.CalendarItem{
		{Source: "ropa_review", Title: "กิจกรรม A, B; ทดสอบ", DueAt: due, RefID: id},
	})
	if !strings.HasPrefix(ics, "BEGIN:VCALENDAR") || !strings.HasSuffix(strings.TrimRight(ics, "\r\n"), "END:VCALENDAR") {
		t.Fatalf("not a valid VCALENDAR envelope: %s", ics)
	}
	if !strings.Contains(ics, "BEGIN:VEVENT") || !strings.Contains(ics, "END:VEVENT") {
		t.Errorf("missing VEVENT block: %s", ics)
	}
	if !strings.Contains(ics, "DTSTART;VALUE=DATE:20261231") {
		t.Errorf("missing or wrong DTSTART: %s", ics)
	}
	if !strings.Contains(ics, "UID:ropa_review-"+id.String()+"@pdpa-platform") {
		t.Errorf("missing stable UID: %s", ics)
	}
	if !strings.Contains(ics, `กิจกรรม A\, B\; ทดสอบ`) {
		t.Errorf("SUMMARY not escaped: %s", ics)
	}
}

func TestComplianceCalendarICS_EmptyListIsStillValid(t *testing.T) {
	ics := dposervice.ComplianceCalendarICS(nil)
	if !strings.Contains(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, "END:VCALENDAR") {
		t.Errorf("empty calendar should still be a valid envelope: %s", ics)
	}
	if strings.Contains(ics, "VEVENT") {
		t.Errorf("empty calendar should have no events: %s", ics)
	}
}

type dpo08Env struct {
	app         *pgxpool.Pool
	tenant      dbtest.Tenant
	legalEntity uuid.UUID
	svc         *dposervice.Service
	ropa        *ropaservice.Service
	org         *orgservice.Service
}

func dpo08Setup(t *testing.T, suffix string) dpo08Env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	dpoSvc := &dposervice.Service{Audit: audit.New(), Org: orgSvc, Ropa: ropaSvc}

	var legalEntity uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"org.structure.update"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPO-08 จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return dpo08Env{app: app, tenant: tenant, legalEntity: legalEntity, svc: dpoSvc, ropa: ropaSvc, org: orgSvc}
}

var dpo08Perms = []string{"dpo.report.read", "ropa.activity.read", "ropa.activity.create", "org.structure.read", "org.structure.update"}

func (e dpo08Env) in(t *testing.T, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// forceNextReview sets an activity's own next_review_at directly, bypassing the review cycle's own
// multi-month arithmetic — deterministic, the same "force the date directly" pattern this package's own
// center_test.go already uses for DSAR/breach deadlines.
func (e dpo08Env) forceNextReview(t *testing.T, ctx context.Context, activityID uuid.UUID, at time.Time) {
	t.Helper()
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE ropa.processing_activities SET next_review_at = $1 WHERE id = $2`, at, activityID); err != nil {
		t.Fatalf("force next_review_at: %v", err)
	}
}

// TestComplianceCalendar_IncludesRopaReviewWithinWindowOnly is the acceptance criterion directly
// ("ปฏิทินแสดงกิจกรรมจากทุกโมดูลตรงกับกำหนดการจริง"): an activity's own review date inside the window
// appears, read live off the real column — one outside the window does not.
func TestComplianceCalendar_IncludesRopaReviewWithinWindowOnly(t *testing.T) {
	e := dpo08Setup(t, "dpo08window")
	var inWindow, outOfWindow uuid.UUID
	now := time.Now().UTC()
	e.in(t, dpo08Perms, func(ctx context.Context) error {
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: e.legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a1, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: e.legalEntity, OrgUnitID: unit.ID,
			Code: "CAL-IN-" + uuid.NewString()[:8], Name: "กิจกรรมในหน้าต่าง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		inWindow = a1.ID
		a2, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: e.legalEntity, OrgUnitID: unit.ID,
			Code: "CAL-OUT-" + uuid.NewString()[:8], Name: "กิจกรรมนอกหน้าต่าง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		outOfWindow = a2.ID
		e.forceNextReview(t, ctx, inWindow, now.AddDate(0, 0, 10))
		e.forceNextReview(t, ctx, outOfWindow, now.AddDate(0, 0, 200))
		return nil
	})

	e.in(t, dpo08Perms, func(ctx context.Context) error {
		items, err := e.svc.ComplianceCalendar(ctx, now, now.AddDate(0, 0, 90))
		if err != nil {
			return err
		}
		var sawIn, sawOut bool
		for _, it := range items {
			if it.RefID == inWindow {
				sawIn = true
				if it.Source != "ropa_review" {
					t.Errorf("source = %q, want ropa_review", it.Source)
				}
			}
			if it.RefID == outOfWindow {
				sawOut = true
			}
		}
		if !sawIn {
			t.Error("expected the in-window activity's review date to appear")
		}
		if sawOut {
			t.Error("expected the out-of-window activity's review date to be excluded")
		}
		return nil
	})
}

// TestComplianceCalendar_SkipsSourceWithoutPermission mirrors DPO-05's own Deadlines access-control
// discipline directly: dpo.report.read is not a blanket grant to every module's own detail.
func TestComplianceCalendar_SkipsSourceWithoutPermission(t *testing.T) {
	e := dpo08Setup(t, "dpo08perm")
	var activityID uuid.UUID
	now := time.Now().UTC()
	e.in(t, dpo08Perms, func(ctx context.Context) error {
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: e.legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: e.legalEntity, OrgUnitID: unit.ID,
			Code: "CAL-PERM-" + uuid.NewString()[:8], Name: "กิจกรรมทดสอบสิทธิ์", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		e.forceNextReview(t, ctx, activityID, now.AddDate(0, 0, 10))
		return nil
	})

	e.in(t, []string{"dpo.report.read"}, func(ctx context.Context) error {
		items, err := e.svc.ComplianceCalendar(ctx, now, now.AddDate(0, 0, 90))
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.RefID == activityID {
				t.Error("expected the activity's review date to be skipped without ropa.activity.read")
			}
		}
		return nil
	})
}

// TestComplianceCalendar_TwoTenantIsolation: RLS on ropa.processing_activities already scopes the list.
func TestComplianceCalendar_TwoTenantIsolation(t *testing.T) {
	a := dpo08Setup(t, "dpo08isoa")
	b := dpo08Setup(t, "dpo08isob")
	var activityID uuid.UUID
	now := time.Now().UTC()
	a.in(t, dpo08Perms, func(ctx context.Context) error {
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: a.legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: a.legalEntity, OrgUnitID: unit.ID,
			Code: "CAL-ISO-" + uuid.NewString()[:8], Name: "กิจกรรม A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID
		a.forceNextReview(t, ctx, activityID, now.AddDate(0, 0, 10))
		return nil
	})

	b.in(t, dpo08Perms, func(ctx context.Context) error {
		items, err := b.svc.ComplianceCalendar(ctx, now, now.AddDate(0, 0, 90))
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.RefID == activityID {
				t.Error("tenant B's ComplianceCalendar saw tenant A's activity")
			}
		}
		return nil
	})
}
