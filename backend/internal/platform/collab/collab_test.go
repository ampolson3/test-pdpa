package collab_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/collab"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

const recordType = "test_record"

type fixture struct {
	app, owner *pgxpool.Pool
	a, b       dbtest.Tenant
	alice, bob uuid.UUID // two more active users of tenant A (a.UserID is a third)
	svc        *collab.Service
	notify     *notify.Service
	records    *sync.Map // entity ids that "exist" per tenant: key tenantID/entityID
	record     uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t), owner: dbtest.OwnerPool(t), records: &sync.Map{}, record: uuid.New()}
	platform := dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "collab-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "collab-b")
	f.records.Store(f.a.ID.String()+"/"+f.record.String(), true)

	if err := pdb.WithTenantTx(ctx, f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		for _, u := range []struct {
			id   *uuid.UUID
			name string
		}{{&f.alice, "Alice"}, {&f.bob, "Bob"}} {
			if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, $3, 'active') RETURNING id`,
				f.a.ID, strings.ToLower(u.name)+"@collab.example", u.name).Scan(u.id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, f.a.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	client, err := jobs.NewInsertClient(f.app)
	if err != nil {
		t.Fatal(err)
	}
	f.notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	f.svc = &collab.Service{Notify: f.notify, Audit: audit.New()}
	f.svc.Register(recordType, collab.Policy{
		ReadPermission: "test.record.read", WritePermission: "test.record.update",
		Exists: func(ctx context.Context, id uuid.UUID) (bool, error) {
			var tenant string
			if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&tenant); err != nil {
				return false, err
			}
			_, ok := f.records.Load(tenant + "/" + id.String())
			return ok, nil
		},
	})

	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.comments`)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.notifications`)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.files`)
				_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE email LIKE '%@collab.example'`)
				return err
			})
			_ = pdb.WithTenantTx(context.Background(), f.owner, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.audit_log`)
				_, err := tx.Exec(ctx, `DELETE FROM platform.tenant_keys`)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	return f
}

var writer = []string{"test.record.read", "test.record.update"}

// as runs fn as user of tenant with perms, like a request reaching the service.
func (f *fixture) as(t *testing.T, tenant dbtest.Tenant, user uuid.UUID, perms []string, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Permissions: perms}))
	})
}

