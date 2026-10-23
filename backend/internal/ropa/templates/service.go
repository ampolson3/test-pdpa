// Package templates is RTG-01's standard activity library: a plain read over the platform's shared catalog
// of ready-made RoPA activities (ropa.template_sets / ropa.activity_templates, already fully specified in
// the baseline migrations — ERD-09). Content is seeded data (migration 00049, decisions.md Q-28), not
// something a tenant authors here — no create/update surface yet; that's RTG-08's job (P3, "แก้ไขและสร้าง
// template ขององค์กร"), not built. Defaults and rationale are kept as opaque jsonb, matching ROPA-03/06/08/09's
// own activity_purposes/activity_data/activity_recipients/retention_rules/activity_controls shapes, keyed by
// code (lawful_basis_code, data_category_code, subject_type_code, control code) rather than id, since a
// global template can't reference a tenant's own org.* rows.
package templates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ErrNotFound is returned when an activity template id doesn't exist or isn't visible under the caller's RLS.
var ErrNotFound = errors.New("ropa/templates: not found")

// TemplateSet is one collection of standard activities (ropa.template_sets) — today only the platform's own
// "standard" set (migration 00049); industry/government/processor/custom sets are later RTG features.
type TemplateSet struct {
	ID        uuid.UUID
	Name      string
	SetType   string // standard | industry | government | processor | custom
	Industry  string
	VersionNo int
}

// ActivityTemplate is one ready-made RoPA activity (ropa.activity_templates): its ม.39 defaults and the
// rationale behind them, both raw JSON the caller decodes as needed (no fixed Go shape here — the admin UI
// and a future RTG-04 "create from template" both just need to show/copy the same JSON this was seeded with).
type ActivityTemplate struct {
	ID            uuid.UUID
	TemplateSetID uuid.UUID
	Code          string
	NameTh        string
	NameEn        string
	JobCategory   string
	Role          string // controller | processor
	Defaults      json.RawMessage
	Rationale     json.RawMessage
	LegalRefs     []string
	VersionNo     int
}

type Service struct{}

func New() *Service { return &Service{} }

// ListTemplateSets returns every published template set visible to the caller (global or this tenant's
// own), for the set picker.
func (s *Service) ListTemplateSets(ctx context.Context) ([]TemplateSet, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListTemplateSets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateSet, 0, len(rows))
	for _, r := range rows {
		out = append(out, TemplateSet{ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo)})
	}
	return out, nil
}

// ListActivityTemplates lists one set's activities, optionally filtered to one job category — the acceptance
// criterion's "หน้าเลือกหมวดงาน / กิจกรรม" picker.
func (s *Service) ListActivityTemplates(ctx context.Context, templateSetID uuid.UUID, jobCategory *string) ([]ActivityTemplate, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityTemplates(ctx, ropastore.ListActivityTemplatesParams{
		TemplateSetID: templateSetID, JobCategory: jobCategory,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityTemplate, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityTemplate(ropastore.GetActivityTemplateRow(r)))
	}
	return out, nil
}

// GetActivityTemplate returns one activity template with its full defaults/rationale — the "ดูค่าตั้งต้น" detail view.
func (s *Service) GetActivityTemplate(ctx context.Context, id uuid.UUID) (ActivityTemplate, error) {
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetActivityTemplate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityTemplate{}, ErrNotFound
	}
	if err != nil {
		return ActivityTemplate{}, err
	}
	return toActivityTemplate(row), nil
}

func toActivityTemplate(r ropastore.GetActivityTemplateRow) ActivityTemplate {
	return ActivityTemplate{
		ID: r.ID, TemplateSetID: r.TemplateSetID, Code: r.Code, NameTh: r.NameTh, NameEn: derefStr(r.NameEn),
		JobCategory: r.JobCategory, Role: r.Role, Defaults: json.RawMessage(r.Defaults), Rationale: json.RawMessage(r.Rationale),
		LegalRefs: r.LegalRefs, VersionNo: int(r.VersionNo),
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
