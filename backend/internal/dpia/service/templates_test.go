package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// templatePerms extends the base perms with the create/publish/delete codes DPIA-03's own endpoints need.
var templatePerms = append(append([]string{}, perms...), "assessment.template.create", "assessment.template.publish", "assessment.template.delete")

// inAs is env.in with a caller-chosen permission set (DPIA-03's template endpoints need codes the base
// screening tests' fixed `perms` doesn't grant).
func (e env) inAs(t *testing.T, permissions []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: permissions}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testDraft(labelTh string) forms.Draft {
	return forms.Draft{Schema: forms.Schema{Sections: []forms.Section{
		{Key: "s1", Title: forms.Text{"th": labelTh}, Questions: []forms.Question{
			{Key: "q1", Type: forms.TypeYesNo, Label: forms.Text{"th": labelTh}},
		}},
	}}}
}

// draftRowVersion finds the form's own open (unpublished) version and returns its RowVersion — the ETag
// PublishTemplate/forms.Publish check for optimistic concurrency.
func (e env) draftRowVersion(ctx context.Context, t *testing.T, formID uuid.UUID) int32 {
	t.Helper()
	f, err := e.svc.Forms.GetForm(ctx, formID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range f.Versions {
		if v.PublishedAt == nil {
			return v.RowVersion
		}
	}
	t.Fatalf("form %s has no open draft version", formID)
	return 0
}

// TestCreateTemplate_ThenListedByType is DPIA-03's own listing acceptance: a freshly created template shows
// up in the library, filtered by its assessment_type, in draft status.
func TestCreateTemplate_ThenListedByType(t *testing.T) {
	e := setup(t, "dpiatplcreate")
	var id string
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		tpl, err := e.svc.CreateTemplate(ctx, "pia", "pia_create_test", "PIA test", []string{"ม.37"}, testDraft("PIA test"))
		if err != nil {
			return err
		}
		if tpl.Status != "draft" || tpl.AssessmentType != "pia" || tpl.VersionNo != 1 {
			t.Errorf("got %+v", tpl)
		}
		id = tpl.ID.String()
		return nil
	})
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		list, err := e.svc.ListTemplates(ctx, "pia")
		if err != nil {
			return err
		}
		found := false
		for _, tpl := range list {
			if tpl.ID.String() == id {
				found = true
			}
			if tpl.AssessmentType != "pia" {
				t.Errorf("filter leaked a %q template", tpl.AssessmentType)
			}
		}
		if !found {
			t.Errorf("created template %s not in pia list", id)
		}
		return nil
	})
}

// TestCreateTemplate_RejectsUnknownAssessmentType is DPIA-03's own input validation.
func TestCreateTemplate_RejectsUnknownAssessmentType(t *testing.T) {
	e := setup(t, "dpiatplbadtype")
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		if _, err := e.svc.CreateTemplate(ctx, "not_a_type", "x", "x", nil, testDraft("x")); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("got %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestCreateTemplate_RejectsDuplicateCode proves a duplicate code is refused, not left to abort the whole
// request transaction (CLAUDE.md's own Savepoint pattern) — it surfaces as the underlying PLT-06 form's own
// code uniqueness (forms.ErrInvalidRequest), since CreateTemplate always creates a brand-new form with the
// caller's code and that collision is checked first, before assess.templates' own insert is ever attempted.
func TestCreateTemplate_RejectsDuplicateCode(t *testing.T) {
	e := setup(t, "dpiatpldup")
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		if _, err := e.svc.CreateTemplate(ctx, "lia", "dup_code", "First", nil, testDraft("First")); err != nil {
			return err
		}
		if _, err := e.svc.CreateTemplate(ctx, "lia", "dup_code", "Second", nil, testDraft("Second")); !errors.Is(err, forms.ErrInvalidRequest) {
			t.Errorf("duplicate code: %v, want forms.ErrInvalidRequest", err)
		}
		return nil
	})
}

