package service_test

import (
	"context"
	"errors"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// Acceptance (ORG-07): a new tenant has the full default set at once; an entry changed in one place is what
// every module reading it by id sees.
func TestMasterData_DefaultsAndOneSource(t *testing.T) {
	e := setup(t, "orgmd")
	e.in(t, func(ctx context.Context) error {
		want := map[string]int{orgservice.KindDataCategories: 19, orgservice.KindSubjectTypes: 9, orgservice.KindPurposes: 10, orgservice.KindLawfulBases: 13, orgservice.KindCountries: 249}
		for kind, n := range want {
			list, err := e.svc.ListMaster(ctx, kind)
			if err != nil || len(list) != n || !list[0].Global {
				t.Errorf("%s: %d defaults, %v (want %d)", kind, len(list), err, n)
			}
		}
		cats, _ := e.svc.ListMaster(ctx, orgservice.KindDataCategories)
		sensitive := 0
		for _, c := range cats {
			if c.IsSensitive {
				sensitive++
			}
		}
		if sensitive != 10 {
			t.Errorf("s.26 sensitive categories: %d", sensitive)
		}
		bases, _ := e.svc.ListMaster(ctx, orgservice.KindLawfulBases)
		for _, b := range bases {
			if b.Code == "LEGITIMATE_INTEREST" && !b.RequiresLIA || b.Code == "EXPLICIT_CONSENT" && !(b.ForSensitive && b.RequiresConsent) {
				t.Errorf("lawful basis flags: %+v", b)
			}
		}
		if _, err := e.svc.ListMaster(ctx, "nope"); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("unknown kind: %v", err)
		}

		// The tenant's own entries: create, change (same id — every reference sees it), delete.
		health := cats[0]
		for _, c := range cats {
			if c.Code == "health" {
				health = c
			}
		}
		own, err := e.svc.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "vaccination", NameTh: "ประวัติการฉีดวัคซีน", IsSensitive: true, SensitiveType: "health", ParentID: health.ID})
		if err != nil || own.Global || *own.ParentID != *health.ID {
			t.Fatalf("create: %+v %v", own, err)
		}
		changed, err := e.svc.UpdateMaster(ctx, orgservice.KindDataCategories, *own.ID, own.RowVersion, orgservice.MasterItem{NameTh: "ข้อมูลการฉีดวัคซีน", IsSensitive: true, SensitiveType: "health", ParentID: health.ID})
		if err != nil || *changed.ID != *own.ID || changed.NameTh != "ข้อมูลการฉีดวัคซีน" {
			t.Errorf("update in place: %+v %v", changed, err)
		}
		if _, err := e.svc.UpdateMaster(ctx, orgservice.KindDataCategories, *own.ID, own.RowVersion, orgservice.MasterItem{NameTh: "x"}); !errors.Is(err, orgservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		// Defaults are read-only; their codes can't be reused; lawful bases and countries can't be added to.
		if _, err := e.svc.UpdateMaster(ctx, orgservice.KindDataCategories, *health.ID, health.RowVersion, orgservice.MasterItem{NameTh: "x"}); !errors.Is(err, orgservice.ErrReadOnly) {
			t.Errorf("edit a default: %v", err)
		}
		if err := e.svc.DeleteMaster(ctx, orgservice.KindDataCategories, *health.ID, health.RowVersion); !errors.Is(err, orgservice.ErrReadOnly) {
			t.Errorf("delete a default: %v", err)
		}
		if _, err := e.svc.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "health", NameTh: "x"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("shadow a default code: %v", err)
		}
		if _, err := e.svc.CreateMaster(ctx, orgservice.KindCountries, orgservice.MasterItem{Code: "zz", NameTh: "x"}); !errors.Is(err, orgservice.ErrReadOnly) {
			t.Errorf("add a country: %v", err)
		}
		if _, err := e.svc.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "Bad Code", NameTh: "x"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("bad code: %v", err)
		}
		if _, err := e.svc.CreateMaster(ctx, orgservice.KindPurposes, orgservice.MasterItem{Code: "loyalty", NameTh: "สะสมแต้ม", Category: "marketing"}); err != nil {
			t.Errorf("purpose: %v", err)
		}
		// A category referred to by another (its child) is in use.
		child, _ := e.svc.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "covid_test", NameTh: "ผลตรวจโควิด", IsSensitive: true, SensitiveType: "health", ParentID: own.ID})
		if err := e.svc.DeleteMaster(ctx, orgservice.KindDataCategories, *own.ID, changed.RowVersion); !errors.Is(err, orgservice.ErrInUse) {
			t.Errorf("delete a parent in use: %v", err)
		}
		if err := e.svc.DeleteMaster(ctx, orgservice.KindDataCategories, *child.ID, child.RowVersion); err != nil {
			t.Errorf("delete: %v", err)
		}
		var n int
		_ = pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE entity_type = 'master_data'`).Scan(&n)
		if n != 5 {
			t.Errorf("audit rows: %d, want 5", n)
		}
		return nil
	})
}

func TestMasterData_TenantIsolation(t *testing.T) {
	a, b := setup(t, "orgmda"), setup(t, "orgmdb")
	var own orgservice.MasterItem
	a.in(t, func(ctx context.Context) error {
		var err error
		own, err = a.svc.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "member", NameTh: "สมาชิก"})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		list, _ := b.svc.ListMaster(ctx, orgservice.KindSubjectTypes)
		for _, it := range list {
			if !it.Global {
				t.Errorf("B sees A's entry %s", it.Code)
			}
		}
		if _, err := b.svc.UpdateMaster(ctx, orgservice.KindSubjectTypes, *own.ID, own.RowVersion, orgservice.MasterItem{NameTh: "x"}); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B edits A's entry: %v", err)
		}
		if _, err := b.svc.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "x_child", NameTh: "x", ParentID: own.ID}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("B links to A's row: %v", err)
		}
		// B may use the same code for its own entry.
		_, err := b.svc.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "member", NameTh: "สมาชิก B"})
		return err
	})
}
