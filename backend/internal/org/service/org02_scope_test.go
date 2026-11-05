package service_test

import (
	"context"
	"errors"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// inScopedTo runs fn as the tenant's admin, but with the caller's own grants restricted to exactly the
// given legal entities (ORG-02) instead of env.in's own unrestricted (scope_type="tenant") default.
func (e env) inScopedTo(t *testing.T, legalEntityIDs []string, fn func(ctx context.Context) error) {
	t.Helper()
	scopes := make([]authz.DataScope, 0, len(legalEntityIDs))
	for _, id := range legalEntityIDs {
		id := id
		scopes = append(scopes, authz.DataScope{ScopeType: "legal_entity", LegalEntityID: &id})
	}
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"org.structure.read", "org.structure.update", "org.structure.create"}, Scopes: scopes}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLegalEntityScope_UserOfCompanyANeverSeesCompanyB is ORG-02's own acceptance criterion directly:
// a caller scoped to company A's legal entity doesn't see company B's — in the legal-entity list, a
// direct Get, or either read of B's own org-unit tree — even though both are in the same tenant and both
// pass RLS; and an unrestricted (scope_type="tenant") caller, the default every seeded role assignment
// already uses, is unaffected and still sees everything.
func TestLegalEntityScope_UserOfCompanyANeverSeesCompanyB(t *testing.T) {
	e := setup(t, "org02scope")
	var coA, coB orgservice.LegalEntity
	var unitB orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		var err error
		if coA, err = e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if coB, err = e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unitB, err = e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: coB.ID, Code: "HR", NameTh: "ฝ่ายบุคคล บี", UnitType: "department"}); err != nil {
			return err
		}
		return nil
	})

	e.inScopedTo(t, []string{coA.ID.String()}, func(ctx context.Context) error {
		list, err := e.svc.ListLegalEntities(ctx)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != coA.ID {
			t.Errorf("ListLegalEntities scoped to A = %+v, want exactly [A]", list)
		}
		if _, err := e.svc.GetLegalEntity(ctx, coA.ID); err != nil {
			t.Errorf("GetLegalEntity(A) scoped to A should succeed: %v", err)
		}
		if _, err := e.svc.GetLegalEntity(ctx, coB.ID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("GetLegalEntity(B) scoped to A = %v, want ErrNotFound", err)
		}
		units, err := e.svc.ListOrgUnits(ctx, nil, true)
		if err != nil {
			return err
		}
		for _, u := range units {
			if u.LegalEntityID == coB.ID {
				t.Errorf("ListOrgUnits(no filter) scoped to A leaked B's own unit %+v", u)
			}
		}
		if _, err := e.svc.GetOrgUnit(ctx, unitB.ID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("GetOrgUnit(B's unit) scoped to A = %v, want ErrNotFound", err)
		}
		return nil
	})

	// An unrestricted (tenant-scoped) caller — the default every seeded role assignment already uses —
	// still sees both companies: no regression from this feature.
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListLegalEntities(ctx)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Errorf("ListLegalEntities for an unscoped caller = %d entities, want 2", len(list))
		}
		if _, err := e.svc.GetLegalEntity(ctx, coB.ID); err != nil {
			t.Errorf("GetLegalEntity(B) for an unscoped caller should succeed: %v", err)
		}
		if _, err := e.svc.GetOrgUnit(ctx, unitB.ID); err != nil {
			t.Errorf("GetOrgUnit(B's unit) for an unscoped caller should succeed: %v", err)
		}
		return nil
	})
}

// TestLegalEntityScope_CrossModuleWriteRefusesOutOfScopeEntity proves the same gate protects every other
// module's own rule-1 FK-visibility check (GetLegalEntity/GetOrgUnit are the shared gate dozens of
// modules already call before storing a legal_entity_id/org_unit_id) — a caller scoped to company A can't
// create an org unit under company B's legal entity either, the same as if B didn't exist for them.
func TestLegalEntityScope_CrossModuleWriteRefusesOutOfScopeEntity(t *testing.T) {
	e := setup(t, "org02write")
	var coA, coB orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		if coA, err = e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		coB, err = e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})

	e.inScopedTo(t, []string{coA.ID.String()}, func(ctx context.Context) error {
		if _, err := e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: coB.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("CreateOrgUnit under out-of-scope company B = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: coA.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); err != nil {
			t.Errorf("CreateOrgUnit under the caller's own scoped company A should succeed: %v", err)
		}
		return nil
	})
}
