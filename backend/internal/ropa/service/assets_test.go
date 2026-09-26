package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *ropaservice.Service
	org    *orgservice.Service
}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.assets`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.external_parties`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `UPDATE org.legal_entities SET parent_id = NULL`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	return env{app: app, tenant: tenant, svc: &ropaservice.Service{Audit: audit.New(), Org: org}, org: org}
}

// in runs fn as the tenant's admin in one transaction (as the Tx middleware would).
func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"ropa.inventory.read", "ropa.inventory.create", "ropa.inventory.update"}}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAssets_CRUDAndValidation(t *testing.T) {
	e := setup(t, "ropaasset")
	var unit orgservice.OrgUnit
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"}); err != nil {
			return err
		}
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "US"}, 0)
		return err
	})

	var created ropaservice.Asset
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveAsset(ctx, ropaservice.Asset{Name: "ระบบ HRIS", AssetType: "application", OrgUnitID: &unit.ID,
			ProviderPartyID: &party.ID, HostingCountryCode: "th", HostingType: "cloud"}, 0)
		if err != nil {
			return err
		}
		if created.HostingCountryCode != "TH" || created.Status != "active" || created.RowVersion != 1 {
			t.Errorf("create did not stick: %+v", created)
		}
		got, err := e.svc.GetAsset(ctx, created.ID)
		if err != nil {
			return err
		}
		if got.Name != created.Name || got.OrgUnitID == nil || *got.OrgUnitID != unit.ID {
			t.Errorf("read back mismatch: %+v", got)
		}
		bogus := uuid.New()
		for name, in := range map[string]ropaservice.Asset{
			"no name":            {AssetType: "application"},
			"bad asset type":     {Name: "x", AssetType: "bogus"},
			"bad hosting type":   {Name: "x", AssetType: "application", HostingType: "bogus"},
			"bad classification": {Name: "x", AssetType: "application", Classification: "bogus"},
			"unknown org unit":   {Name: "x", AssetType: "application", OrgUnitID: &bogus},
			"unknown party":      {Name: "x", AssetType: "application", ProviderPartyID: &bogus},
		} {
			if _, err := e.svc.SaveAsset(ctx, in, 0); !errors.Is(err, ropaservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveAsset(ctx, ropaservice.Asset{ID: created.ID, Name: "x", AssetType: "application"}, 0); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.SaveAsset(ctx, ropaservice.Asset{ID: created.ID, Name: "ระบบ HRIS (สำรอง)", AssetType: "database", Status: "retired"}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.AssetType != "database" || updated.Status != "retired" || updated.RowVersion != 2 {
			t.Errorf("update did not stick: %+v", updated)
		}
		return nil
	})
}

func TestAssets_Isolation(t *testing.T) {
	a := setup(t, "ropaasseta")
	b := setup(t, "ropaassetb")
	var created ropaservice.Asset
	a.in(t, func(ctx context.Context) error {
		var err error
		created, err = a.svc.SaveAsset(ctx, ropaservice.Asset{Name: "เอ", AssetType: "application"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetAsset(ctx, created.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's asset: %v", err)
		}
		list, _, err := b.svc.ListAssets(ctx, ropaservice.AssetFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
