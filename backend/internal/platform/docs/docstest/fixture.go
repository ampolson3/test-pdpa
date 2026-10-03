// Package docstest is the document composer's test fixture: two tenants with a legal entity each, a privacy officer
// (drafts), a DPO (approves and publishes) and a lawyer (clause library) in tenant A, real S3 + clamd for the rendered
// files and a local Chromium for PDFs. Used by the service and HTTP tests.
package docstest

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
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
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
	"pdpa-platform/internal/wiring"
)

// Permission sets of the fixture's people.
var (
	Privacy = []string{"notice.document.create", "notice.document.read", "notice.document.update", "notice.template.read"}
	DPO     = []string{"notice.document.create", "notice.document.read", "notice.document.update", "notice.document.delete",
		"notice.document.approve", "notice.document.publish", "notice.template.create", "notice.template.read", "notice.template.update",
		"agreement.clause.read", "agreement.dpa.read", "agreement.dpa.approve"}
	Legal = []string{"agreement.clause.create", "agreement.clause.read", "agreement.clause.update", "agreement.clause.delete",
		"agreement.clause.publish", "agreement.dpa.create", "agreement.dpa.read", "agreement.dpa.update", "agreement.dpa.publish",
		"notice.document.read"}
	Audit = []string{"notice.document.read", "agreement.clause.read"}
)

type Fixture struct {
	App, Owner         *pgxpool.Pool
	A, B               dbtest.Tenant
	Priv, DPOUser, Law uuid.UUID // tenant A
	EntityA, EntityB   uuid.UUID
	Svc                *docs.Service
	Ver                *versioning.Service
	Files              *files.Service
	Renderer           *docs.Renderer
	HasPDF             bool
	store              *files.S3Store
	scanner            *files.Scanner
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// ChromiumPath finds a local headless Chromium ("" when there is none).
func ChromiumPath() string {
	if p := os.Getenv("CHROMIUM_PATH"); p != "" {
		return p
	}
	matches, _ := filepath.Glob("/opt/pw-browsers/chromium-*/chrome-linux/chrome")
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
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
	f := &Fixture{App: dbtest.Pool(t), Owner: dbtest.OwnerPool(t)}
	platform := dbtest.PlatformPool(t)
	f.A = dbtest.SeedTenant(t, ctx, f.App, platform, "docs-a")
	f.B = dbtest.SeedTenant(t, ctx, f.App, platform, "docs-b")
	f.Priv = f.A.UserID
	for _, tn := range []struct {
		t      dbtest.Tenant
		entity *uuid.UUID
	}{{f.A, &f.EntityA}, {f.B, &f.EntityB}} {
		if err := pdb.WithTenantTx(ctx, f.App, tn.t.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Pim Privacy' WHERE id = $1`, tn.t.UserID); err != nil {
				return err
			}
			if tn.t.ID == f.A.ID {
				for _, u := range []struct {
					id   *uuid.UUID
					name string
				}{{&f.DPOUser, "Dee DPO"}, {&f.Law, "Lek Legal"}} {
					if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, $3, 'active') RETURNING id`,
						tn.t.ID, uuid.NewString()[:8]+"@docs.example", u.name).Scan(u.id); err != nil {
						return err
					}
				}
			}
			return tx.QueryRow(ctx, `INSERT INTO org.legal_entities (tenant_id, name_th, name_en, registration_no, contact_email)
				VALUES ($1, 'บริษัท ตัวอย่าง จำกัด (มหาชน)', 'Example PCL', '0107537000254', 'dpo@example.co.th') RETURNING id`, tn.t.ID).Scan(tn.entity)
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
	n := &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}
	f.Ver = wiring.Versioning(n, audit.New())
	f.Files = &files.Service{Store: store, River: client, Config: files.DefaultConfig()}
	var pdf render.PDFRenderer
	if p := ChromiumPath(); p != "" {
		pdf, f.HasPDF = &render.Chromium{Path: p}, true
	}
	f.Svc = wiring.Docs(f.Ver, f.Files, client, audit.New(), pdf)
	f.Svc.RegisterVersioning()
	f.Files.EntityPermissions = f.Svc.FilePermissions()
	f.Renderer = &docs.Renderer{Service: f.Svc}
	f.scanner = &files.Scanner{Store: store, AV: &files.Clamd{Addr: clamd}, Audit: audit.New()}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func (f *Fixture) cleanup(t *testing.T) {
	for _, tn := range []dbtest.Tenant{f.A, f.B} {
		_ = pdb.WithTenantTx(context.Background(), f.App, tn.ID.String(), "", func(ctx context.Context) error {
			rows, _ := pdb.MustTxFromContext(ctx).Query(ctx, `SELECT object_key FROM platform.files`)
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
			for _, q := range []string{`UPDATE platform.documents SET current_version_id = NULL`, `DELETE FROM platform.document_versions`,
				`DELETE FROM platform.documents`, `DELETE FROM platform.templates WHERE tenant_id IS NOT NULL`,
				`DELETE FROM platform.clause_library WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`,
				`DELETE FROM platform.files`, `DELETE FROM platform.notifications`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`,
				`DELETE FROM org.legal_entities`} {
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

// Try runs fn in one transaction as user (tenant B's own user runs in tenant B) with roles and permissions.
func (f *Fixture) Try(user uuid.UUID, roles, perms []string, fn func(ctx context.Context) error) error {
	tenant := f.A
	if user == f.B.UserID {
		tenant = f.B
	}
	return pdb.WithTenantTx(context.Background(), f.App, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Roles: roles, Permissions: perms}))
	})
}

