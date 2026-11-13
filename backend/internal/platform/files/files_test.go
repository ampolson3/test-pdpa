package files_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/jobs"
)

// EICAR: the industry-standard harmless test file every antivirus reports as malware.
const eicar = `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`

var pdf = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// store connects to the S3 gateway at TEST_S3_ENDPOINT (default: a local SeaweedFS/MinIO on :8333 with
// the dev credentials in README) or skips.
func store(t *testing.T) *files.S3Store {
	t.Helper()
	cfg := files.S3Config{
		Endpoint:  envOr("TEST_S3_ENDPOINT", "127.0.0.1:8333"),
		AccessKey: envOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: envOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"),
		Bucket:    "pdpa-files-test",
		Region:    "us-east-1",
		SSE:       os.Getenv("TEST_S3_SSE") == "true",
	}
	if c, err := net.DialTimeout("tcp", cfg.Endpoint, time.Second); err != nil {
		t.Skipf("no S3 endpoint at %s: %v", cfg.Endpoint, err)
	} else {
		c.Close()
	}
	s, err := files.NewS3Store(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("bucket: %v", err)
	}
	return s
}

func clamd(t *testing.T) *files.Clamd {
	t.Helper()
	addr := envOr("TEST_CLAMD_ADDR", "127.0.0.1:3310")
	if c, err := net.DialTimeout("tcp", addr, time.Second); err != nil {
		t.Skipf("no clamd at %s: %v", addr, err)
	} else {
		c.Close()
	}
	return &files.Clamd{Addr: addr}
}

type fixture struct {
	app   *pgxpool.Pool
	a, b  dbtest.Tenant
	store *files.S3Store
	svc   *files.Service
	scan  *files.Scanner
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t), store: store(t)}
	owner := dbtest.OwnerPool(t)
	platform := dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "files-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "files-b")
	client, err := jobs.NewInsertClient(f.app)
	if err != nil {
		t.Fatal(err)
	}
	f.svc = &files.Service{Store: f.store, River: client, Config: files.DefaultConfig(),
		EntityPermissions: map[string]string{"dsar_package": "dsar.request.read"}}
	f.scan = &files.Scanner{Store: f.store, AV: clamd(t), Audit: audit.New()}

	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				rows, _ := tx.Query(ctx, `SELECT object_key FROM platform.files`)
				var keys []string
				for rows.Next() {
					var k string
					_ = rows.Scan(&k)
					keys = append(keys, k)
				}
				rows.Close()
				for _, k := range keys {
					_ = f.store.Delete(ctx, k)
				}
				_, err := tx.Exec(ctx, `DELETE FROM platform.files`)
				return err
			})
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.audit_log`)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE kind LIKE 'files.%' AND args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	return f
}

// as runs fn in tenant's transaction with user's grants, the way a request reaches the service.
func (f *fixture) as(t *testing.T, tenant dbtest.Tenant, user uuid.UUID, perms []string, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Permissions: perms}))
	})
}

func (f *fixture) upload(t *testing.T, name string, body []byte) files.File {
	t.Helper()
	var out files.File
	if err := f.as(t, f.a, f.a.UserID, nil, func(ctx context.Context) error {
		var err error
		out, err = f.svc.Upload(ctx, name, bytes.NewReader(body))
		return err
	}); err != nil {
		t.Fatalf("upload %s: %v", name, err)
	}
	return out
}

func (f *fixture) runScan(t *testing.T, id uuid.UUID, attempt int) {
	t.Helper()
	job := &river.Job[files.ScanArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: files.ScanMaxAttempts},
		Args:   files.ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, FileID: id.String()},
	}
	if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return f.scan.Work(ctx, job)
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func (f *fixture) downloadURL(t *testing.T, id uuid.UUID) (string, error) {
	t.Helper()
	var u string
	err := f.as(t, f.a, f.a.UserID, nil, func(ctx context.Context) error {
		var err error
		u, err = f.svc.DownloadURL(ctx, id)
		return err
	})
	return u, err
}

func (f *fixture) avStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT av_status FROM platform.files WHERE id = $1`, id).Scan(&s)
	})
	return s
}

