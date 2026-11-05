package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// withUser overrides the caller's identity for an access-control check without touching the Postgres session
// (app.user_id) — requireSubtaskAccess only ever reads authz.FromContext, so this is enough to simulate a
// different real user (owner/other/groupMember, each a real iam.users row from seedUser) acting in the same
// tenant transaction.
func withUser(ctx context.Context, userID uuid.UUID) context.Context {
	g, _ := authz.FromContext(ctx)
	g.UserID = userID.String()
	return authz.WithGrants(ctx, g)
}

// subtaskEnv wires a real notify.Service (the same minimal construction collab_test.go uses) so a created
// subtask's assignee notification is visible as a real platform.notifications row, and the fake Files double
// DSAR-06's own tests use, for UpdateSubtaskStatus's evidence-attach path.
func subtaskEnv(t *testing.T, suffix string) (env, *fakeFiles) {
	t.Helper()
	e := setup(t, suffix)
	ff := newFakeFiles()
	e.svc.Files = ff
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: e.svc.Keyring, River: client, Quiet: notify.QuietHours{}}
	return e, ff
}

func seedGroup(t *testing.T, ctx context.Context, member uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO iam.groups (tenant_id, name) VALUES (current_setting('app.tenant_id')::uuid, 'IT') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx,
		`INSERT INTO iam.group_members (group_id, user_id, tenant_id) VALUES ($1, $2, current_setting('app.tenant_id')::uuid)`,
		id, member); err != nil {
		t.Fatal(err)
	}
	return id
}

// bringToInProgress drives a fresh request through received → verifying → in_review → in_progress (DSAR-13's
// own Transition, no identity check needed since this package's Verification field is nil for these tests).
func bringToInProgress(t *testing.T, ctx context.Context, e env, r dsarservice.Request) dsarservice.Request {
	t.Helper()
	for _, to := range []string{"verifying", "in_review", "in_progress"} {
		var err error
		r, _, err = e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: to})
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	return r
}

// TestCreateSubtask_NotifiesAssignee is DSAR-08's own "มอบหมายงานย่อยให้เจ้าของระบบหรือทีม" half: assigning a
// subtask to a user notifies them (PLT-04); an unknown action or assignee is refused before anything is
// written.
func TestCreateSubtask_NotifiesAssignee(t *testing.T) {
	e, _ := subtaskEnv(t, "dsarsubtaskassign")
	var r dsarservice.Request
	var assignee uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		assignee = seedUser(t, ctx, e)
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-assign@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	var st dsarservice.Subtask
	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "search", AssigneeUserID: &assignee})
		return err
	})
	if st.Status != "open" {
		t.Fatalf("new subtask status = %q, want open", st.Status)
	}

	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications WHERE recipient_user_id = $1
			AND template_id IN (SELECT id FROM platform.notification_templates WHERE code = 'dsar.subtask_assigned')`,
			assignee).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected a dsar.subtask_assigned notification for the assignee")
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "bogus"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bogus action: err = %v, want ErrInvalid", err)
		}
		unknown := uuid.New()
		if _, err := e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "search", AssigneeUserID: &unknown}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown assignee: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestUpdateSubtaskStatus_Transitions is the small open → in_progress → done|not_applicable machine.
func TestUpdateSubtaskStatus_Transitions(t *testing.T) {
	e, ff := subtaskEnv(t, "dsarsubtaskstatus")
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-status@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	var st dsarservice.Subtask
	var evidenceID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "export"})
		if err != nil {
			return err
		}
		// a caller's own clean, unattached PLT-09 upload — the same fixture DSAR-06 uses for its own evidence.
		evidenceID = uuid.New()
		ff.raw[evidenceID] = []byte("evidence")
		_, err = pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status)
			VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, 'evidence.txt', 'text/plain', 8, 'x', 'clean')`,
			evidenceID, evidenceID.String())
		return err
	})

	// Skipping straight from open to done is allowed (a quick subtask).
	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.UpdateSubtaskStatus(ctx, r.ID, st.ID, st.RowVersion, "done", &evidenceID)
		return err
	})
	if st.Status != "done" || st.CompletedAt == nil || st.EvidenceFileID == nil {
		t.Fatalf("subtask after done = %+v, want status=done with completed_at and evidence_file_id set", st)
	}

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.UpdateSubtaskStatus(ctx, r.ID, st.ID, st.RowVersion, "open", nil); !errors.Is(err, dsarservice.ErrInvalidTransition) {
			t.Errorf("done -> open: err = %v, want ErrInvalidTransition (terminal)", err)
		}
		return nil
	})
}