// TestCloneTemplate_IsIndependentOfSource is DPIA-03's core acceptance criterion: cloning copies the
// source's current content into a brand-new form, and publishing the clone (which edits and freezes its own
// draft) never touches the source template's own version or status.
func TestCloneTemplate_IsIndependentOfSource(t *testing.T) {
	e := setup(t, "dpiatplclone")
	var srcID, cloneFormID uuid.UUID
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		src, err := e.svc.CreateTemplate(ctx, "tia", "tia_src", "TIA source", nil, testDraft("Source question"))
		if err != nil {
			return err
		}
		srcID = src.ID

		clone, err := e.svc.CloneTemplate(ctx, src.ID, "tia_clone", "TIA clone")
		if err != nil {
			return err
		}
		if clone.Code != "tia_clone" || clone.FormID == src.FormID {
			t.Errorf("clone should get its own form: %+v vs source form %s", clone, src.FormID)
		}
		cloneFormID = clone.FormID

		// Publish the clone — this freezes a version on the CLONE's own form only.
		rv := e.draftRowVersion(ctx, t, clone.FormID)
		if _, err := e.svc.PublishTemplate(ctx, clone.ID, rv); err != nil {
			return err
		}
		return nil
	})

	e.inAs(t, templatePerms, func(ctx context.Context) error {
		src, err := e.svc.GetTemplateByID(ctx, srcID)
		if err != nil {
			return err
		}
		if src.Status != "draft" {
			t.Errorf("source template status changed to %q after publishing its clone, want draft", src.Status)
		}
		if src.FormID == cloneFormID {
			t.Errorf("source and clone share a form id")
		}
		return nil
	})
}

// TestPublishTemplate_VersionMismatch proves publish relies on the underlying form's own draft-version ETag.
func TestPublishTemplate_VersionMismatch(t *testing.T) {
	e := setup(t, "dpiatplpubver")
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		tpl, err := e.svc.CreateTemplate(ctx, "ai", "ai_pub", "AI test", nil, testDraft("Q"))
		if err != nil {
			return err
		}
		if _, err := e.svc.PublishTemplate(ctx, tpl.ID, 999); !errors.Is(err, forms.ErrVersionMismatch) {
			t.Errorf("got %v, want forms.ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestRetireTemplate_VersionMismatchAndSuccess covers RetireTemplate's own row_version-based optimistic
// concurrency (it has no underlying form ETag to lean on, unlike Publish).
func TestRetireTemplate_VersionMismatchAndSuccess(t *testing.T) {
	e := setup(t, "dpiatplretire")
	e.inAs(t, templatePerms, func(ctx context.Context) error {
		tpl, err := e.svc.CreateTemplate(ctx, "maturity", "maturity_retire", "Maturity test", nil, testDraft("Q"))
		if err != nil {
			return err
		}
		if _, err := e.svc.RetireTemplate(ctx, tpl.ID, tpl.RowVersion+1); !errors.Is(err, dpiaservice.ErrVersionMismatch) {
			t.Errorf("stale row_version: %v, want ErrVersionMismatch", err)
		}
		retired, err := e.svc.RetireTemplate(ctx, tpl.ID, tpl.RowVersion)
		if err != nil {
			return err
		}
		if retired.Status != "retired" {
			t.Errorf("status = %q, want retired", retired.Status)
		}
		return nil
	})
}

// TestTemplates_TwoTenantIsolation proves tenant B can't see or clone tenant A's tenant-scoped template.
func TestTemplates_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiatpliso_A")
	b := setup(t, "dpiatpliso_B")
	var srcID uuid.UUID
	a.inAs(t, templatePerms, func(ctx context.Context) error {
		tpl, err := a.svc.CreateTemplate(ctx, "vendor", "vendor_iso", "Vendor test", nil, testDraft("Q"))
		if err != nil {
			return err
		}
		srcID = tpl.ID
		return nil
	})
	b.inAs(t, templatePerms, func(ctx context.Context) error {
		if _, err := b.svc.GetTemplateByID(ctx, srcID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant GetTemplateByID: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.CloneTemplate(ctx, srcID, "vendor_iso_clone", "clone"); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant CloneTemplate: %v, want ErrNotFound", err)
		}
		return nil
	})
}
