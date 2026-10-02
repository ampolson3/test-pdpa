package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/wiring"
)

// fakeNotifier stands in for PLT-04, the same test double IAM-05's own verification_test.go uses, so the
// OTP code is visible to this test (never logged — rule 3) without a real SMTP/SMS sender.
type fakeNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

// fakeFiles is an in-memory stand-in for PLT-09 (no S3 reachable in this environment) — just enough of
// dsar.Files to exercise SubmitIDDocumentVerification's redact-then-save flow and prove the raw bytes
// handed to Open are never the ones SaveGenerated stores.
type fakeFiles struct {
	raw   map[uuid.UUID][]byte
	saved map[uuid.UUID][]byte
}

func newFakeFiles() *fakeFiles { return &fakeFiles{raw: map[uuid.UUID][]byte{}, saved: map[uuid.UUID][]byte{}} }

func (f *fakeFiles) Get(ctx context.Context, id uuid.UUID) (files.File, error) {
	if _, ok := f.raw[id]; !ok {
		return files.File{}, errors.New("not found")
	}
	return files.File{ID: id}, nil // EntityType "" — unattached, as a fresh raw upload always is
}

func (f *fakeFiles) Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, files.File, error) {
	b, ok := f.raw[id]
	if !ok {
		return nil, files.File{}, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), files.File{ID: id}, nil
}

// SaveGenerated also inserts a minimal real platform.files row (no actual object store here — SeaweedFS
// isn't reachable in this environment) so dsar.verifications.masked_id_file_id's real FK constraint is
// satisfied; the bytes themselves are only ever kept in f.saved for this test to inspect.
func (f *fakeFiles) SaveGenerated(ctx context.Context, fileName string, r io.Reader, entityType string, entityID uuid.UUID) (files.File, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return files.File{}, err
	}
	id := uuid.New()
	f.saved[id] = b
	sum := sha256.Sum256(b)
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx,
		`INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status, entity_type, entity_id)
		 VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, $3, 'image/png', $4, $5, 'clean', $6, $7)`,
		id, id.String(), fileName, len(b), hex.EncodeToString(sum[:]), entityType, entityID); err != nil {
		return files.File{}, err
	}
	return files.File{ID: id, FileName: fileName, EntityType: entityType, EntityID: &entityID}, nil
}

func identityEnv(t *testing.T, suffix string) (env, *fakeNotifier, *fakeFiles) {
	t.Helper()
	e := setup(t, suffix)
	notifier := &fakeNotifier{}
	e.svc.Verification = wiring.IamVerification(e.svc.Keyring, nil, nil)
	e.svc.Verification.Notify = notifier
	ff := newFakeFiles()
	e.svc.Files = ff
	return e, notifier, ff
}

func createTestRequest(t *testing.T, e env, legalEntity uuid.UUID, rtID uuid.UUID) dsarservice.Request {
	t.Helper()
	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: rtID, LegalEntityID: legalEntity, Channel: "web",
			RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail,
		})
		return err
	})
	return req
}

func TestOTPVerification_PassStampsVerifiedAtAndTransitions(t *testing.T) {
	e, notifier, _ := identityEnv(t, "dsaridentotp")
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
	if req.Status != "received" {
		t.Fatalf("new request status = %q, want received", req.Status)
	}

	var v dsarservice.Verification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.StartOTPVerification(ctx, req.ID, "otp_email")
		return err
	})
	if v.Status != "pending" || v.Method != "otp_email" {
		t.Fatalf("started verification = %+v", v)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("expected one OTP sent, got %d", len(notifier.sent))
	}
	code, _ := notifier.sent[0].Vars["code"].(string)
	if code == "" {
		t.Fatal("no code captured")
	}

	e.in(t, func(ctx context.Context) error {
		after, err := e.svc.GetRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if after.Status != "verifying" {
			t.Errorf("after start: status = %q, want verifying", after.Status)
		}
		return nil
	})

	// Wrong code: stays pending, request stays verifying (the acceptance criterion's own retry allowance).
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.ConfirmOTPVerification(ctx, req.ID, v.ID, "000000"); err == nil {
			t.Error("wrong code: expected an error")
		}
		return nil
	})

	var confirmed dsarservice.Verification
	e.in(t, func(ctx context.Context) error {
		var err error
		confirmed, err = e.svc.ConfirmOTPVerification(ctx, req.ID, v.ID, code)
		return err
	})
	if confirmed.Status != "passed" {
		t.Errorf("confirmed status = %q, want passed", confirmed.Status)
	}

	e.in(t, func(ctx context.Context) error {
		after, err := e.svc.GetRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if after.Status != "in_review" {
			t.Errorf("after confirm: status = %q, want in_review", after.Status)
		}
		if after.VerifiedAt == nil {
			t.Error("verified_at not stamped — the acceptance criterion itself")
		}
		return nil
	})
}

