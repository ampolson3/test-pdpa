package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dsarservice "pdpa-platform/internal/dsar/service"
	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	ropaservice "pdpa-platform/internal/ropa/service"
)

type fakeFiles struct {
	clean    map[uuid.UUID]bool
	attached map[uuid.UUID]string
}

func newFakeFiles() *fakeFiles {
	return &fakeFiles{clean: map[uuid.UUID]bool{}, attached: map[uuid.UUID]string{}}
}

func (f *fakeFiles) addClean(id uuid.UUID) { f.clean[id] = true }

func (f *fakeFiles) Get(ctx context.Context, id uuid.UUID) (files.File, error) {
	if !f.clean[id] {
		return files.File{}, errors.New("not found")
	}
	if _, attached := f.attached[id]; attached {
		return files.File{ID: id, AVStatus: "clean", EntityType: "attached"}, nil
	}
	return files.File{ID: id, AVStatus: "clean"}, nil
}

func (f *fakeFiles) AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	f.attached[id] = entityType
	return nil
}

type govEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *govservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
	dsar   *dsarservice.Service
	files  *fakeFiles
}

func govSetup(t *testing.T, suffix string) govEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM gov.disposal_jobs`, `DELETE FROM gov.retention_schedules`, `DELETE FROM dsar.legal_holds`,
				`DELETE FROM ropa.retention_rules`, `DELETE FROM ropa.processing_activities`, `DELETE FROM ropa.assets`,
				`DELETE FROM org.org_units`, `UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: org}
	dsarSvc := &dsarservice.Service{Audit: audit.New()}
	fFiles := newFakeFiles()
	svc := &govservice.Service{Audit: audit.New(), Ropa: ropaSvc, Dsar: dsarSvc, Files: fFiles}
	return govEnv{app: app, tenant: tenant, svc: svc, org: org, ropa: ropaSvc, dsar: dsarSvc, files: fFiles}
}

var govPerms = []string{"dpx.retention.read", "dpx.retention.create", "dpx.retention.update",
	"dpx.retention.approve", "dpx.retention.execute", "ropa.activity.read", "ropa.activity.create", "ropa.activity.update"}

func (e govEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: govPerms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func seedAssetAndRule(t *testing.T, ctx context.Context, e govEnv) (ropaservice.Asset, ropaservice.RetentionRule) {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := e.ropa.SaveAsset(ctx, ropaservice.Asset{Name: "HR system", AssetType: "application", OrgUnitID: &unit.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-01", Name: "HR records", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: activity.ID, RetentionBasis: "ม.37(3)", TriggerEvent: "สิ้นสุดการจ้าง", DisposalMethod: "delete"})
	if err != nil {
		t.Fatal(err)
	}
	return asset, rule
}

// TestRunRetentionCheck_CreatesDisposalJobOnlyWhenDue is the acceptance criterion's first half:
// a schedule reaching its due date automatically gets a disposal job, and only once.
func TestRunRetentionCheck_CreatesDisposalJobOnlyWhenDue(t *testing.T) {
	e := govSetup(t, "govsweep")
	var scheduleID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, e)
		sch, err := e.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: asset.ID, NextDueAt: time.Now().AddDate(0, 0, -1)})
		if err != nil {
			return err
		}
		scheduleID = sch.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		n, err := e.svc.RunRetentionCheck(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("RunRetentionCheck created %d jobs, want 1", n)
		}
		jobs, err := e.svc.ListJobs(ctx, govservice.JobFilter{})
		if err != nil {
			return err
		}
		if len(jobs) != 1 || jobs[0].ScheduleID != scheduleID || jobs[0].Status != "pending" || jobs[0].Method != "delete" {
			t.Errorf("unexpected jobs: %+v", jobs)
		}
		// Running the sweep again must not create a second job for the same schedule.
		n2, err := e.svc.RunRetentionCheck(ctx)
		if err != nil {
			return err
		}
		if n2 != 0 {
			t.Errorf("RunRetentionCheck re-run created %d jobs, want 0 (already has an open job)", n2)
		}
		return nil
	})
}

// TestRunRetentionCheck_NotYetDue proves a schedule due in the future is left alone.
func TestRunRetentionCheck_NotYetDue(t *testing.T) {
	e := govSetup(t, "govnotdue")
	e.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, e)
		if _, err := e.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: asset.ID, NextDueAt: time.Now().AddDate(0, 0, 30)}); err != nil {
			return err
		}
		n, err := e.svc.RunRetentionCheck(ctx)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("RunRetentionCheck created %d jobs for a not-yet-due schedule, want 0", n)
		}
		return nil
	})
}

