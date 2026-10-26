// Package service is the `risk` schema's own module. It started deliberately minimal — only
// ROPA-09's read-only ม.37(1) controls catalog — and gained its first real feature with RRA-02
// (configurable risk matrices): the shared "risk engine" the module doc's own backend note says
// DPIA/vendor/breach will all read from, once those modules compute a risk score. risk.controls still
// carries a global-vs-tenant RLS split identical to ORG-07's master data (tenant_id NULL = platform
// default, seeded as a draft pending legal review — decisions.md Q-25).
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/notify"
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
	Audit       *audit.Service
	Ropa        Ropa           // RRA-01/04: scoring and gap-analyzing an activity (nil until a caller needs one)
	DpiaTrigger DpiaTrigger    // RRA-03: open a DPIA when an activity scores high/very_high (nil until dpia registers itself)
	Dpo         DpoTasks       // DPIA-07: open a remediation task when a control gets an owner/due date (nil until dpo registers itself)
	Notice      Notice         // RRA-04: whether a notice already covers an activity (nil until notice registers itself)
	Vendor      Vendor         // RRA-09: FK-visibility check for a risk's own vendor link (nil until vendormgmt registers itself)
	Forms       *forms.Service // RRA-05: the per-activity checklist's own form engine (nil until a caller needs one)
	// Notify and River drive RRA-08's own acceptance-review reminder; nil in contexts that don't need them.
	Notify *notify.Service
	River  *river.Client[pgx.Tx]
	// Now is clock-injectable for tests (RRA-08's own expiry check); nil means time.Now in production.
	Now func() time.Time
}

func New() *Service { return &Service{} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

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
