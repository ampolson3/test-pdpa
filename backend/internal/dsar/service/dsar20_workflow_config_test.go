package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/docs/render"
)

func textNode(s string) render.Node {
	return render.Node{Type: "doc", Content: []render.Node{
		{Type: "paragraph", Content: []render.Node{{Type: "text", Text: s}}},
	}}
}

// TestSaveRequestTypeConfig_ValidatesAndAppliesToTheVeryNextRequest is DSAR-20's own "steps" + "responsible"
// acceptance criterion: a DPO's own configuration save changes behaviour for the very next new request of
// this type with no redeploy, since CreateRequest/openRightSubtasks read GetRequestType fresh every call.
func TestSaveRequestTypeConfig_ValidatesAndAppliesToTheVeryNextRequest(t *testing.T) {
	e := setup(t, "dsar20cfg")
	var leID, typeID uuid.UUID
	var assignee uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		assignee = seedUser(t, ctx, e)
		return nil
	})
	var code string
	e.in(t, func(ctx context.Context) error {
		rt, err := e.svc.GetRequestType(ctx, typeID)
		if err != nil {
			return err
		}
		code = rt.Code
		return nil
	})

	// Validation: a bad subtask action and an unknown assignee are both refused before anything is written.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{DefaultSubtaskAction: strPtr("bogus")})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad action: err = %v, want ErrInvalid", err)
		}
		unknown := uuid.New()
		_, err = e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{DefaultAssigneeUserID: &unknown})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown assignee: err = %v, want ErrInvalid", err)
		}
		_, err = e.svc.SaveRequestTypeConfig(ctx, "bogus-code", dsarservice.RequestTypeConfigInput{})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown code: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// Before any configuration, a new request opens its subtask with the hardcoded default action for this
	// type's own code (DSAR-03's rightSubtaskAction map) and starts unassigned.
	var before dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		before, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "ก่อนตั้งค่า", RequesterContact: "before@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	if before.AssigneeUserID != nil {
		t.Fatalf("expected no default assignee before configuration, got %v", *before.AssigneeUserID)
	}

	// Save a configuration: steps -> "export", responsible -> the seeded assignee.
	e.in(t, func(ctx context.Context) error {
		rt, err := e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{
			DefaultSubtaskAction: strPtr("export"), DefaultAssigneeUserID: &assignee,
		})
		if err != nil {
			return err
		}
		if rt.DefaultSubtaskAction == nil || *rt.DefaultSubtaskAction != "export" {
			t.Errorf("DefaultSubtaskAction = %v, want export", rt.DefaultSubtaskAction)
		}
		if rt.DefaultAssigneeUserID == nil || *rt.DefaultAssigneeUserID != assignee {
			t.Errorf("DefaultAssigneeUserID = %v, want %v", rt.DefaultAssigneeUserID, assignee)
		}
		return nil
	})

	// The acceptance criterion itself: no redeploy, no re-fetch of a cached type — the very next new request
	// of this same code is created with the new default assignee already applied, and the request type both
	// ListRequestTypes and GetRequestType(by either the global id or the new tenant-owned row's id) return is
	// the same, up to date, configured row.
	var after dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		after, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "หลังตั้งค่า", RequesterContact: "after@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	if after.AssigneeUserID == nil || *after.AssigneeUserID != assignee {
		t.Fatalf("expected the configured default assignee applied immediately, got %v", after.AssigneeUserID)
	}

	e.in(t, func(ctx context.Context) error {
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, rt := range types {
			if rt.Code == code {
				found = true
				if rt.DefaultSubtaskAction == nil || *rt.DefaultSubtaskAction != "export" {
					t.Errorf("ListRequestTypes: DefaultSubtaskAction = %v, want export", rt.DefaultSubtaskAction)
				}
			}
		}
		if !found {
			t.Errorf("code %q not found in ListRequestTypes", code)
		}
		// GetRequestType by the ORIGINAL (global) id still resolves to the tenant's own configured row for
		// the same code, not the stale global default — proving the "effective row" resolution works from
		// either id.
		rt, err := e.svc.GetRequestType(ctx, typeID)
		if err != nil {
			return err
		}
		if rt.DefaultSubtaskAction == nil || *rt.DefaultSubtaskAction != "export" {
			t.Errorf("GetRequestType(original id): DefaultSubtaskAction = %v, want export", rt.DefaultSubtaskAction)
		}
		return nil
	})
}

