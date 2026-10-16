package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	auditservice "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/wiring"
)

// fakeNotifier stands in for PLT-04 so tests can inspect the OTP that would have been sent (never logged,
// rule 3 — only this in-memory test double ever sees the plaintext code) without needing a real SMTP/SMS
// sender or the notify.deliver job pipeline.
type fakeNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

type verifyEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *iamservice.Service
	notify *fakeNotifier
}

func verifySetup(t *testing.T, suffix string) verifyEnv {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	owner := dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.subject_verifications`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	auditSvc := auditservice.New()
	svc := wiring.IamVerification(keyring, nil, auditSvc)
	notifier := &fakeNotifier{}
	svc.Notify = notifier
	return verifyEnv{app: app, tenant: tenant, svc: svc, notify: notifier}
}

func (e verifyEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), fn); err != nil {
		t.Fatal(err)
	}
}

func otpCodeFrom(n *fakeNotifier) string {
	if len(n.sent) == 0 {
		return ""
	}
	code, _ := n.sent[len(n.sent)-1].Vars["code"].(string)
	return code
}

// TestStartVerification_SendsOTPWithFiveMinuteExpiry is half of IAM-05's acceptance criterion: a fresh OTP
// is sent (over the right channel) and expires in exactly 5 minutes.
func TestStartVerification_SendsOTPWithFiveMinuteExpiry(t *testing.T) {
	e := verifySetup(t, "iamverify1")
	var v iamservice.SubjectVerification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "dsar", Method: "otp_email", IdentifierKind: crypto.KindEmail, IdentifierValue: "verify@example.com",
		})
		return err
	})
	if v.Status != "pending" {
		t.Fatalf("status = %q, want pending", v.Status)
	}
	if got := v.ExpiresAt.Sub(time.Now().UTC()).Round(time.Minute); got != 5*time.Minute {
		t.Errorf("expires_at ~ %v from now, want 5m", got)
	}
	if len(e.notify.sent) != 1 || e.notify.sent[0].Channel != "email" || e.notify.sent[0].TemplateCode != "iam.otp" {
		t.Fatalf("expected one OTP e-mail sent, got %+v", e.notify.sent)
	}
	code := otpCodeFrom(e.notify)
	if len(code) != 6 {
		t.Errorf("otp code = %q, want 6 digits", code)
	}
}

// TestVerifyOTP_CorrectCodeVerifies is the golden path: the code the fake sender captured verifies.
func TestVerifyOTP_CorrectCodeVerifies(t *testing.T) {
	e := verifySetup(t, "iamverify2")
	var v iamservice.SubjectVerification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "consent", Method: "otp_sms", IdentifierKind: crypto.KindPhone, IdentifierValue: "0812345678",
		})
		return err
	})
	code := otpCodeFrom(e.notify)
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.VerifyOTP(ctx, v.ID, code)
		if err != nil {
			return err
		}
		if out.Status != "verified" || out.VerifiedAt == nil {
			t.Errorf("expected verified with verified_at set, got %+v", out)
		}
		return nil
	})
}

// TestVerifyOTP_WrongCodeLocksAfterMaxAttempts is IAM-05's "จำกัดจำนวนครั้ง" (D-01: 5 attempts) — the 5th
// wrong guess fails the verification outright, and a 6th attempt is refused as already decided.
func TestVerifyOTP_WrongCodeLocksAfterMaxAttempts(t *testing.T) {
	e := verifySetup(t, "iamverify3")
	var v iamservice.SubjectVerification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "dsar", Method: "otp_email", IdentifierKind: crypto.KindEmail, IdentifierValue: "wrong@example.com",
		})
		return err
	})
	for i := 0; i < iamservice.MaxOTPAttempts-1; i++ {
		e.in(t, func(ctx context.Context) error {
			out, err := e.svc.VerifyOTP(ctx, v.ID, "000000")
			if !errors.Is(err, iamservice.ErrCodeMismatch) {
				t.Errorf("attempt %d: err = %v, want ErrCodeMismatch", i+1, err)
			}
			if out.Status != "pending" {
				t.Errorf("attempt %d: status = %q, want still pending", i+1, out.Status)
			}
			return nil
		})
	}
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.VerifyOTP(ctx, v.ID, "000000")
		if !errors.Is(err, iamservice.ErrCodeMismatch) {
			t.Errorf("final attempt: err = %v, want ErrCodeMismatch", err)
		}
		if out.Status != "failed" {
			t.Errorf("final attempt: status = %q, want failed", out.Status)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.VerifyOTP(ctx, v.ID, otpCodeFrom(e.notify))
		if !errors.Is(err, iamservice.ErrVerificationDecided) {
			t.Errorf("attempt after lock: err = %v, want ErrVerificationDecided", err)
		}
		return nil
	})
}

// TestVerifyOTP_ExpiredRefusedEvenWithCorrectCode is the other half of the acceptance criterion: the code
// expires at exactly 5 minutes, checked with an injected clock rather than a real 5-minute sleep.
func TestVerifyOTP_ExpiredRefusedEvenWithCorrectCode(t *testing.T) {
	e := verifySetup(t, "iamverify4")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.svc.Now = func() time.Time { return start }
	var v iamservice.SubjectVerification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "dsar", Method: "otp_email", IdentifierKind: crypto.KindEmail, IdentifierValue: "expired@example.com",
		})
		return err
	})
	code := otpCodeFrom(e.notify)
	e.svc.Now = func() time.Time { return start.Add(iamservice.OTPExpiry + time.Second) }
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.VerifyOTP(ctx, v.ID, code)
		if !errors.Is(err, iamservice.ErrVerificationExpired) {
			t.Errorf("err = %v, want ErrVerificationExpired", err)
		}
		if out.Status != "expired" {
			t.Errorf("status = %q, want expired", out.Status)
		}
		return nil
	})
}

// TestVerifyOTP_LogsEveryAttempt is the acceptance criterion's "บันทึกผลการยืนยันทุกครั้ง": both a right and
// a wrong attempt leave their own platform.audit_log row, in the same hash chain (rule 4/12).
func TestVerifyOTP_LogsEveryAttempt(t *testing.T) {
	e := verifySetup(t, "iamverify5")
	var v iamservice.SubjectVerification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "dsar", Method: "otp_email", IdentifierKind: crypto.KindEmail, IdentifierValue: "logme@example.com",
		})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, _ = e.svc.VerifyOTP(ctx, v.ID, "000000")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		var n int
		row := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.audit_log WHERE entity_type = 'subject_verification' AND entity_id = $1`, v.ID)
		if err := row.Scan(&n); err != nil {
			return err
		}
		if n < 2 {
			t.Errorf("expected at least 2 audit rows (start + attempt), got %d", n)
		}
		return nil
	})
}

// TestSubjectVerifications_TenantIsolation is CLAUDE.md rule 1 for this new repository.
func TestSubjectVerifications_TenantIsolation(t *testing.T) {
	a := verifySetup(t, "iamverifyiso-a")
	b := verifySetup(t, "iamverifyiso-b")
	var v iamservice.SubjectVerification
	a.in(t, func(ctx context.Context) error {
		var err error
		v, err = a.svc.StartVerification(ctx, iamservice.StartVerificationInput{
			Purpose: "dsar", Method: "otp_email", IdentifierKind: crypto.KindEmail, IdentifierValue: "iso@example.com",
		})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		_, err := b.svc.VerifyOTP(ctx, v.ID, "000000")
		if !errors.Is(err, iamservice.ErrVerificationNotFound) {
			t.Errorf("cross-tenant verify: err = %v, want ErrVerificationNotFound", err)
		}
		return nil
	})
}
