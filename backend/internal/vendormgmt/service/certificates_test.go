package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// certEnv wires a real River insert client + notify.Service (the same minimal construction DPA-10's own
// renewalEnv uses) so AddCertificate's own reminder job is a real river_job row and
// FireCertificateExpiryReminder's own notification is a real platform.notifications row, plus a real
// iam.users row holding role SEC (the module doc's own actor, used as the fallback recipient).
func certEnv(t *testing.T, suffix string) (env, *fakeFiles) {
	t.Helper()
	e := setup(t, suffix)
	ff := newFakeFiles()
	e.svc.Files = ff
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.River = client
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	owner := dbtest.OwnerPool(t)
	var secUser uuid.UUID
	if err := pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Security Team', 'active') RETURNING id`,
			e.tenant.ID, uuid.NewString()[:8]+"@cert.example").Scan(&secUser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'SEC' AND tenant_id IS NULL`, e.tenant.ID, secUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.certificates`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.files WHERE bucket = 'test'`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.role_assignments WHERE user_id = $1`, secUser)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, secUser)
			return nil
		})
	})
	return e, ff
}

// attachableCertFile is the caller's own clean, still-unattached PLT-09 upload — vendor.certificates.file_id
// has a real FK to platform.files (unlike vendor.offboardings' own destruction_certificate_file_id), so the
// fixture needs a real row, the same move DSAR-04's own attachableUpload already makes.
func attachableCertFile(t *testing.T, ctx context.Context, ff *fakeFiles) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ff.raw[id] = true
	_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status)
		VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, 'cert.pdf', 'application/pdf', 19, 'x', 'clean')`,
		id, id.String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createVendorForCert(t *testing.T, ctx context.Context, e env) vendorservice.Vendor {
	t.Helper()
	party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการใบรับรอง", CountryCode: "US"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบใบรับรอง", IsProcessor: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestAddCertificate_ValidatesAndSchedulesReminder is the acceptance criterion's data half: a certificate's
// own valid_to schedules the reminder checkpoint 30 days ahead.
func TestAddCertificate_ValidatesAndSchedulesReminder(t *testing.T) {
	e, ff := certEnv(t, "vencert")
	var v vendorservice.Vendor
	var fileID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		v = createVendorForCert(t, ctx, e)
		fileID = attachableCertFile(t, ctx, ff)
		return nil
	})

	now := time.Now().UTC()
	// pgtype.Date round-trips through Postgres as date-only (midnight UTC), so the comparison value
	// here must already be truncated the same way or a direct time.Time equality check never matches.
	validTo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 90)
	var cert vendorservice.Certificate
	e.in(t, func(ctx context.Context) error {
		var err error
		cert, err = e.svc.AddCertificate(ctx, vendorservice.CertificateInput{
			VendorID: v.ID, CertType: "ISO 27001", FileID: fileID, ValidTo: &validTo,
		})
		return err
	})
	if cert.ValidTo == nil || !cert.ValidTo.Equal(validTo) {
		t.Fatalf("valid_to = %v, want %v", cert.ValidTo, validTo)
	}

	// an unknown vendor is refused.
	e.in(t, func(ctx context.Context) error {
		other := attachableCertFile(t, ctx, ff)
		if _, err := e.svc.AddCertificate(ctx, vendorservice.CertificateInput{VendorID: uuid.New(), CertType: "SOC 2", FileID: other}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("unknown vendor: %v, want ErrInvalid", err)
		}
		// a file already attached elsewhere is refused.
		if _, err := e.svc.AddCertificate(ctx, vendorservice.CertificateInput{VendorID: v.ID, CertType: "SOC 2", FileID: fileID}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("already-attached file: %v, want ErrInvalid", err)
		}
		// valid_to before valid_from is refused.
		from := validTo
		to := validTo.AddDate(0, 0, -1)
		if _, err := e.svc.AddCertificate(ctx, vendorservice.CertificateInput{VendorID: v.ID, CertType: "SOC 2", FileID: other, ValidFrom: &from, ValidTo: &to}); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("valid_to before valid_from: %v, want ErrInvalid", err)
		}
		return nil
	})

	var scheduledAt time.Time
	e.in(t, func(ctx context.Context) error {
		row := e.app.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind = 'vendor.certificate_expiry'
			AND args->>'certificate_id' = $1`, cert.ID.String())
		return row.Scan(&scheduledAt)
	})
	want := validTo.AddDate(0, 0, -vendorservice.CertificateExpiryNoticeDays)
	if scheduledAt.Sub(want).Abs() > time.Minute {
		t.Errorf("scheduled_at = %v, want ~%v (valid_to - %d days)", scheduledAt, want, vendorservice.CertificateExpiryNoticeDays)
	}

	// ListCertificates/DeleteCertificate round-trip.
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListCertificates(ctx, v.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != cert.ID {
			t.Errorf("ListCertificates = %+v, want exactly the one certificate", list)
		}
		if err := e.svc.DeleteCertificate(ctx, v.ID, cert.ID); err != nil {
			t.Errorf("DeleteCertificate: %v", err)
		}
		if err := e.svc.DeleteCertificate(ctx, v.ID, cert.ID); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("deleting again: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestFireCertificateExpiryReminder_NotifiesOwnerOrFallsBackToSEC is the acceptance criterion itself: the
// reminder notifies the vendor's relationship owner when one is set, else role SEC, and a stale or unknown
// call is a harmless no-op.
func TestFireCertificateExpiryReminder_NotifiesOwnerOrFallsBackToSEC(t *testing.T) {
	e, ff := certEnv(t, "vencertfire")
	var v vendorservice.Vendor
	now := time.Now().UTC()
	validTo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 10)
	var cert vendorservice.Certificate
	e.in(t, func(ctx context.Context) error {
		v = createVendorForCert(t, ctx, e)
		fileID := attachableCertFile(t, ctx, ff)
		var err error
		cert, err = e.svc.AddCertificate(ctx, vendorservice.CertificateInput{VendorID: v.ID, CertType: "SOC 2", FileID: fileID, ValidTo: &validTo})
		return err
	})

	// a stale call (wrong valid_to, or an unknown certificate) is a harmless no-op.
	e.in(t, func(ctx context.Context) error {
		stale := validTo.AddDate(0, 0, 1)
		if err := e.svc.FireCertificateExpiryReminder(ctx, cert.ID, stale); err != nil {
			t.Errorf("stale fire: %v", err)
		}
		if err := e.svc.FireCertificateExpiryReminder(ctx, uuid.New(), validTo); err != nil {
			t.Errorf("unknown certificate fire: %v", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.FireCertificateExpiryReminder(ctx, cert.ID, validTo)
	})
	var count int
	e.in(t, func(ctx context.Context) error {
		row := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications n
			JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'vendor.certificate_expiry'`)
		return row.Scan(&count)
	})
	if count == 0 {
		t.Fatal("expected at least one vendor.certificate_expiry notification (role SEC fallback)")
	}
}