// TestUpdateSubtaskStatus_RestrictedToAssignee is "แก้ได้เฉพาะงานที่ได้รับมอบหมาย": a caller holding only
// dsar.subtask.update (not .execute) may change a subtask assigned to them, directly or via their group, but
// not one assigned to someone else.
func TestUpdateSubtaskStatus_RestrictedToAssignee(t *testing.T) {
	e, _ := subtaskEnv(t, "dsarsubtaskaccess")
	restricted := []string{"dsar.request.read", "dsar.subtask.read", "dsar.subtask.update"}
	var r dsarservice.Request
	var owner, other, groupMember, group uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		owner, other, groupMember = seedUser(t, ctx, e), seedUser(t, ctx, e), seedUser(t, ctx, e)
		group = seedGroup(t, ctx, groupMember)
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-access@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	var userSt, groupSt dsarservice.Subtask
	e.in(t, func(ctx context.Context) error {
		var err error
		userSt, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "search", AssigneeUserID: &owner})
		if err != nil {
			return err
		}
		groupSt, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "export", AssigneeGroupID: &group})
		return err
	})

	// The assigned user may move their own subtask.
	e.inAs(t, restricted, func(ctx context.Context) error {
		ctx = withUser(ctx, owner)
		_, err := e.svc.UpdateSubtaskStatus(ctx, r.ID, userSt.ID, userSt.RowVersion, "in_progress", nil)
		return err
	})
	// Someone else, without .execute, may not.
	e.inAs(t, restricted, func(ctx context.Context) error {
		ctx = withUser(ctx, other)
		if _, err := e.svc.UpdateSubtaskStatus(ctx, r.ID, groupSt.ID, groupSt.RowVersion, "in_progress", nil); !errors.Is(err, dsarservice.ErrForbidden) {
			t.Errorf("unassigned caller: err = %v, want ErrForbidden", err)
		}
		return nil
	})
	// A member of the assigned group may.
	e.inAs(t, restricted, func(ctx context.Context) error {
		ctx = withUser(ctx, groupMember)
		_, err := e.svc.UpdateSubtaskStatus(ctx, r.ID, groupSt.ID, groupSt.RowVersion, "in_progress", nil)
		return err
	})
	// DPO/PRIVACY-level (.execute) may change any subtask regardless of assignee.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.UpdateSubtaskStatus(ctx, r.ID, userSt.ID, 2, "done", nil)
		return err
	})
}

// TestTransition_RequiresAllSubtasksDone is DSAR-08's own acceptance criterion: a request with any subtask
// still open or in_progress cannot be completed; it closes once every subtask is done. A request with no
// subtasks at all is unaffected (DSAR-13's own completed tests never created any).
func TestTransition_RequiresAllSubtasksDone(t *testing.T) {
	e, _ := subtaskEnv(t, "dsarsubtaskgate")
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-gate@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		r = bringToInProgress(t, ctx, e, r)
		return nil
	})

	var st dsarservice.Subtask
	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "search"})
		return err
	})

	fulfilled := "fulfilled"
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &fulfilled}); !errors.Is(err, dsarservice.ErrInvalidTransition) {
			t.Errorf("complete with an open subtask: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.UpdateSubtaskStatus(ctx, r.ID, st.ID, st.RowVersion, "done", nil)
		return err
	})

	var closed dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		closed, _, err = e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &fulfilled})
		return err
	})
	if closed.Status != "completed" {
		t.Fatalf("status after every subtask is done = %q, want completed", closed.Status)
	}
}

// TestDeleteSubtask removes a subtask and reopens the close gate.
func TestDeleteSubtask(t *testing.T) {
	e, _ := subtaskEnv(t, "dsarsubtaskdelete")
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-delete@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	var st dsarservice.Subtask
	e.in(t, func(ctx context.Context) error {
		var err error
		st, err = e.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "review"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteSubtask(ctx, r.ID, st.ID); err != nil {
			return err
		}
		list, err := e.svc.ListSubtasks(ctx, r.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("subtasks after delete = %d, want 0", len(list))
		}
		if err := e.svc.DeleteSubtask(ctx, r.ID, st.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("deleting again: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestListSubtasks_TwoTenantIsolation proves RLS, not application code, keeps one tenant's subtasks from
// another's — the same pattern every other repository's isolation test in this codebase follows (rule 1).
func TestListSubtasks_TwoTenantIsolation(t *testing.T) {
	a, _ := subtaskEnv(t, "dsarsubtaskisoa")
	b, _ := subtaskEnv(t, "dsarsubtaskisob")
	var r dsarservice.Request
	a.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, a)
		var err error
		r, err = a.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "subtask-iso@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		_, err = a.svc.CreateSubtask(ctx, r.ID, dsarservice.CreateSubtaskInput{Action: "search"})
		return err
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListSubtasks(ctx, r.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("tenant B listing tenant A's request's subtasks: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
