package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

const ActivityEntityType = "activity"

var activityRoles = []string{"controller", "processor"}
var activityDataSources = []string{"direct", "indirect"}
var recipientRoles = []string{"processor", "controller", "joint_controller", "government"}
var disposalMethods = []string{"delete", "destroy", "anonymize", "return"}
var volumeBands = []string{"lt_1k", "1k_10k", "10k_100k", "gt_100k"}

var ErrIncomplete = errors.New("ropa: activity is missing mandatory items")
var ErrInvalidTransition = errors.New("ropa: invalid state transition")

// IncompleteError carries the specific ม.39 items SubmitActivity refused to submit without —
// ToProblem in the http layer turns this into a 422 field-error list (same pattern as docs.IncompleteError).
type IncompleteError struct{ Missing []string }

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("%v: %s", ErrIncomplete, strings.Join(e.Missing, ", "))
}
func (e *IncompleteError) Unwrap() error { return ErrIncomplete }

// Activity is a RoPA processing activity (ROPA-03, ม.39): the core record OWNER/DPO fill in, plus a
// completeness score derived from its child rows (purposes, data, retention, recipients) and, on
// GetActivity, the itemized list of what's still missing (the acceptance criterion). Retention
// (ROPA-07), recipients/transfers (ROPA-08), security controls (ROPA-09) and DSAR rejections
// (ROPA-10) each get their own dedicated feature later; this pass gives them the minimal CRUD their
// tables already support and counts them toward completeness so nothing here blocks those features'
// own business rules from layering on top.
type Activity struct {
	ID                uuid.UUID
	LegalEntityID     uuid.UUID
	OrgUnitID         uuid.UUID
	Code              string
	Name              string
	Description       string
	Role              string // controller | processor
	ControllerPartyID *uuid.UUID
	OwnerUserID       *uuid.UUID
	RightsAndAccess   string
	Status            string // draft | pending_approval | active | under_review | ended (ST-05)
	Completeness      int
	MissingItems      []string
	RowVersion        int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ActivityCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type ActivityFilter struct {
	OrgUnitID *uuid.UUID
	Status    string
	Query     string
	After     *ActivityCursor
	Limit     int
}

const activityPageSize = 50

type ActivityPurpose struct {
	ID               uuid.UUID
	ActivityID       uuid.UUID
	PurposeID        *uuid.UUID
	PurposeText      string
	LawfulBasisCode  string
	ConsentPurposeID *uuid.UUID
	RowVersion       int32
	CreatedAt        time.Time
}

type ActivityData struct {
	ID             uuid.UUID
	ActivityID     uuid.UUID
	DataCategoryID uuid.UUID
	SubjectTypeID  uuid.UUID
	Source         string // direct | indirect
	SourcePartyID  *uuid.UUID
	IsSensitive    bool
	VolumeBand     string
	RowVersion     int32
	CreatedAt      time.Time
}

type RetentionRule struct {
	ID              uuid.UUID
	ActivityID      uuid.UUID
	DataCategoryID  *uuid.UUID
	RetentionMonths *int
	RetentionBasis  string
	TriggerEvent    string
	DisposalMethod  string
	RowVersion      int32
	CreatedAt       time.Time
}

type ActivityRecipient struct {
	ID              uuid.UUID
	ActivityID      uuid.UUID
	PartyID         uuid.UUID
	RecipientRole   string
	DisclosureBasis string
	DataCategoryIDs []uuid.UUID
	RowVersion      int32
	CreatedAt       time.Time
}

func (a *Activity) normalize() error {
	a.Code = strings.TrimSpace(a.Code)
	if a.Code == "" || len([]rune(a.Code)) > 40 {
		return fmt.Errorf("%w: code", ErrInvalid)
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len([]rune(a.Name)) > 300 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	a.Description = strings.TrimSpace(a.Description)
	if !slices.Contains(activityRoles, a.Role) {
		return fmt.Errorf("%w: role", ErrInvalid)
	}
	a.RightsAndAccess = strings.TrimSpace(a.RightsAndAccess)
	if len([]rune(a.RightsAndAccess)) > 4000 {
		return fmt.Errorf("%w: rights_and_access", ErrInvalid)
	}
	return nil
}

func (s *Service) ListActivities(ctx context.Context, f ActivityFilter) ([]Activity, *ActivityCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > activityPageSize {
		limit = activityPageSize
	}
	p := ropastore.ListActivitiesParams{Lim: int32(limit + 1)}
	p.OrgUnitID = pgUUID(f.OrgUnitID)
	if f.Status != "" {
		p.Status = &f.Status
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		p.Q = &esc
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivities(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Activity, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &ActivityCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toActivity(ropastore.GetActivityRow(r)))
	}
	return out, nil, nil
}

// GetActivity returns the activity with a freshly computed completeness score and missing-item list
// (the acceptance criterion) — computed from its current child rows, not just the stored column.
func (s *Service) GetActivity(ctx context.Context, id uuid.UUID) (Activity, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetActivity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	a := toActivity(r)
	score, missing, err := s.completeness(ctx, a)
	if err != nil {
		return Activity{}, err
	}
	a.Completeness, a.MissingItems = score, missing
	return a, nil
}

// SaveActivity creates (zero ID) or updates (with the If-Match version) a processing activity.
func (s *Service) SaveActivity(ctx context.Context, a Activity, version int32) (Activity, error) {
	if err := a.normalize(); err != nil {
		return Activity{}, err
	}
	if _, err := s.Org.GetLegalEntity(ctx, a.LegalEntityID); err != nil {
		return Activity{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	if _, err := s.Org.GetOrgUnit(ctx, a.OrgUnitID); err != nil {
		return Activity{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
	}
	if a.ControllerPartyID != nil {
		if _, err := s.Org.GetExternalParty(ctx, *a.ControllerPartyID); err != nil {
			return Activity{}, fmt.Errorf("%w: controller_party_id", ErrInvalid)
		}
	}
	if a.OwnerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*a.OwnerUserID})
		if err != nil {
			return Activity{}, err
		}
		if _, ok := names[*a.OwnerUserID]; !ok {
			return Activity{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}
	var before *Activity
	if a.ID != uuid.Nil {
		cur, err := s.GetActivity(ctx, a.ID)
		if err != nil {
			return Activity{}, err
		}
		if cur.RowVersion != version {
			return Activity{}, ErrVersionMismatch
		}
		before = &cur
	}
	isNew := a.ID == uuid.Nil
	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return Activity{}, err
		}
		a.ID = id
	}
	// In its own savepoint: a duplicate code hits the unique constraint, and without a savepoint that
	// error would abort the whole request transaction instead of just this write.
	err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := ropastore.New(pdb.MustTxFromContext(ctx))
		var err error
		if isNew {
			_, err = q.InsertActivity(ctx, ropastore.InsertActivityParams{ID: a.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
				Code: a.Code, Name: a.Name, Description: opt(a.Description), Role: a.Role, ControllerPartyID: pgUUID(a.ControllerPartyID),
				OwnerUserID: pgUUID(a.OwnerUserID), RightsAndAccess: opt(a.RightsAndAccess)})
		} else {
			_, err = q.UpdateActivity(ctx, ropastore.UpdateActivityParams{ID: a.ID, RowVersion: version, LegalEntityID: a.LegalEntityID,
				OrgUnitID: a.OrgUnitID, Code: a.Code, Name: a.Name, Description: opt(a.Description), Role: a.Role,
				ControllerPartyID: pgUUID(a.ControllerPartyID), OwnerUserID: pgUUID(a.OwnerUserID), RightsAndAccess: opt(a.RightsAndAccess)})
		}
		return err
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return Activity{}, fmt.Errorf("%w: code", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return Activity{}, ErrVersionMismatch
	case err != nil:
		return Activity{}, err
	}
	out, err := s.GetActivity(ctx, a.ID)
	if err != nil {
		return Activity{}, err
	}
	action, b := "ropa.activity.create", any(nil)
	if !isNew {
		action, b = "ropa.activity.update", activityAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, activityAudit(out))
}

// SubmitActivity moves a draft (or returned-for-review) activity to pending_approval (ST-05) —
// refused with the itemized missing list when the ม.39 mandatory items aren't all in place (BP-05
// rule 1) or sensitive data lacks evidence of explicit consent (BP-05 rule 2). Approval itself
// (pending_approval -> active) is ROPA-13's job (versioning & approval, PLT-08).
func (s *Service) SubmitActivity(ctx context.Context, id uuid.UUID, version int32) (Activity, error) {
	a, err := s.GetActivity(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if a.RowVersion != version {
		return Activity{}, ErrVersionMismatch
	}
	if a.Status != "draft" && a.Status != "under_review" {
		return Activity{}, ErrInvalidTransition
	}
	if len(a.MissingItems) > 0 {
		return Activity{}, &IncompleteError{Missing: a.MissingItems}
	}
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).SubmitActivity(ctx, ropastore.SubmitActivityParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Activity{}, err
	}
	out := toActivity(ropastore.GetActivityRow(r))
	out.Completeness, out.MissingItems = a.Completeness, a.MissingItems
	return out, s.audit(ctx, "ropa.activity.submit", id, map[string]any{"status": a.Status}, map[string]any{"status": out.Status})
}

// completeness checks the ม.39 mandatory items against the activity's current child rows. Pure (no
// write) so a plain read (GetActivity) never disturbs row_version — the trigger that maintains it
// fires on any UPDATE, including one that only touches the completeness column. recompute wraps this
// with the actual persistence for mutation paths (ListActivities' badge reads that persisted column).
func (s *Service) completeness(ctx context.Context, a Activity) (int, []string, error) {
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	purposes, err := q.ListActivityPurposes(ctx, a.ID)
	if err != nil {
		return 0, nil, err
	}
	data, err := q.ListActivityData(ctx, a.ID)
	if err != nil {
		return 0, nil, err
	}
	retention, err := q.ListRetentionRules(ctx, a.ID)
	if err != nil {
		return 0, nil, err
	}
	recipients, err := q.ListActivityRecipients(ctx, a.ID)
	if err != nil {
		return 0, nil, err
	}

	var missing []string
	checks, satisfied := 0, 0

	checks++
	if len(data) == 0 {
		missing = append(missing, "data")
	} else {
		satisfied++
	}

	checks++
	if len(purposes) == 0 {
		missing = append(missing, "purpose")
	} else {
		satisfied++
	}

	checks++
	if a.Role == "processor" && a.ControllerPartyID == nil {
		missing = append(missing, "controller")
	} else {
		satisfied++
	}

	checks++
	if len(retention) == 0 {
		missing = append(missing, "retention")
	} else {
		satisfied++
	}

	checks++
	if a.RightsAndAccess == "" {
		missing = append(missing, "rights_access")
	} else {
		satisfied++
	}

	// Conditional items — legally required when applicable, but their absence (no external
	// recipients at all, no sensitive data) is itself a valid state, so they don't count toward
	// the fixed 5-item denominator above; they still block submission when they do apply.
	for _, r := range recipients {
		if r.DisclosureBasis == nil || strings.TrimSpace(*r.DisclosureBasis) == "" {
			missing = append(missing, "recipient_basis")
			break
		}
	}
	sensitive := false
	for _, d := range data {
		if d.IsSensitive {
			sensitive = true
			break
		}
	}
	if sensitive {
		hasConsent := false
		for _, p := range purposes {
			if p.ConsentPurposeID.Valid {
				hasConsent = true
				break
			}
		}
		if !hasConsent {
			missing = append(missing, "sensitive_consent")
		}
	}

	// ROPA-08 (ม.28/29): a recipient outside Thailand is a cross-border transfer that needs its own
	// logged mechanism (ropa.activity_transfers) — one uncovered foreign recipient is enough to warn.
	if len(recipients) > 0 {
		transfers, err := q.ListActivityTransfers(ctx, a.ID)
		if err != nil {
			return 0, nil, err
		}
		covered := make(map[uuid.UUID]bool, len(transfers))
		for _, t := range transfers {
			if t.RecipientID.Valid {
				covered[uuid.UUID(t.RecipientID.Bytes)] = true
			}
		}
		for _, r := range recipients {
			party, err := s.Org.GetExternalParty(ctx, r.PartyID)
			if err != nil {
				return 0, nil, err
			}
			if party.CountryCode != "" && party.CountryCode != "TH" && !covered[r.ID] {
				missing = append(missing, "transfer_basis")
				break
			}
		}
	}

	score := int(math.Round(100 * float64(satisfied) / float64(checks)))
	return score, missing, nil
}

// --- Purposes ---

func (p *ActivityPurpose) normalize() error {
	p.PurposeText = strings.TrimSpace(p.PurposeText)
	if p.PurposeText == "" {
		return fmt.Errorf("%w: purpose_text", ErrInvalid)
	}
	p.LawfulBasisCode = strings.TrimSpace(p.LawfulBasisCode)
	if p.LawfulBasisCode == "" {
		return fmt.Errorf("%w: lawful_basis_code", ErrInvalid)
	}
	return nil
}

func (s *Service) ListActivityPurposes(ctx context.Context, activityID uuid.UUID) ([]ActivityPurpose, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityPurposes(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityPurpose, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityPurpose(ropastore.InsertActivityPurposeRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityPurpose(ctx context.Context, p ActivityPurpose) (ActivityPurpose, error) {
	if err := p.normalize(); err != nil {
		return ActivityPurpose{}, err
	}
	if _, err := s.mustActivity(ctx, p.ActivityID); err != nil {
		return ActivityPurpose{}, err
	}
	if p.PurposeID != nil {
		if _, err := s.Org.GetMaster(ctx, "processing_purposes", *p.PurposeID); err != nil {
			return ActivityPurpose{}, fmt.Errorf("%w: purpose_id", ErrInvalid)
		}
	}
	basis, err := s.validLawfulBasis(ctx, p.LawfulBasisCode)
	if err != nil {
		return ActivityPurpose{}, err
	}
	// ROPA-06: a purpose relying on the consent lawful basis must already point at a real Purpose
	// in the consent module before it can be saved at all — not just flagged at submit time.
	if basis.RequiresConsent && p.ConsentPurposeID == nil {
		return ActivityPurpose{}, fmt.Errorf("%w: consent_purpose_id", ErrInvalid)
	}
	if p.ConsentPurposeID != nil {
		if s.Consent == nil {
			return ActivityPurpose{}, fmt.Errorf("%w: consent_purpose_id", ErrInvalid)
		}
		if _, err := s.Consent.GetPurpose(ctx, *p.ConsentPurposeID); err != nil {
			return ActivityPurpose{}, fmt.Errorf("%w: consent_purpose_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityPurpose{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	r, err := q.InsertActivityPurpose(ctx, ropastore.InsertActivityPurposeParams{ID: id, ActivityID: p.ActivityID, PurposeID: pgUUID(p.PurposeID),
		PurposeText: p.PurposeText, LawfulBasisCode: p.LawfulBasisCode, ConsentPurposeID: pgUUID(p.ConsentPurposeID)})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ActivityPurpose{}, fmt.Errorf("%w: lawful_basis_code", ErrInvalid)
		}
		return ActivityPurpose{}, err
	}
	out := toActivityPurpose(r)
	if _, _, err := s.recompute(ctx, p.ActivityID); err != nil {
		return ActivityPurpose{}, err
	}
	return out, s.audit(ctx, "ropa.activity_purpose.create", p.ActivityID, nil, purposeAudit(out))
}

func (s *Service) DeleteActivityPurpose(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityPurpose(ctx, ropastore.DeleteActivityPurposeParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_purpose.delete", activityID, map[string]any{"id": id}, nil)
}

// --- Data ---

func (d *ActivityData) normalize() error {
	if d.DataCategoryID == uuid.Nil {
		return fmt.Errorf("%w: data_category_id", ErrInvalid)
	}
	if d.SubjectTypeID == uuid.Nil {
		return fmt.Errorf("%w: subject_type_id", ErrInvalid)
	}
	if !slices.Contains(activityDataSources, d.Source) {
		return fmt.Errorf("%w: source", ErrInvalid)
	}
	if d.VolumeBand != "" && !slices.Contains(volumeBands, d.VolumeBand) {
		return fmt.Errorf("%w: volume_band", ErrInvalid)
	}
	return nil
}

func (s *Service) ListActivityData(ctx context.Context, activityID uuid.UUID) ([]ActivityData, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityData(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityData, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityData(ropastore.InsertActivityDataRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityData(ctx context.Context, d ActivityData) (ActivityData, error) {
	if err := d.normalize(); err != nil {
		return ActivityData{}, err
	}
	if _, err := s.mustActivity(ctx, d.ActivityID); err != nil {
		return ActivityData{}, err
	}
	category, err := s.Org.GetMaster(ctx, "data_categories", d.DataCategoryID)
	if err != nil {
		return ActivityData{}, fmt.Errorf("%w: data_category_id", ErrInvalid)
	}
	if _, err := s.Org.GetMaster(ctx, "data_subject_types", d.SubjectTypeID); err != nil {
		return ActivityData{}, fmt.Errorf("%w: subject_type_id", ErrInvalid)
	}
	if d.SourcePartyID != nil {
		if _, err := s.Org.GetExternalParty(ctx, *d.SourcePartyID); err != nil {
			return ActivityData{}, fmt.Errorf("%w: source_party_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityData{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	r, err := q.InsertActivityData(ctx, ropastore.InsertActivityDataParams{ID: id, ActivityID: d.ActivityID, DataCategoryID: d.DataCategoryID,
		SubjectTypeID: d.SubjectTypeID, Source: d.Source, SourcePartyID: pgUUID(d.SourcePartyID), IsSensitive: category.IsSensitive,
		VolumeBand: opt(d.VolumeBand)})
	if err != nil {
		return ActivityData{}, err
	}
	out := toActivityData(r)
	if _, _, err := s.recompute(ctx, d.ActivityID); err != nil {
		return ActivityData{}, err
	}
	return out, s.audit(ctx, "ropa.activity_data.create", d.ActivityID, nil, dataAudit(out))
}

func (s *Service) DeleteActivityData(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityData(ctx, ropastore.DeleteActivityDataParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_data.delete", activityID, map[string]any{"id": id}, nil)
}

// --- Retention rules ---

func (r *RetentionRule) normalize() error {
	r.RetentionBasis = strings.TrimSpace(r.RetentionBasis)
	if r.RetentionBasis == "" {
		return fmt.Errorf("%w: retention_basis", ErrInvalid)
	}
	r.TriggerEvent = strings.TrimSpace(r.TriggerEvent)
	if r.TriggerEvent == "" || len([]rune(r.TriggerEvent)) > 60 {
		return fmt.Errorf("%w: trigger_event", ErrInvalid)
	}
	if !slices.Contains(disposalMethods, r.DisposalMethod) {
		return fmt.Errorf("%w: disposal_method", ErrInvalid)
	}
	if r.RetentionMonths != nil && *r.RetentionMonths <= 0 {
		return fmt.Errorf("%w: retention_months", ErrInvalid)
	}
	return nil
}

func (s *Service) ListRetentionRules(ctx context.Context, activityID uuid.UUID) ([]RetentionRule, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListRetentionRules(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]RetentionRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRetentionRule(ropastore.InsertRetentionRuleRow(r)))
	}
	return out, nil
}

func (s *Service) AddRetentionRule(ctx context.Context, r RetentionRule) (RetentionRule, error) {
	if err := r.normalize(); err != nil {
		return RetentionRule{}, err
	}
	if _, err := s.mustActivity(ctx, r.ActivityID); err != nil {
		return RetentionRule{}, err
	}
	if r.DataCategoryID != nil {
		if _, err := s.Org.GetMaster(ctx, "data_categories", *r.DataCategoryID); err != nil {
			return RetentionRule{}, fmt.Errorf("%w: data_category_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return RetentionRule{}, err
	}
	var months *int32
	if r.RetentionMonths != nil {
		m := int32(*r.RetentionMonths)
		months = &m
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertRetentionRule(ctx, ropastore.InsertRetentionRuleParams{ID: id, ActivityID: r.ActivityID, DataCategoryID: pgUUID(r.DataCategoryID),
		RetentionMonths: months, RetentionBasis: r.RetentionBasis, TriggerEvent: r.TriggerEvent, DisposalMethod: r.DisposalMethod})
	if err != nil {
		return RetentionRule{}, err
	}
	out := toRetentionRule(row)
	if _, _, err := s.recompute(ctx, r.ActivityID); err != nil {
		return RetentionRule{}, err
	}
	return out, s.audit(ctx, "ropa.retention_rule.create", r.ActivityID, nil, retentionAudit(out))
}

func (s *Service) DeleteRetentionRule(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteRetentionRule(ctx, ropastore.DeleteRetentionRuleParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.retention_rule.delete", activityID, map[string]any{"id": id}, nil)
}

// --- Recipients ---

func (r *ActivityRecipient) normalize() error {
	if !slices.Contains(recipientRoles, r.RecipientRole) {
		return fmt.Errorf("%w: recipient_role", ErrInvalid)
	}
	r.DisclosureBasis = strings.TrimSpace(r.DisclosureBasis)
	if r.DataCategoryIDs == nil {
		r.DataCategoryIDs = []uuid.UUID{}
	}
	return nil
}

func (s *Service) ListActivityRecipients(ctx context.Context, activityID uuid.UUID) ([]ActivityRecipient, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityRecipients(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityRecipient, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityRecipient(ropastore.InsertActivityRecipientRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityRecipient(ctx context.Context, r ActivityRecipient) (ActivityRecipient, error) {
	if err := r.normalize(); err != nil {
		return ActivityRecipient{}, err
	}
	if _, err := s.mustActivity(ctx, r.ActivityID); err != nil {
		return ActivityRecipient{}, err
	}
	if _, err := s.Org.GetExternalParty(ctx, r.PartyID); err != nil {
		return ActivityRecipient{}, fmt.Errorf("%w: party_id", ErrInvalid)
	}
	for _, cid := range r.DataCategoryIDs {
		if _, err := s.Org.GetMaster(ctx, "data_categories", cid); err != nil {
			return ActivityRecipient{}, fmt.Errorf("%w: data_category_ids", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityRecipient{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertActivityRecipient(ctx, ropastore.InsertActivityRecipientParams{ID: id, ActivityID: r.ActivityID, PartyID: r.PartyID,
		RecipientRole: r.RecipientRole, DisclosureBasis: opt(r.DisclosureBasis), DataCategoryIds: r.DataCategoryIDs})
	if err != nil {
		return ActivityRecipient{}, err
	}
	out := toActivityRecipient(row)
	if _, _, err := s.recompute(ctx, r.ActivityID); err != nil {
		return ActivityRecipient{}, err
	}
	return out, s.audit(ctx, "ropa.activity_recipient.create", r.ActivityID, nil, recipientAudit(out))
}

func (s *Service) DeleteActivityRecipient(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityRecipient(ctx, ropastore.DeleteActivityRecipientParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_recipient.delete", activityID, map[string]any{"id": id}, nil)
}

// --- shared helpers ---

func (s *Service) mustActivity(ctx context.Context, id uuid.UUID) (Activity, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetActivity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	return toActivity(r), nil
}

func (s *Service) recompute(ctx context.Context, activityID uuid.UUID) (int, []string, error) {
	a, err := s.mustActivity(ctx, activityID)
	if err != nil {
		return 0, nil, err
	}
	return s.completeness(ctx, a)
}

// validLawfulBasis checks a code against ORG-07's small lawful-bases list — that master-data kind is
// keyed by code, not id, so unlike GetMaster's by-id lookups this scans ListMaster's ~13 rows.
func (s *Service) validLawfulBasis(ctx context.Context, code string) (orgservice.MasterItem, error) {
	items, err := s.Org.ListMaster(ctx, "lawful_bases")
	if err != nil {
		return orgservice.MasterItem{}, err
	}
	for _, it := range items {
		if it.Code == code {
			return it, nil
		}
	}
	return orgservice.MasterItem{}, fmt.Errorf("%w: lawful_basis_code", ErrInvalid)
}

func activityAudit(a Activity) map[string]any {
	return map[string]any{"code": a.Code, "name": a.Name, "role": a.Role, "legal_entity_id": a.LegalEntityID, "org_unit_id": a.OrgUnitID,
		"controller_party_id": a.ControllerPartyID, "owner_user_id": a.OwnerUserID, "status": a.Status}
}

func purposeAudit(p ActivityPurpose) map[string]any {
	return map[string]any{"id": p.ID, "purpose_id": p.PurposeID, "lawful_basis_code": p.LawfulBasisCode, "consent_purpose_id": p.ConsentPurposeID}
}

func dataAudit(d ActivityData) map[string]any {
	return map[string]any{"id": d.ID, "data_category_id": d.DataCategoryID, "subject_type_id": d.SubjectTypeID, "is_sensitive": d.IsSensitive}
}

func retentionAudit(r RetentionRule) map[string]any {
	return map[string]any{"id": r.ID, "data_category_id": r.DataCategoryID, "disposal_method": r.DisposalMethod}
}

func recipientAudit(r ActivityRecipient) map[string]any {
	return map[string]any{"id": r.ID, "party_id": r.PartyID, "recipient_role": r.RecipientRole}
}

func toActivity(r ropastore.GetActivityRow) Activity {
	return Activity{ID: r.ID, LegalEntityID: r.LegalEntityID, OrgUnitID: r.OrgUnitID, Code: r.Code, Name: r.Name,
		Description: deref(r.Description), Role: r.Role, ControllerPartyID: uuidPtr(r.ControllerPartyID), OwnerUserID: uuidPtr(r.OwnerUserID),
		Status: r.Status, Completeness: int(r.Completeness), RightsAndAccess: deref(r.RightsAndAccess), RowVersion: r.RowVersion,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func toActivityPurpose(r ropastore.InsertActivityPurposeRow) ActivityPurpose {
	return ActivityPurpose{ID: r.ID, ActivityID: r.ActivityID, PurposeID: uuidPtr(r.PurposeID), PurposeText: r.PurposeText,
		LawfulBasisCode: r.LawfulBasisCode, ConsentPurposeID: uuidPtr(r.ConsentPurposeID), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
}

func toActivityData(r ropastore.InsertActivityDataRow) ActivityData {
	return ActivityData{ID: r.ID, ActivityID: r.ActivityID, DataCategoryID: r.DataCategoryID, SubjectTypeID: r.SubjectTypeID, Source: r.Source,
		SourcePartyID: uuidPtr(r.SourcePartyID), IsSensitive: r.IsSensitive, VolumeBand: deref(r.VolumeBand), RowVersion: r.RowVersion,
		CreatedAt: r.CreatedAt.Time}
}

func toRetentionRule(r ropastore.InsertRetentionRuleRow) RetentionRule {
	var months *int
	if r.RetentionMonths != nil {
		m := int(*r.RetentionMonths)
		months = &m
	}
	return RetentionRule{ID: r.ID, ActivityID: r.ActivityID, DataCategoryID: uuidPtr(r.DataCategoryID), RetentionMonths: months,
		RetentionBasis: r.RetentionBasis, TriggerEvent: r.TriggerEvent, DisposalMethod: r.DisposalMethod, RowVersion: r.RowVersion,
		CreatedAt: r.CreatedAt.Time}
}

func toActivityRecipient(r ropastore.InsertActivityRecipientRow) ActivityRecipient {
	ids := r.DataCategoryIds
	if ids == nil {
		ids = []uuid.UUID{}
	}
	return ActivityRecipient{ID: r.ID, ActivityID: r.ActivityID, PartyID: r.PartyID, RecipientRole: r.RecipientRole,
		DisclosureBasis: deref(r.DisclosureBasis), DataCategoryIDs: ids, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
}
