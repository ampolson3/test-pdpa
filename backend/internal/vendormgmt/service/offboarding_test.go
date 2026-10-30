package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/files"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

func withGrants(ctx context.Context, tenant dbtest.Tenant, perms []string) context.Context {
	return authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: perms})
}

// createApprovedVendor seeds a vendor already at ST-06#1's "approved" status — VEN-14's own transitions
// only start from approved/conditional, which this package's other tests never produce (VEN-01/02 never
// move the vendor past "prospect"/"onboarding"); forcing the status directly with SQL is the same move
// assessments_test.go's own TestDecideAssessment_RefusesOutsideOnboarding already makes.
func createApprovedVendor(t *testing.T, e env) vendorservice.Vendor {
	t.Helper()
	var party orgservice.ExternalParty
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการทดสอบ", CountryCode: "US"}, 0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var v vendorservice.Vendor
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}, func(ctx context.Context) error {
		var err error
		v, err = e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบการยุติ", IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`UPDATE vendor.vendors SET status = 'approved', row_version = row_version + 1 WHERE id = $1 RETURNING row_version`,
			v.ID).Scan(&v.RowVersion)
	}); err != nil {
		t.Fatal(err)
	}
	v.Status = "approved"
	return v
}

// fakeFiles is the minimal "caller's own clean, still-unattached upload" fixture this feature needs — the
// same shape DSAR-04/DSAR-06's own fakeFiles already use, trimmed to just Get/Attach.
type fakeFiles struct {
	raw      map[uuid.UUID]bool
	attached map[uuid.UUID]files.File
}

func newFakeFiles() *fakeFiles {
	return &fakeFiles{raw: map[uuid.UUID]bool{}, attached: map[uuid.UUID]files.File{}}
}

func (f *fakeFiles) Get(ctx context.Context, id uuid.UUID) (files.File, error) {
	if a, ok := f.attached[id]; ok {
		return a, nil
	}
	if !f.raw[id] {
		return files.File{}, errors.New("not found")
	}
	return files.File{ID: id}, nil
}

func (f *fakeFiles) Attach(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	eid := entityID
	f.attached[id] = files.File{ID: id, EntityType: entityType, EntityID: &eid}
	return nil
}

func attachableCertificate(t *testing.T, ff *fakeFiles) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ff.raw[id] = true
	return id
}

// offboardingEnv builds on vendors_test.go's own setup(), additionally granting the caller every VEN-14
// permission (update + the permanent-status-change delete) and wiring a fakeFiles for the destruction
// certificate.
func offboardingEnv(t *testing.T, suffix string) (env, *fakeFiles) {
	t.Helper()
	e := setup(t, suffix)
	ff := newFakeFiles()
	e.svc.Files = ff
	return e, ff
}

func (e env) withPerms(perms []string, fn func(ctx context.Context) error) error {
	return pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(withGrants(ctx, e.tenant, perms))
	})
}

