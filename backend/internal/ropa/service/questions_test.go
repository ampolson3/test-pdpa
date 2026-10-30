package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/collab"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// wireCollab registers "ropa_activity" with collab.Service the same way cmd/api/main.go does, so these
// tests exercise the real registration, not a stand-in.
func wireCollab(e env) {
	c := &collab.Service{Audit: audit.New()}
	c.Register(ropaservice.ActivityCollabEntityType, collab.Policy{
		ReadPermission: "ropa.activity.read", WritePermission: "ropa.activity.update",
		Exists: func(ctx context.Context, id uuid.UUID) (bool, error) {
			_, err := e.svc.GetActivity(ctx, id)
			return err == nil, nil
		},
	})
	e.svc.Collab = c
}

// inAs runs fn as a specific user of the tenant with the given permissions (ROPA-12 needs a second,
// distinct user besides the tenant's own default one, to ask and to be asked).
func (e env) inAs(t *testing.T, userID uuid.UUID, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), userID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: userID.String(), Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func seedUser(t *testing.T, ctx context.Context, e env, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'active') RETURNING id`,
		fmt.Sprintf("%s@dbtest.example", uuid.NewString()[:8]), name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestMyOpenQuestions is ROPA-12's own acceptance criterion: a question (an unresolved top-level
// comment @mentioning someone) shows up in that someone's own open-question task list, and drops off
// once it's answered (resolved).
func TestMyOpenQuestions(t *testing.T) {
	e := setup(t, "ropaqa")
	wireCollab(e)

	var activity ropaservice.Activity
	var dpoID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activity = completeActivity(t, ctx, e, "QA-01")
		dpoID = seedUser(t, ctx, e, "Dee DPO")
		return nil
	})

	// The owner asks the DPO a question on this activity.
	perms := []string{"ropa.activity.read", "ropa.activity.update"}
	var commentID uuid.UUID
	e.inAs(t, e.tenant.UserID, perms, func(ctx context.Context) error {
		c, err := e.svc.Collab.AddComment(ctx, ropaservice.ActivityCollabEntityType, activity.ID, nil,
			fmt.Sprintf("@[Dee DPO](%s) ข้อมูลนี้เก็บไว้นานแค่ไหนครับ", dpoID))
		if err != nil {
			return err
		}
		commentID = c.ID
		return nil
	})

	// It shows up in the DPO's own open-question task list.
	e.inAs(t, dpoID, perms, func(ctx context.Context) error {
		list, err := e.svc.MyOpenQuestions(ctx)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("got %d open questions, want 1", len(list))
		}
		q := list[0]
		if q.ActivityID != activity.ID || q.ActivityCode != activity.Code || q.CommentID != commentID {
			t.Errorf("open question doesn't name the right activity/comment: %+v", q)
		}
		return nil
	})

	// The owner's own task list is empty — they asked it, they didn't get asked.
	e.inAs(t, e.tenant.UserID, perms, func(ctx context.Context) error {
		list, err := e.svc.MyOpenQuestions(ctx)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("the asker's own task list should be empty, got %d", len(list))
		}
		return nil
	})

	// The DPO answers (a reply) and resolves the thread — it drops off the task list.
	e.inAs(t, dpoID, perms, func(ctx context.Context) error {
		if _, err := e.svc.Collab.AddComment(ctx, ropaservice.ActivityCollabEntityType, activity.ID, &commentID, "เก็บ 5 ปีครับ"); err != nil {
			return err
		}
		if _, err := e.svc.Collab.SetResolved(ctx, commentID, true); err != nil {
			return err
		}
		return nil
	})
	e.inAs(t, dpoID, perms, func(ctx context.Context) error {
		list, err := e.svc.MyOpenQuestions(ctx)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("an answered question must drop off the task list, got %d", len(list))
		}
		return nil
	})
}

// TestMyOpenQuestions_TwoTenantIsolation (rule 1): platform.comments is RLS-scoped, so a question asked
// in one tenant never surfaces in another tenant's task list even for a same-named mentioned user.
func TestMyOpenQuestions_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "ropaqaiso1")
	e2 := setup(t, "ropaqaiso2")
	wireCollab(e1)
	wireCollab(e2)

	var activity ropaservice.Activity
	var dpoID uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		activity = completeActivity(t, ctx, e1, "QA-ISO-01")
		dpoID = seedUser(t, ctx, e1, "Dee DPO")
		return nil
	})
	perms := []string{"ropa.activity.read", "ropa.activity.update"}
	e1.inAs(t, e1.tenant.UserID, perms, func(ctx context.Context) error {
		_, err := e1.svc.Collab.AddComment(ctx, ropaservice.ActivityCollabEntityType, activity.ID, nil,
			fmt.Sprintf("@[Dee DPO](%s) คำถามข้ามเทนแนนต์", dpoID))
		return err
	})

	// The same user id, asked under tenant 2's own RLS scope, sees nothing from tenant 1.
	e2.inAs(t, dpoID, perms, func(ctx context.Context) error {
		list, err := e2.svc.MyOpenQuestions(ctx)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant isolation leak: got %d open questions from another tenant, want 0", len(list))
		}
		return nil
	})
}
