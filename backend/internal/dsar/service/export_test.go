package service_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	consentservice "pdpa-platform/internal/consent/service"
	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/platform/crypto"
)

// fakeConsent is an in-memory stand-in for consentservice.Service (DSAR-15's own cross-module read) — just
// enough of dsar.Consent to prove a matching identifier surfaces that subject's real decisions, and a
// non-matching one surfaces none.
type fakeConsent struct {
	byContact map[string]uuid.UUID
	profiles  map[uuid.UUID]consentservice.Profile
}

func newFakeConsent() *fakeConsent {
	return &fakeConsent{byContact: map[string]uuid.UUID{}, profiles: map[uuid.UUID]consentservice.Profile{}}
}

func (f *fakeConsent) addSubject(contact string, statuses []consentservice.PurposeStatus) uuid.UUID {
	id := uuid.New()
	f.byContact[contact] = id
	f.profiles[id] = consentservice.Profile{SubjectSummary: consentservice.SubjectSummary{ID: id}, Statuses: statuses}
	return id
}

func (f *fakeConsent) Subjects(ctx context.Context, identifierType, value string) ([]consentservice.SubjectSummary, error) {
	id, ok := f.byContact[value]
	if !ok {
		return nil, nil
	}
	return []consentservice.SubjectSummary{{ID: id}}, nil
}

func (f *fakeConsent) GetProfile(ctx context.Context, id uuid.UUID) (consentservice.Profile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return consentservice.Profile{}, errors.New("not found")
	}
	return p, nil
}

func exportEnv(t *testing.T, suffix string) (env, *fakeFiles, *fakeConsent) {
	t.Helper()
	e, _, ff := identityEnv(t, suffix)
	fc := newFakeConsent()
	e.svc.Consent = fc
	return e, ff, fc
}

func exportedBytes(t *testing.T, ff *fakeFiles, fileID uuid.UUID) []byte {
	t.Helper()
	b, ok := ff.saved[fileID]
	if !ok {
		t.Fatalf("no saved bytes for file %s", fileID)
	}
	return b
}

// TestGeneratePortabilityExport_MatchesSystemData is the acceptance criterion directly ("ไฟล์ portability
// เปิดอ่านด้วยเครื่องได้และตรงกับข้อมูลในระบบ"): every format decodes cleanly with a standard library parser
// and every value in it is exactly what CreateRequest stored (the decrypted name/contact, the real request
// number/type/status) plus the matching consent subject's own real decision.
func TestGeneratePortabilityExport_MatchesSystemData(t *testing.T) {
	e, ff, fc := exportEnv(t, "dsarexportmatch")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ส่งออก จำกัด")
	rt := requestTypeID(t, e)

	const contact = "export-subject@example.com"
	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: rt, LegalEntityID: legalEntity, Channel: "web",
			RequesterName: "ส่งออก ทดสอบ", RequesterContact: contact, ContactKind: crypto.KindEmail,
		})
		return err
	})
	fc.addSubject(contact, []consentservice.PurposeStatus{
		{PurposeCode: "marketing", PurposeName: consentservice.Text{Th: "การตลาด"}, Status: "active", VersionNo: 1},
	})

	for _, format := range []string{"json", "csv", "xml"} {
		var pkg dsarservice.Package
		e.in(t, func(ctx context.Context) error {
			var err error
			pkg, _, err = e.svc.GeneratePortabilityExport(ctx, req.ID, dsarservice.GeneratePortabilityExportInput{Format: format})
			return err
		})
		if pkg.Format != format || pkg.PublicKey == "" {
			t.Fatalf("[%s] package = %+v", format, pkg)
		}

		var url string
		e.in(t, func(ctx context.Context) error {
			var err error
			url, err = e.svc.PublicDownload(ctx, pkg.ID, "")
			return err
		})
		if url == "" {
			t.Fatalf("[%s] expected a signed download URL", format)
		}

		raw := exportedBytes(t, ff, pkg.FileID)
		switch format {
		case "json":
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatalf("[json] not machine-readable: %v", err)
			}
			if data["request_no"] != req.RequestNo {
				t.Errorf("[json] request_no = %v, want %v", data["request_no"], req.RequestNo)
			}
			if data["requester_name"] != "ส่งออก ทดสอบ" {
				t.Errorf("[json] requester_name = %v", data["requester_name"])
			}
			if data["requester_contact"] != contact {
				t.Errorf("[json] requester_contact = %v, want %v", data["requester_contact"], contact)
			}
			decisions, _ := data["consent_decisions"].([]any)
			if len(decisions) != 1 {
				t.Fatalf("[json] consent_decisions = %v, want 1 entry", data["consent_decisions"])
			}
			d := decisions[0].(map[string]any)
			if d["purpose_code"] != "marketing" || d["status"] != "active" {
				t.Errorf("[json] decision = %v", d)
			}
		case "csv":
			rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
			if err != nil {
				t.Fatalf("[csv] not machine-readable: %v", err)
			}
			fields := map[string]string{}
			for _, r := range rows[1:] {
				fields[r[0]] = r[1]
			}
			if fields["request_no"] != req.RequestNo {
				t.Errorf("[csv] request_no = %q, want %q", fields["request_no"], req.RequestNo)
			}
			if fields["requester_contact"] != contact {
				t.Errorf("[csv] requester_contact = %q, want %q", fields["requester_contact"], contact)
			}
			if fields["consent_decisions.0.purpose_code"] != "marketing" {
				t.Errorf("[csv] consent decision purpose_code = %q", fields["consent_decisions.0.purpose_code"])
			}
		case "xml":
			var data struct {
				RequestNo        string `xml:"request_no"`
				RequesterContact string `xml:"requester_contact"`
			}
			if err := xml.Unmarshal(raw, &data); err != nil {
				t.Fatalf("[xml] not machine-readable: %v", err)
			}
			if data.RequestNo != req.RequestNo || data.RequesterContact != contact {
				t.Errorf("[xml] decoded = %+v", data)
			}
		}
	}
}

