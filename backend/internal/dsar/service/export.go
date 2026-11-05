package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	consentservice "pdpa-platform/internal/consent/service"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/platform/crypto"
)

// DSAR-15 (ม.31, portability): a machine-readable copy of the personal data this system actually holds
// about the request's own data subject — generated on demand, delivered through DSAR-14's exact secure-
// delivery mechanism (expiring link, access code, logged downloads, auto-purge), never a new delivery path.
// "Direct transfer to another controller via API/SFTP" (the module doc's own "เมื่อทำได้") is deliberately
// not built: no such controller endpoint exists anywhere in this codebase to transfer to — the file itself,
// handed to the data subject, is what ม.31 requires regardless.

var portabilityFormats = []string{"json", "csv", "xml"}

// consentDecision is one purpose this subject currently has a decision on record for (consent.Service's own
// PurposeStatus, flattened to the fields a portability file needs — never the subject's internal id).
type consentDecision struct {
	PurposeCode string `json:"purpose_code" xml:"purpose_code"`
	PurposeName string `json:"purpose_name" xml:"purpose_name"`
	Status      string `json:"status" xml:"status"`
	VersionNo   int32  `json:"version_no" xml:"version_no"`
	Preferences string `json:"preferences,omitempty" xml:"preferences,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty" xml:"expires_at,omitempty"`
}

// portabilityData is the export's full content — one record, this request's own subject, not a generic
// "every table" dump: ROPA only ever catalogs categories of data processed, never an individual's own
// records, so consent is the one other module with real per-subject data to include (rule 9: read only
// through consent's own exported service).
type portabilityData struct {
	XMLName          xml.Name          `json:"-" xml:"portability_export"`
	GeneratedAt      time.Time         `json:"generated_at" xml:"generated_at"`
	RequestNo        string            `json:"request_no" xml:"request_no"`
	RequestTypeCode  string            `json:"request_type_code" xml:"request_type_code"`
	RequestTypeName  string            `json:"request_type_name" xml:"request_type_name"`
	Channel          string            `json:"channel" xml:"channel"`
	Status           string            `json:"status" xml:"status"`
	ReceivedAt       time.Time         `json:"received_at" xml:"received_at"`
	RequesterName    string            `json:"requester_name" xml:"requester_name"`
	RequesterContact string            `json:"requester_contact" xml:"requester_contact"`
	ConsentDecisions []consentDecision `json:"consent_decisions" xml:"consent_decisions>decision"`
}

// GeneratePortabilityExportInput mirrors CreatePackageInput minus the file id, which this call generates
// itself rather than taking one from the caller.
type GeneratePortabilityExportInput struct {
	Format            string
	PasswordProtected bool
	ExpiresAt         *time.Time
}

// GeneratePortabilityExport is the acceptance criterion itself ("ไฟล์ portability เปิดอ่านด้วยเครื่องได้และ
// ตรงกับข้อมูลในระบบ"): builds a standard-format file from exactly what the system holds about this
// request's own subject, then hands it straight to DSAR-14's own createPackage core — the generated file is
// trusted by construction (this package's own output, never a user upload), so it skips CreatePackage's own
// clean-AV-status gate the same way PLT-16's rendered documents and PLT-14's import error report already do
// via the same files.Service.SaveGenerated trusted path.
func (s *Service) GeneratePortabilityExport(ctx context.Context, requestID uuid.UUID, in GeneratePortabilityExportInput) (Package, string, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("dsar.request.read") {
		return Package{}, "", ErrForbidden
	}
	if !g.Has("dsar.package.create") {
		return Package{}, "", ErrForbidden
	}
	if !slices.Contains(portabilityFormats, in.Format) {
		return Package{}, "", fmt.Errorf("%w: format", ErrInvalid)
	}
	if s.Files == nil {
		return Package{}, "", fmt.Errorf("dsar: file storage not wired")
	}
	req, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return Package{}, "", err
	}
	rt, err := s.GetRequestType(ctx, req.RequestTypeID)
	if err != nil {
		return Package{}, "", err
	}
	name, err := s.decryptRequesterName(ctx, req)
	if err != nil {
		return Package{}, "", err
	}
	contact, err := s.decryptRequesterContact(ctx, requestID)
	if err != nil {
		return Package{}, "", err
	}

	data := portabilityData{
		GeneratedAt: s.now(), RequestNo: req.RequestNo, RequestTypeCode: rt.Code, RequestTypeName: rt.NameTh,
		Channel: req.Channel, Status: req.Status, ReceivedAt: req.ReceivedAt, RequesterName: name, RequesterContact: contact,
		ConsentDecisions: s.consentDecisionsFor(ctx, contact),
	}

	body, fileName, err := encodePortability(data, in.Format)
	if err != nil {
		return Package{}, "", err
	}
	f, err := s.Files.SaveGenerated(ctx, fileName, bytes.NewReader(body), "dsar_request", requestID)
	if err != nil {
		return Package{}, "", err
	}
	return s.createPackage(ctx, requestID, f.ID, in.Format, in.PasswordProtected, in.ExpiresAt)
}

