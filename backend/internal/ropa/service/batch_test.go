package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// twentyTemplateIDs returns 20 of RTG-01's seeded global activity templates — enough to exercise
// RTG-04's own acceptance criterion without hardcoding 20 codes this package doesn't otherwise need.
func twentyTemplateIDs(t *testing.T, ctx context.Context) []uuid.UUID {
	t.Helper()
	rows, err := pdb.MustTxFromContext(ctx).Query(ctx, `SELECT id FROM ropa.activity_templates ORDER BY code LIMIT 20`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 20 {
		t.Fatalf("expected 20 seeded templates, got %d (RTG-01 migration 00049 should seed at least 50)", len(ids))
	}
	return ids
}

// TestCreateActivitiesFromTemplates_CreatesTwentyInOneCall is RTG-04's own acceptance criterion
// ("สร้างร่าง RoPA 20 กิจกรรมได้ในครั้งเดียว"): one call with 20 template ids against one department
// yields 20 real, independent activities, each already carrying its own template's defaults (ROPA-05).
func TestCreateActivitiesFromTemplates_CreatesTwentyInOneCall(t *testing.T) {
	e := setup(t, "ropabatch20")
	var unit orgservice.OrgUnit
	var ids []uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		ids = twentyTemplateIDs(t, ctx)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.CreateActivitiesFromTemplates(ctx, unit.ID, ids, nil)
		if err != nil {
			t.Fatalf("CreateActivitiesFromTemplates: %v", err)
		}
		if len(list) != 20 {
			t.Fatalf("expected 20 activities, got %d", len(list))
		}
		seen := map[uuid.UUID]bool{}
		for _, a := range list {
			if a.OrgUnitID != unit.ID || a.LegalEntityID != unit.LegalEntityID {
				t.Errorf("activity %s not scoped to the chosen department: %+v", a.ID, a)
			}
			if seen[a.ID] {
				t.Errorf("duplicate activity id %s in the batch", a.ID)
			}
			seen[a.ID] = true

			fresh, err := e.svc.GetActivity(ctx, a.ID)
			if err != nil {
				return err
			}
			if fresh.Name == "" {
				t.Errorf("activity %s has no name copied from its template", a.ID)
			}
		}
		return nil
	})
}

// TestCreateActivitiesFromTemplates_AllOrNothing proves a bad id anywhere in the batch rolls back the
// whole call rather than leaving a partial set of activities behind — in production this is the Tx
// middleware rolling back the one request transaction; here the test drives that same rollback
// directly (returning the service's own error from the transaction closure) rather than through e.in,
// which would swallow it and commit the activities created before the failure.
func TestCreateActivitiesFromTemplates_AllOrNothing(t *testing.T) {
	e := setup(t, "ropabatchbad")
	var unit orgservice.OrgUnit
	var goodID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		ids := twentyTemplateIDs(t, ctx)
		goodID = ids[0]
		return nil
	})

	var batchErr error
	txErr := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"ropa.inventory.read", "ropa.inventory.create", "ropa.inventory.update"}})
		_, batchErr = e.svc.CreateActivitiesFromTemplates(ctx, unit.ID, []uuid.UUID{goodID, uuid.New()}, nil)
		return batchErr
	})
	if !errors.Is(batchErr, ropaservice.ErrInvalid) {
		t.Errorf("batch with one unknown template: %v, want ErrInvalid", batchErr)
	}
	if txErr == nil {
		t.Fatal("expected the batch's own error to roll back the transaction")
	}

	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListActivities(ctx, ropaservice.ActivityFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("expected nothing committed from the failed batch, got %d activities", len(list))
		}
		return nil
	})
}

// TestCreateActivitiesFromTemplates_EmptyOrOversizedRefused validates the batch-size bounds directly.
func TestCreateActivitiesFromTemplates_EmptyOrOversizedRefused(t *testing.T) {
	e := setup(t, "ropabatchsize")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CreateActivitiesFromTemplates(ctx, unit.ID, nil, nil); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("empty batch: %v, want ErrInvalid", err)
		}
		oversized := make([]uuid.UUID, ropaservice.MaxBatchActivityTemplates+1)
		for i := range oversized {
			oversized[i] = uuid.New()
		}
		if _, err := e.svc.CreateActivitiesFromTemplates(ctx, unit.ID, oversized, nil); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("oversized batch: %v, want ErrInvalid", err)
		}
		return nil
	})
}
