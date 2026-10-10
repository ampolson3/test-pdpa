package versioning_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
)

type fixture struct {
	app                    *pgxpool.Pool
	a, b                   dbtest.Tenant
	alice, bob, carol, eve uuid.UUID
	svc                    *versioning.Service
	published              map[uuid.UUID]string // what OnPublish applied
}

var editor = []string{"x.doc.read", "x.doc.update", "x.doc.publish"}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t), published: map[uuid.UUID]string{}}
	owner, platform := dbtest.OwnerPool(t), dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "ver-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "ver-b")
	f.alice = f.a.UserID
	if err := pdb.WithTenantTx(ctx, f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, f.alice); err != nil {
			return err
		}
		for _, u := range []struct {
			id   *uuid.UUID
			name string
		}{{&f.bob, "Bob"}, {&f.carol, "Carol"}, {&f.eve, "Eve"}} {
			if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, $3, 'active') RETURNING id`,
				f.a.ID, u.name+"@ver.example", u.name).Scan(u.id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				for _, q := range []string{`DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`, `DELETE FROM platform.notifications`,
					`DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`} {
					_, _ = tx.Exec(ctx, q)
				}
				_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id <> $1`, tenant.UserID)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	client, _ := jobs.NewInsertClient(f.app)
	f.svc = &versioning.Service{Notify: &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}, Audit: audit.New()}
	f.svc.Register("test_doc", versioning.Policy{
		ReadPermission: "x.doc.read", EditPermission: "x.doc.update", PublishPermission: "x.doc.publish",
		Steps: []versioning.Step{{Role: "DPO"}, {Role: "LEGAL"}},
		Title: func(context.Context, uuid.UUID) (string, error) { return "ประกาศทดสอบ", nil },
		OnPublish: func(_ context.Context, id uuid.UUID, snap json.RawMessage) error {
			var v struct{ Title string }
			_ = json.Unmarshal(snap, &v)
			f.published[id] = v.Title
			return nil
		},
	})
	return f
}

// as runs fn in one transaction of tenant a as user with roles and permissions.
func (f *fixture) as(t *testing.T, user uuid.UUID, roles, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: f.a.ID.String(), UserID: user.String(), Roles: roles, Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

type doc struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// approveAll takes a submitted version through both steps (Bob for DPO, Carol for LEGAL).
func (f *fixture) approveAll(t *testing.T, v versioning.Version) versioning.Version {
	t.Helper()
	for _, who := range []struct {
		id   uuid.UUID
		role string
	}{{f.bob, "DPO"}, {f.carol, "LEGAL"}} {
		f.as(t, who.id, []string{who.role}, nil, func(ctx context.Context) error {
			inbox, err := f.svc.Inbox(ctx)
			if err != nil || len(inbox) != 1 || inbox[0].VersionID != v.ID || inbox[0].Title != "ประกาศทดสอบ" || inbox[0].AuthorName != "Alice" {
				t.Fatalf("%s inbox: %+v %v", who.role, inbox, err)
			}
			v, err = f.svc.Decide(ctx, inbox[0].ID, inbox[0].RowVersion, "approved", "")
			return err
		})
	}
	return v
}

// Acceptance (PLT-08): a published record can't be edited — a change is a new version; and the maker
// can't approve their own work.
func TestPublishedIsImmutable_ChangesMakeNewVersions(t *testing.T) {
	f := setup(t)
	record := uuid.New()
	var v1 versioning.Version
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		var err error
		if v1, err = f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "ประกาศ", Body: "ร่างแรก"}); err != nil {
			return err
		}
		if v1, err = f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "ประกาศ", Body: "ร่างแก้"}); err != nil || v1.No != 1 {
			t.Errorf("editing the draft keeps version 1: %+v %v", v1, err)
		}
		v1, err = f.svc.Submit(ctx, v1.ID, v1.RowVersion)
		if err == nil && (v1.Status != "in_review" || len(v1.Diff) != 1 || v1.Diff[0].Path != "$") {
			t.Errorf("submitted: %+v", v1)
		}
		if _, err := f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "x"}); !errors.Is(err, versioning.ErrLocked) {
			t.Errorf("edit under review: %v", err)
		}
		return err
	})
	v1 = f.approveAll(t, v1)
	if v1.Status != "approved" {
		t.Fatalf("after both steps: %s", v1.Status)
	}
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		var err error
		if v1, err = f.svc.Publish(ctx, v1.ID, v1.RowVersion); err != nil || v1.Status != "published" || f.published[record] != "ประกาศ" {
			t.Fatalf("publish: %+v %v %v", v1, err, f.published)
		}
		v2, err := f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "ประกาศ ฉบับปรับปรุง", Body: "ร่างแก้"})
		if err != nil || v2.No != 2 || v2.Status != "draft" {
			t.Fatalf("change after publish: %+v %v", v2, err)
		}
		pub, err := f.svc.Published(ctx, "test_doc", record)
		if err != nil || pub.ID != v1.ID || string(pub.Snapshot) != string(v1.Snapshot) {
			t.Errorf("published version untouched: %+v %v", pub, err)
		}
		if v2, err = f.svc.Submit(ctx, v2.ID, v2.RowVersion); err != nil || len(v2.Diff) != 1 || v2.Diff[0].Path != "title" {
			t.Fatalf("v2 diff against the published version: %+v %v", v2.Diff, err)
		}
		changes, err := f.svc.Compare(ctx, v1.ID, v2.ID)
		if err != nil || len(changes) != 1 {
			t.Errorf("compare: %+v %v", changes, err)
		}
		if _, err := f.svc.Publish(ctx, v2.ID, v2.RowVersion); !errors.Is(err, versioning.ErrInvalidTransition) {
			t.Errorf("publishing before approval: %v", err)
		}
		return nil
	})
	var v2 versioning.Version
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		list, err := f.svc.List(ctx, "test_doc", record)
		if err != nil || len(list) != 2 || list[0].No != 2 || list[1].Status != "published" || list[0].AuthorName != "Alice" {
			t.Fatalf("list: %+v %v", list, err)
		}
		v2 = list[0]
		return nil
	})
	v2 = f.approveAll(t, v2)
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		var err error
		if _, err = f.svc.Publish(ctx, v2.ID, v2.RowVersion); err != nil || f.published[record] != "ประกาศ ฉบับปรับปรุง" {
			t.Fatalf("publish v2: %v", err)
		}
		list, _ := f.svc.List(ctx, "test_doc", record)
		if list[1].Status != "superseded" || list[0].Status != "published" {
			t.Errorf("v1 superseded by v2: %s / %s", list[1].Status, list[0].Status)
		}
		return nil
	})
}

