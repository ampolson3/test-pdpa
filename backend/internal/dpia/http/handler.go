// Package dpiahttp holds the dpia module's admin endpoints (DPIA-01 screening, DPIA-02 thresholds). Types in
// dpia.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package dpiahttp

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
)

type Strict struct {
	svc *dpiaservice.Service
}

func NewStrict(svc *dpiaservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// encodeCursor/decodeCursor are the same (created_at, id) keyset shape every other module's list uses.
func encodeCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.UUID{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	u, err := uuid.Parse(id)
	return t, u, err
}

func encodeAssessmentCursor(c dpiaservice.AssessmentCursor) string {
	return encodeCursor(c.CreatedAt, c.ID)
}

func decodeAssessmentCursor(s string) (dpiaservice.AssessmentCursor, error) {
	t, u, err := decodeCursor(s)
	return dpiaservice.AssessmentCursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	var ve *dpiaservice.ValidationError
	switch {
	case errors.Is(err, dpiaservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, dpiaservice.ErrVersionMismatch), errors.Is(err, forms.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, dpiaservice.ErrBadTemplate):
		return httpx.Problem{Status: 422, Code: "dpia.bad_template", Title: "No published DPIA template for this checklist"}
	case errors.Is(err, forms.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, forms.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, forms.ErrInvalidState), errors.Is(err, forms.ErrUnknownType):
		return httpx.Problem{Status: 409, Code: "dpia.invalid_state", Title: "Not allowed in the form's current state"}
	case errors.Is(err, forms.ErrInvalidRequest), errors.Is(err, forms.ErrInvalidSchema):
		return httpx.UnprocessableEntity("dpia.invalid_input", err.Error())
	case errors.As(err, &ve):
		p := httpx.Problem{Status: 422, Code: "dpia.invalid_input", Title: "Invalid input"}
		for _, f := range ve.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Code: f.Code})
		}
		return p
	case errors.Is(err, dpiaservice.ErrInvalid):
		msg := err.Error()
		field := ""
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			field = msg[i+2:]
		}
		return httpx.Problem{Status: 422, Code: "dpia.invalid_input", Title: "Invalid input", Errors: []httpx.FieldError{{Field: field, Code: "invalid"}}}
	default:
		return err
	}
}
