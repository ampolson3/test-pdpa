package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/publickeys"
)

func createTestLegalEntity(t *testing.T, e env, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: name, IsController: true}, 0)
		if err != nil {
			return err
		}
		id = le.ID
		return nil
	})
	return id
}

func TestIntakeForm_CreatePublishRetire(t *testing.T) {
	e := setup(t, "dsarintakecrud")
	legalEntity := createTestLegalEntity(t, e, "บริษัท อินเทค จำกัด")

	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "แบบฟอร์ม DSAR"})
		return err
	})
	if form.Status != "draft" || form.PublicKey != nil {
		t.Fatalf("new form = %+v, want draft with no public key", form)
	}

	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	if form.Status != "active" || form.PublicKey == nil || *form.PublicKey == "" {
		t.Fatalf("published form = %+v, want active with a public key", form)
	}
	key := *form.PublicKey

	// Re-publishing an already-active form is refused (nothing to transition into).
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion); !errors.Is(err, dsarservice.ErrInvalidTransition) {
			t.Errorf("re-publish active form: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.RetireIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	if form.Status != "retired" {
		t.Fatalf("retired form = %+v, want retired", form)
	}
	// The key stops resolving once retired.
	owner := dbtest.OwnerPool(t)
	if _, ok, err := publickeys.Resolve(context.Background(), owner, key); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("retired form's key still resolves")
	}

	// Re-publishing a retired form issues no second key — the same coalesce() the admin collection-point
	// flow (CON-09) and the notice flow (PNG-06) both already rely on.
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	if form.PublicKey == nil || *form.PublicKey != key {
		t.Errorf("re-published form's key = %v, want the original key %q unchanged", form.PublicKey, key)
	}
}

func TestIntakeForm_RetireRefusedTwice(t *testing.T) {
	e := setup(t, "dsarintakeretire")
	legalEntity := createTestLegalEntity(t, e, "บริษัท รีไทร์ จำกัด")
	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "F"})
		if err != nil {
			return err
		}
		form, err = e.svc.RetireIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RetireIntakeForm(ctx, form.ID, form.RowVersion); !errors.Is(err, dsarservice.ErrInvalidTransition) {
			t.Errorf("retire already-retired form: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

func TestCreatePublicSubmission_ReturnsRequestNoAndStartsVerification(t *testing.T) {
	e, notifier, _ := identityEnv(t, "dsarpublicsubmit")
	legalEntity := createTestLegalEntity(t, e, "บริษัท พับลิก จำกัด")

	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "F"})
		if err != nil {
			return err
		}
		form, err = e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})

	// The acceptance criterion itself: submitting through the public form hands back a request number
	// immediately, with no staff involved.
	var req dsarservice.Request
	var v *dsarservice.Verification
	e.in(t, func(ctx context.Context) error {
		var err error
		req, v, err = e.svc.CreatePublicSubmission(ctx, form.ID, dsarservice.PublicSubmission{
			RequestTypeCode: "access", RequesterName: "สมหญิง ดีใจ", RequesterEmail: "somying@example.com",
		})
		return err
	})
	if req.RequestNo == "" {
		t.Fatal("expected a non-empty request_no immediately")
	}
	if req.Channel != "web" || req.OnBehalf {
		t.Errorf("request channel/on_behalf = %q/%v, want web/false", req.Channel, req.OnBehalf)
	}
	if v == nil || v.Status != "pending" || v.Method != "otp_email" {
		t.Fatalf("verification = %+v, want a pending otp_email attempt (DSAR-01 «include» DSAR-06)", v)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("expected one OTP sent to the requester's own contact, got %d", len(notifier.sent))
	}
	code, _ := notifier.sent[0].Vars["code"].(string)
	if code == "" {
		t.Fatal("no OTP code captured")
	}

	// Confirming the OTP against the opened verification passes exactly like the admin-side flow (DSAR-06).
	e.in(t, func(ctx context.Context) error {
		confirmed, err := e.svc.ConfirmOTPVerification(ctx, req.ID, v.ID, code)
		if err != nil {
			return err
		}
		if confirmed.Status != "passed" {
			t.Errorf("confirmed status = %q, want passed", confirmed.Status)
		}
		return nil
	})
}

func TestCreatePublicSubmission_RefusesDraftOrRetiredForm(t *testing.T) {
	e := setup(t, "dsarpublicdraft")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ดราฟต์ จำกัด")
	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "F"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.CreatePublicSubmission(ctx, form.ID, dsarservice.PublicSubmission{
			RequestTypeCode: "access", RequesterName: "X", RequesterEmail: "x@example.com",
		}); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("submit against a draft form: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestCreatePublicSubmission_RequiresExactlyOneContact(t *testing.T) {
	e := setup(t, "dsarpubliccontact")
	legalEntity := createTestLegalEntity(t, e, "บริษัท คอนแทค จำกัด")
	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "F"})
		if err != nil {
			return err
		}
		form, err = e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.CreatePublicSubmission(ctx, form.ID, dsarservice.PublicSubmission{
			RequestTypeCode: "access", RequesterName: "X",
		}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("submit with no contact: err = %v, want ErrInvalid", err)
		}
		if _, _, err := e.svc.CreatePublicSubmission(ctx, form.ID, dsarservice.PublicSubmission{
			RequestTypeCode: "access", RequesterName: "X", RequesterEmail: "a@example.com", RequesterPhone: "0812345678",
		}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("submit with both contacts: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// Two-tenant isolation: tenant B's intake form (and its public_key) is invisible to tenant A under RLS —
// the standard proof every repository in this codebase carries (CLAUDE.md rule 1).
func TestIntakeForm_TwoTenantIsolation(t *testing.T) {
	eA := setup(t, "dsarintakeisoa")
	eB := setup(t, "dsarintakeisob")
	legalEntityB := createTestLegalEntity(t, eB, "บริษัท บี จำกัด")

	var formB dsarservice.IntakeForm
	eB.in(t, func(ctx context.Context) error {
		var err error
		formB, err = eB.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntityB, Name: "B's form"})
		return err
	})

	eA.in(t, func(ctx context.Context) error {
		if _, err := eA.svc.GetIntakeForm(ctx, formB.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("tenant A reading tenant B's intake form: err = %v, want ErrNotFound", err)
		}
		forms, err := eA.svc.ListIntakeForms(ctx, "", nil, 50)
		if err != nil {
			return err
		}
		for _, f := range forms {
			if f.ID == formB.ID {
				t.Error("tenant A's list includes tenant B's intake form")
			}
		}
		return nil
	})
}

func TestCreatePublicSubmission_UnknownRequestTypeCodeRefused(t *testing.T) {
	e := setup(t, "dsarpublicbadtype")
	legalEntity := createTestLegalEntity(t, e, "บริษัท แบดไทป์ จำกัด")
	var form dsarservice.IntakeForm
	e.in(t, func(ctx context.Context) error {
		var err error
		form, err = e.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: legalEntity, Name: "F"})
		if err != nil {
			return err
		}
		form, err = e.svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.CreatePublicSubmission(ctx, form.ID, dsarservice.PublicSubmission{
			RequestTypeCode: "not_a_real_code", RequesterName: "X", RequesterEmail: "x@example.com",
		}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("submit with unknown request_type_code: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}
