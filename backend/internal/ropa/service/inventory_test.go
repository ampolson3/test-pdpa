package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// twoCategories picks one sensitive and one non-sensitive data category from ORG-07's seeded defaults.
func twoCategories(t *testing.T, ctx context.Context, org *orgservice.Service) (sensitive, plain orgservice.MasterItem) {
	t.Helper()
	items, err := org.ListMaster(ctx, "data_categories")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.IsSensitive && sensitive.ID == nil {
			sensitive = it
		}
		if !it.IsSensitive && plain.ID == nil {
			plain = it
		}
	}
	if sensitive.ID == nil || plain.ID == nil {
		t.Fatal("expected both a sensitive and a non-sensitive default data category to be seeded (ORG-07)")
	}
	return sensitive, plain
}

func TestDataInventory_CRUDAndValidation(t *testing.T) {
	e := setup(t, "ropainv")
	var asset ropaservice.Asset
	var sensitive, plain orgservice.MasterItem
	e.in(t, func(ctx context.Context) error {
		var err error
		if asset, err = e.svc.SaveAsset(ctx, ropaservice.Asset{Name: "ระบบ HRIS", AssetType: "application"}, 0); err != nil {
			return err
		}
		sensitive, plain = twoCategories(t, ctx, e.org)
		return nil
	})

	var created ropaservice.DataInventoryItem
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: asset.ID, DataCategoryID: *sensitive.ID, Source: "direct"}, 0)
		if err != nil {
			return err
		}
		if !created.IsSensitive || created.RowVersion != 1 {
			t.Errorf("create did not stick or lost the sensitive flag: %+v", created)
		}
		got, err := e.svc.GetDataInventoryItem(ctx, created.ID)
		if err != nil {
			return err
		}
		if !got.IsSensitive || got.CategoryNameTh == "" {
			t.Errorf("read back mismatch: %+v", got)
		}
		bogus := uuid.New()
		for name, in := range map[string]ropaservice.DataInventoryItem{
			"no asset":         {DataCategoryID: *sensitive.ID},
			"no category":      {AssetID: asset.ID},
			"bad source":       {AssetID: asset.ID, DataCategoryID: *sensitive.ID, Source: "bogus"},
			"unknown asset":    {AssetID: bogus, DataCategoryID: *sensitive.ID},
			"unknown category": {AssetID: asset.ID, DataCategoryID: bogus},
			"unknown org unit": {AssetID: asset.ID, DataCategoryID: *sensitive.ID, OrgUnitID: &bogus},
			"unknown owner":    {AssetID: asset.ID, DataCategoryID: *sensitive.ID, OwnerUserID: &bogus},
		} {
			if _, err := e.svc.SaveDataInventoryItem(ctx, in, 0); !errors.Is(err, ropaservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{ID: created.ID, AssetID: asset.ID, DataCategoryID: *sensitive.ID}, 0); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{ID: created.ID, AssetID: asset.ID, DataCategoryID: *plain.ID, Source: "indirect"}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.IsSensitive || updated.RowVersion != 2 {
			t.Errorf("update did not stick or kept the old category's sensitive flag: %+v", updated)
		}
		return nil
	})
}

// TestDataInventory_SensitiveAcrossEveryDepartment is the acceptance criterion: sensitive entries are
// identified and can be filtered regardless of which department (org unit) they belong to.
func TestDataInventory_SensitiveAcrossEveryDepartment(t *testing.T) {
	e := setup(t, "ropainvsens")
	var assetA, assetB ropaservice.Asset
	var unitA, unitB orgservice.OrgUnit
	var sensitive, plain orgservice.MasterItem
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		if unitA, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"}); err != nil {
			return err
		}
		if unitB, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"}); err != nil {
			return err
		}
		if assetA, err = e.svc.SaveAsset(ctx, ropaservice.Asset{Name: "ระบบ A", AssetType: "application"}, 0); err != nil {
			return err
		}
		if assetB, err = e.svc.SaveAsset(ctx, ropaservice.Asset{Name: "ระบบ B", AssetType: "application"}, 0); err != nil {
			return err
		}
		sensitive, plain = twoCategories(t, ctx, e.org)
		if _, err := e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: assetA.ID, DataCategoryID: *sensitive.ID, OrgUnitID: &unitA.ID}, 0); err != nil {
			return err
		}
		if _, err := e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: assetB.ID, DataCategoryID: *sensitive.ID, OrgUnitID: &unitB.ID}, 0); err != nil {
			return err
		}
		if _, err := e.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: assetA.ID, DataCategoryID: *plain.ID, OrgUnitID: &unitA.ID}, 0); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		all, _, err := e.svc.ListDataInventory(ctx, ropaservice.InventoryFilter{SensitiveOnly: true})
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Errorf("sensitive_only across every department: want 2 entries (from both HR and IT), got %d: %+v", len(all), all)
		}
		seen := map[uuid.UUID]bool{}
		for _, it := range all {
			if !it.IsSensitive {
				t.Errorf("a non-sensitive entry leaked into sensitive_only: %+v", it)
			}
			if it.OrgUnitID != nil {
				seen[*it.OrgUnitID] = true
			}
		}
		if !seen[unitA.ID] || !seen[unitB.ID] {
			t.Errorf("expected sensitive entries from both departments, saw %v", seen)
		}
		hr, _, err := e.svc.ListDataInventory(ctx, ropaservice.InventoryFilter{OrgUnitID: &unitA.ID})
		if err != nil {
			return err
		}
		if len(hr) != 2 {
			t.Errorf("HR-only filter: want 2 (one sensitive, one not), got %d", len(hr))
		}
		return nil
	})
}

func TestDataInventory_Isolation(t *testing.T) {
	a := setup(t, "ropainva")
	b := setup(t, "ropainvb")
	var created ropaservice.DataInventoryItem
	a.in(t, func(ctx context.Context) error {
		asset, err := a.svc.SaveAsset(ctx, ropaservice.Asset{Name: "เอ", AssetType: "application"}, 0)
		if err != nil {
			return err
		}
		cat, _ := twoCategories(t, ctx, a.org)
		created, err = a.svc.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: asset.ID, DataCategoryID: *cat.ID}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetDataInventoryItem(ctx, created.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's entry: %v", err)
		}
		list, _, err := b.svc.ListDataInventory(ctx, ropaservice.InventoryFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
