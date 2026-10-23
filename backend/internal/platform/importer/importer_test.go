package importer_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
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
	"github.com/xuri/excelize/v2"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/importer"
	"pdpa-platform/internal/platform/jobs"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

const perm = "test.people.import"

// peopleType imports "people" into platform.comments (a real tenant table, so the test sees exactly what
// was written): code and name required, e-mail optional but must contain @, and code "BOOM" fails to apply.
func peopleType(record uuid.UUID) importer.Type {
	return importer.Type{
		Permission: perm,
		Columns: []importer.Column{
			{Key: "code", Label: map[string]string{"th": "รหัส", "en": "Code"}, Required: true},
			{Key: "name", Label: map[string]string{"th": "ชื่อ", "en": "Name"}, Required: true},
			{Key: "email", Label: map[string]string{"th": "อีเมล", "en": "E-mail"}, Aliases: []string{"mail"}},
		},
		Validate: func(_ context.Context, _ int, r importer.Row) []importer.FieldError {
			if r["email"] != "" && !strings.Contains(r["email"], "@") {
				return []importer.FieldError{{Column: "email", Message: "not an e-mail address"}}
			}
			return nil
		},
		Apply: func(ctx context.Context, _ int, r importer.Row) error {
			if r["code"] == "BOOM" {
				return errors.New("simulated write failure")
			}
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
				`INSERT INTO platform.comments (tenant_id, entity_type, entity_id, author_type, author_id, body)
				 VALUES (current_setting('app.tenant_id')::uuid, 'import_test', $1, 'user', $1, $2)`, record, r["code"]+"|"+r["name"])
			return err
		},
	}
}

type fixture struct {
	app    *pgxpool.Pool
	a, b   dbtest.Tenant
	svc    *importer.Service
	files  *files.Service
	scan   *files.Scanner
	record uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	endpoint := envOr("TEST_S3_ENDPOINT", "127.0.0.1:8333")
	clamd := envOr("TEST_CLAMD_ADDR", "127.0.0.1:3310")
	for _, addr := range []string{endpoint, clamd} {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err != nil {
			t.Skipf("needs S3 and clamd (%s unreachable)", addr)
		} else {
			c.Close()
		}
	}
	f := &fixture{app: dbtest.Pool(t), record: uuid.New()}
	owner := dbtest.OwnerPool(t)
	platform := dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "import-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "import-b")
	store, err := files.NewS3Store(files.S3Config{Endpoint: endpoint, AccessKey: envOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: envOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"), Bucket: "pdpa-files-test", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	client, _ := jobs.NewInsertClient(f.app)
	f.files = &files.Service{Store: store, River: client, Config: files.DefaultConfig()}
	f.scan = &files.Scanner{Store: store, AV: &files.Clamd{Addr: clamd}, Audit: audit.New()}
	f.svc = &importer.Service{Types: importer.Registry{"test.people": peopleType(f.record)}, Files: f.files, River: client, Audit: audit.New()}

	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.comments WHERE entity_type = 'import_test'`)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.import_jobs`)
				rows, _ := tx.Query(ctx, `SELECT object_key FROM platform.files`)
				var keys []string
				for rows.Next() {
					var k string
					_ = rows.Scan(&k)
					keys = append(keys, k)
				}
				rows.Close()
				for _, k := range keys {
					_ = store.Delete(ctx, k)
				}
				_, err := tx.Exec(ctx, `DELETE FROM platform.files`)
				return err
			})
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.audit_log`)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	return f
}

func (f *fixture) as(t *testing.T, perms []string, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), f.a.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: f.a.ID.String(), UserID: f.a.UserID.String(), Permissions: perms}))
	})
}

// start uploads content, scans it and creates the import; returns the import after preparation.
func (f *fixture) start(t *testing.T, name string, content []byte) importer.Job {
	t.Helper()
	var job importer.Job
	if err := f.as(t, []string{perm}, func(ctx context.Context) error {
		up, err := f.files.Upload(ctx, name, bytes.NewReader(content))
		if err != nil {
			return err
		}
		if job, err = f.svc.Create(ctx, "test.people", up.ID); err != nil {
			return err
		}
		return f.runPrepareExpectSnooze(ctx, job.ID) // the scan hasn't run yet
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	f.runScan(t, job.FileID)
	f.run(t, &importer.Preparer{Service: f.svc}, importer.PrepareArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, ImportID: job.ID.String()})
	return f.get(t, job.ID)
}

func (f *fixture) runPrepareExpectSnooze(ctx context.Context, id uuid.UUID) error {
	err := (&importer.Preparer{Service: f.svc}).Work(ctx, &river.Job[importer.PrepareArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 25},
		Args:   importer.PrepareArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, ImportID: id.String()},
	})
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		return fmt.Errorf("prepare before the scan: %v, want a snooze", err)
	}
	return nil
}

func (f *fixture) runScan(t *testing.T, fileID uuid.UUID) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return f.scan.Work(ctx, &river.Job[files.ScanArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 10},
			Args: files.ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, FileID: fileID.String()}})
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

type worker[T river.JobArgs] interface {
	Work(context.Context, *river.Job[T]) error
}

func (f *fixture) run(t *testing.T, w any, args river.JobArgs) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		switch a := args.(type) {
		case importer.PrepareArgs:
			return w.(worker[importer.PrepareArgs]).Work(ctx, &river.Job[importer.PrepareArgs]{JobRow: &rivertype.JobRow{}, Args: a})
		case importer.ValidateArgs:
			return w.(worker[importer.ValidateArgs]).Work(ctx, &river.Job[importer.ValidateArgs]{JobRow: &rivertype.JobRow{}, Args: a})
		case importer.ApplyArgs:
			return w.(worker[importer.ApplyArgs]).Work(ctx, &river.Job[importer.ApplyArgs]{JobRow: &rivertype.JobRow{}, Args: a})
		}
		return fmt.Errorf("unexpected args %T", args)
	})
	if err != nil {
		t.Fatalf("%s: %v", args.Kind(), err)
	}
}

func (f *fixture) get(t *testing.T, id uuid.UUID) importer.Job {
	t.Helper()
	var j importer.Job
	if err := f.as(t, []string{perm}, func(ctx context.Context) error {
		var err error
		j, err = f.svc.Get(ctx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return j
}

func (f *fixture) mapAndValidate(t *testing.T, j importer.Job, mapping map[string]string) importer.Job {
	t.Helper()
	if err := f.as(t, []string{perm}, func(ctx context.Context) error {
		_, err := f.svc.SetMapping(ctx, j.ID, j.RowVersion, mapping)
		return err
	}); err != nil {
		t.Fatalf("map: %v", err)
	}
	f.run(t, &importer.Validator{Service: f.svc}, importer.ValidateArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, ImportID: j.ID.String()})
	return f.get(t, j.ID)
}

func (f *fixture) confirmAndApply(t *testing.T, j importer.Job) importer.Job {
	t.Helper()
	if err := f.as(t, []string{perm}, func(ctx context.Context) error {
		_, err := f.svc.Confirm(ctx, j.ID, j.RowVersion)
		return err
	}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	f.run(t, &importer.Applier{Service: f.svc}, importer.ApplyArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, ImportID: j.ID.String()})
	return f.get(t, j.ID)
}

func (f *fixture) imported(t *testing.T) int {
	t.Helper()
	var n int
	_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.comments WHERE entity_type = 'import_test'`).Scan(&n)
	})
	return n
}