func (f *fixture) comment(t *testing.T, user uuid.UUID, parent *uuid.UUID, body string) collab.Comment {
	t.Helper()
	var c collab.Comment
	if err := f.as(t, f.a, user, writer, func(ctx context.Context) error {
		var err error
		c, err = f.svc.AddComment(ctx, recordType, f.record, parent, body)
		return err
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	return c
}

func (f *fixture) inbox(t *testing.T, user uuid.UUID) []notify.InboxItem {
	t.Helper()
	var items []notify.InboxItem
	_ = f.as(t, f.a, user, nil, func(ctx context.Context) error {
		var err error
		items, _, err = f.notify.Inbox(ctx, user, 50)
		return err
	})
	return items
}

func mention(name string, id uuid.UUID) string { return fmt.Sprintf("@[%s](%s)", name, id) }

// Acceptance criterion: "การ mention ส่งแจ้งเตือน".
func TestMention_NotifiesTheMentionedUser(t *testing.T) {
	f := setup(t)
	c := f.comment(t, f.alice, nil, "ช่วยตรวจ "+mention("Bob", f.bob)+" ด้วยครับ")
	if len(c.Mentions) != 1 || c.Mentions[0] != f.bob || c.AuthorName != "Alice" {
		t.Fatalf("comment = %+v", c)
	}
	items := f.inbox(t, f.bob)
	if len(items) != 1 || items[0].Title != "Alice กล่าวถึงคุณ" || items[0].Body != "Alice กล่าวถึงคุณในความเห็น: ช่วยตรวจ @Bob ด้วยครับ" {
		t.Errorf("Bob's inbox = %+v", items)
	}
	if got := f.inbox(t, f.alice); len(got) != 0 {
		t.Errorf("the author was notified of their own comment: %+v", got)
	}
}

// Only active users of the same tenant can be mentioned; the author mentioning themselves is ignored.
func TestMention_IgnoresNonUsersAndSelf(t *testing.T) {
	f := setup(t)
	c := f.comment(t, f.alice, nil, strings.Join([]string{
		mention("Ghost", uuid.New()), mention("Other tenant", f.b.UserID), mention("Alice", f.alice), mention("Bob", f.bob), mention("Bob again", f.bob),
	}, " "))
	if len(c.Mentions) != 1 || c.Mentions[0] != f.bob {
		t.Errorf("mentions = %v, want only Bob once", c.Mentions)
	}
}

func TestReply_NotifiesTheParentAuthor(t *testing.T) {
	f := setup(t)
	root := f.comment(t, f.alice, nil, "ต้องแก้ข้อ 3")
	reply := f.comment(t, f.bob, &root.ID, "แก้แล้ว")
	if reply.ParentID == nil || *reply.ParentID != root.ID {
		t.Fatalf("reply = %+v", reply)
	}
	items := f.inbox(t, f.alice)
	if len(items) != 1 || items[0].Title != "Bob ตอบความเห็นของคุณ" {
		t.Errorf("Alice's inbox = %+v", items)
	}
	err := f.as(t, f.a, f.alice, writer, func(ctx context.Context) error {
		_, err := f.svc.AddComment(ctx, recordType, f.record, &reply.ID, "reply to a reply")
		return err
	})
	if !errors.Is(err, collab.ErrInvalid) {
		t.Errorf("reply to a reply: %v, want ErrInvalid (one level)", err)
	}
}

func TestAccessRules(t *testing.T) {
	f := setup(t)
	f.comment(t, f.alice, nil, "hello")
	cases := []struct {
		name  string
		as    dbtest.Tenant
		perms []string
		typ   string
		rec   uuid.UUID
		want  error
	}{
		{"no read permission", f.a, nil, recordType, f.record, collab.ErrForbidden},
		{"unregistered type", f.a, writer, "nope", f.record, collab.ErrUnknownType},
		{"record that doesn't exist", f.a, writer, recordType, uuid.New(), collab.ErrNotFound},
		{"other tenant, same record id", f.b, writer, recordType, f.record, collab.ErrNotFound},
	}
	for _, tc := range cases {
		err := f.as(t, tc.as, tc.as.UserID, tc.perms, func(ctx context.Context) error {
			_, err := f.svc.Comments(ctx, tc.typ, tc.rec)
			return err
		})
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	err := f.as(t, f.a, f.bob, []string{"test.record.read"}, func(ctx context.Context) error {
		_, err := f.svc.AddComment(ctx, recordType, f.record, nil, "read-only user")
		return err
	})
	if !errors.Is(err, collab.ErrForbidden) {
		t.Errorf("comment with read permission only: %v, want ErrForbidden", err)
	}
	// Tenant B's own comments table has nothing of A's (RLS), whatever the record id.
	var n int
	_ = pdb.WithTenantTx(context.Background(), f.app, f.b.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.comments`).Scan(&n)
	})
	if n != 0 {
		t.Errorf("tenant B sees %d of tenant A's comments", n)
	}
}

func TestEditDeleteResolve(t *testing.T) {
	f := setup(t)
	root := f.comment(t, f.alice, nil, "draft")
	reply := f.comment(t, f.bob, &root.ID, "ok")
	err := f.as(t, f.a, f.alice, writer, func(ctx context.Context) error {
		if _, err := f.svc.EditComment(ctx, reply.ID, reply.RowVersion, "hijack"); !errors.Is(err, collab.ErrForbidden) {
			t.Errorf("editing someone else's comment: %v", err)
		}
		if _, err := f.svc.EditComment(ctx, root.ID, root.RowVersion+5, "x"); !errors.Is(err, collab.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		edited, err := f.svc.EditComment(ctx, root.ID, root.RowVersion, "final "+mention("Bob", f.bob))
		if err != nil {
			return err
		}
		if edited.RowVersion != root.RowVersion+1 || len(edited.Mentions) != 1 {
			t.Errorf("edited = %+v", edited)
		}
		if err := f.svc.DeleteComment(ctx, root.ID, edited.RowVersion); !errors.Is(err, collab.ErrHasReplies) {
			t.Errorf("delete with replies: %v, want ErrHasReplies", err)
		}
		if _, err := f.svc.SetResolved(ctx, reply.ID, true); !errors.Is(err, collab.ErrInvalid) {
			t.Errorf("resolve a reply: %v, want ErrInvalid", err)
		}
		resolved, err := f.svc.SetResolved(ctx, root.ID, true)
		if err != nil || !resolved.Resolved {
			t.Errorf("resolve thread: %+v %v", resolved, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Bob got one notification for the new mention added by the edit.
	if items := f.inbox(t, f.bob); len(items) != 1 {
		t.Errorf("Bob's inbox after the edit mention: %d items, want 1", len(items))
	}
	if err := f.as(t, f.a, f.bob, writer, func(ctx context.Context) error {
		return f.svc.DeleteComment(ctx, reply.ID, reply.RowVersion)
	}); err != nil {
		t.Errorf("author deleting a leaf comment: %v", err)
	}
}

// Attachments (PLT-09 files) and the activity feed built from the audit log.
func TestAttachmentsAndActivity(t *testing.T) {
	f := setup(t)
	endpoint := envOr("TEST_S3_ENDPOINT", "127.0.0.1:8333")
	if c, err := net.DialTimeout("tcp", endpoint, time.Second); err != nil {
		t.Skipf("no S3 at %s", endpoint)
	} else {
		c.Close()
	}
	store, err := files.NewS3Store(files.S3Config{Endpoint: endpoint, AccessKey: envOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: envOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"), Bucket: "pdpa-files-test", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.svc.Files = &files.Service{Store: store, River: f.notify.River, Config: files.DefaultConfig(), EntityPermissions: f.svc.FilePermissions()}

	f.comment(t, f.alice, nil, "แนบสัญญาแล้ว")
	var upload files.File
	if err := f.as(t, f.a, f.alice, writer, func(ctx context.Context) error {
		var err error
		if upload, err = f.svc.Files.Upload(ctx, "สัญญา.pdf", bytes.NewReader([]byte("%PDF-1.4\n%%EOF\n"))); err != nil {
			return err
		}
		return f.svc.Attach(ctx, recordType, f.record, upload.ID)
	}); err != nil {
		t.Fatal(err)
	}
	// Bob didn't upload it, but once attached he sees it through the record's read permission.
	err = f.as(t, f.a, f.bob, []string{"test.record.read"}, func(ctx context.Context) error {
		atts, err := f.svc.Attachments(ctx, recordType, f.record)
		if err != nil {
			return err
		}
		if len(atts) != 1 || atts[0].FileName != "สัญญา.pdf" || atts[0].UploaderName != "Alice" {
			t.Errorf("attachments = %+v", atts)
		}
		if _, err := f.svc.Files.Get(ctx, upload.ID); err != nil {
			t.Errorf("Bob reading the attached file: %v", err)
		}
		acts, err := f.svc.Activity(ctx, recordType, f.record, 10)
		if err != nil {
			return err
		}
		if len(acts) != 2 || acts[0].Action != "platform.file.attach" || acts[1].Action != "platform.comment.create" || acts[0].ActorName != "Alice" {
			t.Errorf("activity = %+v", acts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.as(t, f.a, f.bob, writer, func(ctx context.Context) error {
		return f.svc.Attach(ctx, recordType, f.record, upload.ID)
	}); !errors.Is(err, collab.ErrInvalid) {
		t.Errorf("re-attaching: %v, want ErrInvalid", err)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
