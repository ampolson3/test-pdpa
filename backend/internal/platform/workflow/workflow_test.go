package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/bizcal"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/workflow"
)

func bkk(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, bizcal.Bangkok)
}

type fixture struct {
	app               *pgxpool.Pool
	a, b              dbtest.Tenant
	alice, bob, carol uuid.UUID // a.UserID is alice; bob escalates; carol is in the Privacy group
	group             uuid.UUID
	svc               *workflow.Service
	now               time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t)}
	owner, platform := dbtest.OwnerPool(t), dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "wf-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "wf-b")
	f.alice = f.a.UserID
	org := &orgservice.Service{}
	err := pdb.WithTenantTx(ctx, f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, f.alice); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'bob@wf.example', 'Bob', 'active') RETURNING id`, f.a.ID).Scan(&f.bob); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'carol@wf.example', 'Carol', 'active') RETURNING id`, f.a.ID).Scan(&f.carol); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO iam.groups (tenant_id, name) VALUES ($1, 'Privacy') RETURNING id`, f.a.ID).Scan(&f.group); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO iam.group_members (group_id, user_id, tenant_id) VALUES ($1, $2, $3)`, f.group, f.carol, f.a.ID); err != nil {
			return err
		}
		// The tenant's calendar: Mon–Fri with Songkran 2026 off (ORG-20).
		cal, err := org.CreateCalendar(ctx, orgservice.CalendarInput{Name: "HQ", Timezone: "Asia/Bangkok", Workdays: []int{1, 2, 3, 4, 5}})
		if err != nil {
			return err
		}
		for _, d := range []int{13, 14, 15} {
			if _, err := org.PutHoliday(ctx, cal.ID, time.Date(2026, 4, d, 0, 0, 0, 0, time.UTC), "สงกรานต์"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				for _, q := range []string{`DELETE FROM platform.sla_timers`, `DELETE FROM platform.workflow_tasks`, `DELETE FROM platform.workflow_instances`,
					`DELETE FROM platform.workflow_definitions WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.notifications`,
					`DELETE FROM org.holidays`, `DELETE FROM org.org_settings`, `DELETE FROM org.business_calendars`,
					`DELETE FROM iam.group_members`, `DELETE FROM iam.groups`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`} {
					if _, err := tx.Exec(ctx, q); err != nil {
						t.Logf("cleanup %q: %v", q, err)
					}
				}
				_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id <> $1`, tenant.UserID)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	client, _ := jobs.NewInsertClient(f.app)
	f.svc = &workflow.Service{Calendars: org, Notify: &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client},
		River: client, Audit: audit.New(), Now: func() time.Time { return f.now }}
	return f
}

// as runs fn in one transaction of tenant a as user (uuid.Nil = the worker, no user).
func (f *fixture) as(t *testing.T, user uuid.UUID, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	uid := ""
	if user != uuid.Nil {
		uid = user.String()
	}
	err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), uid, func(ctx context.Context) error {
		if user != uuid.Nil {
			ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.a.ID.String(), UserID: uid, Permissions: perms})
		}
		return fn(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// count counts rows of tenant a (as the owner, so RLS is on but every column is readable).
func (f *fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	f.as(t, uuid.Nil, nil, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, sql, args...).Scan(&n)
	})
	return n
}

func (f *fixture) definition() workflow.Definition {
	return workflow.Definition{
		Initial: "review",
		States: []workflow.State{
			{Key: "review", Label: workflow.Text{"th": "ตรวจสอบ", "en": "Review"}, Task: &workflow.TaskSpec{Title: workflow.Text{"th": "ตรวจคำขอ"}, AssigneeUserID: &f.alice}},
			{Key: "awaiting_info", Label: workflow.Text{"th": "รอข้อมูล"}, PauseSLA: true},
			{Key: "approval", Label: workflow.Text{"th": "อนุมัติ"}, Task: &workflow.TaskSpec{Title: workflow.Text{"th": "อนุมัติคำตอบ"}, AssigneeGroupID: &f.group}},
			{Key: "done", Label: workflow.Text{"th": "เสร็จ"}, Terminal: true},
		},
		Transitions: []workflow.Transition{{From: "review", To: "awaiting_info"}, {From: "awaiting_info", To: "review"}, {From: "review", To: "approval"}, {From: "approval", To: "done"}},
		SLA:         &workflow.SLASpec{Code: "response", Duration: workflow.Duration{Mode: workflow.ModeBusinessDays, Amount: 5}, RemindBefore: []int{2}, EscalateUserIDs: []uuid.UUID{f.bob}},
	}
}

