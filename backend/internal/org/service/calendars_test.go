package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/bizcal"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/importer"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *orgservice.Service
}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_settings`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.holidays`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.business_calendars`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `UPDATE org.legal_entities SET parent_id = NULL`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.files`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	return env{app: app, tenant: tenant, svc: &orgservice.Service{Audit: audit.New()}}
}

// in runs fn as the tenant's admin in one transaction (as the Tx middleware would).
func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"org.settings.read", "org.settings.update"}}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestCalendars_DefaultAndValidation(t *testing.T) {
	e := setup(t, "orgcal")
	var hq, factory orgservice.Calendar
	e.in(t, func(ctx context.Context) error {
		var err error
		if hq, err = e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "สำนักงานใหญ่", Timezone: "Asia/Bangkok", Workdays: []int{5, 1, 2, 3, 4, 1}}); err != nil {
			return err
		}
		if !hq.IsDefault || len(hq.Workdays) != 5 || hq.Workdays[0] != 1 {
			t.Errorf("first calendar should be default with sorted, deduplicated workdays: %+v", hq)
		}
		if factory, err = e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "Factory", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5, 6}}); err != nil {
			return err
		}
		if factory.IsDefault {
			t.Error("second calendar should not become default by itself")
		}
		for name, in := range map[string]orgservice.CalendarInput{
			"empty name":   {Name: " ", Timezone: "Asia/Bangkok", Workdays: []int{1}},
			"bad timezone": {Name: "x", Timezone: "Mars/Olympus", Workdays: []int{1}},
			"no workdays":  {Name: "x", Timezone: "Asia/Bangkok"},
			"workday 8":    {Name: "x", Timezone: "Asia/Bangkok", Workdays: []int{8}},
		} {
			if _, err := e.svc.CreateCalendar(ctx, in); !errors.Is(err, orgservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		// Duplicate names (case-insensitive) are refused and the transaction stays usable.
		if _, err := e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "FACTORY", Timezone: "Asia/Bangkok", Workdays: []int{1}}); !errors.Is(err, orgservice.ErrDuplicateName) {
			t.Errorf("duplicate name: %v", err)
		}
		// Moving the default.
		moved, err := e.svc.UpdateCalendar(ctx, factory.ID, factory.RowVersion, orgservice.CalendarInput{Name: "Factory", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5, 6}, MakeDefault: true})
		if err != nil || !moved.IsDefault {
			t.Fatalf("make default: %+v %v", moved, err)
		}
		if _, err := e.svc.UpdateCalendar(ctx, factory.ID, factory.RowVersion, orgservice.CalendarInput{Name: "F", Timezone: "UTC", Workdays: []int{1}}); !errors.Is(err, orgservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		list, err := e.svc.ListCalendars(ctx)
		if err != nil || len(list) != 2 || list[0].ID != factory.ID || list[1].IsDefault {
			t.Errorf("list: %+v %v (default first, only one default)", list, err)
		}
		var setting uuid.UUID
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT default_calendar_id FROM org.org_settings`).Scan(&setting); err != nil || setting != factory.ID {
			t.Errorf("org_settings.default_calendar_id = %v %v, want %v", setting, err, factory.ID)
		}
		var n int
		_ = pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE entity_type = 'business_calendar'`).Scan(&n)
		if n < 3 {
			t.Errorf("audit rows: %d, want create ×2 + update", n)
		}
		return nil
	})
}

// Acceptance (ORG-20): SLAs count on the tenant's calendar — its workdays and its holidays.
func TestBusinessCalendar_CountsOnTenantCalendar(t *testing.T) {
	e := setup(t, "orgsla")
	e.in(t, func(ctx context.Context) error {
		c, id, err := e.svc.BusinessCalendar(ctx, nil)
		if err != nil || id != nil || !c.IsBusinessDay(time.Date(2026, 4, 13, 9, 0, 0, 0, bizcal.Bangkok)) {
			t.Errorf("no calendar yet: want the built-in default (%v, %v)", id, err)
		}
		cal, err := e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "HQ", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5}})
		if err != nil {
			return err
		}
		for _, d := range []int{13, 14, 15} {
			if _, err := e.svc.PutHoliday(ctx, cal.ID, day(2026, 4, d), "สงกรานต์"); err != nil {
				return err
			}
		}
		c, id, err = e.svc.BusinessCalendar(ctx, nil)
		if err != nil || id == nil || *id != cal.ID {
			t.Fatalf("default calendar: %v %v", id, err)
		}
		due, err := c.AddBusinessDays(time.Date(2026, 4, 10, 14, 0, 0, 0, bizcal.Bangkok), 1) // Friday before Songkran
		if want := time.Date(2026, 4, 16, 14, 0, 0, 0, bizcal.Bangkok); err != nil || !due.Equal(want) {
			t.Errorf("due %v, want %v", due, want)
		}
		if _, _, err := e.svc.BusinessCalendar(ctx, &cal.ID); err != nil {
			t.Errorf("by id: %v", err)
		}
		missing := uuid.New()
		if _, _, err := e.svc.BusinessCalendar(ctx, &missing); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("unknown id: %v", err)
		}
		return nil
	})
}

