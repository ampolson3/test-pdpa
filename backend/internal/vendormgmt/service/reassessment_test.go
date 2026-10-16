package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

func notificationCount(t *testing.T, ctx context.Context, templateCode string) int {
	t.Helper()
	var n int
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`SELECT count(*)::int FROM platform.notifications no JOIN platform.notification_templates t ON t.id = no.template_id
		 WHERE t.code = $1`, templateCode).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestCheckDueReassessments is VEN-10's own acceptance criterion ("คู่ค้าถูกส่งแบบประเมินซ้ำตามรอบโดยไม่ต้องสั่งเอง"):
// an approved vendor whose own next_assessment_at has arrived gets notified (its relationship owner, or
// role PROC when none is set) and the clock reschedules one more tier interval out, so a second immediate
// check is a harmless no-op.
func TestCheckDueReassessments(t *testing.T) {
	e := setup(t, "venreassess")
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	var party orgservice.ExternalParty
	var owner uuid.UUID
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการทดสอบ", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Vendor Owner', 'active') RETURNING id`,
			"owner-"+party.ID.String()[:8]+"@dbtest.example").Scan(&owner); err != nil {
			return err
		}
		v, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบรอบประเมินซ้ำ", RelationshipOwnerID: &owner}, 0)
		if err != nil {
			return err
		}
		vendorID = v.ID
		// Mark the vendor approved with a tier and a next_assessment_at already due — the same end state
		// VEN-02's own RecordIntake would eventually reach, skipped straight to here since this test is
		// only about the reminder sweep, not the intake flow that sets those columns.
		_, err = pdb.MustTxFromContext(ctx).Exec(ctx,
			`UPDATE vendor.vendors SET status = 'approved', tier = 'high', next_assessment_at = current_date WHERE id = $1`, vendorID)
		return err
	})

	e.in(t, func(ctx context.Context) error { return e.svc.CheckDueReassessments(ctx) })

	e.in(t, func(ctx context.Context) error {
		if n := notificationCount(t, ctx, "vendor.reassessment_due"); n == 0 {
			t.Errorf("expected at least one vendor.reassessment_due notification, got 0")
		}
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT next_assessment_at FROM vendor.vendors WHERE id = $1`, vendorID).Scan(&next); err != nil {
			return err
		}
		if !next.Valid || !next.Time.After(time.Now().Add(24*time.Hour)) {
			t.Errorf("expected next_assessment_at rescheduled into the future, got %+v", next)
		}
		return nil
	})

	// A second immediate check is a harmless no-op: no new notification, since next_assessment_at is now
	// in the future.
	e.in(t, func(ctx context.Context) error {
		before := notificationCount(t, ctx, "vendor.reassessment_due")
		if err := e.svc.CheckDueReassessments(ctx); err != nil {
			return err
		}
		after := notificationCount(t, ctx, "vendor.reassessment_due")
		if after != before {
			t.Errorf("expected no new notification on a stale tick, got %d -> %d", before, after)
		}
		return nil
	})
}

// TestCheckDueReassessments_TwoTenantIsolation (rule 1): one tenant's due vendor never notifies another
// tenant's user, and running the sweep for tenant B finds nothing of tenant A's.
func TestCheckDueReassessments_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "venreassessiso1")
	e2 := setup(t, "venreassessiso2")
	client, err := jobs.NewInsertClient(e1.app)
	if err != nil {
		t.Fatal(err)
	}
	notifySvc := &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	e1.svc.Notify = notifySvc
	e2.svc.Notify = notifySvc

	var party orgservice.ExternalParty
	var vendorID uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		var err error
		party, err = e1.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการ A", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		v, err := e1.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "A"}, 0)
		if err != nil {
			return err
		}
		vendorID = v.ID
		_, err = pdb.MustTxFromContext(ctx).Exec(ctx,
			`UPDATE vendor.vendors SET status = 'approved', tier = 'low', next_assessment_at = current_date WHERE id = $1`, vendorID)
		return err
	})

	// Tenant B's sweep sees nothing of tenant A's due vendor.
	e2.in(t, func(ctx context.Context) error {
		if err := e2.svc.CheckDueReassessments(ctx); err != nil {
			return err
		}
		if n := notificationCount(t, ctx, "vendor.reassessment_due"); n != 0 {
			t.Errorf("tenant B's sweep should see no vendors, got %d notifications", n)
		}
		return nil
	})
}