// consentDecisionsFor matches the request's own decrypted contact against consent's own subject registry
// through the exact blind-index lookup DSAR-17's search already uses (never decrypted to search, rule 3). A
// contact with no matching consent subject — or no Consent source wired at all — is simply an empty list,
// not an error: most requesters have never submitted a consent form.
func (s *Service) consentDecisionsFor(ctx context.Context, contact string) []consentDecision {
	if s.Consent == nil || strings.TrimSpace(contact) == "" {
		return []consentDecision{}
	}
	kind := string(crypto.KindPhone)
	if strings.Contains(contact, "@") {
		kind = string(crypto.KindEmail)
	}
	subjects, err := s.Consent.Subjects(ctx, kind, contact)
	if err != nil || len(subjects) == 0 {
		return []consentDecision{}
	}
	profile, err := s.Consent.GetProfile(ctx, subjects[0].ID)
	if err != nil {
		return []consentDecision{}
	}
	out := make([]consentDecision, 0, len(profile.Statuses))
	for _, st := range profile.Statuses {
		d := consentDecision{PurposeCode: st.PurposeCode, PurposeName: purposeNameFor(st.PurposeName), Status: st.Status, VersionNo: st.VersionNo}
		if len(st.Preferences) > 0 {
			d.Preferences = string(st.Preferences)
		}
		if st.ExpiresAt != nil {
			d.ExpiresAt = st.ExpiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, d)
	}
	return out
}

func purposeNameFor(t consentservice.Text) string {
	if t.Th != "" {
		return t.Th
	}
	return t.En
}

func encodePortability(data portabilityData, format string) ([]byte, string, error) {
	switch format {
	case "json":
		b, err := json.MarshalIndent(data, "", "  ")
		return b, "portability-export.json", err
	case "xml":
		b, err := xml.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, "", err
		}
		return append([]byte(xml.Header), b...), "portability-export.xml", nil
	case "csv":
		b, err := portabilityCSV(data)
		return b, "portability-export.csv", err
	default:
		return nil, "", fmt.Errorf("%w: format", ErrInvalid)
	}
}

// portabilityCSV flattens the export to plain field,value rows — the simplest shape any spreadsheet or CSV
// parser opens correctly, with one indexed row per consent decision rather than a nested structure.
func portabilityCSV(data portabilityData) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	rows := [][]string{
		{"field", "value"},
		{"generated_at", data.GeneratedAt.UTC().Format(time.RFC3339)},
		{"request_no", data.RequestNo},
		{"request_type_code", data.RequestTypeCode},
		{"request_type_name", data.RequestTypeName},
		{"channel", data.Channel},
		{"status", data.Status},
		{"received_at", data.ReceivedAt.UTC().Format(time.RFC3339)},
		{"requester_name", data.RequesterName},
		{"requester_contact", data.RequesterContact},
	}
	for i, d := range data.ConsentDecisions {
		p := "consent_decisions." + strconv.Itoa(i) + "."
		rows = append(rows,
			[]string{p + "purpose_code", d.PurposeCode},
			[]string{p + "purpose_name", d.PurposeName},
			[]string{p + "status", d.Status},
			[]string{p + "version_no", strconv.Itoa(int(d.VersionNo))},
			[]string{p + "preferences", d.Preferences},
			[]string{p + "expires_at", d.ExpiresAt},
		)
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
