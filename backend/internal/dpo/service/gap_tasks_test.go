package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

// taskEnv wires risk + ropa + org + dpo together the same way RRA-04's own gapSetup/scoreSetup does, but
// in the dpo package, since RRA-07's own acceptance criterion needs to drive the loop from this side:
// closing a "ropa_gap" dpo.tasks job must call back into risk.AnalyzeActivity and see the finding clear.
type taskEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	dpo    *dposervice.Service
	risk   *riskservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
}

func taskSetup(t *testing.T, suffix string) taskEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.gap_findings`, `DELETE FROM dpo.tasks`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM iam.users WHERE email LIKE 'rra07-%@dbtest.example'`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := riskservice.New()
	riskSvc.Audit = audit.New()
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	dpoSvc := &dposervice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Dpo = dpoSvc
	return taskEnv{app: app, tenant: tenant, dpo: dpoSvc, risk: riskSvc, org: orgSvc, ropa: ropaSvc}
}

var taskPerms = []string{"ropa.risk.read", "ropa.risk.create", "ropa.risk.update",
	"org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create",
	"dpo.task.read", "dpo.task.update", "dpo.task.execute"}

func (e taskEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	e.inAs(t, e.tenant.UserID, taskPerms, fn)
}

func (e taskEnv) inAs(t *testing.T, userID uuid.UUID, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), userID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: userID.String(), Permissions: perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func seedTaskUser(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Assignee', 'active') RETURNING id`,
		"rra07-"+uuid.NewString()+"@dbtest.example").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// gapTaskFixture creates an activity missing a lawful basis, analyzes it, and opens a remediation task
// against the resulting finding — the common starting point for every UpdateTaskStatus test below.
func gapTaskFixture(t *testing.T, e taskEnv, ctx context.Context, assignee *uuid.UUID) (taskID, activityID, findingID uuid.UUID) {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-TASK-" + uuid.NewString()[:8],
		Name: "กิจกรรมทดสอบงานแก้ไข", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	activityID = a.ID
	findings, err := e.risk.AnalyzeActivity(ctx, activityID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.RuleCode == "no_lawful_basis" {
			findingID = f.ID
		}
	}
	if findingID == uuid.Nil {
		t.Fatal("expected a no_lawful_basis finding")
	}
	updated, err := e.risk.RemediateFinding(ctx, findingID, assignee, nil, "high")
	if err != nil {
		t.Fatal(err)
	}
	if updated.TaskID == nil {
		t.Fatal("expected RemediateFinding to link a task")
	}
	return *updated.TaskID, activityID, findingID
}