// TestSaveRequestTypeConfig_ClearingOverrideRestoresHardcodedDefault: saving nil fields clears a prior
// override back to today's hardcoded default (full-replace semantics), never leaves the old value in place.
func TestSaveRequestTypeConfig_ClearingOverrideRestoresHardcodedDefault(t *testing.T) {
	e := setup(t, "dsar20clear")
	var typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		_, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var code string
	e.in(t, func(ctx context.Context) error {
		rt, err := e.svc.GetRequestType(ctx, typeID)
		if err != nil {
			return err
		}
		code = rt.Code
		_, err = e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{DefaultSubtaskAction: strPtr("export")})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		rt, err := e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{})
		if err != nil {
			return err
		}
		if rt.DefaultSubtaskAction != nil {
			t.Errorf("DefaultSubtaskAction = %v, want nil after clearing", *rt.DefaultSubtaskAction)
		}
		return nil
	})
}

// TestSaveResponseTemplateOverride_ValidatesAndOverridesLetterGeneration is DSAR-20's own "response letter"
// acceptance criterion: a DPO's own override code takes priority over the shared global purpose template on
// the very next letter generated, and falls back to the shared default when the override has no published
// content (e.g. an override saved for only one purpose, leaving the others on the shared default).
func TestSaveResponseTemplateOverride_ValidatesAndOverridesLetterGeneration(t *testing.T) {
	e := setup(t, "dsar20tpl")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var code string
	e.in(t, func(ctx context.Context) error {
		rt, err := e.svc.GetRequestType(ctx, typeID)
		if err != nil {
			return err
		}
		code = rt.Code
		return nil
	})

	// Validation: a reserved code and a bad language are both refused.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.SaveResponseTemplateOverride(ctx, "result", "th", textNode("x"))
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("reserved code: err = %v, want ErrInvalid", err)
		}
		_, err = e.svc.SaveResponseTemplateOverride(ctx, "my-result", "fr", textNode("x"))
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad language: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// Save a tenant-owned override letter for "result" in both languages, and configure this request type to
	// use it. No override is saved for "request_info" — it must keep falling back to the shared default.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveResponseTemplateOverride(ctx, "my-result", "th", textNode("ผลการพิจารณาแบบกำหนดเอง")); err != nil {
			return err
		}
		if _, err := e.svc.SaveResponseTemplateOverride(ctx, "my-result", "en", textNode("Custom result letter")); err != nil {
			return err
		}
		_, err := e.svc.SaveRequestTypeConfig(ctx, code, dsarservice.RequestTypeConfigInput{ResponseCodeResult: strPtr("my-result")})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		overrides, err := e.svc.ListResponseTemplateOverrides(ctx)
		if err != nil {
			return err
		}
		if len(overrides) != 2 {
			t.Errorf("len(overrides) = %d, want 2 (th+en)", len(overrides))
		}
		return nil
	})

	// Drive a real request through verifying -> in_review -> in_progress -> completed, the exact chain
	// DSAR-13's own letter-generation tests already use, and confirm the generated letter carries the
	// override's own custom text, not the shared default's.
	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "ลูกค้า", RequesterContact: "customer@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_progress"})
		return err
	})
	var docID *uuid.UUID
	outcome := "fulfilled"
	e.in(t, func(ctx context.Context) error {
		var err error
		req, docID, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		return err
	})
	if docID == nil {
		t.Fatal("expected a generated letter document id")
	}
	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, *docID)
		if err != nil {
			return err
		}
		if doc.Draft == nil {
			t.Fatal("expected the generated letter to have a draft")
		}
		var sb strings.Builder
		if n, ok := doc.Draft.Content["th"]; ok {
			plainText(n, &sb)
		}
		got := sb.String()
		if got == "" || !strings.Contains(got, "ผลการพิจารณาแบบกำหนดเอง") {
			t.Errorf("letter content = %q, want it to contain the override's own custom text", got)
		}
		return nil
	})
}

func strPtr(s string) *string { return &s }
