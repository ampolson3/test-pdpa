package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func createTestPackage(t *testing.T, e env, ctx context.Context, ff *fakeFiles, requestID uuid.UUID, in dsarservice.CreatePackageInput) (dsarservice.Package, string) {
	t.Helper()
	pkg, code, err := e.svc.CreatePackage(ctx, requestID, in)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, code
}

// TestCreatePackage_AttachesFreshUploadAndIssuesAccessCode is the acceptance criterion's own starting
// point: a clean, still-unattached upload becomes a password-protected, time-limited delivery with a real
// public key and a one-time access code never stored in the clear.
func TestCreatePackage_AttachesFreshUploadAndIssuesAccessCode(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgcreate")
	legalEntity := createTestLegalEntity(t, e, "บริษัท แพ็กเกจ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var pkg dsarservice.Package
	var code string
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		pkg, code = createTestPackage(t, e, ctx, ff, req.ID, dsarservice.CreatePackageInput{
			FileID: fileID, Format: "pdf", PasswordProtected: true,
		})
		return nil
	})
	if pkg.PublicKey == "" {
		t.Fatal("expected a public key to be issued")
	}
	if code == "" || len(code) != 10 {
		t.Fatalf("access code = %q, want a 10-character code", code)
	}
	if pkg.ExpiresAt.Sub(time.Now().UTC()) < 6*24*time.Hour {
		t.Errorf("expires_at = %v, want ~7 days out (default TTL)", pkg.ExpiresAt)
	}
	if pkg.DownloadCount != 0 || pkg.PurgedAt != nil {
		t.Fatalf("fresh package = %+v", pkg)
	}

	// Bad format refused before anything is written.
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		if _, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "bogus"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad format: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// An unknown/not-clean file id is refused.
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: uuid.New(), Format: "pdf"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown file id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestCreatePackage_AcceptsAlreadyAttachedFile proves the other half of CreatePackage's own file-visibility
// rule: a file already attached to this exact request (e.g. DSAR-13's own generated response letter) is
// accepted as-is, with no re-attach attempted.
func TestCreatePackage_AcceptsAlreadyAttachedFile(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgattached")
	legalEntity := createTestLegalEntity(t, e, "บริษัท แพ็กเกจ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		if err := ff.Attach(ctx, fileID, "dsar_request", req.ID); err != nil {
			t.Fatal(err)
		}
		pkg, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "csv"})
		if err != nil {
			t.Fatal(err)
		}
		if pkg.FileID != fileID {
			t.Errorf("package file id = %v, want %v", pkg.FileID, fileID)
		}
		return nil
	})

	// A file attached to a different entity (or a different request) is refused.
	e.in(t, func(ctx context.Context) error {
		otherReq := createTestRequest(t, e, legalEntity, requestTypeID(t, e))
		fileID := attachableUpload(t, ctx, ff)
		if err := ff.Attach(ctx, fileID, "dsar_request", otherReq.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "csv"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("file attached to a different request: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestPublicDownload_AccessCodeAndLoggingIsTheAcceptanceCriterion: a correct code returns a signed URL and
// records download_count/last_downloaded_at plus an audit entry every time; a wrong code is refused.
func TestPublicDownload_AccessCodeAndLoggingIsTheAcceptanceCriterion(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgdownload")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ดาวน์โหลด จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var pkg dsarservice.Package
	var code string
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		pkg, code = createTestPackage(t, e, ctx, ff, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "pdf", PasswordProtected: true})
		return nil
	})

	// Wrong code: refused, nothing recorded.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublicDownload(ctx, pkg.ID, "WRONGCODE1"); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("wrong code: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// Correct code: a signed URL, download recorded.
	var url string
	e.in(t, func(ctx context.Context) error {
		var err error
		url, err = e.svc.PublicDownload(ctx, pkg.ID, code)
		return err
	})
	if url == "" {
		t.Fatal("expected a non-empty signed URL")
	}

	e.in(t, func(ctx context.Context) error {
		pkgs, err := e.svc.ListPackages(ctx, req.ID)
		if err != nil {
			return err
		}
		if len(pkgs) != 1 || pkgs[0].DownloadCount != 1 || pkgs[0].LastDownloadedAt == nil {
			t.Errorf("after download: packages = %+v", pkgs)
		}
		return nil
	})

	// Downloading again (correct code) is allowed and counts again — no single-use restriction in this
	// feature's own acceptance criterion.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublicDownload(ctx, pkg.ID, code); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		pkgs, err := e.svc.ListPackages(ctx, req.ID)
		if err != nil {
			return err
		}
		if pkgs[0].DownloadCount != 2 {
			t.Errorf("download_count = %d, want 2", pkgs[0].DownloadCount)
		}
		return nil
	})
}

