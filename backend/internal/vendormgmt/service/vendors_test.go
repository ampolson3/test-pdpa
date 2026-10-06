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
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *vendorservice.Service
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
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.vendors`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.external_parties`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE email LIKE 'owner-%@dbtest.example'`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	return env{app: app, tenant: tenant, svc: &vendorservice.Service{Audit: audit.New(), Org: org}, org: org}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVendors_CRUDAndValidation(t *testing.T) {
	e := setup(t, "vendor")
	var party orgservice.ExternalParty
	var owner uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Owner', 'active') RETURNING id`,
			"owner-vendor@dbtest.example").Scan(&owner)
	})

	var created vendorservice.Vendor
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ประมวลผลเงินเดือนบนคลาวด์",
			RelationshipOwnerID: &owner, IsProcessor: true, ProcessingCountries: []string{"th", "sg"}}, 0)
		if err != nil {
			return err
		}
		if created.Status != "prospect" || created.RowVersion != 1 || created.ProcessingCountries[0] != "TH" {
			t.Errorf("create did not stick: %+v", created)
		}
		got, err := e.svc.GetVendor(ctx, created.ID)
		if err != nil {
			return err
		}
		if got.ServiceDescription != created.ServiceDescription || got.RelationshipOwnerID == nil || *got.RelationshipOwnerID != owner {
			t.Errorf("read back mismatch: %+v", got)
		}
		bogusParty, bogusOwner := uuid.New(), uuid.New()
		for name, in := range map[string]vendorservice.Vendor{
			"no service description": {PartyID: party.ID},
			"unknown party":           {PartyID: bogusParty, ServiceDescription: "x"},
			"unknown owner":           {PartyID: party.ID, ServiceDescription: "x", RelationshipOwnerID: &bogusOwner},
			"bad country":             {PartyID: party.ID, ServiceDescription: "x", ProcessingCountries: []string{"thx"}},
		} {
			if _, err := e.svc.SaveVendor(ctx, in, 0); !errors.Is(err, vendorservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		// one vendor per party (uq_vendors_party_id)
		if _, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ซ้ำ"}, 0); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("duplicate party: %v, want ErrInvalid", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{ID: created.ID, PartyID: party.ID, ServiceDescription: "x"}, 0); !errors.Is(err, vendorservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{ID: created.ID, PartyID: party.ID, ServiceDescription: "ประมวลผลเงินเดือน (แก้ไข)",
			IsProcessor: false}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.ServiceDescription != "ประมวลผลเงินเดือน (แก้ไข)" || updated.IsProcessor || updated.RowVersion != 2 {
			t.Errorf("update did not stick: %+v", updated)
		}
		// update never touches status
		if updated.Status != "prospect" {
			t.Errorf("update changed status: %+v", updated)
		}
		return nil
	})
}

func TestVendors_Isolation(t *testing.T) {
	a := setup(t, "vendora")
	b := setup(t, "vendorb")
	var party orgservice.ExternalParty
	var created vendorservice.Vendor
	a.in(t, func(ctx context.Context) error {
		var err error
		party, err = a.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "เอ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		created, err = a.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "เอ"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetVendor(ctx, created.ID); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's vendor: %v", err)
		}
		list, _, err := b.svc.ListVendors(ctx, vendorservice.VendorFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		// tenant B's own external party (not tenant A's) is a valid party_id for tenant B
		var partyB orgservice.ExternalParty
		partyB, err = b.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "บี", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		if _, err := b.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ข้ามเช่า"}, 0); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("tenant B using tenant A's party_id: %v, want ErrInvalid", err)
		}
		if _, err := b.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: partyB.ID, ServiceDescription: "บี"}, 0); err != nil {
			t.Error(err)
		}
		return nil
	})
}
