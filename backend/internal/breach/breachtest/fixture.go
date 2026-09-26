// Package breachtest is the breach module's test fixture: two tenants with a legal entity each; in tenant A an
// incident handler (SEC), two DPOs (role DPO — the alert recipients) and an executive (role EXEC); real S3 + clamd
// for evidence and recipient files; and a published breach assessment form. Used by the service and HTTP tests.
package breachtest

import (
	"bytes"
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	breach "pdpa-platform/internal/breach/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/wiring"
)

// Permission sets by role, as docs/security/permissions.yaml grants them.
var (
	SEC      = []string{"breach.incident.create", "breach.incident.read", "breach.incident.update", "breach.incident.execute", "breach.notification.read"}
	DPO      = []string{"breach.incident.create", "breach.incident.read", "breach.incident.update", "breach.incident.delete", "breach.incident.approve", "breach.incident.execute", "breach.notification.create", "breach.notification.read", "breach.notification.update", "breach.notification.approve", "breach.notification.publish"}
	Privacy  = []string{"breach.incident.create", "breach.incident.read", "breach.incident.update", "breach.incident.execute", "breach.notification.create", "breach.notification.read", "breach.notification.update"}
	Employee = []string{"breach.incident.create"}
)

type Fixture struct {
	App, Owner       *pgxpool.Pool
	A, B             dbtest.Tenant
	Sec, DPO, DPO2   uuid.UUID
	Exec, Employee   uuid.UUID
	EntityA, EntityB uuid.UUID
	Svc              *breach.Service
	Files            *files.Service
	Forms            *forms.Service
	Notify           *notify.Service
	scan             *files.Scanner
	store            *files.S3Store
	Clock            time.Time
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Setup needs Postgres, S3 and clamd; it skips the test when they aren't reachable.
func Setup(t *testing.T) *Fixture {
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
	f := &Fixture{App: dbtest.Pool(t), Owner: dbtest.OwnerPool(t), Clock: time.Now().UTC().Truncate(time.Second)}
	platform := dbtest.PlatformPool(t)
	f.A = dbtest.SeedTenant(t, ctx, f.App, platform, "breach-a")
	f.B = dbtest.SeedTenant(t, ctx, f.App, platform, "breach-b")
	f.Sec = f.A.UserID
	for _, tn := range []struct {
		t      dbtest.Tenant
		entity *uuid.UUID
	}{{f.A, &f.EntityA}, {f.B, &f.EntityB}} {
		if err := pdb.WithTenantTx(ctx, f.App, tn.t.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Sam Sec', email = 'sec@breach.example' WHERE id = $1`, tn.t.UserID); err != nil {
				return err
			}
			if tn.t.ID == f.A.ID {
				for _, u := range []struct {
					id         *uuid.UUID
					name, role string
				}{{&f.DPO, "Dee DPO", "DPO"}, {&f.DPO2, "Dana DPO", "DPO"}, {&f.Exec, "Eve Exec", "EXEC"}, {&f.Employee, "Emma Emp", ""}} {
					if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, $3, 'active') RETURNING id`,
						tn.t.ID, uuid.NewString()[:8]+"@breach.example", u.name).Scan(u.id); err != nil {
						return err
					}
					if u.role != "" {
						if _, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
							SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = $3 AND tenant_id IS NULL`, tn.t.ID, *u.id, u.role); err != nil {
							return err
						}
					}
				}
			}
			return tx.QueryRow(ctx, `INSERT INTO org.legal_entities (tenant_id, name_th) VALUES ($1, 'บริษัท ทดสอบ จำกัด') RETURNING id`, tn.t.ID).Scan(tn.entity)
		}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := files.NewS3Store(files.S3Config{Endpoint: endpoint, AccessKey: envOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: envOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"), Bucket: "pdpa-files-test", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	f.store = store
	client, _ := jobs.NewInsertClient(f.App)
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	f.Notify = &notify.Service{Keyring: keyring, River: client}
	f.Files = &files.Service{Store: store, River: client, Config: files.DefaultConfig()}
	f.scan = &files.Scanner{Store: store, AV: &files.Clamd{Addr: clamd}, Audit: audit.New()}
	f.Forms = wiring.Forms(f.Notify, audit.New())
	f.Svc = &breach.Service{Events: &events.Publisher{River: client}, Notify: f.Notify, Forms: f.Forms, Files: f.Files, Keyring: keyring,
		Audit: audit.New(), Org: &orgservice.Service{}, River: client, Now: func() time.Time { return f.Clock }}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func (f *Fixture) cleanup(t *testing.T) {
	for _, tn := range []dbtest.Tenant{f.A, f.B} {
		_ = pdb.WithTenantTx(context.Background(), f.App, tn.ID.String(), "", func(ctx context.Context) error {
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
			return nil
		})
		_ = pdb.WithTenantTx(context.Background(), f.Owner, tn.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{`DELETE FROM breach.notification_recipients`, `DELETE FROM breach.subject_notifications`, `DELETE FROM breach.evidence`,
				`DELETE FROM breach.assessments`, `DELETE FROM breach.timeline_events`, `DELETE FROM breach.incidents`, `DELETE FROM platform.form_submissions`,
				`UPDATE platform.form_definitions SET current_version_id = NULL`, `DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
				`DELETE FROM platform.files`, `DELETE FROM platform.notifications`, `DELETE FROM platform.outbox_events`, `DELETE FROM platform.audit_log`,
				`DELETE FROM platform.tenant_keys`, `DELETE FROM iam.role_assignments`, `DELETE FROM org.legal_entities`} {
				if _, err := tx.Exec(ctx, q); err != nil {
					t.Logf("cleanup %q: %v", q, err)
				}
			}
			_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id <> $1`, tn.UserID)
			return err
		})
		_, _ = f.App.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tn.ID.String())
	}
}

