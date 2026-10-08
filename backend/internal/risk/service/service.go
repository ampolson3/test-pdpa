// Package service is the `risk` schema's own module. It started deliberately minimal — only
// ROPA-09's read-only ม.37(1) controls catalog — and gained its first real feature with RRA-02
// (configurable risk matrices): the shared "risk engine" the module doc's own backend note says
// DPIA/vendor/breach will all read from, once those modules compute a risk score. risk.controls still
// carries a global-vs-tenant RLS split identical to ORG-07's master data (tenant_id NULL = platform
// default, seeded as a draft pending legal review — decisions.md Q-25).
package service

import (
	"context"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
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

type Service struct {
	Audit *audit.Service
	Ropa  Ropa // RRA-01: scoring an activity (nil until a caller needs Score/LatestScore)
}

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