// Acceptance criterion: "นำเข้า 50,000 แถวได้ และได้ไฟล์รายงานแถวที่ผิดพร้อมเหตุผล".
func TestImport_50000RowsWithErrorReport(t *testing.T) {
	f := setup(t)
	var b bytes.Buffer
	b.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"รหัส", "ชื่อ", "mail", "หมายเหตุ"})
	for i := 1; i <= 50000; i++ {
		rec := []string{fmt.Sprintf("P%05d", i), fmt.Sprintf("บุคคล %d", i), fmt.Sprintf("p%d@example.com", i), ""}
		switch i {
		case 17: // line 18
			rec[1] = ""
		case 4242: // line 4243
			rec[2] = "not-an-email"
		case 49999: // line 50000
			rec[0] = ""
		}
		_ = w.Write(rec)
	}
	w.Flush()

	start := time.Now()
	j := f.start(t, "people.csv", b.Bytes())
	if j.Status != "queued" || len(j.Headers) != 4 {
		t.Fatalf("after prepare: %+v", j)
	}
	if j.Suggested["code"] != "รหัส" || j.Suggested["name"] != "ชื่อ" || j.Suggested["email"] != "mail" {
		t.Errorf("suggested mapping = %v (labels and aliases should match)", j.Suggested)
	}
	j = f.mapAndValidate(t, j, j.Suggested)
	if j.Status != "ready" || *j.TotalRows != 50000 || *j.ValidRows != 49997 || *j.ErrorRows != 3 || !j.HasErrorReport {
		t.Fatalf("after validation: status %s total %v valid %v errors %v report %v", j.Status, *j.TotalRows, *j.ValidRows, *j.ErrorRows, j.HasErrorReport)
	}
	if n := f.imported(t); n != 0 {
		t.Fatalf("dry run wrote %d rows", n)
	}

	// The report: one line per problem, with the file's line number and the reason.
	f.runScanOfErrorReport(t, j)
	var reportURL string
	_ = f.as(t, []string{perm}, func(ctx context.Context) error {
		var err error
		reportURL, err = f.svc.ErrorReportURL(ctx, j.ID)
		return err
	})
	res, err := http.Get(reportURL)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	recs, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"line", "column", "error"}, {"18", "name", "required"}, {"4243", "email", "not an e-mail address"}, {"50000", "code", "required"}}
	if fmt.Sprint(recs) != fmt.Sprint(want) {
		t.Errorf("error report = %v, want %v", recs, want)
	}

	j = f.confirmAndApply(t, j)
	if j.Status != "done" || *j.ValidRows != 49997 {
		t.Fatalf("after import: %+v", j)
	}
	if n := f.imported(t); n != 49997 {
		t.Errorf("imported %d rows, want 49997", n)
	}
	t.Logf("50,000 rows: upload → scan → validate → import in %s", time.Since(start).Round(time.Millisecond))
}

