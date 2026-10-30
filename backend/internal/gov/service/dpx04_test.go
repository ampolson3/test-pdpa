package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// fakeDpo is DPX-04's own test double for govservice.Dpo — the gov package's own govEnv (govSetup) has no
// real dpo/service wired at all (the same reasoning fakeFiles already stands in for platform/files), so this
// hands back a real uuid.UUID per call without ever touching a real dpo.tasks row.
type fakeDpo struct {
	calls int
}

func (f *fakeDpo) OpenAuditFindingTask(ctx context.Context, findingID uuid.UUID, title, description string,
	assigneeUserID *uuid.UUID, dueAt *time.Time, priority string) (uuid.UUID, error) {
	f.calls++
	return uuid.New(), nil
}

// dpx04Fixture runs an audit that opens exactly one finding, returning its id and current row_version.
func dpx04Fixture(t *testing.T, e govEnv, le orgservice.LegalEntity) (findingID uuid.UUID, rowVersion int32) {
	t.Helper()
	answers := allYesAnswers()
	answers["dsar_sla_tracked"] = "no"
	e.dpx03In(t, func(ctx context.Context) error {
		a, err := e.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: le.ID, Period: "2026-Q4", TemplateCode: "org_maturity", Answers: answers})
		if err != nil {
			return err
		}
		if len(a.Findings) != 1 {
			t.Fatalf("expected exactly 1 finding, got %d", len(a.Findings))
		}
		findingID = a.Findings[0].ID
		rowVersion = a.Findings[0].RowVersion
		return nil
	})
	return
}

// TestAttachFindingEvidence_RequiredBeforeClosing is DPX-04's own acceptance criterion directly:
// "ข้อตรวจพบปิดได้เมื่อแนบหลักฐาน" — closing (done) is refused until evidence is on record, then succeeds.
func TestAttachFindingEvidence_RequiredBeforeClosing(t *testing.T) {
	e := dpx03Setup(t, "dpx04close")
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPX-04 จำกัด", IsController: true}, 0)
		return err
	})
	findingID, version := dpx04Fixture(t, e, le)

	e.dpx03In(t, func(ctx context.Context) error {
		if _, err := e.svc.SetAuditFindingStatus(ctx, findingID, version, "done"); !errors.Is(err, govservice.ErrEvidenceRequired) {
			t.Errorf("close without evidence: %v, want ErrEvidenceRequired", err)
		}
		return nil
	})

	fileID := uuid.New()
	e.files.addClean(fileID)
	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.AttachFindingEvidence(ctx, findingID, version, fileID)
		if err != nil {
			return err
		}
		if f.EvidenceFileID == nil || *f.EvidenceFileID != fileID {
			t.Errorf("evidence_file_id = %v, want %v", f.EvidenceFileID, fileID)
		}
		version = f.RowVersion
		return nil
	})

	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.SetAuditFindingStatus(ctx, findingID, version, "done")
		if err != nil {
			t.Fatalf("close with evidence attached: %v", err)
		}
		if f.Status != "done" {
			t.Errorf("status = %q, want done", f.Status)
		}
		return nil
	})
}

// TestAttachFindingEvidence_RefusesUnusableFile: a file that isn't the caller's own clean, still-unattached
// upload is refused, the same rule every other PLT-09 evidence attachment in this codebase follows.
func TestAttachFindingEvidence_RefusesUnusableFile(t *testing.T) {
	e := dpx03Setup(t, "dpx04badfile")
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPX-04 จำกัด", IsController: true}, 0)
		return err
	})
	findingID, version := dpx04Fixture(t, e, le)

	e.dpx03In(t, func(ctx context.Context) error {
		if _, err := e.svc.AttachFindingEvidence(ctx, findingID, version, uuid.New()); !errors.Is(err, govservice.ErrFileNotUsable) {
			t.Errorf("unknown file: %v, want ErrFileNotUsable", err)
		}
		return nil
	})

	alreadyAttached := uuid.New()
	e.files.addClean(alreadyAttached)
	e.files.attached[alreadyAttached] = "something_else"
	e.dpx03In(t, func(ctx context.Context) error {
		if _, err := e.svc.AttachFindingEvidence(ctx, findingID, version, alreadyAttached); !errors.Is(err, govservice.ErrFileNotUsable) {
			t.Errorf("already-attached file: %v, want ErrFileNotUsable", err)
		}
		return nil
	})
}