// TestUpdateTaskStatus_ValidTransitionsOnly walks the whole created -> assigned -> in_review -> done ->
// closed chain and proves every edge the module doesn't declare is refused.
func TestUpdateTaskStatus_ValidTransitionsOnly(t *testing.T) {
	e := taskSetup(t, "rra07trans")
	var taskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		assignee := seedTaskUser(t, ctx)
		taskID, _, _ = gapTaskFixture(t, e, ctx, &assignee)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		// Already "assigned" (an assignee was given at creation) — "assigned" -> "closed" is not a declared edge.
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "closed", 1); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("assigned -> closed: %v, want ErrInvalid", err)
		}
		task, err := e.dpo.UpdateTaskStatus(ctx, taskID, "in_review", 1)
		if err != nil {
			t.Fatalf("assigned -> in_review: %v", err)
		}
		if task.Status != "in_review" {
			t.Errorf("status = %q, want in_review", task.Status)
		}
		task, err = e.dpo.UpdateTaskStatus(ctx, taskID, "done", task.RowVersion)
		if err != nil {
			t.Fatalf("in_review -> done: %v", err)
		}
		task, err = e.dpo.UpdateTaskStatus(ctx, taskID, "closed", task.RowVersion)
		if err != nil {
			t.Fatalf("done -> closed: %v", err)
		}
		if task.Status != "closed" {
			t.Errorf("status = %q, want closed", task.Status)
		}
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "assigned", task.RowVersion); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("closed -> assigned: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestUpdateTaskStatus_VersionMismatch(t *testing.T) {
	e := taskSetup(t, "rra07ver")
	var taskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		assignee := seedTaskUser(t, ctx)
		taskID, _, _ = gapTaskFixture(t, e, ctx, &assignee)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "in_review", 99); !errors.Is(err, dposervice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestUpdateTaskStatus_AccessRestrictedToAssigneeUnlessExecute is docs/security/permissions.md's own note
// on dpo.task: a caller holding only dpo.task.update may move only a task assigned to them; dpo.task.execute
// bypasses that restriction entirely — the same two-tier shape DSAR-08's own requireSubtaskAccess uses.
func TestUpdateTaskStatus_AccessRestrictedToAssigneeUnlessExecute(t *testing.T) {
	e := taskSetup(t, "rra07access")
	var taskID uuid.UUID
	var assignee, bystander uuid.UUID
	e.in(t, func(ctx context.Context) error {
		assignee = seedTaskUser(t, ctx)
		bystander = seedTaskUser(t, ctx)
		taskID, _, _ = gapTaskFixture(t, e, ctx, &assignee)
		return nil
	})

	limitedPerms := []string{"dpo.task.read", "dpo.task.update"}
	e.inAs(t, bystander, limitedPerms, func(ctx context.Context) error {
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "in_review", 1); !errors.Is(err, dposervice.ErrForbidden) {
			t.Errorf("non-assignee without execute: %v, want ErrForbidden", err)
		}
		return nil
	})
	e.inAs(t, assignee, limitedPerms, func(ctx context.Context) error {
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "in_review", 1); err != nil {
			t.Errorf("the assignee themselves: %v, want success", err)
		}
		return nil
	})
	e.inAs(t, bystander, append(limitedPerms, "dpo.task.execute"), func(ctx context.Context) error {
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "done", 2); err != nil {
			t.Errorf("a caller with dpo.task.execute: %v, want success", err)
		}
		return nil
	})
}

// TestUpdateTaskStatus_ClosingGapTaskReanalyzesAndClears is RRA-07's own acceptance criterion directly:
// "ปิดงานแล้วช่องว่างหายไปเมื่อ rule ผ่าน" — closing the task re-runs the finding's own rule, and the gap
// disappears only because the fix actually landed, not merely because the task was closed.
func TestUpdateTaskStatus_ClosingGapTaskReanalyzesAndClears(t *testing.T) {
	e := taskSetup(t, "rra07close")
	var taskID, activityID uuid.UUID
	var basis string
	e.in(t, func(ctx context.Context) error {
		taskID, activityID, _ = gapTaskFixture(t, e, ctx, nil)
		items, err := e.org.ListMaster(ctx, orgservice.KindLawfulBases)
		if err != nil {
			return err
		}
		for _, it := range items {
			if !it.RequiresConsent {
				basis = it.Code
				break
			}
		}
		if basis == "" {
			t.Fatal("expected at least one non-consent lawful basis to be seeded (ORG-07)")
		}
		return nil
	})

	// Fix the actual gap (add a purpose) before closing — proves the clear happens because the rule now
	// passes, not merely because the task closed.
	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activityID, PurposeText: "ทดสอบ", LawfulBasisCode: basis})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		task, err := e.dpo.UpdateTaskStatus(ctx, taskID, "in_review", 1)
		if err != nil {
			return err
		}
		task, err = e.dpo.UpdateTaskStatus(ctx, taskID, "done", task.RowVersion)
		if err != nil {
			return err
		}
		if _, err := e.dpo.UpdateTaskStatus(ctx, taskID, "closed", task.RowVersion); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		findings, err := e.risk.ListGapFindingsForActivity(ctx, activityID)
		if err != nil {
			return err
		}
		for _, f := range findings {
			if f.RuleCode == "no_lawful_basis" && f.Status != "resolved" {
				t.Errorf("no_lawful_basis finding status = %q, want resolved after closing its remediation task", f.Status)
			}
		}
		return nil
	})
}
