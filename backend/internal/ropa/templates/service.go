// Package templates is RTG-01's standard activity library: a read over the platform's shared catalog of
// ready-made RoPA activities (ropa.template_sets / ropa.activity_templates, already fully specified in the
// baseline migrations — ERD-09), seeded content for the global standard/industry/government/processor sets
// (migrations 00049/00082/00084/00087, decisions.md Q-28/39/40/42). RTG-08 (this file plus write.go) adds the
// one thing those seeds never needed: a tenant's own custom sets, cloned from a global one or authored from
// scratch, with full CRUD on their activities. Defaults and rationale are kept as opaque jsonb, matching
// ROPA-03/06/08/09's own activity_purposes/activity_data/activity_recipients/retention_rules/activity_controls
// shapes, keyed by code (lawful_basis_code, data_category_code, subject_type_code, control code) rather than
// id, since a global template can't reference a tenant's own org.* rows — RTG-08's own custom activities keep
// that same convention, so ROPA-05's existing from-template create path needs no change to consume them.
package templates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
	auditservice "pdpa-platform/internal/platform/audit/service"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ErrNotFound is returned when an activity template id doesn't exist or isn't visible under the caller's RLS.
var ErrNotFound = errors.New("ropa/templates: not found")

// TemplateSet is one collection of standard activities (ropa.template_sets) — the platform's own global
// standard/industry/government/processor sets (RTG-01/02/03/07) plus, as of RTG-08, a tenant's own custom
// sets. Status/RowVersion are only meaningful on a tenant's own custom set — every global set is always
// "published" with no ETag a tenant could ever hold (it isn't theirs to publish/retire).
type TemplateSet struct {
	ID         uuid.UUID
	Name       string
	SetType    string // standard | industry | government | processor | custom
	Industry   string
	VersionNo  int
	Status     string // draft | published | retired
	RowVersion int32
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
	RowVersion    int32 // only meaningful on a tenant's own custom activity (RTG-08); 0 on a global one.
}

type Service struct {
	// Audit is nil-means-skip (the convention every other optional cross-cutting field on a module Service
	// uses) — read-only callers like RTG-01/02/03/07's own browsing never need it; RTG-08's writes do.
	Audit *auditservice.Service
}

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
		out = append(out, TemplateSet{
			ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo),
			Status: r.Status, RowVersion: r.RowVersion,
		})
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

// ListTemplateVersions is RTG-10's own "หน้าประวัติ template": every version ever issued of one
// template's (template_set_id, code), newest first.
func (s *Service) ListTemplateVersions(ctx context.Context, templateSetID uuid.UUID, code string) ([]ActivityTemplate, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListTemplateVersionsForCode(ctx, ropastore.ListTemplateVersionsForCodeParams{
		TemplateSetID: templateSetID, Code: code,
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

// LatestVersion is RTG-10's own comparison point: the current (highest version_no) row for a template's
// own (template_set_id, code) — used to decide whether an activity created from an older version is now
// outdated. Never errors ErrNotFound for a code that still exists (it always does, since the caller's own
// template row is itself one of the versions being compared against).
func (s *Service) LatestVersion(ctx context.Context, templateSetID uuid.UUID, code string) (ActivityTemplate, error) {
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetLatestTemplateVersionForCode(ctx, ropastore.GetLatestTemplateVersionForCodeParams{
		TemplateSetID: templateSetID, Code: code,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityTemplate{}, ErrNotFound
	}
	if err != nil {
		return ActivityTemplate{}, err
	}
	return toActivityTemplate(ropastore.GetActivityTemplateRow(row)), nil
}

func toActivityTemplate(r ropastore.GetActivityTemplateRow) ActivityTemplate {
	return ActivityTemplate{
		ID: r.ID, TemplateSetID: r.TemplateSetID, Code: r.Code, NameTh: r.NameTh, NameEn: derefStr(r.NameEn),
		JobCategory: r.JobCategory, Role: r.Role, Defaults: json.RawMessage(r.Defaults), Rationale: json.RawMessage(r.Rationale),
		LegalRefs: r.LegalRefs, VersionNo: int(r.VersionNo), RowVersion: r.RowVersion,
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