// TestOffboarding_FullLifecycle is VEN-14's own acceptance criterion: a vendor can only be terminated once
// a destruction certificate is on file and access has been revoked.
func TestOffboarding_FullLifecycle(t *testing.T) {
	e, ff := offboardingEnv(t, "offboardlifecycle")
	v := createApprovedVendor(t, e)
	perms := []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update", "vendor.vendor.delete"}

	// Terminating a still-approved vendor is refused — ST-06#1 has no approved -> terminated edge.
	if err := e.withPerms(perms, func(ctx context.Context) error {
		_, err := e.svc.TransitionVendor(ctx, v.ID, "terminated", v.RowVersion)
		if !errors.Is(err, vendorservice.ErrInvalidTransition) {
			t.Errorf("terminate from approved: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var afterStart vendorservice.Vendor
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		afterStart, err = e.svc.TransitionVendor(ctx, v.ID, "offboarding", v.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if afterStart.Status != "offboarding" {
		t.Fatalf("status after start = %q, want offboarding", afterStart.Status)
	}

	var off vendorservice.Offboarding
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		off, err = e.svc.GetOffboarding(ctx, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if off.DataReturnStatus != "pending" || off.CompletedAt != nil {
		t.Fatalf("fresh offboarding record = %+v", off)
	}

	// Terminating before anything is recorded is refused — the acceptance criterion itself.
	if err := e.withPerms(perms, func(ctx context.Context) error {
		_, err := e.svc.TransitionVendor(ctx, v.ID, "terminated", afterStart.RowVersion)
		if !errors.Is(err, vendorservice.ErrOffboardingIncomplete) {
			t.Errorf("terminate with nothing recorded: err = %v, want ErrOffboardingIncomplete", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Access revoked but no certificate yet — still refused.
	var afterAccess vendorservice.Offboarding
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		afterAccess, err = e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{DataReturnStatus: "pending", AccessRevoked: true}, off.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if afterAccess.AccessRevokedAt == nil {
		t.Fatal("access_revoked_at should be set")
	}
	if err := e.withPerms(perms, func(ctx context.Context) error {
		_, err := e.svc.TransitionVendor(ctx, v.ID, "terminated", afterStart.RowVersion)
		if !errors.Is(err, vendorservice.ErrOffboardingIncomplete) {
			t.Errorf("terminate with no certificate: err = %v, want ErrOffboardingIncomplete", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Record the destruction certificate — now terminating succeeds.
	certID := attachableCertificate(t, ff)
	var afterCert vendorservice.Offboarding
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		afterCert, err = e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{
			DataReturnStatus: "destroyed", DestructionCertificateFileID: &certID,
		}, afterAccess.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if afterCert.DestructionCertificateFileID == nil || *afterCert.DestructionCertificateFileID != certID {
		t.Fatalf("certificate not recorded: %+v", afterCert)
	}
	if ff.attached[certID].EntityType != vendorservice.OffboardingEntityType {
		t.Errorf("certificate not attached to the offboarding record: %+v", ff.attached[certID])
	}

	var terminated vendorservice.Vendor
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		terminated, err = e.svc.TransitionVendor(ctx, v.ID, "terminated", afterStart.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if terminated.Status != "terminated" || terminated.OffboardedAt == nil {
		t.Fatalf("terminated vendor = %+v", terminated)
	}

	if err := e.withPerms(perms, func(ctx context.Context) error {
		done, err := e.svc.GetOffboarding(ctx, v.ID)
		if err != nil {
			return err
		}
		if done.CompletedAt == nil {
			t.Error("offboarding record should be marked completed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestOffboarding_NotApplicableNeedsNoCertificate: data never shared with the vendor — only access
// revocation is required to terminate.
func TestOffboarding_NotApplicableNeedsNoCertificate(t *testing.T) {
	e, _ := offboardingEnv(t, "offboardnotapplicable")
	v := createApprovedVendor(t, e)
	perms := []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update", "vendor.vendor.delete"}

	var started vendorservice.Vendor
	if err := e.withPerms(perms, func(ctx context.Context) error {
		var err error
		started, err = e.svc.TransitionVendor(ctx, v.ID, "offboarding", v.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.withPerms(perms, func(ctx context.Context) error {
		off, err := e.svc.GetOffboarding(ctx, v.ID)
		if err != nil {
			return err
		}
		if _, err := e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{DataReturnStatus: "not_applicable", AccessRevoked: true}, off.RowVersion); err != nil {
			return err
		}
		_, err = e.svc.TransitionVendor(ctx, v.ID, "terminated", started.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// TestOffboarding_RequiresDeletePermission: starting offboarding only needs .update, but the terminal,
// permanent transition needs .delete — the same reasoning ORG-06's party merge and DPIA-03's template
// retire already use for a permanent status change.
func TestOffboarding_RequiresDeletePermission(t *testing.T) {
	e, ff := offboardingEnv(t, "offboardperm")
	v := createApprovedVendor(t, e)

	var started vendorservice.Vendor
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}, func(ctx context.Context) error {
		var err error
		started, err = e.svc.TransitionVendor(ctx, v.ID, "offboarding", v.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	certID := attachableCertificate(t, ff)
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update"}, func(ctx context.Context) error {
		off, err := e.svc.GetOffboarding(ctx, v.ID)
		if err != nil {
			return err
		}
		if _, err := e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{
			DataReturnStatus: "destroyed", DestructionCertificateFileID: &certID, AccessRevoked: true,
		}, off.RowVersion); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Everything is in order, but the caller lacks .delete.
	if err := e.withPerms([]string{"vendor.vendor.read", "vendor.vendor.update"}, func(ctx context.Context) error {
		_, err := e.svc.TransitionVendor(ctx, v.ID, "terminated", started.RowVersion)
		if !errors.Is(err, vendorservice.ErrForbidden) {
			t.Errorf("terminate without .delete: err = %v, want ErrForbidden", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestOffboarding_TwoTenantIsolation: tenant B cannot read tenant A's offboarding record.
func TestOffboarding_TwoTenantIsolation(t *testing.T) {
	e, _ := offboardingEnv(t, "offboardisoa")
	eB, _ := offboardingEnv(t, "offboardisob")
	v := createApprovedVendor(t, e)
	perms := []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}

	if err := e.withPerms(perms, func(ctx context.Context) error {
		_, err := e.svc.TransitionVendor(ctx, v.ID, "offboarding", v.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := eB.withPerms(perms, func(ctx context.Context) error {
		_, err := e.svc.GetOffboarding(ctx, v.ID)
		if !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("cross-tenant read: err = %v, want ErrNotFound", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestOffboarding_ClockInjection proves access_revoked_at comes from the service's own injectable clock,
// not left blank, and that re-saving without changing access_revoked never clears an already-set time.
func TestOffboarding_ClockInjection(t *testing.T) {
	e, _ := offboardingEnv(t, "offboardclock")
	fixed := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	e.svc.Now = func() time.Time { return fixed }
	v := createApprovedVendor(t, e)
	perms := []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"}

	var off vendorservice.Offboarding
	if err := e.withPerms(perms, func(ctx context.Context) error {
		if _, err := e.svc.TransitionVendor(ctx, v.ID, "offboarding", v.RowVersion); err != nil {
			return err
		}
		var err error
		off, err = e.svc.GetOffboarding(ctx, v.ID)
		if err != nil {
			return err
		}
		off, err = e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{DataReturnStatus: "pending", AccessRevoked: true}, off.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if off.AccessRevokedAt == nil || !off.AccessRevokedAt.Equal(fixed) {
		t.Fatalf("access_revoked_at = %v, want %v", off.AccessRevokedAt, fixed)
	}

	// Re-saving with access_revoked left false doesn't clear the already-recorded time.
	if err := e.withPerms(perms, func(ctx context.Context) error {
		again, err := e.svc.UpdateOffboarding(ctx, v.ID, vendorservice.UpdateOffboardingInput{DataReturnStatus: "returned"}, off.RowVersion)
		if err != nil {
			return err
		}
		if again.AccessRevokedAt == nil || !again.AccessRevokedAt.Equal(fixed) {
			t.Errorf("access_revoked_at cleared on re-save: %v", again.AccessRevokedAt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
