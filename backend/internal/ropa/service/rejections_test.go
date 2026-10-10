package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// seedDsarRequest inserts a minimal dsar.requests row so a real FK can point at it — ropa's own tests don't
// pull in the dsar module, so a real request is seeded via raw SQL, the same pattern other modules' tests use
// for an FK column they don't otherwise need a whole sibling service for.
func seedDsarRequest(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	tx := pdb.MustTxFromContext(ctx)
	var typeID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM dsar.request_types LIMIT 1`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO dsar.requests (tenant_id, request_no, request_type_id, legal_entity_id, channel,
		requester_name_enc, requester_contact_enc, requester_blind_index, received_at, due_at)
		SELECT current_setting('app.tenant_id')::uuid, 'DSAR-T-'||substr(gen_random_uuid()::text, 1, 8), $1, id, 'web', '\x00', '\x00', '\x00', now(), now()
		FROM org.legal_entities WHERE tenant_id = current_setting('app.tenant_id')::uuid LIMIT 1
		RETURNING id`, typeID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRecordRejection_LogsAgainstActivityAndIsIdempotent is ROPA-10's acceptance criterion: a DSAR rejection
// shows up against the referenced activity automatically, and a redelivery of the same (activity, request)
// pair (the outbox's at-least-once delivery) is a harmless no-op rather than a duplicate row.
func TestRecordRejection_LogsAgainstActivityAndIsIdempotent(t *testing.T) {
	e := setup(t, "roparej")
	var activityID uuid.UUID
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		var err error
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "REJ-01", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		activityID = a.ID
		return err
	})

	var requestID uuid.UUID
	rejectedAt := time.Now().UTC().Truncate(time.Second)
	e.in(t, func(ctx context.Context) error {
		requestID = seedDsarRequest(t, ctx)
		return e.svc.RecordRejection(ctx, activityID, requestID, "ไม่พบข้อมูล", rejectedAt)
	})
	e.in(t, func(ctx context.Context) error {
		// Redelivery of the same event: same (activity, request) pair, must not duplicate.
		return e.svc.RecordRejection(ctx, activityID, requestID, "ไม่พบข้อมูล", rejectedAt)
	})
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListActivityRejections(ctx, activityID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("rejections = %d, want 1 (idempotent)", len(list))
		}
		if list[0].DsarRequestID != requestID || list[0].ReasonCode != "ไม่พบข้อมูล" {
			t.Errorf("rejection = %+v", list[0])
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		err := e.svc.RecordRejection(ctx, uuid.New(), requestID, "reason", rejectedAt)
		if !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown activity: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestRecordRejection_TenantIsolation: tenant B's activity cannot be logged against by tenant A's call.
func TestRecordRejection_TenantIsolation(t *testing.T) {
	a := setup(t, "roparejiso-a")
	b := setup(t, "roparejiso-b")
	var activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "REJ-A", Name: "x", Role: "controller"}, 0)
		activityID = act.ID
		return err
	})
	b.in(t, func(ctx context.Context) error {
		err := b.svc.RecordRejection(ctx, activityID, uuid.New(), "reason", time.Now())
		if !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("cross-tenant activity: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}