var admin = []string{"admin.workflow.read", "admin.workflow.create", "admin.workflow.update"}

func TestDefinitions_VersionsAndReferences(t *testing.T) {
	f := setup(t)
	f.as(t, f.alice, admin, func(ctx context.Context) error {
		in := workflow.DefinitionInput{Code: "test_request", Name: "คำขอทดสอบ", EntityType: "test_record", Definition: f.definition()}
		v1, err := f.svc.CreateDefinition(ctx, in)
		if err != nil || v1.Version != 1 || !v1.Active {
			t.Fatalf("create: %+v %v", v1, err)
		}
		if _, err := f.svc.CreateDefinition(ctx, in); !errors.Is(err, workflow.ErrInvalidRequest) {
			t.Errorf("create existing code: %v", err)
		}
		ghost := uuid.New()
		bad := f.definition()
		bad.States[0].Task.AssigneeUserID = &ghost
		if _, err := f.svc.SaveVersion(ctx, v1.ID, v1.RowVersion, workflow.DefinitionInput{Name: "x", EntityType: "test_record", Definition: bad}); !errors.Is(err, workflow.ErrInvalidDefinition) {
			t.Errorf("unknown assignee: %v", err)
		}
		cal := uuid.New()
		bad = f.definition()
		bad.SLA.CalendarID = &cal
		if _, err := f.svc.SaveVersion(ctx, v1.ID, v1.RowVersion, workflow.DefinitionInput{Name: "x", EntityType: "test_record", Definition: bad}); !errors.Is(err, workflow.ErrInvalidDefinition) {
			t.Errorf("unknown calendar: %v", err)
		}
		in.Name = "คำขอทดสอบ v2"
		v2, err := f.svc.SaveVersion(ctx, v1.ID, v1.RowVersion, in)
		if err != nil || v2.Version != 2 {
			t.Fatalf("save version: %+v %v", v2, err)
		}
		if _, err := f.svc.SaveVersion(ctx, v1.ID, v1.RowVersion+1, in); !errors.Is(err, workflow.ErrVersionMismatch) {
			t.Errorf("saving over an old version: %v", err)
		}
		list, err := f.svc.ListDefinitions(ctx)
		if err != nil || len(list) != 1 || list[0].ID != v2.ID {
			t.Errorf("list shows only the newest version: %+v %v", list, err)
		}
		return nil
	})
}