func TestHolidays(t *testing.T) {
	e := setup(t, "orghol")
	e.in(t, func(ctx context.Context) error {
		cal, err := e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "HQ", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5}})
		if err != nil {
			return err
		}
		for _, h := range []orgservice.Holiday{{Date: day(2026, 12, 31), Name: "สิ้นปี"}, {Date: day(2027, 1, 1), Name: "ปีใหม่"}, {Date: day(2026, 4, 13), Name: "x"}} {
			if _, err := e.svc.PutHoliday(ctx, cal.ID, h.Date, h.Name); err != nil {
				return err
			}
		}
		if h, err := e.svc.PutHoliday(ctx, cal.ID, day(2026, 4, 13), "วันสงกรานต์"); err != nil || h.Name != "วันสงกรานต์" {
			t.Errorf("rename: %+v %v", h, err)
		}
		if _, err := e.svc.PutHoliday(ctx, cal.ID, day(2026, 5, 1), " "); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("blank name: %v", err)
		}
		y26, err := e.svc.ListHolidays(ctx, cal.ID, 2026)
		if err != nil || len(y26) != 2 || !y26[0].Date.Equal(day(2026, 4, 13)) || y26[0].Name != "วันสงกรานต์" {
			t.Errorf("2026: %+v %v", y26, err)
		}
		if all, _ := e.svc.ListHolidays(ctx, cal.ID, 0); len(all) != 3 {
			t.Errorf("all years: %d", len(all))
		}
		if err := e.svc.DeleteHoliday(ctx, cal.ID, day(2026, 12, 31)); err != nil {
			t.Errorf("delete: %v", err)
		}
		if err := e.svc.DeleteHoliday(ctx, cal.ID, day(2026, 12, 31)); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("delete again: %v", err)
		}
		if _, err := e.svc.ListHolidays(ctx, uuid.New(), 0); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("unknown calendar: %v", err)
		}
		return nil
	})
}

// A tenant sees, uses and changes only its own calendars (CLAUDE.md rule 1).
func TestCalendars_TenantIsolation(t *testing.T) {
	a, b := setup(t, "orgisoa"), setup(t, "orgisob")
	var cal orgservice.Calendar
	a.in(t, func(ctx context.Context) error {
		var err error
		cal, err = a.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "A", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5}})
		if err != nil {
			return err
		}
		_, err = a.svc.PutHoliday(ctx, cal.ID, day(2026, 4, 13), "A holiday")
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if list, err := b.svc.ListCalendars(ctx); err != nil || len(list) != 0 {
			t.Errorf("B lists A's calendars: %+v %v", list, err)
		}
		if _, err := b.svc.ListHolidays(ctx, cal.ID, 0); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B reads A's holidays: %v", err)
		}
		if _, err := b.svc.PutHoliday(ctx, cal.ID, day(2026, 5, 1), "B"); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B writes into A's calendar: %v", err)
		}
		if err := b.svc.DeleteHoliday(ctx, cal.ID, day(2026, 4, 13)); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B deletes A's holiday: %v", err)
		}
		if _, err := b.svc.UpdateCalendar(ctx, cal.ID, cal.RowVersion, orgservice.CalendarInput{Name: "B", Timezone: "UTC", Workdays: []int{1}}); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B updates A's calendar: %v", err)
		}
		if _, _, err := b.svc.BusinessCalendar(ctx, &cal.ID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B counts on A's calendar: %v", err)
		}
		// B's default is still the built-in one, and B may reuse A's calendar name.
		if _, id, _ := b.svc.BusinessCalendar(ctx, nil); id != nil {
			t.Error("B has a default calendar it never created")
		}
		_, err := b.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "A", Timezone: "Asia/Bangkok", Workdays: []int{1}})
		return err
	})
	a.in(t, func(ctx context.Context) error {
		if hs, _ := a.svc.ListHolidays(ctx, cal.ID, 0); len(hs) != 1 || hs[0].Name != "A holiday" {
			t.Errorf("A's holidays changed: %+v", hs)
		}
		return nil
	})
}

func TestHolidayImportType(t *testing.T) {
	e := setup(t, "orgimp")
	typ := e.svc.HolidayImport()
	e.in(t, func(ctx context.Context) error {
		row := importer.Row{"date": "13/4/2569", "name": "สงกรานต์", "calendar": ""}
		if errs := typ.Validate(ctx, 2, row); len(errs) != 1 || errs[0].Message != "no_default_calendar" {
			t.Errorf("no calendar yet: %+v", errs)
		}
		hq, err := e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "HQ", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5}})
		if err != nil {
			return err
		}
		plant, err := e.svc.CreateCalendar(ctx, orgservice.CalendarInput{Name: "Plant", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5, 6}})
		if err != nil {
			return err
		}
		if errs := typ.Validate(ctx, 2, importer.Row{"date": "4/13/2026", "name": "x", "calendar": "Nowhere"}); len(errs) != 2 {
			t.Errorf("bad date + unknown calendar: %+v", errs)
		}
		for _, r := range []importer.Row{
			{"date": "13/4/2569", "name": "สงกรานต์", "calendar": ""},       // BE year, default calendar
			{"date": "2026-04-14", "name": "สงกรานต์", "calendar": "plant"}, // by name, any case
			{"date": "46127", "name": "สงกรานต์", "calendar": "HQ"},         // Excel serial = 2026-04-15
		} {
			if errs := typ.Validate(ctx, 2, r); len(errs) > 0 {
				t.Errorf("%v: %+v", r, errs)
				continue
			}
			if err := typ.Apply(ctx, 2, r); err != nil {
				return err
			}
		}
		if hs, _ := e.svc.ListHolidays(ctx, hq.ID, 2026); len(hs) != 2 || !hs[0].Date.Equal(day(2026, 4, 13)) || !hs[1].Date.Equal(day(2026, 4, 15)) {
			t.Errorf("HQ holidays: %+v", hs)
		}
		if hs, _ := e.svc.ListHolidays(ctx, plant.ID, 2026); len(hs) != 1 || !hs[0].Date.Equal(day(2026, 4, 14)) {
			t.Errorf("Plant holidays: %+v", hs)
		}
		return nil
	})
}
