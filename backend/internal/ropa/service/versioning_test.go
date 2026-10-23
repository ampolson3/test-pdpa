package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// completeActivity builds a controller-role activity with every ม.39 core item in place (data, purpose,
// retention, rights_and_access, security control), ready for SubmitActivity -> ApproveActivity (ROPA-13).
func completeActivity(t *testing.T, ctx context.Context, e env, code string) ropaservice.Activity {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U-" + code, NameTh: "U", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: code,
		Name: "กิจกรรมทดสอบ", Role: "controller", RightsAndAccess: "ติดต่อ DPO"}, 0)
	if err != nil {
		t.Fatal(err)
	}

	categories, err := e.org.ListMaster(ctx, "data_categories")
	if err != nil {
		t.Fatal(err)
	}
	subjectTypes, err := e.org.ListMaster(ctx, "data_subject_types")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: a.ID, DataCategoryID: *categories[0].ID, SubjectTypeID: *subjectTypes[0].ID, Source: "direct"}); err != nil {
		t.Fatal(err)
	}
	basis := lawfulBasisCode(t, ctx, e.org)
	if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a.ID, PurposeText: "ทดสอบ", LawfulBasisCode: basis}); err != nil {
		t.Fatal(err)
	}
	months := 12
	if _, err := e.svc.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: a.ID, RetentionBasis: "กฎหมาย", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete", RetentionMonths: &months}); err != nil {
		t.Fatal(err)
	}
	controls, err := e.svc.ListControls(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(controls) == 0 {
		t.Fatal("expected at least one seeded security control (migration 00040)")
	}
	if _, err := e.svc.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: a.ID, ControlID: controls[0].ID}); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.GetActivity(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Completeness != 100 || len(got.MissingItems) != 0 {
		t.Fatalf("expected a fully complete activity, got completeness=%d missing=%v", got.Completeness, got.MissingItems)
	}
	return got
}

// TestActivityApproval_Lifecycle is the acceptance criterion end to end (ROPA-13, ST-05): submit ->
// approve freezes version 1; reopen for review -> edit -> submit -> approve freezes version 2; the two
// versions diff on exactly the field that changed. An approved activity can't be edited directly.
func TestActivityApproval_Lifecycle(t *testing.T) {
	e := setup(t, "ropaver")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e, "VER-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		if submitted.Status != "pending_approval" {
			t.Errorf("expected pending_approval, got %s", submitted.Status)
		}
		a = submitted
		return nil
	})

	// Direct edits are refused while pending_approval.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: a.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
			Code: a.Code, Name: "x", Role: a.Role, RightsAndAccess: a.RightsAndAccess}, a.RowVersion); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("edit while pending_approval: %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		approved, err := e.svc.ApproveActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		if approved.Status != "active" {
			t.Errorf("expected active, got %s", approved.Status)
		}
		a = approved
		return nil
	})

	// Approving freezes exactly one version.
	e.in(t, func(ctx context.Context) error {
		versions, err := e.svc.ListActivityVersions(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(versions) != 1 || versions[0].VersionNo != 1 {
			t.Fatalf("expected exactly one version (no. 1), got %+v", versions)
		}
		if versions[0].Snapshot.Name != "กิจกรรมทดสอบ" {
			t.Errorf("snapshot name mismatch: %+v", versions[0].Snapshot)
		}
		return nil
	})

	// An active activity can't be edited directly.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: a.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
			Code: a.Code, Name: "x", Role: a.Role, RightsAndAccess: a.RightsAndAccess}, a.RowVersion); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("direct edit of an active activity: %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	// Reopen for review, edit, re-submit, re-approve -> a second version.
	e.in(t, func(ctx context.Context) error {
		reopened, err := e.svc.RequestActivityReview(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		if reopened.Status != "under_review" {
			t.Errorf("expected under_review, got %s", reopened.Status)
		}
		a = reopened
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		edited, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: a.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
			Code: a.Code, Name: "กิจกรรมทดสอบ (แก้ไข)", Role: a.Role, RightsAndAccess: a.RightsAndAccess}, a.RowVersion)
		if err != nil {
			return err
		}
		a = edited
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = submitted
		approved, err := e.svc.ApproveActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = approved
		return nil
	})

	var v1, v2 string
	e.in(t, func(ctx context.Context) error {
		versions, err := e.svc.ListActivityVersions(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(versions) != 2 {
			t.Fatalf("expected two versions after the second approval, got %d", len(versions))
		}
		// Newest first.
		if versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
			t.Fatalf("expected version_no 2 then 1, got %+v", versions)
		}
		v1, v2 = versions[1].ID.String(), versions[0].ID.String()
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		from, err := uuid.Parse(v1)
		if err != nil {
			return err
		}
		to, err := uuid.Parse(v2)
		if err != nil {
			return err
		}
		diffs, err := e.svc.CompareActivityVersions(ctx, from, to)
		if err != nil {
			return err
		}
		found := false
		for _, d := range diffs {
			if d.Section == "name" {
				found = true
				if d.Before != "กิจกรรมทดสอบ" || d.After != "กิจกรรมทดสอบ (แก้ไข)" {
					t.Errorf("name diff mismatch: before=%v after=%v", d.Before, d.After)
				}
			}
		}
		if !found {
			t.Errorf("expected a diff on the name section, got %+v", diffs)
		}
		return nil
	})
}

