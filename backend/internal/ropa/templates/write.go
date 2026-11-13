// RTG-08: a tenant's own custom template sets — "คัดลอก template กลางมาปรับ และกำหนด template มาตรฐาน
// ของกลุ่มบริษัท" (clone a central template to adjust, and designate it as the group's own standard).
// "กลุ่มบริษัท" (the group) is this tenant; "บริษัทในเครือ" (its subsidiary companies) are the legal
// entities/org units RLS already scopes to the same tenant — so the acceptance criterion ("บริษัทในเครือ
// ใช้ template มาตรฐานของกลุ่มได้") is satisfied the moment a custom set is published: ListTemplateSets
// (RTG-01) already returns every published set regardless of set_type, and ROPA-05's own from-template
// create path is already set-type/role-agnostic, so no other code anywhere needs to change for a
// published custom set to be immediately usable by every legal entity under this tenant.
package templates

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ErrInvalid is returned for a bad input or an attempt to write to a set this tenant doesn't own.
var ErrInvalid = errors.New("ropa/templates: invalid input")

// ErrVersionMismatch is returned when the caller's If-Match no longer matches the row's own row_version.
var ErrVersionMismatch = errors.New("ropa/templates: version mismatch")

// EntityType is the audit entity type for both ropa.template_sets and ropa.activity_templates rows this
// package writes (RTG-08) — the set and its activities are one record from an audit-reader's point of view.
const EntityType = "ropa_template_set"

// ActivityTemplateInput is what the caller supplies to add or edit one activity in a tenant's own custom
// template set — the same ม.39-topic-keyed defaults/rationale shape every seeded template already uses
// (RopaActivityTemplate's own wire schema), so a custom activity is indistinguishable from a seeded one to
// every downstream reader (ROPA-05's from-template create, RTG-01's own browse/detail view).
type ActivityTemplateInput struct {
	Code        string
	NameTh      string
	NameEn      string
	JobCategory string
	Role        string // controller | processor
	Defaults    []byte
	Rationale   []byte
	LegalRefs   []string
}

func (in ActivityTemplateInput) normalize() (ActivityTemplateInput, error) {
	if in.Code == "" || in.NameTh == "" || in.JobCategory == "" {
		return in, fmt.Errorf("%w: code, name_th and job_category are required", ErrInvalid)
	}
	if in.Role != "controller" && in.Role != "processor" {
		return in, fmt.Errorf("%w: role", ErrInvalid)
	}
	if len(in.Defaults) == 0 || len(in.Rationale) == 0 {
		return in, fmt.Errorf("%w: defaults and rationale are required", ErrInvalid)
	}
	if in.LegalRefs == nil {
		in.LegalRefs = []string{}
	}
	return in, nil
}

