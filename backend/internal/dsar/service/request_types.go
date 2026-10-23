package service

import (
	"context"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// RequestType is one of the 9 fixed DSAR right types (dsar.requests.request_type_id CHECK), seeded globally
// (migration 00043) since the set of codes is fixed by the DDL itself, not tenant-editable master data.
type RequestType struct {
	ID       uuid.UUID
	Code     string
	NameTh   string
	NameEn   string
	LegalRef string
	SLADays  int16
}

func (s *Service) ListRequestTypes(ctx context.Context) ([]RequestType, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListRequestTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RequestType, 0, len(rows))
	for _, r := range rows {
		out = append(out, RequestType{ID: r.ID, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), LegalRef: deref(r.LegalRef), SLADays: r.SlaDays})
	}
	return out, nil
}

func (s *Service) GetRequestType(ctx context.Context, id uuid.UUID) (RequestType, error) {
	r, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetRequestType(ctx, id)
	if err != nil {
		return RequestType{}, ErrNotFound
	}
	return RequestType{ID: r.ID, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), LegalRef: deref(r.LegalRef), SLADays: r.SlaDays}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
