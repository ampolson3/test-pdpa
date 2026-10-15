package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ActivitySnapshot is what ropa.activity_versions freezes on every approval — the core record plus every
// child row, keyed the same way this module's own completeness() and HTTP layer already read them. Kept
// as a plain exported struct (not a wire type) so both the version-write path and the diff both use the
// exact same field names.
type ActivitySnapshot struct {
	Code              string              `json:"code"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	Role              string              `json:"role"`
	ControllerPartyID *uuid.UUID          `json:"controller_party_id,omitempty"`
	OwnerUserID       *uuid.UUID          `json:"owner_user_id,omitempty"`
	RightsAndAccess   string              `json:"rights_and_access"`
	Purposes          []ActivityPurpose   `json:"purposes"`
	Data              []ActivityData      `json:"data"`
	Retention         []RetentionRule     `json:"retention"`
	Recipients        []ActivityRecipient `json:"recipients"`
	Transfers         []ActivityTransfer  `json:"transfers"`
	Controls          []ActivityControl   `json:"controls"`
}

// ActivityVersion is one approved round of a processing activity (ROPA-13's own version history).
type ActivityVersion struct {
	ID         uuid.UUID
	ActivityID uuid.UUID
	VersionNo  int32
	Snapshot   ActivitySnapshot
	ApprovedBy *uuid.UUID
	ApprovedAt time.Time
	CreatedAt  time.Time
}

// buildSnapshot composes the current state of an activity's core fields and every child table — the same
// reads completeness() already does, in one place so the version-write path and any future export share
// one definition of "what a round of this activity actually contains".
func (s *Service) buildSnapshot(ctx context.Context, a Activity) (ActivitySnapshot, error) {
	purposes, err := s.ListActivityPurposes(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	data, err := s.ListActivityData(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	retention, err := s.ListRetentionRules(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	recipients, err := s.ListActivityRecipients(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	transfers, err := s.ListActivityTransfers(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	controls, err := s.ListActivityControls(ctx, a.ID)
	if err != nil {
		return ActivitySnapshot{}, err
	}
	return ActivitySnapshot{
		Code: a.Code, Name: a.Name, Description: a.Description, Role: a.Role,
		ControllerPartyID: a.ControllerPartyID, OwnerUserID: a.OwnerUserID, RightsAndAccess: a.RightsAndAccess,
		Purposes: purposes, Data: data, Retention: retention, Recipients: recipients, Transfers: transfers, Controls: controls,
	}, nil
}

// ApproveActivity moves pending_approval -> active (ST-05, "DPO อนุมัติ") and freezes a new
// ropa.activity_versions row from the activity's current state — the acceptance criterion itself ("เก็บ
// ประวัติการแก้ไข"): from this point the activity is active and SaveActivity refuses to touch it directly
// again until RequestActivityReview reopens it.
func (s *Service) ApproveActivity(ctx context.Context, id uuid.UUID, version int32) (Activity, error) {
	before, err := s.GetActivity(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if before.RowVersion != version {
		return Activity{}, ErrVersionMismatch
	}
	if before.Status != "pending_approval" {
		return Activity{}, ErrInvalidTransition
	}
	snap, err := s.buildSnapshot(ctx, before)
	if err != nil {
		return Activity{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	r, err := q.ApproveActivity(ctx, ropastore.ApproveActivityParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Activity{}, err
	}
	out := toActivity(ropastore.GetActivityRow(r))
	out.Completeness, out.MissingItems = before.Completeness, before.MissingItems

	next, err := q.MaxActivityVersionNo(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return Activity{}, err
	}
	vid, err := uuid.NewV7()
	if err != nil {
		return Activity{}, err
	}
	if _, err := q.InsertActivityVersion(ctx, ropastore.InsertActivityVersionParams{ID: vid, ActivityID: id, VersionNo: next + 1, Snapshot: raw}); err != nil {
		return Activity{}, err
	}
	// ROPA-15: every approval restarts the review clock from today, review_cycle_months out.
	cycle, err := q.GetActivityReviewCycle(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if err := q.SetActivityNextReview(ctx, ropastore.SetActivityNextReviewParams{
		ID: id, NextReviewAt: pgtype.Date{Time: s.now().AddDate(0, int(cycle), 0), Valid: true},
	}); err != nil {
		return Activity{}, err
	}
	return out, s.audit(ctx, "ropa.activity.approve", id, map[string]any{"status": before.Status}, map[string]any{"status": out.Status, "version_no": next + 1})
}

// ReturnActivity sends a pending_approval activity back to draft (ST-05, "ส่งกลับ"); reason is kept only
// in the audit row, the same way breach's own aware_at change reason is — the table itself has no column
// for it.
func (s *Service) ReturnActivity(ctx context.Context, id uuid.UUID, version int32, reason string) (Activity, error) {
	before, err := s.GetActivity(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if before.RowVersion != version {
		return Activity{}, ErrVersionMismatch
	}
	if before.Status != "pending_approval" {
		return Activity{}, ErrInvalidTransition
	}
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).ReturnActivity(ctx, ropastore.ReturnActivityParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Activity{}, err
	}
	out := toActivity(ropastore.GetActivityRow(r))
	out.Completeness, out.MissingItems = before.Completeness, before.MissingItems
	return out, s.audit(ctx, "ropa.activity.return", id, map[string]any{"status": before.Status}, map[string]any{"status": out.Status, "reason": reason})
}

// RequestActivityReview opens an active activity for editing again (ST-05, active -> under_review) — the
// only way to change an approved activity, per this feature's own acceptance criterion; SaveActivity
// itself refuses direct edits outside draft/under_review.
func (s *Service) RequestActivityReview(ctx context.Context, id uuid.UUID, version int32) (Activity, error) {
	before, err := s.GetActivity(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if before.RowVersion != version {
		return Activity{}, ErrVersionMismatch
	}
	if before.Status != "active" {
		return Activity{}, ErrInvalidTransition
	}
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).RequestActivityReview(ctx, ropastore.RequestActivityReviewParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Activity{}, err
	}
	out := toActivity(ropastore.GetActivityRow(r))
	out.Completeness, out.MissingItems = before.Completeness, before.MissingItems
	return out, s.audit(ctx, "ropa.activity.request_review", id, map[string]any{"status": before.Status}, map[string]any{"status": out.Status})
}

// ConfirmActivityNoChange closes a review round with nothing to re-approve (ST-05, under_review ->
// active, "ยืนยันไม่เปลี่ยน") — no new ropa.activity_versions row, since nothing actually changed.
func (s *Service) ConfirmActivityNoChange(ctx context.Context, id uuid.UUID, version int32) (Activity, error) {
	before, err := s.GetActivity(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if before.RowVersion != version {
		return Activity{}, ErrVersionMismatch
	}
	if before.Status != "under_review" {
		return Activity{}, ErrInvalidTransition
	}
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).ConfirmActivityNoChange(ctx, ropastore.ConfirmActivityNoChangeParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Activity{}, err
	}
	out := toActivity(ropastore.GetActivityRow(r))
	out.Completeness, out.MissingItems = before.Completeness, before.MissingItems
	return out, s.audit(ctx, "ropa.activity.confirm_no_change", id, map[string]any{"status": before.Status}, map[string]any{"status": out.Status})
}

// ListActivityVersions returns an activity's approved rounds, newest first.
func (s *Service) ListActivityVersions(ctx context.Context, activityID uuid.UUID) ([]ActivityVersion, error) {
	if _, err := s.GetActivity(ctx, activityID); err != nil {
		return nil, err
	}
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityVersions(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityVersion, 0, len(rows))
	for _, r := range rows {
		v, err := toActivityVersion(ropastore.GetActivityVersionRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ActivityVersionDiff is a named section (purposes/data/retention/recipients/transfers/controls/core) whose
// content differs between two approved rounds.
type ActivityVersionDiff struct {
	Section string
	Before  any // nil when the section didn't exist in the "from" version
	After   any // nil when the section didn't exist in the "to" version
}

// CompareActivityVersions diffs two of an activity's approved rounds, section by section (the acceptance
// criterion's "เปรียบเทียบเวอร์ชัน") — the same reflect.DeepEqual-per-field approach DPIA-14's own
// CompareToPrevious already established, just per named section here instead of per question.
func (s *Service) CompareActivityVersions(ctx context.Context, fromID, toID uuid.UUID) ([]ActivityVersionDiff, error) {
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	fromRow, err := q.GetActivityVersion(ctx, fromID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	toRow, err := q.GetActivityVersion(ctx, toID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Two-tenant isolation (rule 1) comes from RLS on the SELECTs above — a cross-tenant id reads as
	// ErrNotFound already, the same as GetActivity's own lookups.
	from, err := toActivityVersion(ropastore.GetActivityVersionRow(fromRow))
	if err != nil {
		return nil, err
	}
	to, err := toActivityVersion(ropastore.GetActivityVersionRow(toRow))
	if err != nil {
		return nil, err
	}

	fb, err := snapshotSections(from.Snapshot)
	if err != nil {
		return nil, err
	}
	tb, err := snapshotSections(to.Snapshot)
	if err != nil {
		return nil, err
	}

	sections := make(map[string]struct{}, len(fb)+len(tb))
	for k := range fb {
		sections[k] = struct{}{}
	}
	for k := range tb {
		sections[k] = struct{}{}
	}
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []ActivityVersionDiff
	for _, k := range keys {
		b, hasB := fb[k]
		a, hasA := tb[k]
		if hasB && hasA && reflect.DeepEqual(b, a) {
			continue
		}
		d := ActivityVersionDiff{Section: k}
		if hasB {
			d.Before = b
		}
		if hasA {
			d.After = a
		}
		out = append(out, d)
	}
	return out, nil
}

// snapshotSections decodes a stored snapshot into named sections for a generic field-by-field diff —
// json round-tripping (rather than reflect on the struct directly) keeps this independent of
// ActivitySnapshot's exact Go layout, the same indirection DPIA-14's own forms.Contribution decode uses.
func snapshotSections(s ActivitySnapshot) (map[string]any, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func toActivityVersion(r ropastore.GetActivityVersionRow) (ActivityVersion, error) {
	var snap ActivitySnapshot
	if err := json.Unmarshal(r.Snapshot, &snap); err != nil {
		return ActivityVersion{}, err
	}
	v := ActivityVersion{ID: r.ID, ActivityID: r.ActivityID, VersionNo: r.VersionNo, Snapshot: snap, CreatedAt: r.CreatedAt.Time}
	if r.ApprovedBy.Valid {
		id := uuid.UUID(r.ApprovedBy.Bytes)
		v.ApprovedBy = &id
	}
	if r.ApprovedAt.Valid {
		v.ApprovedAt = r.ApprovedAt.Time
	}
	return v, nil
}