// As runs fn in one transaction of tenant A as user with perms.
func (f *Fixture) As(t *testing.T, user uuid.UUID, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := f.Try(user, perms, fn); err != nil {
		t.Fatal(err)
	}
}

// Try is As returning the error.
func (f *Fixture) Try(user uuid.UUID, perms []string, fn func(ctx context.Context) error) error {
	tenant := f.A
	if user == f.B.UserID {
		tenant = f.B
	}
	return pdb.WithTenantTx(context.Background(), f.App, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Permissions: perms}))
	})
}

// System runs fn as a worker job of tenant A would (no user).
func (f *Fixture) System(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", fn); err != nil {
		t.Fatal(err)
	}
}

// Upload stores content as user's upload and runs its virus scan; returns the clean file's id.
func (f *Fixture) Upload(t *testing.T, user uuid.UUID, name string, content []byte) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	f.As(t, user, nil, func(ctx context.Context) error {
		up, err := f.Files.Upload(ctx, name, bytes.NewReader(content))
		id = up.ID
		return err
	})
	f.System(t, func(ctx context.Context) error {
		return f.scan.Work(ctx, &river.Job[files.ScanArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 10},
			Args: files.ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: f.A.ID.String()}, FileID: id.String()}})
	})
	return id
}

func score(v float64) *float64 { return &v }

// AssessmentForm publishes a breach assessment form (as the DPO): data sensitivity, volume, encryption — bands
// none (< 3), low (3–7), high (8+).
func (f *Fixture) AssessmentForm(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "factors", Title: forms.Text{"th": "ปัจจัย", "en": "Factors"},
		Questions: []forms.Question{
			{Key: "sensitive", Type: forms.TypeYesNo, Label: forms.Text{"th": "มีข้อมูลอ่อนไหวหรือไม่", "en": "Sensitive data involved?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(5)}, {Value: "no", Score: score(0)}}},
			{Key: "volume", Type: forms.TypeSingle, Label: forms.Text{"th": "จำนวนเจ้าของข้อมูล", "en": "Data subjects affected"}, Required: true,
				Options: []forms.Option{{Value: "few", Label: forms.Text{"th": "น้อยกว่า 100", "en": "Under 100"}, Score: score(1)},
					{Value: "many", Label: forms.Text{"th": "100 ขึ้นไป", "en": "100 or more"}, Score: score(3)}}},
			{Key: "encrypted", Type: forms.TypeYesNo, Label: forms.Text{"th": "ข้อมูลถูกเข้ารหัสหรือไม่", "en": "Was the data encrypted?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(-4)}, {Value: "no", Score: score(0)}}},
		}}}},
		Scoring: &forms.Scoring{Bands: []forms.Band{{Key: "none", Label: forms.Text{"th": "ไม่มีความเสี่ยง"}, Min: -100, Max: score(2.99)},
			{Key: "low", Label: forms.Text{"th": "ความเสี่ยง"}, Min: 3, Max: score(7.99)}, {Key: "high", Label: forms.Text{"th": "ความเสี่ยงสูง"}, Min: 8}}}}
	f.As(t, f.DPO, DPO, func(ctx context.Context) error {
		form, err := f.Forms.CreateForm(ctx, "breach_risk_"+uuid.NewString()[:6], "ประเมินความเสี่ยงเหตุละเมิด", "breach", d)
		if err != nil {
			return err
		}
		form, err = f.Forms.Publish(ctx, form.ID, form.LatestVersion)
		id = form.ID
		return err
	})
	return id
}

// Incident records an incident as the SEC user, aware at the given moment.
func (f *Fixture) Incident(t *testing.T, title string, aware time.Time) breach.Incident {
	t.Helper()
	var in breach.Incident
	f.As(t, f.Sec, SEC, func(ctx context.Context) error {
		var err error
		n := int32(250)
		in, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: title, Description: "ไฟล์ลูกค้ารั่วจากระบบ CRM",
			BreachTypes: []string{"confidentiality"}, AwareAt: aware, AffectedSubjects: &n})
		return err
	})
	return in
}
