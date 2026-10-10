package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// seedRiskUser is the same pattern dsar/service's own seedUser already uses: dbtest.SeedTenant's tenant
// user defaults to 'invited', which iamservice.Names doesn't count as usable.
func seedRiskUser(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Assignee', 'active') RETURNING id`,
		"assignee-"+uuid.NewString()+"@dbtest.example").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func gapFindingFixture(t *testing.T, e scoreEnv, ctx context.Context) uuid.UUID {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-RT-01",
		Name: "กิจกรรมทดสอบแก้ไขช่องว่าง", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := e.svc.AnalyzeActivity(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.RuleCode == "no_lawful_basis" {
			return f.ID
		}
	}
	t.Fatal("expected a no_lawful_basis finding")
	return uuid.Nil
}

// TestRemediateFinding_OpensLinkedTask is RRA-07's own acceptance criterion's write half: choosing a
// finding, an assignee, a due date and a priority opens a real dpo.tasks job and links it back onto the
// finding's own (until now unused) task_id column.
func TestRemediateFinding_OpensLinkedTask(t *testing.T) {
	e := gapSetup(t, "rra07open", true)
	var findingID uuid.UUID
	var assignee uuid.UUID
	e.in(t, func(ctx context.Context) error {
		findingID = gapFindingFixture(t, e, ctx)
		assignee = seedRiskUser(t, ctx)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		due := time.Now().UTC().AddDate(0, 0, 14)
		f, err := e.svc.RemediateFinding(ctx, findingID, &assignee, &due, "high")
		if err != nil {
			t.Fatal(err)
		}
		if f.TaskID == nil {
			t.Fatal("expected the finding to be linked to a task")
		}
		return nil
	})
}

func TestRemediateFinding_UnknownAssigneeRefused(t *testing.T) {
	e := gapSetup(t, "rra07badassignee", true)
	var findingID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		findingID = gapFindingFixture(t, e, ctx)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		bogus := uuid.New()
		if _, err := e.svc.RemediateFinding(ctx, findingID, &bogus, nil, ""); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown assignee: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestRemediateFinding_InvalidPriorityRefused(t *testing.T) {
	e := gapSetup(t, "rra07badpriority", true)
	var findingID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		findingID = gapFindingFixture(t, e, ctx)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RemediateFinding(ctx, findingID, nil, nil, "urgent-ish"); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("bad priority: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestRemediateFinding_AlreadyResolvedRefused proves only an open finding can be remediated.
func TestRemediateFinding_AlreadyResolvedRefused(t *testing.T) {
	e := gapSetup(t, "rra07resolved", true)
	var findingID uuid.UUID
	var basis string
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "GAP-RT-02",
			Name: "กิจกรรมทดสอบปิดแล้ว", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		basis = nonConsentLawfulBasis(t, ctx, e.org)
		findings, err := e.svc.AnalyzeActivity(ctx, activityID)
		if err != nil {
			return err
		}
		for _, f := range findings {
			if f.RuleCode == "no_lawful_basis" {
				findingID = f.ID
			}
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activityID, PurposeText: "ทดสอบ", LawfulBasisCode: basis})
		if err != nil {
			return err
		}
		if _, err := e.svc.AnalyzeActivity(ctx, activityID); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RemediateFinding(ctx, findingID, nil, nil, ""); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("already-resolved finding: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestRemediateFinding_UnknownFindingRefused(t *testing.T) {
	e := gapSetup(t, "rra07unknown", true)
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RemediateFinding(ctx, uuid.New(), nil, nil, ""); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("unknown finding: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestRemediateFinding_TwoTenantIsolation mirrors AnalyzeActivity's own isolation test.
func TestRemediateFinding_TwoTenantIsolation(t *testing.T) {
	a := gapSetup(t, "rra07iso1", true)
	b := gapSetup(t, "rra07iso2", true)
	var findingID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		findingID = gapFindingFixture(t, a, ctx)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.RemediateFinding(ctx, findingID, nil, nil, ""); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("tenant B remediating tenant A's finding: %v, want ErrNotFound", err)
		}
		return nil
	})
}