// Acceptance (PLT-05): the SLA counts across Songkran on the tenant's calendar, reminds before the due
// time as configured, and escalates when it passes.
func TestSLA_RemindersAndBreachAcrossHolidays(t *testing.T) {
	f := setup(t)
	record := uuid.New()
	var inst workflow.Instance
	f.now = bkk(2026, 4, 9, 9) // Thursday before Songkran
	f.as(t, f.alice, admin, func(ctx context.Context) error {
		if _, err := f.svc.CreateDefinition(ctx, workflow.DefinitionInput{Code: "test_request", Name: "คำขอทดสอบ", EntityType: "test_record", Definition: f.definition()}); err != nil {
			return err
		}
		if _, err := f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "other", EntityID: record}); !errors.Is(err, workflow.ErrInvalidRequest) {
			t.Errorf("wrong entity type: %v", err)
		}
		var err error
		inst, err = f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "test_record", EntityID: record})
		return err
	})
	var due, remind time.Time
	var timer uuid.UUID
	f.as(t, uuid.Nil, nil, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT id, due_at, (reminders->0->>'at')::timestamptz FROM platform.sla_timers WHERE instance_id = $1`, inst.ID).Scan(&timer, &due, &remind)
	})
	// 5 business days from Thu 9 Apr: Fri 10, Thu 16, Fri 17, Mon 20, Tue 21 (weekend + Songkran skipped).
	if want := bkk(2026, 4, 21, 9); !due.Equal(want) {
		t.Fatalf("due %v, want %v", due.In(bizcal.Bangkok), want)
	}
	// Reminded 2 business days before: Fri 17 April.
	if want := bkk(2026, 4, 17, 9); !remind.Equal(want) {
		t.Fatalf("reminder %v, want %v", remind.In(bizcal.Bangkok), want)
	}
	if n := f.count(t, `SELECT count(*) FROM river_job WHERE kind = 'workflow.sla_tick' AND args->>'timer_id' = $1 AND scheduled_at = $2`, timer.String(), remind); n != 1 {
		t.Errorf("tick scheduled for the reminder: %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE t.code = 'workflow.task_assigned' AND n.recipient_user_id = $1`, f.alice); n != 0 {
		t.Errorf("the starter assigned herself: no notification expected, got %d", n)
	}

	tick := func(at time.Time) {
		f.now = at
		f.as(t, uuid.Nil, nil, func(ctx context.Context) error { return f.svc.Tick(ctx, timer) })
	}
	sent := func(code string, user uuid.UUID) int {
		return f.count(t, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE t.code = $1 AND n.recipient_user_id = $2`, code, user)
	}
	status := func() string {
		var s string
		f.as(t, uuid.Nil, nil, func(ctx context.Context) error {
			return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT sla_status FROM platform.workflow_instances WHERE id = $1`, inst.ID).Scan(&s)
		})
		return s
	}

	tick(bkk(2026, 4, 17, 8)) // an hour early: nothing yet
	if sent("workflow.sla_reminder", f.alice) != 0 || status() != "on_track" {
		t.Error("reminder sent early")
	}
	tick(bkk(2026, 4, 17, 9))
	if sent("workflow.sla_reminder", f.alice) != 1 || status() != "at_risk" {
		t.Errorf("reminder at its time: sent %d, status %s", sent("workflow.sla_reminder", f.alice), status())
	}
	tick(bkk(2026, 4, 17, 10)) // a duplicate tick sends nothing twice
	if sent("workflow.sla_reminder", f.alice) != 1 {
		t.Error("reminder sent twice")
	}
	if n := f.count(t, `SELECT count(*) FROM river_job WHERE kind = 'workflow.sla_tick' AND args->>'timer_id' = $1 AND scheduled_at = $2`, timer.String(), due); n < 1 {
		t.Error("tick not scheduled for the due time")
	}
	tick(bkk(2026, 4, 21, 9))
	if sent("workflow.sla_breached", f.bob) != 1 || sent("workflow.sla_breached", f.alice) != 1 || status() != "overdue" {
		t.Errorf("breach: bob %d alice %d status %s", sent("workflow.sla_breached", f.bob), sent("workflow.sla_breached", f.alice), status())
	}
	if n := f.count(t, `SELECT count(*) FROM platform.sla_timers WHERE id = $1 AND status = 'breached' AND escalated_at IS NOT NULL`, timer); n != 1 {
		t.Error("timer not marked breached")
	}
	if n := f.count(t, `SELECT count(*) FROM platform.audit_log WHERE entity_id = $1 AND action IN ('platform.workflow.sla_reminder', 'platform.workflow.sla_breached')`, inst.ID); n != 2 {
		t.Errorf("SLA audit rows: %d", n)
	}
}

