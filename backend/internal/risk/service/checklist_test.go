package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/forms"
	riskservice "pdpa-platform/internal/risk/service"
)

func scorePtr(v float64) *float64 { return &v }

// checklistSetup wires scoreSetup's own real Ropa/Org adapter (so ActivityVisible is a real RLS check,
// not a stub) with a Forms engine too, and grants the checklist type's own permissions
// (ropa.risk.create/update/approve/read).
func checklistSetup(t *testing.T, suffix string) scoreEnv {
	t.Helper()
	e := scoreSetup(t, suffix)
	e.svc.Forms = &forms.Service{Audit: e.svc.Audit}
	e.svc.Forms.Register("checklist", forms.Policy{Read: "ropa.risk.read", Create: "ropa.risk.create",
		Update: "ropa.risk.approve", Publish: "ropa.risk.approve", Respond: "ropa.risk.update"})
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.activity_checklists`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	return e
}

func (e scoreEnv) inChecklist(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{
			"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "ropa.risk.approve",
			"org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create",
		}}))
	}); err != nil {
		t.Fatal(err)
	}
}

// publishedChecklistForm publishes a minimal two-question checklist of the "checklist" type.
func publishedChecklistForm(t *testing.T, ctx context.Context, e scoreEnv) uuid.UUID {
	t.Helper()
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "controls",
		Title: forms.Text{"th": "คำถาม", "en": "Questions"}, Questions: []forms.Question{
			{Key: "has_dpia", Type: forms.TypeYesNo, Label: forms.Text{"th": "ทำ DPIA แล้วหรือยัง", "en": "DPIA done?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: scorePtr(1)}, {Value: "no", Score: scorePtr(0)}}},
		}}}},
		Scoring: &forms.Scoring{Bands: []forms.Band{{Key: "incomplete", Label: forms.Text{"th": "ยังไม่ครบ"}, Min: 0, Max: scorePtr(0)},
			{Key: "complete", Label: forms.Text{"th": "ครบ"}, Min: 1}}}}
	form, err := e.svc.Forms.CreateForm(ctx, "checklist_"+uuid.NewString()[:8], "Activity checklist", "checklist", d)
	if err != nil {
		t.Fatal(err)
	}
	form, err = e.svc.Forms.Publish(ctx, form.ID, form.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	return form.ID
}

// TestAnswerChecklist_StoresAnswersAgainstTheActivity is RRA-05's own acceptance criterion:
// "คำถามและคำตอบถูกเก็บในกิจกรรม" — the submission's own entityType/entityID point at the activity, and
// a later read (ListActivityChecklists/GetActivityChecklist) returns exactly what was answered.
func TestAnswerChecklist_StoresAnswersAgainstTheActivity(t *testing.T) {
	e := checklistSetup(t, "rra05")
	var activityID uuid.UUID
	var formID uuid.UUID
	e.inChecklist(t, func(ctx context.Context) error {
		activityID = seedActivity(t, ctx, e)
		formID = publishedChecklistForm(t, ctx, e)
		return nil
	})

	var run riskservice.ActivityChecklist
	e.inChecklist(t, func(ctx context.Context) error {
		var err error
		run, err = e.svc.AnswerChecklist(ctx, activityID, formID, forms.Answers{"has_dpia": "yes"})
		if err != nil {
			t.Fatalf("answer checklist: %v", err)
		}
		if run.ActivityID != activityID {
			t.Errorf("activity_id = %v, want %v", run.ActivityID, activityID)
		}
		if run.Result != "complete" || run.Score != 1 {
			t.Errorf("result/score = %q/%v", run.Result, run.Score)
		}
		if len(run.Factors) != 1 || run.Factors[0].Question != "has_dpia" || run.Factors[0].Answer != "yes" {
			t.Errorf("factors not stored as answered: %+v", run.Factors)
		}
		return nil
	})

	e.inChecklist(t, func(ctx context.Context) error {
		list, err := e.svc.ListActivityChecklists(ctx, activityID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 || list[0].ID != run.ID {
			t.Fatalf("list should contain exactly the run just recorded: %+v", list)
		}
		got, err := e.svc.GetActivityChecklist(ctx, run.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Result != "complete" {
			t.Errorf("get result = %q", got.Result)
		}
		return nil
	})
}

// A form that isn't the published "checklist" type is refused.
func TestAnswerChecklist_RefusesNonChecklistOrUnpublishedForm(t *testing.T) {
	e := checklistSetup(t, "rra05bad")
	var activityID uuid.UUID
	e.inChecklist(t, func(ctx context.Context) error {
		activityID = seedActivity(t, ctx, e)
		return nil
	})
	e.inChecklist(t, func(ctx context.Context) error {
		if _, err := e.svc.AnswerChecklist(ctx, activityID, uuid.New(), forms.Answers{}); !errors.Is(err, riskservice.ErrBadChecklistForm) {
			t.Errorf("unknown form id: %v, want ErrBadChecklistForm", err)
		}
		return nil
	})
}

// An unknown activity id is refused before any form lookup.
func TestAnswerChecklist_UnknownActivity(t *testing.T) {
	e := checklistSetup(t, "rra05unk")
	e.inChecklist(t, func(ctx context.Context) error {
		if _, err := e.svc.AnswerChecklist(ctx, uuid.New(), uuid.New(), forms.Answers{}); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown activity: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// Two-tenant isolation: tenant B can never list or answer against tenant A's own activity.
func TestAnswerChecklist_TwoTenantIsolation(t *testing.T) {
	a := checklistSetup(t, "rra05a")
	b := checklistSetup(t, "rra05b")

	var activityID uuid.UUID
	a.inChecklist(t, func(ctx context.Context) error {
		activityID = seedActivity(t, ctx, a)
		return nil
	})

	b.inChecklist(t, func(ctx context.Context) error {
		if _, err := b.svc.AnswerChecklist(ctx, activityID, uuid.New(), forms.Answers{}); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("tenant B should not see tenant A's activity: %v", err)
		}
		if _, err := b.svc.ListActivityChecklists(ctx, activityID); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("tenant B should not list tenant A's activity checklists: %v", err)
		}
		return nil
	})
}