// ListOwnTemplateSets is this tenant's own management list — every custom set it has, any status
// (draft/published/retired), distinct from ListTemplateSets' own published-only browse list.
func (s *Service) ListOwnTemplateSets(ctx context.Context) ([]TemplateSet, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListOwnTemplateSets(ctx)
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

// CreateTemplateSet starts a brand-new, empty custom set (draft) — add activities to it with
// AddActivityTemplate, or use CloneTemplateSet instead to start from an existing set's own activities.
func (s *Service) CreateTemplateSet(ctx context.Context, name string) (TemplateSet, error) {
	if name == "" {
		return TemplateSet{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return TemplateSet{}, err
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertTemplateSet(ctx, ropastore.InsertTemplateSetParams{ID: id, Name: name})
	if err != nil {
		return TemplateSet{}, err
	}
	out := toTemplateSetFromInsert(row)
	return out, s.audit(ctx, "ropa.template_set.create", id, nil, map[string]any{"name": name, "set_type": "custom"})
}

// CloneTemplateSet is the acceptance criterion's own first half ("คัดลอก template กลางมาปรับ"): a new,
// independent custom set (draft) carrying the source set's own activities verbatim — editing the clone
// afterward never touches the source, since every cloned activity is a brand-new row from the start.
func (s *Service) CloneTemplateSet(ctx context.Context, sourceSetID uuid.UUID, name string) (TemplateSet, error) {
	if name == "" {
		return TemplateSet{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	source, err := q.ListActivityTemplates(ctx, ropastore.ListActivityTemplatesParams{TemplateSetID: sourceSetID})
	if err != nil {
		return TemplateSet{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return TemplateSet{}, err
	}
	setRow, err := q.InsertTemplateSet(ctx, ropastore.InsertTemplateSetParams{ID: id, Name: name})
	if err != nil {
		return TemplateSet{}, err
	}
	for _, a := range source {
		activityID, err := uuid.NewV7()
		if err != nil {
			return TemplateSet{}, err
		}
		if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			_, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertActivityTemplate(ctx, ropastore.InsertActivityTemplateParams{
				ID: activityID, TemplateSetID: id, Code: a.Code, NameTh: a.NameTh, NameEn: a.NameEn,
				JobCategory: a.JobCategory, Role: a.Role, Defaults: a.Defaults, Rationale: a.Rationale, LegalRefs: a.LegalRefs,
			})
			return err
		}); err != nil {
			if isUnique(err) {
				return TemplateSet{}, fmt.Errorf("%w: code %q already exists in the new set", ErrInvalid, a.Code)
			}
			return TemplateSet{}, err
		}
	}
	out := toTemplateSetFromInsert(setRow)
	return out, s.audit(ctx, "ropa.template_set.clone", id, nil, map[string]any{"name": name, "source_set_id": sourceSetID, "activities_copied": len(source)})
}

// PublishTemplateSet is the acceptance criterion's own second half ("กำหนด template มาตรฐานของกลุ่มบริษัท")
// — once published, every legal entity/org unit under this tenant can browse and create activities from it
// through the exact same paths RTG-01's own standard set already uses (ListTemplateSets filters to
// status='published'; ROPA-05's CreateActivityFromTemplate is already set-type-agnostic).
func (s *Service) PublishTemplateSet(ctx context.Context, id uuid.UUID, rowVersion int32) (TemplateSet, error) {
	before, err := s.ownSet(ctx, id)
	if err != nil {
		return TemplateSet{}, err
	}
	if before.Status != "draft" {
		return TemplateSet{}, fmt.Errorf("%w: only a draft set can be published", ErrInvalid)
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).PublishTemplateSetRow(ctx, ropastore.PublishTemplateSetRowParams{ID: id, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return TemplateSet{}, ErrVersionMismatch
	}
	if err != nil {
		return TemplateSet{}, err
	}
	out := toTemplateSetFromPublish(row)
	return out, s.audit(ctx, "ropa.template_set.publish", id, map[string]any{"status": "draft"}, map[string]any{"status": "published"})
}

// RetireTemplateSet withdraws a custom set from use — a permanent status change, gated by
// ropa.template.delete at the handler (the same permanent-decision reasoning ORG-06/VEN-14/DPIA-03's own
// retire actions already use), not .update.
func (s *Service) RetireTemplateSet(ctx context.Context, id uuid.UUID, rowVersion int32) (TemplateSet, error) {
	before, err := s.ownSet(ctx, id)
	if err != nil {
		return TemplateSet{}, err
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).RetireTemplateSetRow(ctx, ropastore.RetireTemplateSetRowParams{ID: id, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return TemplateSet{}, ErrVersionMismatch
	}
	if err != nil {
		return TemplateSet{}, err
	}
	out := toTemplateSetFromRetire(row)
	return out, s.audit(ctx, "ropa.template_set.retire", id, map[string]any{"status": before.Status}, map[string]any{"status": "retired"})
}

// AddActivityTemplate adds one new activity to a tenant's own custom set.
func (s *Service) AddActivityTemplate(ctx context.Context, setID uuid.UUID, in ActivityTemplateInput) (ActivityTemplate, error) {
	if _, err := s.ownSet(ctx, setID); err != nil {
		return ActivityTemplate{}, err
	}
	in, err := in.normalize()
	if err != nil {
		return ActivityTemplate{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityTemplate{}, err
	}
	var row ropastore.InsertActivityTemplateRow
	if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = ropastore.New(pdb.MustTxFromContext(ctx)).InsertActivityTemplate(ctx, ropastore.InsertActivityTemplateParams{
			ID: id, TemplateSetID: setID, Code: in.Code, NameTh: in.NameTh, NameEn: optStr(in.NameEn),
			JobCategory: in.JobCategory, Role: in.Role, Defaults: in.Defaults, Rationale: in.Rationale, LegalRefs: in.LegalRefs,
		})
		return err
	}); err != nil {
		if isUnique(err) {
			return ActivityTemplate{}, fmt.Errorf("%w: code %q already exists in this set", ErrInvalid, in.Code)
		}
		return ActivityTemplate{}, err
	}
	out := toActivityTemplate(ropastore.GetActivityTemplateRow(row))
	return out, s.audit(ctx, "ropa.activity_template.create", id, nil, map[string]any{"template_set_id": setID, "code": in.Code})
}

// UpdateActivityTemplate edits one of a tenant's own custom activity templates (If-Match).
func (s *Service) UpdateActivityTemplate(ctx context.Context, id uuid.UUID, in ActivityTemplateInput, rowVersion int32) (ActivityTemplate, error) {
	before, err := s.GetActivityTemplate(ctx, id)
	if err != nil {
		return ActivityTemplate{}, err
	}
	if _, err := s.ownSet(ctx, before.TemplateSetID); err != nil {
		return ActivityTemplate{}, err
	}
	in, err = in.normalize()
	if err != nil {
		return ActivityTemplate{}, err
	}
	var row ropastore.UpdateActivityTemplateRowRow
	if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = ropastore.New(pdb.MustTxFromContext(ctx)).UpdateActivityTemplateRow(ctx, ropastore.UpdateActivityTemplateRowParams{
			ID: id, RowVersion: rowVersion, Code: in.Code, NameTh: in.NameTh, NameEn: optStr(in.NameEn),
			JobCategory: in.JobCategory, Role: in.Role, Defaults: in.Defaults, Rationale: in.Rationale, LegalRefs: in.LegalRefs,
		})
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActivityTemplate{}, ErrVersionMismatch
		}
		if isUnique(err) {
			return ActivityTemplate{}, fmt.Errorf("%w: code %q already exists in this set", ErrInvalid, in.Code)
		}
		return ActivityTemplate{}, err
	}
	out := toActivityTemplate(ropastore.GetActivityTemplateRow(row))
	return out, s.audit(ctx, "ropa.activity_template.update", id, map[string]any{"code": before.Code}, map[string]any{"code": in.Code})
}

// DeleteActivityTemplate removes one of a tenant's own custom activity templates (If-Match).
func (s *Service) DeleteActivityTemplate(ctx context.Context, id uuid.UUID, rowVersion int32) error {
	before, err := s.GetActivityTemplate(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.ownSet(ctx, before.TemplateSetID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityTemplateRow(ctx, ropastore.DeleteActivityTemplateRowParams{ID: id, RowVersion: rowVersion})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return s.audit(ctx, "ropa.activity_template.delete", id, map[string]any{"code": before.Code}, nil)
}

// ownSet proves a template set is one of this tenant's own editable custom sets (not a global
// standard/industry/government/processor set, which RLS's own tenant_write policy already refuses to
// touch — this check gives a clear ErrInvalid instead of a confusing "0 rows affected" on every write).
func (s *Service) ownSet(ctx context.Context, id uuid.UUID) (TemplateSet, error) {
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetOwnTemplateSet(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return TemplateSet{}, fmt.Errorf("%w: not an editable custom template set of this tenant", ErrInvalid)
	}
	if err != nil {
		return TemplateSet{}, err
	}
	return toTemplateSetFromOwn(row), nil
}

func toTemplateSetFromInsert(r ropastore.InsertTemplateSetRow) TemplateSet {
	return TemplateSet{ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo), Status: r.Status, RowVersion: r.RowVersion}
}

func toTemplateSetFromOwn(r ropastore.GetOwnTemplateSetRow) TemplateSet {
	return TemplateSet{ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo), Status: r.Status, RowVersion: r.RowVersion}
}

func toTemplateSetFromPublish(r ropastore.PublishTemplateSetRowRow) TemplateSet {
	return TemplateSet{ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo), Status: r.Status, RowVersion: r.RowVersion}
}

func toTemplateSetFromRetire(r ropastore.RetireTemplateSetRowRow) TemplateSet {
	return TemplateSet{ID: r.ID, Name: r.Name, SetType: r.SetType, Industry: derefStr(r.Industry), VersionNo: int(r.VersionNo), Status: r.Status, RowVersion: r.RowVersion}
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("ropa/templates: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: EntityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