// Pausing (awaiting_info, decisions Q-06) stops the clock; resuming pushes the due time back by the
// business days the pause covered — the weekend and Songkran cost nothing.
func TestSLA_PauseAndResume(t *testing.T) {
	f := setup(t)
	var inst workflow.Instance
	f.now = bkk(2026, 4, 9, 9)
	f.as(t, f.alice, admin, func(ctx context.Context) error {
		if _, err := f.svc.CreateDefinition(ctx, workflow.DefinitionInput{Code: "test_request", Name: "คำขอทดสอบ", EntityType: "test_record", Definition: f.definition()}); err != nil {
			return err
		}
		var err error
		inst, err = f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "test_record", EntityID: uuid.New()})
		return err
	})
	f.now = bkk(2026, 4, 10, 10)
	f.as(t, f.alice, nil, func(ctx context.Context) error {
		var err error
		inst, err = f.svc.Transition(ctx, inst.ID, inst.RowVersion, "awaiting_info", "ขอสำเนาบัตร")
		if err == nil && inst.SLAStatus != "paused" {
			t.Errorf("status %s, want paused", inst.SLAStatus)
		}
		return err
	})
	var timer uuid.UUID
	f.as(t, uuid.Nil, nil, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT id FROM platform.sla_timers WHERE instance_id = $1`, inst.ID).Scan(&timer)
	})
	f.now = bkk(2026, 4, 30, 9) // long past the old due time, but paused: no breach
	f.as(t, uuid.Nil, nil, func(ctx context.Context) error { return f.svc.Tick(ctx, timer) })
	if n := f.count(t, `SELECT count(*) FROM platform.sla_timers WHERE id = $1 AND status = 'running'`, timer); n != 1 {
		t.Fatal("a paused timer was breached")
	}
	// The awaiting_info state has no task, so only the record's writers can move it on.
	f.now = bkk(2026, 4, 16, 10)
	f.as(t, f.alice, nil, func(ctx context.Context) error {
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "review", ""); !errors.Is(err, workflow.ErrNotFound) && !errors.Is(err, workflow.ErrForbidden) {
			t.Errorf("no task and no permission: %v", err)
		}
		return nil
	})
	f.svc.Register("test_record", workflow.Policy{ReadPermission: "x.record.read", WritePermission: "x.record.update"})
	f.as(t, f.bob, []string{"x.record.update"}, func(ctx context.Context) error {
		var err error
		inst, err = f.svc.Transition(ctx, inst.ID, inst.RowVersion, "review", "ได้รับเอกสารแล้ว")
		return err
	})
	var due time.Time
	f.as(t, uuid.Nil, nil, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT due_at FROM platform.sla_timers WHERE id = $1 AND paused_at IS NULL`, timer).Scan(&due)
	})
	// Paused Fri 10 → resumed Thu 16: one business day (the 16th) → due Tue 21 + 1 = Wed 22 April.
	if want := bkk(2026, 4, 22, 9); !due.Equal(want) || inst.SLAStatus != "on_track" {
		t.Errorf("due after resume %v (%s), want %v on_track", due.In(bizcal.Bangkok), inst.SLAStatus, want)
	}
}