func (f *fixture) runScanOfErrorReport(t *testing.T, j importer.Job) {
	t.Helper()
	var id uuid.UUID
	_ = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT error_file_id FROM platform.import_jobs WHERE id = $1`, j.ID).Scan(&id)
	})
	f.runScan(t, id)
}

// A failure while applying rolls back every row of the import.
func TestImport_ApplyFailureRollsBackEverything(t *testing.T) {
	f := setup(t)
	content := "Code,Name\nA1,one\nA2,two\nBOOM,three\nA4,four\n"
	j := f.start(t, "people.csv", []byte(content))
	j = f.mapAndValidate(t, j, j.Suggested)
	j = f.confirmAndApply(t, j)
	if j.Status != "failed" || j.Failure != "apply_failed_line_4" {
		t.Fatalf("status %s failure %q, want failed at line 4", j.Status, j.Failure)
	}
	if n := f.imported(t); n != 0 {
		t.Errorf("%d rows left behind by a failed import, want 0", n)
	}
}

func TestImport_Excel(t *testing.T) {
	f := setup(t)
	x := excelize.NewFile()
	_ = x.SetSheetRow("Sheet1", "A1", &[]string{"Code", "Name", "E-mail"})
	_ = x.SetSheetRow("Sheet1", "A2", &[]string{"X1", "สมชาย", "a@example.com"})
	_ = x.SetSheetRow("Sheet1", "A3", &[]string{"X2", "สมหญิง", ""})
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		t.Fatal(err)
	}
	j := f.start(t, "people.xlsx", buf.Bytes())
	j = f.mapAndValidate(t, j, j.Suggested)
	if *j.ValidRows != 2 || *j.ErrorRows != 0 || j.HasErrorReport {
		t.Fatalf("validation: %+v", j)
	}
	if j = f.confirmAndApply(t, j); j.Status != "done" || f.imported(t) != 2 {
		t.Errorf("excel import: status %s, %d rows", j.Status, f.imported(t))
	}
}

func TestImport_MappingRulesAndStates(t *testing.T) {
	f := setup(t)
	j := f.start(t, "people.csv", []byte("Code,Name,Other\nA,B,C\n"))
	bad := []map[string]string{
		{"code": "Code"},                                  // name is required
		{"code": "Code", "name": "Missing"},               // header not in the file
		{"code": "Code", "name": "Code"},                  // one header, two columns
		{"code": "Code", "name": "Name", "nope": "Other"}, // unknown column
	}
	err := f.as(t, []string{perm}, func(ctx context.Context) error {
		for _, m := range bad {
			if _, err := f.svc.SetMapping(ctx, j.ID, j.RowVersion, m); !errors.Is(err, importer.ErrInvalidMapping) {
				t.Errorf("mapping %v: %v, want ErrInvalidMapping", m, err)
			}
		}
		if _, err := f.svc.Confirm(ctx, j.ID, j.RowVersion); !errors.Is(err, importer.ErrInvalidState) {
			t.Errorf("confirm before validation: %v, want ErrInvalidState", err)
		}
		if _, err := f.svc.SetMapping(ctx, j.ID, j.RowVersion+3, j.Suggested); !errors.Is(err, importer.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestImport_AccessAndIsolation(t *testing.T) {
	f := setup(t)
	j := f.start(t, "people.csv", []byte("Code,Name\nA,B\n"))
	err := f.as(t, nil, func(ctx context.Context) error {
		if _, err := f.svc.Get(ctx, j.ID); !errors.Is(err, importer.ErrNotFound) {
			t.Errorf("without the type's permission: %v, want ErrNotFound", err)
		}
		if _, err := f.svc.Create(ctx, "test.people", j.FileID); !errors.Is(err, importer.ErrForbidden) {
			t.Errorf("create without permission: %v, want ErrForbidden", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = pdb.WithTenantTx(context.Background(), f.app, f.b.ID.String(), f.b.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.b.ID.String(), UserID: f.b.UserID.String(), Permissions: []string{perm}})
		if _, err := f.svc.Get(ctx, j.ID); !errors.Is(err, importer.ErrNotFound) {
			t.Errorf("other tenant: %v, want ErrNotFound", err)
		}
		list, _ := f.svc.List(ctx, "")
		if len(list) != 0 {
			t.Errorf("other tenant lists %d imports", len(list))
		}
		return nil
	})
}

func TestImport_InfectedFileFails(t *testing.T) {
	f := setup(t)
	eicar := `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`
	j := f.start(t, "people.csv", []byte(eicar))
	if j.Status != "failed" || j.Failure != "file_rejected" {
		t.Errorf("infected import file: status %s failure %q, want failed/file_rejected", j.Status, j.Failure)
	}
}