// As is Try failing the test on an error.
func (f *Fixture) As(t *testing.T, user uuid.UUID, roles, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := f.Try(user, roles, perms, fn); err != nil {
		t.Fatal(err)
	}
}

// Text is a paragraph node with inline nodes (strings become text nodes).
func Para(parts ...any) render.Node {
	p := render.Node{Type: "paragraph"}
	for _, x := range parts {
		switch v := x.(type) {
		case string:
			p.Content = append(p.Content, render.Node{Type: "text", Text: v})
		case render.Node:
			p.Content = append(p.Content, v)
		}
	}
	return p
}

// Field is a merge field node.
func Field(key string) render.Node {
	return render.Node{Type: "mergeField", Attrs: map[string]any{"key": key}}
}

// ClauseRef is a clause block referring to code@version.
func ClauseRef(code string, version int) render.Node {
	return render.Node{Type: "clause", Attrs: map[string]any{"code": code, "version": version}}
}

// Doc is a ProseMirror doc of blocks.
func Doc(blocks ...render.Node) render.Node { return render.Node{Type: "doc", Content: blocks} }

// Heading is a heading of level with text.
func Heading(level int, text string) render.Node {
	return render.Node{Type: "heading", Attrs: map[string]any{"level": level}, Content: []render.Node{{Type: "text", Text: text}}}
}

// Approve submits the document's open draft (as its author), has the DPO approve it and publishes it (as the DPO);
// it returns the publish error.
func (f *Fixture) Approve(t *testing.T, doc docs.Document, author uuid.UUID, authorPerms []string) error {
	t.Helper()
	var v versioning.Version
	f.As(t, author, nil, authorPerms, func(ctx context.Context) error {
		d, err := f.Svc.Get(ctx, doc.ID)
		if err != nil {
			return err
		}
		v, err = f.Ver.Submit(ctx, d.Latest.ID, d.Latest.RowVersion)
		return err
	})
	f.As(t, f.DPOUser, []string{"DPO"}, DPO, func(ctx context.Context) error {
		inbox, err := f.Ver.Inbox(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(inbox, func(it versioning.InboxItem) bool { return it.VersionID == v.ID })
		if i < 0 {
			t.Fatalf("not in the DPO's inbox: %+v", inbox)
		}
		v, err = f.Ver.Decide(ctx, inbox[i].ID, inbox[i].RowVersion, "approved", "")
		return err
	})
	return f.Try(f.DPOUser, []string{"DPO"}, DPO, func(ctx context.Context) error {
		_, err := f.Ver.Publish(ctx, v.ID, v.RowVersion)
		return err
	})
}

// Render runs the docs.render job of a published version in tenant A.
func (f *Fixture) Render(t *testing.T, versionID uuid.UUID) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
		return f.Renderer.Work(ctx, &river.Job[docs.RenderArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
			Args: docs.RenderArgs{TenantArgs: jobs.TenantArgs{TenantID: f.A.ID.String()}, VersionID: versionID.String()}})
	})
}

// Scan runs the virus scan of a file of tenant A (the worker's files.scan job).
func (f *Fixture) Scan(t *testing.T, id uuid.UUID) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
		return f.scanner.Work(ctx, &river.Job[files.ScanArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 10},
			Args: files.ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: f.A.ID.String()}, FileID: id.String()}})
	}); err != nil {
		t.Fatal(err)
	}
}

// ReadFile scans a stored file of tenant A and returns its bytes.
func (f *Fixture) ReadFile(t *testing.T, id uuid.UUID) []byte {
	t.Helper()
	f.Scan(t, id)
	var out []byte
	if err := pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
		r, _, err := f.Files.Open(ctx, id)
		if err != nil {
			return err
		}
		defer r.Close()
		out, err = io.ReadAll(r)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