func TestUpload_CleanFileIsScannedThenDownloadable(t *testing.T) {
	f := setup(t)
	up := f.upload(t, "หนังสือมอบอำนาจ.pdf", pdf)
	if up.AVStatus != "pending" || up.MimeType != "application/pdf" || up.SizeBytes != int64(len(pdf)) {
		t.Fatalf("upload = %+v", up)
	}
	if _, err := f.downloadURL(t, up.ID); !errors.Is(err, files.ErrScanPending) {
		t.Fatalf("download before scan: %v, want ErrScanPending", err)
	}
	var queued int
	_ = f.app.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind IN ('files.scan', 'files.expire') AND args->>'file_id' = $1`, up.ID.String()).Scan(&queued)
	if queued != 2 {
		t.Errorf("queued jobs for the upload = %d, want scan + expire", queued)
	}

	f.runScan(t, up.ID, 1)
	if s := f.avStatus(t, up.ID); s != "clean" {
		t.Fatalf("av_status after scan = %s, want clean", s)
	}
	u, err := f.downloadURL(t, up.ID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, pdf) {
		t.Fatalf("signed URL: %d, %d bytes; want 200 with the file", res.StatusCode, len(body))
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("Content-Disposition = %q, want an attachment", cd)
	}
}

// Acceptance criterion (1): "ไฟล์ติดไวรัสถูกปฏิเสธ".
func TestUpload_InfectedFileIsRejected(t *testing.T) {
	f := setup(t)
	up := f.upload(t, "invoice.txt", []byte(eicar))
	f.runScan(t, up.ID, 1)

	if s := f.avStatus(t, up.ID); s != "infected" {
		t.Fatalf("av_status = %s, want infected", s)
	}
	if _, err := f.downloadURL(t, up.ID); !errors.Is(err, files.ErrInfected) {
		t.Errorf("download: %v, want ErrInfected", err)
	}
	var key string
	_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT object_key FROM platform.files WHERE id = $1`, up.ID).Scan(&key)
	})
	obj, err := f.store.Get(context.Background(), key)
	if err == nil {
		_, err = io.ReadAll(obj)
		obj.Close()
	}
	if err == nil {
		t.Error("infected object still in storage")
	}
	var audited int
	_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.audit_log WHERE entity_id = $1 AND action = 'platform.file.scan' AND after->>'av_status' = 'infected'`, up.ID).Scan(&audited)
	})
	if audited != 1 {
		t.Errorf("audit entries for the rejection = %d, want 1", audited)
	}
}

// Acceptance criterion (2): "ลิงก์ดาวน์โหลดหมดอายุตามเวลาที่ตั้ง".
func TestDownloadURL_ExpiresAfterConfiguredTTL(t *testing.T) {
	f := setup(t)
	f.svc.Config.DownloadTTL = 2 * time.Second
	up := f.upload(t, "report.pdf", pdf)
	f.runScan(t, up.ID, 1)

	u, err := f.downloadURL(t, up.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "X-Amz-Expires=2") {
		t.Errorf("URL does not carry the 2-second expiry: %s", u)
	}
	get := func() int {
		res, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("fresh link: %d, want 200", code)
	}
	time.Sleep(3 * time.Second)
	if code := get(); code != http.StatusForbidden {
		t.Errorf("expired link: %d, want 403", code)
	}
}

func TestUpload_RejectsBadFiles(t *testing.T) {
	f := setup(t)
	f.svc.Config.MaxBytes = 1024
	cases := []struct {
		name string
		body []byte
		want error
	}{
		{"big.pdf", append(append([]byte{}, pdf...), bytes.Repeat([]byte{' '}, 2048)...), files.ErrTooLarge},
		{"empty.pdf", nil, files.ErrEmpty},
		{"setup.exe", []byte("MZ\x90\x00"), files.ErrTypeNotAllowed},
		{"photo.png", pdf, files.ErrTypeNotAllowed}, // content doesn't match the extension
		{"noext", pdf, files.ErrTypeNotAllowed},
		{"   ", pdf, files.ErrInvalidName},
	}
	for _, tc := range cases {
		err := f.as(t, f.a, f.a.UserID, nil, func(ctx context.Context) error {
			_, err := f.svc.Upload(ctx, tc.name, bytes.NewReader(tc.body))
			return err
		})
		if !errors.Is(err, tc.want) {
			t.Errorf("%q: %v, want %v", tc.name, err, tc.want)
		}
	}
	up := f.upload(t, "../../etc/สัญญา.pdf", pdf)
	if up.FileName != "สัญญา.pdf" {
		t.Errorf("stored name = %q, want the base name only", up.FileName)
	}
}

// Who may see a file: the uploader while it's unattached; once attached, holders of the permission the
// owning module registered; never another tenant.
func TestVisibility(t *testing.T) {
	f := setup(t)
	up := f.upload(t, "evidence.pdf", pdf)
	f.runScan(t, up.ID, 1)
	colleague := uuid.New()

	get := func(tenant dbtest.Tenant, user uuid.UUID, perms []string) error {
		return f.as(t, tenant, user, perms, func(ctx context.Context) error {
			_, err := f.svc.Get(ctx, up.ID)
			return err
		})
	}
	if err := get(f.a, f.a.UserID, nil); err != nil {
		t.Errorf("uploader: %v", err)
	}
	if err := get(f.a, colleague, []string{"dsar.request.read"}); !errors.Is(err, files.ErrNotFound) {
		t.Errorf("colleague, unattached: %v, want ErrNotFound", err)
	}
	if err := get(f.b, f.b.UserID, []string{"dsar.request.read"}); !errors.Is(err, files.ErrNotFound) {
		t.Errorf("other tenant: %v, want ErrNotFound", err)
	}

	pkg := uuid.New()
	if err := f.as(t, f.a, f.a.UserID, nil, func(ctx context.Context) error {
		if err := f.svc.Attach(ctx, up.ID, "unregistered", pkg); !errors.Is(err, files.ErrUnknownEntity) {
			t.Errorf("attach to unregistered type: %v", err)
		}
		if err := f.svc.Attach(ctx, up.ID, "dsar_package", pkg); err != nil {
			return err
		}
		if err := f.svc.Attach(ctx, up.ID, "dsar_package", uuid.New()); !errors.Is(err, files.ErrAlreadyAttached) {
			t.Errorf("re-attach: %v, want ErrAlreadyAttached", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := get(f.a, colleague, []string{"dsar.request.read"}); err != nil {
		t.Errorf("colleague with the permission, attached: %v", err)
	}
	if err := get(f.a, colleague, nil); !errors.Is(err, files.ErrNotFound) {
		t.Errorf("colleague without the permission: %v, want ErrNotFound", err)
	}
}

// "อายุไฟล์": an upload nobody attached is deleted after the orphan TTL; an attached one is kept.
func TestExpirer_DeletesOnlyOrphans(t *testing.T) {
	f := setup(t)
	f.svc.Now = func() time.Time { return time.Now().Add(-48 * time.Hour) } // uploaded two days ago
	orphan := f.upload(t, "orphan.pdf", pdf)
	kept := f.upload(t, "kept.pdf", pdf)
	if err := f.as(t, f.a, f.a.UserID, nil, func(ctx context.Context) error {
		return f.svc.Attach(ctx, kept.ID, "dsar_package", uuid.New())
	}); err != nil {
		t.Fatal(err)
	}

	exp := &files.Expirer{Store: f.store}
	for _, id := range []uuid.UUID{orphan.ID, kept.ID} {
		job := &river.Job[files.ExpireArgs]{Args: files.ExpireArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, FileID: id.String()}}
		if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
			return exp.Work(ctx, job)
		}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(id uuid.UUID) (n int) {
		_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
			return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.files WHERE id = $1`, id).Scan(&n)
		})
		return n
	}
	if count(orphan.ID) != 0 {
		t.Error("orphan upload not deleted after its TTL")
	}
	if count(kept.ID) != 1 {
		t.Error("attached file deleted by the orphan expirer")
	}
}

type failingAV struct{}

func (failingAV) Scan(context.Context, io.Reader) (files.Verdict, error) {
	return files.Verdict{}, errors.New("clamd unavailable")
}

// A scan that keeps failing ends in av_status error (never clean) on its last attempt.
func TestScanner_FailingScanEndsInError(t *testing.T) {
	f := setup(t)
	f.scan.AV = failingAV{}
	up := f.upload(t, "x.pdf", pdf)

	err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return f.scan.Work(ctx, &river.Job[files.ScanArgs]{
			JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: files.ScanMaxAttempts},
			Args:   files.ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, FileID: up.ID.String()},
		})
	})
	if err == nil || f.avStatus(t, up.ID) != "pending" {
		t.Fatalf("early attempt: err %v status %s, want a retryable error and pending", err, f.avStatus(t, up.ID))
	}
	f.runScan(t, up.ID, files.ScanMaxAttempts)
	if s := f.avStatus(t, up.ID); s != "error" {
		t.Errorf("after the last attempt: %s, want error", s)
	}
	if _, err := f.downloadURL(t, up.ID); !errors.Is(err, files.ErrScanFailed) {
		t.Errorf("download: %v, want ErrScanFailed", err)
	}
}