// TestGeneratePortabilityExport_NoMatchingConsentIsEmptyNotError proves a requester with no consent record
// on file gets a valid export with an empty decisions list, not a failure.
func TestGeneratePortabilityExport_NoMatchingConsentIsEmptyNotError(t *testing.T) {
	e, ff, _ := exportEnv(t, "dsarexportnomatch")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ไม่มีข้อมูล จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var pkg dsarservice.Package
	e.in(t, func(ctx context.Context) error {
		var err error
		pkg, _, err = e.svc.GeneratePortabilityExport(ctx, req.ID, dsarservice.GeneratePortabilityExportInput{Format: "json"})
		return err
	})
	raw := exportedBytes(t, ff, pkg.FileID)
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	decisions, _ := data["consent_decisions"].([]any)
	if len(decisions) != 0 {
		t.Errorf("consent_decisions = %v, want empty", data["consent_decisions"])
	}
}

// TestGeneratePortabilityExport_Validation: bad format and the permission gate.
func TestGeneratePortabilityExport_Validation(t *testing.T) {
	e, _, _ := exportEnv(t, "dsarexportvalidation")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ตรวจสอบ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.GeneratePortabilityExport(ctx, req.ID, dsarservice.GeneratePortabilityExportInput{Format: "pdf"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad format: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// Without dsar.package.create, the export is refused — the same gate CreatePackage itself has.
	e.inAs(t, []string{"dsar.request.read"}, func(ctx context.Context) error {
		if _, _, err := e.svc.GeneratePortabilityExport(ctx, req.ID, dsarservice.GeneratePortabilityExportInput{Format: "json"}); !errors.Is(err, dsarservice.ErrForbidden) {
			t.Errorf("no package.create permission: err = %v, want ErrForbidden", err)
		}
		return nil
	})
}

// TestGeneratePortabilityExport_TwoTenantIsolation: standard rule-1 isolation via the request id.
func TestGeneratePortabilityExport_TwoTenantIsolation(t *testing.T) {
	e, _, _ := exportEnv(t, "dsarexportisoA")
	other, _, _ := exportEnv(t, "dsarexportisoB")
	legalEntity := createTestLegalEntity(t, e, "บริษัท เอ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	other.in(t, func(ctx context.Context) error {
		if _, _, err := other.svc.GeneratePortabilityExport(ctx, req.ID, dsarservice.GeneratePortabilityExportInput{Format: "json"}); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("cross-tenant export: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