func TestIDDocumentVerification_RedactsBeforeStoringAndNeverKeepsTheRaw(t *testing.T) {
	e, _, ff := identityEnv(t, "dsaridentid")
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

	// A 10x10 all-white image with the "number" in the bottom strip (rows 8-9) — the rectangle we redact.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.White)
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	rawID := uuid.New()
	ff.raw[rawID] = raw.Bytes()

	var v dsarservice.Verification
	e.in(t, func(ctx context.Context) error {
		var err error
		v, err = e.svc.SubmitIDDocumentVerification(ctx, req.ID, rawID, []dsarservice.Rect{{X: 0, Y: 8, Width: 10, Height: 2}})
		return err
	})
	if v.Status != "pending" || v.Method != "id_document" || v.MaskedIDFileID == nil {
		t.Fatalf("submitted verification = %+v", v)
	}

	saved, ok := ff.saved[*v.MaskedIDFileID]
	if !ok {
		t.Fatal("masked file was never saved")
	}
	decoded, err := png.Decode(bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := decoded.At(5, 9).RGBA(); r != 0 || g != 0 || b != 0 {
		t.Errorf("redacted pixel at (5,9) = (%d,%d,%d), want black", r, g, b)
	}
	if r, g, b, _ := decoded.At(5, 0).RGBA(); r == 0 && g == 0 && b == 0 {
		t.Error("pixel outside the redacted rectangle was blacked out too")
	}

	// Out-of-bounds rectangle: refused rather than silently clipped.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SubmitIDDocumentVerification(ctx, req.ID, rawID, []dsarservice.Rect{{X: 5, Y: 5, Width: 50, Height: 50}}); !errors.Is(err, dsarservice.ErrRedactionOutOfBounds) && !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("out-of-bounds redaction: got %v", err)
		}
		return nil
	})

	// Staff confirms: passes, stamps verified_at, moves the request on (same as the OTP path).
	var decided dsarservice.Verification
	e.in(t, func(ctx context.Context) error {
		var err error
		decided, err = e.svc.ConfirmIDDocument(ctx, req.ID, v.ID, true)
		return err
	})
	if decided.Status != "passed" {
		t.Errorf("decided status = %q, want passed", decided.Status)
	}
	e.in(t, func(ctx context.Context) error {
		after, err := e.svc.GetRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if after.Status != "in_review" || after.VerifiedAt == nil {
			t.Errorf("after decide: status=%q verified_at=%v", after.Status, after.VerifiedAt)
		}
		return nil
	})

	// Deciding an already-decided verification is refused, not a silent re-apply.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.ConfirmIDDocument(ctx, req.ID, v.ID, true); !errors.Is(err, dsarservice.ErrVerificationNotPending) {
			t.Errorf("re-deciding: got %v, want ErrVerificationNotPending", err)
		}
		return nil
	})
}

func TestListVerifications_TwoTenantIsolation(t *testing.T) {
	e, _, _ := identityEnv(t, "dsaridentlist")
	other := identityEnvOther(t)
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
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.StartOTPVerification(ctx, req.ID, "otp_email")
		return err
	})

	other.in(t, func(ctx context.Context) error {
		if _, err := other.svc.ListVerifications(ctx, req.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("cross-tenant list: got %v, want ErrNotFound", err)
		}
		return nil
	})
}

func identityEnvOther(t *testing.T) env {
	t.Helper()
	e, _, _ := identityEnv(t, "dsaridentother")
	return e
}
