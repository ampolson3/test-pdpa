package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// subProcessorEnv wires a real River insert client + notify.Service (the same minimal construction
// VEN-06/VEN-10's own test envs already use) plus a real iam.users row holding role DPO, the recipient
// AddSubProcessor's own acceptance-criterion alert notifies.
func subProcessorEnv(t *testing.T, suffix string) env {
	t.Helper()
	e := setup(t, suffix)
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.River = client
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	owner := dbtest.OwnerPool(t)
	var dpoUser uuid.UUID
	if err := pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'DPO Team', 'active') RETURNING id`,
			e.tenant.ID, uuid.NewString()[:8]+"@subproc.example").Scan(&dpoUser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'DPO' AND tenant_id IS NULL`, e.tenant.ID, dpoUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.sub_processors`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.role_assignments WHERE user_id = $1`, dpoUser)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, dpoUser)
			return nil
		})
	})
	return e
}

func createVendorForSubProcessor(t *testing.T, ctx context.Context, e env) vendorservice.Vendor {
	t.Helper()
	party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการทดสอบ", CountryCode: "US"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบผู้ประมวลผลช่วง", IsProcessor: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestAddSubProcessor_StartsPendingAndAlertsDPO is the acceptance criterion's data half: a new
// sub-processor always starts pending (so the UI can flag it as unapproved) and notifies every DPO.
func TestAddSubProcessor_StartsPendingAndAlertsDPO(t *testing.T) {
	e := subProcessorEnv(t, "vensubproc")
	var v vendorservice.Vendor
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		v = createVendorForSubProcessor(t, ctx, e)
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ประมวลผลช่วงทดสอบ", CountryCode: "SG"}, 0)
		return err
	})

	var sp vendorservice.SubProcessor
	e.in(t, func(ctx context.Context) error {
		var err error
		sp, err = e.svc.AddSubProcessor(ctx, v.ID, vendorservice.SubProcessorInput{PartyID: party.ID, Service: "โฮสติง", Countries: []string{"SG"}})
		return err
	})
	if sp.Status != "pending" {
		t.Errorf("status = %q, want pending", sp.Status)
	}

	// an unknown vendor or party is refused.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddSubProcessor(ctx, uuid.New(), vendorservice.SubProcessorInput{PartyID: party.ID, Service: "x"}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("unknown vendor: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddSubProcessor(ctx, v.ID, vendorservice.SubProcessorInput{PartyID: uuid.New(), Service: "x"}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("unknown party: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddSubProcessor(ctx, v.ID, vendorservice.SubProcessorInput{PartyID: party.ID}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("empty service: %v, want ErrInvalid", err)
		}
		return nil
	})

	var count int
	e.in(t, func(ctx context.Context) error {
		row := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications n
			JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'vendor.sub_processor_added'`)
		return row.Scan(&count)
	})
	if count == 0 {
		t.Fatal("expected at least one vendor.sub_processor_added notification (role DPO)")
	}

	// ListSubProcessors round-trip, newest first.
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListSubProcessors(ctx, v.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != sp.ID {
			t.Errorf("ListSubProcessors = %+v, want exactly the one sub-processor", list)
		}
		return nil
	})
}

// TestDecideSubProcessor_NeedsApprovePermissionAndClearsPending is the acceptance criterion's other half:
// deciding a pending sub-processor needs vendor.vendor.approve (not just .update), and a decision clears
// it from "unapproved" (pending) — a stale ETag or a non-pending row is refused.
func TestDecideSubProcessor_NeedsApprovePermissionAndClearsPending(t *testing.T) {
	e := subProcessorEnv(t, "vensubprocdecide")
	var v vendorservice.Vendor
	var sp vendorservice.SubProcessor
	e.in(t, func(ctx context.Context) error {
		v = createVendorForSubProcessor(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ประมวลผลช่วงทดสอบ", CountryCode: "SG"}, 0)
		if err != nil {
			return err
		}
		sp, err = e.svc.AddSubProcessor(ctx, v.ID, vendorservice.SubProcessorInput{PartyID: party.ID, Service: "โฮสติง"})
		return err
	})

	// a caller with only .update (no .approve) is refused.
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}, func(ctx context.Context) error {
		_, err := e.svc.DecideSubProcessor(ctx, v.ID, sp.ID, "approved", sp.RowVersion)
		return err
	}); !errors.Is(err, vendorservice.ErrForbidden) {
		t.Errorf("no-approve decide: %v, want ErrForbidden", err)
	}

	// a bad decision value is refused.
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update", "vendor.vendor.approve"}, func(ctx context.Context) error {
		_, err := e.svc.DecideSubProcessor(ctx, v.ID, sp.ID, "maybe", sp.RowVersion)
		return err
	}); !errors.Is(err, vendorservice.ErrInvalid) {
		t.Errorf("bad decision: %v, want ErrInvalid", err)
	}

	// a stale ETag is refused.
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update", "vendor.vendor.approve"}, func(ctx context.Context) error {
		_, err := e.svc.DecideSubProcessor(ctx, v.ID, sp.ID, "approved", sp.RowVersion+1)
		return err
	}); !errors.Is(err, vendorservice.ErrVersionMismatch) {
		t.Errorf("stale ETag: %v, want ErrVersionMismatch", err)
	}

	// the real decision succeeds and clears pending.
	var decided vendorservice.SubProcessor
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update", "vendor.vendor.approve"}, func(ctx context.Context) error {
		var err error
		decided, err = e.svc.DecideSubProcessor(ctx, v.ID, sp.ID, "approved", sp.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if decided.Status != "approved" || decided.ApprovedBy == nil || decided.ApprovedAt == nil {
		t.Errorf("decided = %+v, want approved with approver + timestamp", decided)
	}

	// deciding an already-decided row again is refused (status != pending).
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update", "vendor.vendor.approve"}, func(ctx context.Context) error {
		_, err := e.svc.DecideSubProcessor(ctx, v.ID, sp.ID, "rejected", decided.RowVersion)
		return err
	}); !errors.Is(err, vendorservice.ErrVersionMismatch) {
		t.Errorf("re-decide: %v, want ErrVersionMismatch", err)
	}
}

// TestSubProcessors_TwoTenantIsolation (rule 1).
func TestSubProcessors_TwoTenantIsolation(t *testing.T) {
	eA := subProcessorEnv(t, "vensubprociso-a")
	eB := subProcessorEnv(t, "vensubprociso-b")

	var vA vendorservice.Vendor
	var spA vendorservice.SubProcessor
	eA.in(t, func(ctx context.Context) error {
		vA = createVendorForSubProcessor(t, ctx, eA)
		party, err := eA.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "A", CountryCode: "SG"}, 0)
		if err != nil {
			return err
		}
		spA, err = eA.svc.AddSubProcessor(ctx, vA.ID, vendorservice.SubProcessorInput{PartyID: party.ID, Service: "x"})
		return err
	})

	eB.in(t, func(ctx context.Context) error {
		list, err := eB.svc.ListSubProcessors(ctx, vA.ID)
		if err != nil && !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("cross-tenant list: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("cross-tenant list leaked rows: %+v", list)
		}
		if err := eB.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update", "vendor.vendor.approve"}, func(ctx context.Context) error {
			_, err := eB.svc.DecideSubProcessor(ctx, vA.ID, spA.ID, "approved", spA.RowVersion)
			return err
		}); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("cross-tenant decide: %v, want ErrNotFound", err)
		}
		return nil
	})
}
