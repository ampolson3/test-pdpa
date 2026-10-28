package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestPublishSnapshot_RoundTripsAndListsNewestFirst is DFG-07's own acceptance criterion directly:
// publishing the live diagram freezes it as version 1, a second publish after a real change takes
// version 2, ListSnapshots shows both newest first, and GetSnapshot reads back each version's own graph
// exactly — "ดูแผนผังที่ publish ย้อนหลังได้".
func TestPublishSnapshot_RoundTripsAndListsNewestFirst(t *testing.T) {
	e := setup(t, "dataflowsnap1")

	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		_, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "SNAP-01", Name: "กิจกรรม 1", Role: "controller"}, 0)
		return err
	})

	var firstID string
	e.in(t, func(ctx context.Context) error {
		s, err := e.svc.PublishSnapshot(ctx, nil)
		if err != nil {
			return err
		}
		if s.VersionNo != 1 {
			t.Errorf("first publish: got version_no=%d, want 1", s.VersionNo)
		}
		if s.Graph.ActivityCount != 1 {
			t.Errorf("first publish: got activity_count=%d, want 1", s.Graph.ActivityCount)
		}
		firstID = s.ID.String()
		return nil
	})

	// a real change (a second activity) before the second publish
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท สอง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		_, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "SNAP-02", Name: "กิจกรรม 2", Role: "controller"}, 0)
		return err
	})

	var secondID string
	e.in(t, func(ctx context.Context) error {
		s, err := e.svc.PublishSnapshot(ctx, nil)
		if err != nil {
			return err
		}
		if s.VersionNo != 2 {
			t.Errorf("second publish: got version_no=%d, want 2", s.VersionNo)
		}
		if s.Graph.ActivityCount != 2 {
			t.Errorf("second publish: got activity_count=%d, want 2", s.Graph.ActivityCount)
		}
		secondID = s.ID.String()
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListSnapshots(ctx, nil)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("got %d snapshots, want 2", len(list))
		}
		if list[0].VersionNo != 2 || list[1].VersionNo != 1 {
			t.Errorf("got versions [%d, %d], want [2, 1] (newest first)", list[0].VersionNo, list[1].VersionNo)
		}

		first, err := e.svc.GetSnapshot(ctx, list[1].ID)
		if err != nil {
			return err
		}
		if first.ID.String() != firstID || first.Graph.ActivityCount != 1 {
			t.Errorf("GetSnapshot(first) = %+v, want version 1's own frozen graph (activity_count=1)", first)
		}

		second, err := e.svc.GetSnapshot(ctx, list[0].ID)
		if err != nil {
			return err
		}
		if second.ID.String() != secondID || second.Graph.ActivityCount != 2 {
			t.Errorf("GetSnapshot(second) = %+v, want version 2's own frozen graph (activity_count=2)", second)
		}
		return nil
	})
}

// TestGetSnapshot_UnknownIsNotFound mirrors every other module's own ErrNotFound-on-unknown-id convention.
func TestGetSnapshot_UnknownIsNotFound(t *testing.T) {
	e := setup(t, "dataflowsnap2")
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.GetSnapshot(ctx, uuid.New())
		if err == nil {
			t.Error("expected an error for an unknown snapshot id")
		}
		return nil
	})
}

// TestListSnapshots_TwoTenantIsolation proves RLS: tenant B never sees tenant A's own published versions.
func TestListSnapshots_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dataflowsnapisoA")
	b := setup(t, "dataflowsnapisoB")

	var snapshotID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "A", NameTh: "A", UnitType: "department"})
		if err != nil {
			return err
		}
		if _, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ISO-01", Name: "A", Role: "controller"}, 0); err != nil {
			return err
		}
		s, err := a.svc.PublishSnapshot(ctx, nil)
		if err != nil {
			return err
		}
		snapshotID = s.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		list, err := b.svc.ListSnapshots(ctx, nil)
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.ID == snapshotID {
				t.Error("tenant B must never see tenant A's own published snapshot")
			}
		}
		if _, err := b.svc.GetSnapshot(ctx, snapshotID); err == nil {
			t.Error("tenant B must not be able to read tenant A's own snapshot by id")
		}
		return nil
	})
}