// TestActivityApproval_ConfirmNoChange closes a review round without taking a new version — the
// acceptance criterion's "ยืนยันไม่เปลี่ยน" path.
func TestActivityApproval_ConfirmNoChange(t *testing.T) {
	e := setup(t, "ropaverconfirm")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		a = completeActivity(t, ctx, e, "VER-02")
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		approved, err := e.svc.ApproveActivity(ctx, submitted.ID, submitted.RowVersion)
		if err != nil {
			return err
		}
		a = approved
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		reopened, err := e.svc.RequestActivityReview(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		confirmed, err := e.svc.ConfirmActivityNoChange(ctx, reopened.ID, reopened.RowVersion)
		if err != nil {
			return err
		}
		if confirmed.Status != "active" {
			t.Errorf("expected active after confirm-no-change, got %s", confirmed.Status)
		}
		versions, err := e.svc.ListActivityVersions(ctx, confirmed.ID)
		if err != nil {
			return err
		}
		if len(versions) != 1 {
			t.Errorf("confirm-no-change should not take a new version, got %d", len(versions))
		}
		return nil
	})
}

// TestActivityApproval_Return sends a pending activity back to draft with no version taken.
func TestActivityApproval_Return(t *testing.T) {
	e := setup(t, "ropaverreturn")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		a = completeActivity(t, ctx, e, "VER-03")
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = submitted
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		returned, err := e.svc.ReturnActivity(ctx, a.ID, a.RowVersion, "ข้อมูลไม่ครบ")
		if err != nil {
			return err
		}
		if returned.Status != "draft" {
			t.Errorf("expected draft after return, got %s", returned.Status)
		}
		versions, err := e.svc.ListActivityVersions(ctx, returned.ID)
		if err != nil {
			return err
		}
		if len(versions) != 0 {
			t.Errorf("a returned activity should have no version yet, got %d", len(versions))
		}
		// Now editable again (back in draft).
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: returned.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
			Code: a.Code, Name: "x", Role: a.Role, RightsAndAccess: a.RightsAndAccess}, returned.RowVersion); err != nil {
			t.Errorf("draft should be editable again: %v", err)
		}
		return nil
	})
}

// TestActivityApproval_InvalidTransitions exercises every transition's status guard and stale-ETag check.
func TestActivityApproval_InvalidTransitions(t *testing.T) {
	e := setup(t, "ropaverinvalid")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e, "VER-04"); return nil })

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.ApproveActivity(ctx, a.ID, a.RowVersion); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("approve a draft: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.ReturnActivity(ctx, a.ID, a.RowVersion, ""); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("return a draft: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.RequestActivityReview(ctx, a.ID, a.RowVersion); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("request review on a draft: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.ConfirmActivityNoChange(ctx, a.ID, a.RowVersion); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("confirm-no-change on a draft: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.ApproveActivity(ctx, a.ID, 99); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("stale version on approve: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestActivityVersions_Isolation proves ListActivityVersions/CompareActivityVersions respect RLS — a
// version id belonging to tenant A's activity is invisible to tenant B.
func TestActivityVersions_Isolation(t *testing.T) {
	a := setup(t, "ropaverisoa")
	b := setup(t, "ropaverisob")
	var activity ropaservice.Activity
	var versionID string
	a.in(t, func(ctx context.Context) error {
		activity = completeActivity(t, ctx, a, "VER-05")
		submitted, err := a.svc.SubmitActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		approved, err := a.svc.ApproveActivity(ctx, submitted.ID, submitted.RowVersion)
		if err != nil {
			return err
		}
		versions, err := a.svc.ListActivityVersions(ctx, approved.ID)
		if err != nil {
			return err
		}
		versionID = versions[0].ID.String()
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListActivityVersions(ctx, activity.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B listing tenant A's activity versions: %v, want ErrNotFound", err)
		}
		id, err := uuid.Parse(versionID)
		if err != nil {
			return err
		}
		if _, err := b.svc.CompareActivityVersions(ctx, id, id); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B comparing tenant A's version: %v, want ErrNotFound", err)
		}
		return nil
	})
}