// TestSetAuditFindingStatus_AcceptedNeedsApprovePermission: "accepted" is a permanent decision, gated on
// dpx.audit.approve beyond the endpoint's own dpx.audit.update — the same reasoning VEN-14/ORG-06/DPIA-03
// already use for reusing a stronger permission.
func TestSetAuditFindingStatus_AcceptedNeedsApprovePermission(t *testing.T) {
	e := dpx03Setup(t, "dpx04approve")
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPX-04 จำกัด", IsController: true}, 0)
		return err
	})
	findingID, version := dpx04Fixture(t, e, le)
	fileID := uuid.New()
	e.files.addClean(fileID)
	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.AttachFindingEvidence(ctx, findingID, version, fileID)
		if err != nil {
			return err
		}
		version = f.RowVersion
		return nil
	})

	limitedPerms := []string{"dpx.audit.read", "dpx.audit.update"}
	if err := pdbWithTenant(e, limitedPerms, func(ctx context.Context) error {
		if _, err := e.svc.SetAuditFindingStatus(ctx, findingID, version, "accepted"); !errors.Is(err, govservice.ErrForbidden) {
			t.Errorf("accepted without dpx.audit.approve: %v, want ErrForbidden", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.SetAuditFindingStatus(ctx, findingID, version, "accepted")
		if err != nil {
			t.Fatalf("accepted with dpx.audit.approve: %v", err)
		}
		if f.Status != "accepted" {
			t.Errorf("status = %q, want accepted", f.Status)
		}
		return nil
	})
}

// TestAssignFindingTask_OpensOneTaskAndDedupes is the other half of this feature's own description
// ("มอบหมายงานตามข้อตรวจพบ"): a finding gets a real linked dpo.tasks job, and a repeat call never opens a
// second one — the same DPIA-08/RRA-08/RTG-10 dedup discipline.
func TestAssignFindingTask_OpensOneTaskAndDedupes(t *testing.T) {
	e := dpx03Setup(t, "dpx04assign")
	fd := &fakeDpo{}
	e.svc.Dpo = fd
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPX-04 จำกัด", IsController: true}, 0)
		return err
	})
	findingID, version := dpx04Fixture(t, e, le)

	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.AssignFindingTask(ctx, findingID, version, govservice.AssignFindingTaskInput{Priority: "high"})
		if err != nil {
			return err
		}
		if f.TaskID == nil {
			t.Fatal("expected a linked task_id")
		}
		version = f.RowVersion
		return nil
	})
	if fd.calls != 1 {
		t.Fatalf("OpenAuditFindingTask called %d times, want 1", fd.calls)
	}

	e.dpx03In(t, func(ctx context.Context) error {
		f, err := e.svc.AssignFindingTask(ctx, findingID, version, govservice.AssignFindingTaskInput{Priority: "urgent"})
		if err != nil {
			return err
		}
		if f.TaskID == nil {
			t.Fatal("expected the same linked task_id on a repeat call")
		}
		return nil
	})
	if fd.calls != 1 {
		t.Errorf("OpenAuditFindingTask called %d times on a repeat assign, want still 1 (deduped)", fd.calls)
	}
}

// TestAttachFindingEvidence_TwoTenantIsolation: a finding created in tenant A can't be touched by tenant B.
func TestAttachFindingEvidence_TwoTenantIsolation(t *testing.T) {
	a := dpx03Setup(t, "dpx04isoa")
	b := dpx03Setup(t, "dpx04isob")
	var le orgservice.LegalEntity
	a.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท A จำกัด", IsController: true}, 0)
		return err
	})
	findingID, version := dpx04Fixture(t, a, le)

	fileID := uuid.New()
	b.files.addClean(fileID)
	b.dpx03In(t, func(ctx context.Context) error {
		if _, err := b.svc.AttachFindingEvidence(ctx, findingID, version, fileID); !errors.Is(err, govservice.ErrVersionMismatch) && !errors.Is(err, govservice.ErrNotFound) {
			t.Errorf("tenant B attaching to tenant A's finding: %v, want ErrVersionMismatch or ErrNotFound (RLS)", err)
		}
		return nil
	})
}

// pdbWithTenant runs fn with the given permission set against the same tenant/user dpx03In already uses —
// a small helper so TestSetAuditFindingStatus_AcceptedNeedsApprovePermission can exercise a limited grant
// without its own bespoke setup.
func pdbWithTenant(e govEnv, perms []string, fn func(ctx context.Context) error) error {
	return pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	})
}