func TestTransitions_TasksAndAccess(t *testing.T) {
	f := setup(t)
	record := uuid.New()
	var inst workflow.Instance
	f.now = bkk(2026, 4, 1, 9)
	f.as(t, f.alice, admin, func(ctx context.Context) error {
		if _, err := f.svc.CreateDefinition(ctx, workflow.DefinitionInput{Code: "test_request", Name: "คำขอทดสอบ", EntityType: "test_record", Definition: f.definition()}); err != nil {
			return err
		}
		var err error
		inst, err = f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "test_record", EntityID: record})
		return err
	})
	// Carol is not involved yet and the record type grants nothing: she cannot see it.
	f.as(t, f.carol, nil, func(ctx context.Context) error {
		if _, err := f.svc.Get(ctx, inst.ID); !errors.Is(err, workflow.ErrNotFound) {
			t.Errorf("uninvolved user sees the instance: %v", err)
		}
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "approval", ""); !errors.Is(err, workflow.ErrNotFound) {
			t.Errorf("uninvolved user moves it: %v", err)
		}
		return nil
	})
	f.as(t, f.alice, nil, func(ctx context.Context) error {
		v, err := f.svc.Get(ctx, inst.ID)
		if err != nil || len(v.Transitions) != 2 || len(v.Tasks) != 1 || v.Tasks[0].AssigneeName != "Alice" || len(v.Timers) != 1 {
			t.Errorf("assignee view: %+v %v", v, err)
		}
		mine, err := f.svc.MyTasks(ctx)
		if err != nil || len(mine) != 1 || mine[0].Title["th"] != "ตรวจคำขอ" || mine[0].SLADueAt == nil {
			t.Errorf("my tasks: %+v %v", mine, err)
		}
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "done", ""); !errors.Is(err, workflow.ErrInvalidTransition) {
			t.Errorf("review → done is not a transition: %v", err)
		}
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion+5, "approval", ""); !errors.Is(err, workflow.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		inst, err = f.svc.Transition(ctx, inst.ID, inst.RowVersion, "approval", "ถูกต้องครบถ้วน")
		return err
	})
	if n := f.count(t, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE t.code = 'workflow.task_assigned' AND n.recipient_user_id = $1`, f.carol); n != 1 {
		t.Errorf("group member notified of the group task: %d", n)
	}
	f.as(t, f.alice, nil, func(ctx context.Context) error {
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "done", ""); !errors.Is(err, workflow.ErrForbidden) {
			t.Errorf("alice's task is done; approval is the group's: %v", err)
		}
		return nil
	})
	f.as(t, f.carol, nil, func(ctx context.Context) error {
		mine, err := f.svc.MyTasks(ctx)
		if err != nil || len(mine) != 1 || mine[0].GroupName != "Privacy" || mine[0].AssigneeUserID != nil {
			t.Fatalf("group task on carol's board: %+v %v", mine, err)
		}
		if _, err := f.svc.UpdateTask(ctx, mine[0].ID, mine[0].RowVersion, workflow.TaskChange{AssigneeUserID: &f.bob}); !errors.Is(err, workflow.ErrForbidden) {
			t.Errorf("a member assigns someone else: %v", err)
		}
		task, err := f.svc.UpdateTask(ctx, mine[0].ID, mine[0].RowVersion, workflow.TaskChange{AssigneeUserID: &f.carol, Status: "in_progress"})
		if err != nil || task.AssigneeName != "Carol" || task.Status != "in_progress" {
			t.Fatalf("claim: %+v %v", task, err)
		}
		inst, err = f.svc.Transition(ctx, inst.ID, inst.RowVersion, "done", "อนุมัติ")
		if err != nil || inst.CompletedAt == nil || inst.SLAStatus != "done" {
			t.Fatalf("complete: %+v %v", inst, err)
		}
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "review", ""); !errors.Is(err, workflow.ErrInvalidTransition) {
			t.Errorf("moving a finished instance: %v", err)
		}
		v, err := f.svc.Get(ctx, inst.ID)
		if err != nil || len(v.Transitions) != 0 || v.Timers[0].Status != "met" || len(v.History) != 4 {
			t.Errorf("finished view: transitions %d, timer %+v, history %d, %v", len(v.Transitions), v.Timers, len(v.History), err)
		}
		for _, task := range v.Tasks {
			if task.Status != "done" {
				t.Errorf("task %s left %s", task.State, task.Status)
			}
		}
		return nil
	})
	f.as(t, f.alice, nil, func(ctx context.Context) error {
		if mine, _ := f.svc.MyTasks(ctx); len(mine) != 0 {
			t.Errorf("finished work on the board: %d", len(mine))
		}
		return nil
	})
}

// Tenant B sees nothing of tenant A's workflows (CLAUDE.md rule 1).
func TestTenantIsolation(t *testing.T) {
	f := setup(t)
	var inst workflow.Instance
	f.now = bkk(2026, 4, 1, 9)
	f.as(t, f.alice, admin, func(ctx context.Context) error {
		if _, err := f.svc.CreateDefinition(ctx, workflow.DefinitionInput{Code: "test_request", Name: "คำขอทดสอบ", EntityType: "test_record", Definition: f.definition()}); err != nil {
			return err
		}
		var err error
		inst, err = f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "test_record", EntityID: uuid.New()})
		return err
	})
	f.svc.Register("test_record", workflow.Policy{ReadPermission: "x.record.read", WritePermission: "x.record.update"})
	err := pdb.WithTenantTx(context.Background(), f.app, f.b.ID.String(), f.b.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.b.ID.String(), UserID: f.b.UserID.String(), Permissions: append([]string{"x.record.read", "x.record.update"}, admin...)})
		if list, _ := f.svc.ListDefinitions(ctx); len(list) != 0 {
			t.Errorf("B lists A's definitions: %d", len(list))
		}
		if _, err := f.svc.Start(ctx, workflow.StartInput{DefinitionCode: "test_request", EntityType: "test_record", EntityID: uuid.New()}); !errors.Is(err, workflow.ErrNotFound) {
			t.Errorf("B starts A's workflow: %v", err)
		}
		if _, err := f.svc.Get(ctx, inst.ID); !errors.Is(err, workflow.ErrNotFound) {
			t.Errorf("B reads A's instance: %v", err)
		}
		if _, err := f.svc.Transition(ctx, inst.ID, inst.RowVersion, "approval", ""); !errors.Is(err, workflow.ErrNotFound) {
			t.Errorf("B moves A's instance: %v", err)
		}
		if mine, _ := f.svc.MyTasks(ctx); len(mine) != 0 {
			t.Errorf("B's board shows A's tasks")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