func TestMakerChecker(t *testing.T) {
	f := setup(t)
	record := uuid.New()
	var v versioning.Version
	// Alice writes and submits — and holds the DPO role herself.
	f.as(t, f.alice, []string{"DPO"}, editor, func(ctx context.Context) error {
		var err error
		if v, err = f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "a"}); err != nil {
			return err
		}
		if v, err = f.svc.Submit(ctx, v.ID, v.RowVersion); err != nil {
			return err
		}
		if inbox, _ := f.svc.Inbox(ctx); len(inbox) != 0 {
			t.Errorf("own work in the maker's inbox: %d", len(inbox))
		}
		full, _ := f.svc.Get(ctx, v.ID)
		if _, err := f.svc.Decide(ctx, full.Approvals[0].ID, full.Approvals[0].RowVersion, "approved", ""); !errors.Is(err, versioning.ErrSelfApproval) {
			t.Errorf("maker approves own work: %v", err)
		}
		return nil
	})
	var steps []versioning.Approval
	f.as(t, f.bob, []string{"DPO", "LEGAL"}, nil, func(ctx context.Context) error {
		full, err := f.svc.Get(ctx, v.ID)
		steps = full.Approvals
		return err
	})
	// Carol (LEGAL) can't decide level 2 before level 1; Eve without the role can't decide at all.
	f.as(t, f.carol, []string{"LEGAL"}, nil, func(ctx context.Context) error {
		if _, err := f.svc.Decide(ctx, steps[1].ID, steps[1].RowVersion, "approved", ""); !errors.Is(err, versioning.ErrInvalidTransition) {
			t.Errorf("level 2 first: %v", err)
		}
		if inbox, _ := f.svc.Inbox(ctx); len(inbox) != 0 {
			t.Errorf("level 2 in the inbox before level 1: %d", len(inbox))
		}
		return nil
	})
	f.as(t, f.eve, []string{"EMP"}, []string{"x.doc.read"}, func(ctx context.Context) error {
		if _, err := f.svc.Decide(ctx, steps[0].ID, steps[0].RowVersion, "approved", ""); !errors.Is(err, versioning.ErrForbidden) {
			t.Errorf("without the step's role: %v", err)
		}
		return nil
	})
	// Bob holds both roles: one person, one level.
	f.as(t, f.bob, []string{"DPO", "LEGAL"}, nil, func(ctx context.Context) error {
		if _, err := f.svc.Decide(ctx, steps[0].ID, steps[0].RowVersion+3, "approved", ""); !errors.Is(err, versioning.ErrVersionMismatch) {
			t.Errorf("stale approval: %v", err)
		}
		if _, err := f.svc.Decide(ctx, steps[0].ID, steps[0].RowVersion, "approved", ""); err != nil {
			return err
		}
		full, _ := f.svc.Get(ctx, v.ID)
		if _, err := f.svc.Decide(ctx, full.Approvals[1].ID, full.Approvals[1].RowVersion, "approved", ""); !errors.Is(err, versioning.ErrSelfApproval) {
			t.Errorf("same person approves two levels: %v", err)
		}
		return nil
	})
	// Carol returns it — a reason is required — and it goes back to Alice as a draft.
	f.as(t, f.carol, []string{"LEGAL"}, nil, func(ctx context.Context) error {
		inbox, err := f.svc.Inbox(ctx)
		if err != nil || len(inbox) != 1 || inbox[0].Step != 2 {
			t.Fatalf("level 2 now in Carol's inbox: %+v %v", inbox, err)
		}
		if _, err := f.svc.Decide(ctx, inbox[0].ID, inbox[0].RowVersion, "returned", " "); !errors.Is(err, versioning.ErrInvalidRequest) {
			t.Errorf("return without reason: %v", err)
		}
		back, err := f.svc.Decide(ctx, inbox[0].ID, inbox[0].RowVersion, "returned", "แก้ข้อ 3")
		if err != nil || back.Status != "draft" {
			t.Errorf("returned: %+v %v", back, err)
		}
		return err
	})
	var n int
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'approval.returned' AND n.recipient_user_id = $1`, f.alice).Scan(&n); err != nil {
			return err
		}
		v2, err := f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "a (แก้แล้ว)"})
		if err != nil || v2.ID != v.ID {
			t.Errorf("the returned draft is edited in place: %+v %v", v2, err)
		}
		return nil
	})
	if n != 1 {
		t.Errorf("requester notified of the return: %d", n)
	}
}

func TestAccessAndIsolation(t *testing.T) {
	f := setup(t)
	record := uuid.New()
	var v versioning.Version
	f.as(t, f.alice, nil, editor, func(ctx context.Context) error {
		var err error
		v, err = f.svc.SaveDraft(ctx, "test_doc", record, doc{Title: "a"})
		if _, err := f.svc.SaveDraft(ctx, "unregistered", record, doc{}); !errors.Is(err, versioning.ErrNotFound) {
			t.Errorf("unregistered type: %v", err)
		}
		return err
	})
	f.as(t, f.eve, nil, []string{"x.doc.read"}, func(ctx context.Context) error {
		if _, err := f.svc.SaveDraft(ctx, "test_doc", record, doc{}); !errors.Is(err, versioning.ErrForbidden) {
			t.Errorf("save without edit permission: %v", err)
		}
		if _, err := f.svc.Submit(ctx, v.ID, v.RowVersion); !errors.Is(err, versioning.ErrForbidden) {
			t.Errorf("submit without edit permission: %v", err)
		}
		if list, err := f.svc.List(ctx, "test_doc", record); err != nil || len(list) != 1 {
			t.Errorf("read permission lists: %v", err)
		}
		return nil
	})
	f.as(t, f.eve, nil, nil, func(ctx context.Context) error {
		if _, err := f.svc.Get(ctx, v.ID); !errors.Is(err, versioning.ErrNotFound) {
			t.Errorf("no permission reads: %v", err)
		}
		return nil
	})
	err := pdb.WithTenantTx(context.Background(), f.app, f.b.ID.String(), f.b.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.b.ID.String(), UserID: f.b.UserID.String(), Roles: []string{"DPO", "LEGAL"}, Permissions: editor})
		if _, err := f.svc.Get(ctx, v.ID); !errors.Is(err, versioning.ErrNotFound) {
			t.Errorf("B reads A's version: %v", err)
		}
		if _, err := f.svc.Submit(ctx, v.ID, v.RowVersion); !errors.Is(err, versioning.ErrNotFound) {
			t.Errorf("B submits A's version: %v", err)
		}
		if list, _ := f.svc.List(ctx, "test_doc", record); len(list) != 0 {
			t.Errorf("B lists A's versions")
		}
		if inbox, _ := f.svc.Inbox(ctx); len(inbox) != 0 {
			t.Errorf("B's inbox has A's approvals")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
