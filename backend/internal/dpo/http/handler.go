// Package dpohttp holds the dpo module's admin endpoints (DPO-01 appointment register so far). Types in
// dpo.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package dpohttp

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"

	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *dposervice.Service
}

func NewStrict(svc *dposervice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// encodeCursor/decodeCursor are shared by every (created_at, id) keyset-paginated list in this module.
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

func encodeAppointmentCursor(c dposervice.AppointmentCursor) string { return encodeCursor(c.CreatedAt, c.ID) }

func decodeAppointmentCursor(s string) (dposervice.AppointmentCursor, error) {
	t, u, err := decodeCursor(s)
	return dposervice.AppointmentCursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, dposervice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, dposervice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, dposervice.ErrFileNotUsable):
		return httpx.Problem{Status: 422, Code: "dpo.file_not_usable", Title: "The file isn't usable (already attached, still scanning or infected)"}
	case errors.Is(err, dposervice.ErrInvalid):
		msg := err.Error()
		field := ""
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			field = msg[i+2:]
		}
		return httpx.Problem{Status: 422, Code: "dpo.invalid_input", Title: "Invalid input", Errors: []httpx.FieldError{{Field: field, Code: "invalid"}}}
	default:
		return err
	}
}
