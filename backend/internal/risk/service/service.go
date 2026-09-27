// Package service is the first, deliberately minimal package on the `risk` schema — the full RRA module
// (risk matrices, factor-weighted scoring, gap findings) is P2 and not built yet. This exists only so
// ROPA-09 (ม.37(1): every processing activity references its security measures) has a catalog to point at.
// risk.controls already carries a global-vs-tenant RLS split identical to ORG-07's master data (tenant_id
// NULL = platform default, seeded as a draft pending legal review — decisions.md Q-25); this package is a
// plain read, with no create/update surface, since nothing needs tenant-authored controls yet.
package service

import (
	"context"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	riskstore "pdpa-platform/internal/risk/store"
)

// Control is one row of the ม.37(1) security-measures catalog (risk.controls).
type Control struct {
	ID            uuid.UUID
	Code          string
	Name          string
	Category      string // organizational | technical | physical | access_control | legal
	Description   string
	FrameworkRefs []string
}

type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) ListControls(ctx context.Context) ([]Control, error) {
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListControls(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Control, 0, len(rows))
	for _, r := range rows {
		out = append(out, toControl(riskstore.GetControlRow(r)))
	}
	return out, nil
}

func (s *Service) GetControl(ctx context.Context, id uuid.UUID) (Control, error) {
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).GetControl(ctx, id)
	if err != nil {
		return Control{}, err
	}
	return toControl(row), nil
}

func toControl(r riskstore.GetControlRow) Control {
	return Control{ID: r.ID, Code: r.Code, Name: r.Name, Category: r.Category,
		Description: derefStr(r.Description), FrameworkRefs: r.FrameworkRefs}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
