package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
)

// DSAR-19: self-service status tracking + submitting additional documents — reuses identityEnv/
// createTestRequest (DSAR-06's own fixture, an existing request with contact "somchai@example.com").

func trackingEnv(t *testing.T, suffix string) (env, *fakeNotifier, *fakeFiles, dsarservice.Request) {
	t.Helper()
	e, notifier, ff := identityEnv(t, suffix)
	var legalEntity uuid.UUID
	var rt dsarservice.RequestType
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		rt = types[0]
		return nil
	})
	req := createTestRequest(t, e, legalEntity, rt.ID)
	return e, notifier, ff, req
}

func TestTrackingPortal_PublishCoalescesAndRetireIssuesFresh(t *testing.T) {
	e, _, _, _ := trackingEnv(t, "dsartrackportal")
	var legalEntity uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด 2", IsController: true}, 0)
		legalEntity = le.ID
		return err
	})

	var first string
	e.in(t, func(ctx context.Context) error {
		key, _, err := e.svc.GetTrackingPortalLink(ctx, legalEntity)
		if err != nil {
			return err
		}
		if key != "" {
			t.Errorf("link before publish = %q, want empty", key)
		}
		first, err = e.svc.PublishTrackingPortal(ctx, legalEntity)
		return err
	})
	if first == "" {
		t.Fatal("publish returned no key")
	}

	e.in(t, func(ctx context.Context) error {
		again, err := e.svc.PublishTrackingPortal(ctx, legalEntity)
		if err != nil {
			return err
		}
		if again != first {
			t.Errorf("re-publish key = %q, want the same %q (coalesce)", again, first)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.RetireTrackingPortal(ctx, legalEntity)
	})
	e.in(t, func(ctx context.Context) error {
		key, ok, err := e.svc.GetTrackingPortalLink(ctx, legalEntity)
		if err != nil {
			return err
		}
		if ok || key != "" {
			t.Errorf("link after retire = %q, want none", key)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		again, err := e.svc.PublishTrackingPortal(ctx, legalEntity)
		if err != nil {
			return err
		}
		if again == first {
			t.Error("publishing after retire returned the already-revoked key")
		}
		return nil
	})
}

// TestTracking_StartStatusAndSubmitDocument is the acceptance criterion directly: the data subject sees
// the current status and submits an additional document themselves, both gated by one OTP round.
func TestTracking_StartStatusAndSubmitDocument(t *testing.T) {
	e, notifier, ff, req := trackingEnv(t, "dsartrack")

	var ref dsarservice.TrackingRef
	e.in(t, func(ctx context.Context) error {
		var err error
		ref, err = e.svc.StartTrackingVerification(ctx, req.RequestNo, "otp_email", "somchai@example.com")
		return err
	})
	if ref.VerificationID == uuid.Nil || ref.Channel != "email" {
		t.Fatalf("tracking ref = %+v", ref)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("expected one OTP sent, got %d", len(notifier.sent))
	}
	code, _ := notifier.sent[0].Vars["code"].(string)
	if code == "" {
		t.Fatal("no code captured")
	}

	// Before any verification, status is refused.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.GetTrackedStatus(ctx, req.RequestNo, "otp_email", "somchai@example.com", ref.VerificationID, "000000"); err == nil {
			t.Error("wrong code: expected an error")
		}
		return nil
	})

	var status dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		status, err = e.svc.GetTrackedStatus(ctx, req.RequestNo, "otp_email", "somchai@example.com", ref.VerificationID, code)
		return err
	})
	if status.RequestNo != req.RequestNo || status.Status != "received" {
		t.Fatalf("tracked status = %+v", status)
	}

	// A later call in the same session may omit the code (VerifiedIdentifier, not a fresh OTP spend).
	e.in(t, func(ctx context.Context) error {
		again, err := e.svc.GetTrackedStatus(ctx, req.RequestNo, "otp_email", "somchai@example.com", ref.VerificationID, "")
		if err != nil {
			return err
		}
		if again.RequestNo != req.RequestNo {
			t.Errorf("second status call = %+v", again)
		}
		return nil
	})

	var fileID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		fileID, err = e.svc.SubmitTrackingDocument(ctx, req.RequestNo, "otp_email", "somchai@example.com", ref.VerificationID, "",
			"evidence.txt", strings.NewReader("ขอส่งเอกสารเพิ่มเติม"))
		return err
	})
	if fileID == uuid.Nil {
		t.Fatal("no file id returned")
	}
	attached, ok := ff.attached[fileID]
	if !ok {
		t.Fatal("submitted document was never attached")
	}
	if attached.EntityType != "dsar_request" || attached.EntityID == nil || *attached.EntityID != req.ID {
		t.Errorf("submitted document attached to %+v, want dsar_request/%s", attached, req.ID)
	}
}

// TestTracking_UnknownRequestOrContactIsNotFound: an unknown request number and a contact that doesn't
// match the real one both read identically — no information leak either way.
func TestTracking_UnknownRequestOrContactIsNotFound(t *testing.T) {
	e, _, _, req := trackingEnv(t, "dsartracknotfound")

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.StartTrackingVerification(ctx, "DSAR-0000-9999", "otp_email", "somchai@example.com"); !isNotFound(err) {
			t.Errorf("unknown request_no: got %v, want ErrNotFound", err)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.StartTrackingVerification(ctx, req.RequestNo, "otp_email", "someoneelse@example.com"); !isNotFound(err) {
			t.Errorf("mismatched contact: got %v, want ErrNotFound", err)
		}
		return nil
	})
}

func isNotFound(err error) bool {
	return errors.Is(err, dsarservice.ErrNotFound)
}

// TestTracking_TwoTenantIsolation: a request number that happens to exist in another tenant never resolves
// through this tenant's own tracking portal — RLS already scopes the lookup, this proves it end to end.
func TestTracking_TwoTenantIsolation(t *testing.T) {
	_, _, _, req := trackingEnv(t, "dsartrackisoA")
	other := identityEnvOther(t)

	other.in(t, func(ctx context.Context) error {
		if _, err := other.svc.StartTrackingVerification(ctx, req.RequestNo, "otp_email", "somchai@example.com"); !isNotFound(err) {
			t.Errorf("cross-tenant start: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