// TestDisposalJob_LegalHoldBlocksApproval is the "legal hold suspends disposal" rule: a new job
// against a held asset is created on_hold, and approving it outright is refused.
func TestDisposalJob_LegalHoldBlocksApproval(t *testing.T) {
	e := govSetup(t, "govhold")
	var assetID, jobID uuid.UUID
	var jobRowVersion int32
	e.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, e)
		assetID = asset.ID
		if _, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`INSERT INTO dsar.legal_holds (tenant_id, asset_id, law_ref, hold_rule, is_active) VALUES (current_setting('app.tenant_id')::uuid, $1, 'ม.33', '{}', true)`, assetID); err != nil {
			return err
		}
		if _, err := e.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: assetID, NextDueAt: time.Now().AddDate(0, 0, -1)}); err != nil {
			return err
		}
		if _, err := e.svc.RunRetentionCheck(ctx); err != nil {
			return err
		}
		jobs, err := e.svc.ListJobs(ctx, govservice.JobFilter{})
		if err != nil {
			return err
		}
		if len(jobs) != 1 || jobs[0].Status != "on_hold" || jobs[0].LegalHoldID == nil {
			t.Fatalf("job should be created on_hold while the asset's legal hold is active: %+v", jobs)
		}
		jobID, jobRowVersion = jobs[0].ID, jobs[0].RowVersion
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.ApproveJob(ctx, jobID, jobRowVersion, nil); !errors.Is(err, govservice.ErrOnLegalHold) {
			t.Errorf("approving a held job: %v, want ErrOnLegalHold", err)
		}
		return nil
	})

	// Lift the hold, release, and now approval should succeed.
	e.in(t, func(ctx context.Context) error {
		if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE dsar.legal_holds SET is_active = false WHERE asset_id = $1`, assetID); err != nil {
			return err
		}
		released, err := e.svc.ReleaseHold(ctx, jobID, jobRowVersion)
		if err != nil {
			return err
		}
		if released.Status != "pending" || released.LegalHoldID != nil {
			t.Errorf("release should clear the hold back to pending: %+v", released)
		}
		approved, err := e.svc.ApproveJob(ctx, jobID, released.RowVersion, nil)
		if err != nil {
			t.Errorf("approval after release: %v", err)
		}
		if approved.Status != "approved" || approved.ApprovedBy == nil {
			t.Errorf("unexpected approved job: %+v", approved)
		}
		return nil
	})
}

// TestDisposalJob_CannotCloseWithoutEvidence is the acceptance criterion's second half.
func TestDisposalJob_CannotCloseWithoutEvidence(t *testing.T) {
	e := govSetup(t, "govevidence")
	var jobID uuid.UUID
	var rv int32
	e.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, e)
		if _, err := e.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: asset.ID, NextDueAt: time.Now().AddDate(0, 0, -1)}); err != nil {
			return err
		}
		if _, err := e.svc.RunRetentionCheck(ctx); err != nil {
			return err
		}
		jobs, err := e.svc.ListJobs(ctx, govservice.JobFilter{})
		if err != nil || len(jobs) != 1 {
			t.Fatalf("expected one job, got %v err=%v", jobs, err)
		}
		approved, err := e.svc.ApproveJob(ctx, jobs[0].ID, jobs[0].RowVersion, nil)
		if err != nil {
			return err
		}
		started, err := e.svc.StartJob(ctx, approved.ID, approved.RowVersion)
		if err != nil {
			return err
		}
		jobID, rv = started.ID, started.RowVersion
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CompleteJob(ctx, jobID, rv, uuid.Nil, nil); !errors.Is(err, govservice.ErrEvidenceRequired) {
			t.Errorf("completing without evidence: %v, want ErrEvidenceRequired", err)
		}
		evidenceID := uuid.New()
		e.files.addClean(evidenceID)
		count := int32(42)
		done, err := e.svc.CompleteJob(ctx, jobID, rv, evidenceID, &count)
		if err != nil {
			t.Fatalf("completing with evidence: %v", err)
		}
		if done.Status != "done" || done.EvidenceFileID == nil || *done.EvidenceFileID != evidenceID || done.CompletedAt == nil {
			t.Errorf("unexpected completed job: %+v", done)
		}
		if e.files.attached[evidenceID] != govservice.JobType {
			t.Errorf("evidence file was not attached to the job")
		}
		return nil
	})
}

// TestSchedules_PausedNeverPickedUp proves a paused schedule is skipped by the sweep.
func TestSchedules_PausedNeverPickedUp(t *testing.T) {
	e := govSetup(t, "govpaused")
	e.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, e)
		sch, err := e.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: asset.ID, NextDueAt: time.Now().AddDate(0, 0, -1)})
		if err != nil {
			return err
		}
		if _, err := e.svc.SetScheduleStatus(ctx, sch.ID, sch.RowVersion, "paused"); err != nil {
			return err
		}
		n, err := e.svc.RunRetentionCheck(ctx)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("a paused schedule should never be picked up, got %d jobs", n)
		}
		return nil
	})
}

// TestRunRetentionCheck_TwoTenantIsolation mirrors the rest of this codebase's own isolation tests.
func TestRunRetentionCheck_TwoTenantIsolation(t *testing.T) {
	a := govSetup(t, "goviso1")
	b := govSetup(t, "goviso2")
	var scheduleID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		asset, rule := seedAssetAndRule(t, ctx, a)
		sch, err := a.svc.SaveSchedule(ctx, govservice.RetentionSchedule{RetentionRuleID: rule.ID, AssetID: asset.ID, NextDueAt: time.Now().AddDate(0, 0, -1)})
		scheduleID = sch.ID
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetSchedule(ctx, scheduleID); !errors.Is(err, govservice.ErrNotFound) {
			t.Errorf("tenant B reading tenant A's schedule: %v, want ErrNotFound", err)
		}
		n, err := b.svc.RunRetentionCheck(ctx)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("tenant B's sweep should not see tenant A's schedule, got %d jobs", n)
		}
		return nil
	})
}