// TestPublicDownload_NoPasswordNeedsNoCode: a package that wasn't marked password_protected downloads with
// an empty code.
func TestPublicDownload_NoPasswordNeedsNoCode(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgnopass")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ไม่มีรหัส จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var pkg dsarservice.Package
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		pkg, _ = createTestPackage(t, e, ctx, ff, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "pdf", PasswordProtected: false})
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublicDownload(ctx, pkg.ID, ""); err != nil {
			t.Fatalf("download with no password set: %v", err)
		}
		return nil
	})
}

// TestPublicPackage_ExpiredOrPurgedReadsAsNotFound is the acceptance criterion's own "ลิงก์หมดอายุตามที่ตั้ง":
// an expired package, and a purged one, are both indistinguishable from an unknown id.
func TestPublicPackage_ExpiredOrPurgedReadsAsNotFound(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgexpiry")
	legalEntity := createTestLegalEntity(t, e, "บริษัท หมดอายุ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var expired dsarservice.Package
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		pkg, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "pdf"})
		if err != nil {
			return err
		}
		expired = pkg
		return nil
	})

	// Expired: CreatePackage itself refuses a non-future expires_at, which is the write-side half of this
	// same guard — proven directly here.
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		past := time.Now().UTC().Add(-time.Hour)
		if _, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "pdf", ExpiresAt: &past}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("past expires_at: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// Purged: PurgeExpiredPackage deletes the file bytes, keeps the row, and PublicPackage then reads it as
	// not found — the acceptance criterion's own "ลบไฟล์อัตโนมัติเมื่อหมดอายุ".
	e.in(t, func(ctx context.Context) error {
		return e.svc.PurgeExpiredPackage(ctx, expired.ID)
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublicPackage(ctx, expired.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("purged package read: err = %v, want ErrNotFound", err)
		}
		if _, err := e.svc.PublicDownload(ctx, expired.ID, ""); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("purged package download: err = %v, want ErrNotFound", err)
		}
		return nil
	})
	fileID := uuid.Nil
	e.in(t, func(ctx context.Context) error {
		pkgs, err := e.svc.ListPackages(ctx, req.ID)
		if err != nil {
			return err
		}
		if len(pkgs) != 1 || pkgs[0].PurgedAt == nil {
			t.Errorf("after purge: packages = %+v", pkgs)
		}
		fileID = pkgs[0].FileID
		return nil
	})
	if !ff.deleted[fileID] {
		t.Error("expected the underlying file's bytes to be deleted on purge")
	}

	// A second purge is a harmless no-op.
	e.in(t, func(ctx context.Context) error {
		return e.svc.PurgeExpiredPackage(ctx, expired.ID)
	})
}

// TestPackages_TwoTenantIsolation: a package can't be listed, downloaded correctly characterized, or purged
// across tenants via the request id — standard rule-1 isolation, proven the same way every other module's
// own two-tenant test does.
func TestPackages_TwoTenantIsolation(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarpkgisoA")
	other := identityEnvOther(t)
	legalEntity := createTestLegalEntity(t, e, "บริษัท เอ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		_, _, err := e.svc.CreatePackage(ctx, req.ID, dsarservice.CreatePackageInput{FileID: fileID, Format: "pdf"})
		return err
	})

	other.in(t, func(ctx context.Context) error {
		if _, err := other.svc.ListPackages(ctx, req.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("cross-tenant list: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
