package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	breachstore "pdpa-platform/internal/breach/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// RootCauseEntityType is this feature's own audit entity type for breach.root_causes.
const RootCauseEntityType = "breach_root_cause"

// CauseCategories are the fixed root-cause buckets BP-07's own "หมวดสาเหตุ" names — a tunable classification,
// not a legal value, so no docs/decisions.md entry: human_error, process_gap, technical_control,
// third_party, malicious.
var CauseCategories = []string{"human_error", "process_gap", "technical_control", "third_party", "malicious"}

// CorrectiveAction is one item of breach.root_causes.corrective_actions (jsonb array) — the acceptance
// criterion itself tracks each one from open to done.
type CorrectiveAction struct {
	ID             string     `json:"id"`
	Description    string     `json:"description"`
	AssigneeUserID *uuid.UUID `json:"assignee_user_id,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	Status         string     `json:"status"` // open | done
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// RootCause is one breach.root_causes row — BP-07's "วิเคราะห์สาเหตุและบทเรียน" (BRE-15).
type RootCause struct {
	ID                uuid.UUID
	IncidentID        uuid.UUID
	CauseCategory     string
	Description       string
	CorrectiveActions []CorrectiveAction
	RiskID            *uuid.UUID
	RowVersion        int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AllActionsDone is the acceptance criterion's own completion check — true once every corrective action
// (if any) reads "done". A root cause with no corrective actions yet is not considered done — there is
// nothing on record to have tracked to completion.
func (r RootCause) AllActionsDone() bool {
	if len(r.CorrectiveActions) == 0 {
		return false
	}
	for _, a := range r.CorrectiveActions {
		if a.Status != "done" {
			return false
		}
	}
	return true
}

func decodeCorrectiveActions(raw []byte) ([]CorrectiveAction, error) {
	var out []CorrectiveAction
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// rootCauseRow is the shape every root_causes.sql query's own Row type shares (sqlc generates one named
// type per query, so this is filled by hand per call site rather than a type conversion).
type rootCauseRow struct {
	ID                uuid.UUID
	IncidentID        uuid.UUID
	CauseCategory     string
	Description       string
	CorrectiveActions []byte
	RiskID            pgtype.UUID
	RowVersion        int32
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
}

func toRootCause(r rootCauseRow) (RootCause, error) {
	actions, err := decodeCorrectiveActions(r.CorrectiveActions)
	if err != nil {
		return RootCause{}, err
	}
	out := RootCause{
		ID: r.ID, IncidentID: r.IncidentID, CauseCategory: r.CauseCategory, Description: r.Description,
		CorrectiveActions: actions, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.RiskID.Valid {
		id := uuid.UUID(r.RiskID.Bytes)
		out.RiskID = &id
	}
	return out, nil
}

func validateRootCause(causeCategory, description string, actions []CorrectiveAction) error {
	var errs []FieldError
	if !slicesContains(CauseCategories, causeCategory) {
		errs = append(errs, FieldError{"cause_category", "invalid"})
	}
	if strings.TrimSpace(description) == "" {
		errs = append(errs, FieldError{"description", "required"})
	}
	for i, a := range actions {
		if strings.TrimSpace(a.Description) == "" {
			errs = append(errs, FieldError{"corrective_actions[" + strconv.Itoa(i) + "].description", "required"})
		}
		if a.Status != "open" && a.Status != "done" {
			errs = append(errs, FieldError{"corrective_actions[" + strconv.Itoa(i) + "].status", "invalid"})
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

func slicesContains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// assignActionIDs fills in a fresh id for any corrective action the caller submitted without one (new
// items), leaving existing ids (already-tracked actions) untouched.
func assignActionIDs(actions []CorrectiveAction) ([]CorrectiveAction, error) {
	out := make([]CorrectiveAction, len(actions))
	for i, a := range actions {
		if strings.TrimSpace(a.ID) == "" {
			id, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			a.ID = id.String()
		}
		if a.Status == "" {
			a.Status = "open"
		}
		out[i] = a
	}
	return out, nil
}

// checkRiskVisible is the FK-visibility check (rule 1) for an optional link to an existing risk register
// entry — risk_id bypasses RLS as a plain FK, so it must be proven visible under the caller's own RLS first.
func (s *Service) checkRiskVisible(ctx context.Context, riskID *uuid.UUID) error {
	if riskID == nil {
		return nil
	}
	if s.Risk == nil {
		return ErrInvalidRequest
	}
	if _, err := s.Risk.GetRisk(ctx, *riskID); err != nil {
		return ErrInvalidRequest
	}
	return nil
}

// AddRootCause records one root cause + its corrective actions against an incident (BRE-15).
func (s *Service) AddRootCause(ctx context.Context, incidentID uuid.UUID, causeCategory, description string,
	actions []CorrectiveAction, riskID *uuid.UUID) (RootCause, error) {
	if !has(ctx, PermUpdate) {
		return RootCause{}, ErrForbidden
	}
	if _, err := s.Get(ctx, incidentID); err != nil {
		return RootCause{}, err
	}
	if err := validateRootCause(causeCategory, description, actions); err != nil {
		return RootCause{}, err
	}
	if err := s.checkRiskVisible(ctx, riskID); err != nil {
		return RootCause{}, err
	}
	actions, err := assignActionIDs(actions)
	if err != nil {
		return RootCause{}, err
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return RootCause{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return RootCause{}, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertRootCause(ctx, breachstore.InsertRootCauseParams{
		ID: id, IncidentID: incidentID, CauseCategory: causeCategory, Description: description,
		CorrectiveActions: actionsJSON, RiskID: pgUUIDPtr(riskID),
	})
	if err != nil {
		return RootCause{}, err
	}
	out, err := toRootCause(rootCauseRow(row))
	if err != nil {
		return RootCause{}, err
	}
	if err := s.audit(ctx, "breach.root_cause.create", RootCauseEntityType, id, nil,
		map[string]any{"incident_id": incidentID, "cause_category": causeCategory}); err != nil {
		return RootCause{}, err
	}
	return out, nil
}

// GetRootCause returns one root cause.
func (s *Service) GetRootCause(ctx context.Context, id uuid.UUID) (RootCause, error) {
	if !has(ctx, PermRead) {
		return RootCause{}, ErrForbidden
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).GetRootCause(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return RootCause{}, ErrNotFound
	}
	if err != nil {
		return RootCause{}, err
	}
	return toRootCause(rootCauseRow(row))
}

// ListRootCauses returns every root cause recorded against an incident, oldest first.
func (s *Service) ListRootCauses(ctx context.Context, incidentID uuid.UUID) ([]RootCause, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	if _, err := s.Get(ctx, incidentID); err != nil {
		return nil, err
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListRootCausesByIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]RootCause, 0, len(rows))
	for _, r := range rows {
		rc, err := toRootCause(rootCauseRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

// UpdateRootCause edits the cause/description/risk link and replaces the corrective-action list wholesale
// (the caller sends the full list back, same shape every other module's "edit a jsonb list" write uses).
func (s *Service) UpdateRootCause(ctx context.Context, id uuid.UUID, rowVersion int32, causeCategory, description string,
	actions []CorrectiveAction, riskID *uuid.UUID) (RootCause, error) {
	if !has(ctx, PermUpdate) {
		return RootCause{}, ErrForbidden
	}
	if err := validateRootCause(causeCategory, description, actions); err != nil {
		return RootCause{}, err
	}
	if err := s.checkRiskVisible(ctx, riskID); err != nil {
		return RootCause{}, err
	}
	actions, err := assignActionIDs(actions)
	if err != nil {
		return RootCause{}, err
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return RootCause{}, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateRootCause(ctx, breachstore.UpdateRootCauseParams{
		ID: id, CauseCategory: causeCategory, Description: description, CorrectiveActions: actionsJSON,
		RiskID: pgUUIDPtr(riskID), RowVersion: rowVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return RootCause{}, ErrVersionMismatch
	}
	if err != nil {
		return RootCause{}, err
	}
	out, err := toRootCause(rootCauseRow(row))
	if err != nil {
		return RootCause{}, err
	}
	if err := s.audit(ctx, "breach.root_cause.update", RootCauseEntityType, id, nil, nil); err != nil {
		return RootCause{}, err
	}
	return out, nil
}

// CompleteCorrectiveAction marks one corrective action done — the acceptance criterion itself
// ("มาตรการป้องกันซ้ำถูกติดตามจนเสร็จ"): a corrective action is tracked, one at a time, until it's done.
// A redelivery/double-click on an already-done action is a harmless no-op, not an error.
func (s *Service) CompleteCorrectiveAction(ctx context.Context, rootCauseID uuid.UUID, actionID string, rowVersion int32) (RootCause, error) {
	if !has(ctx, PermUpdate) {
		return RootCause{}, ErrForbidden
	}
	rc, err := s.GetRootCause(ctx, rootCauseID)
	if err != nil {
		return RootCause{}, err
	}
	if rc.RowVersion != rowVersion {
		return RootCause{}, ErrVersionMismatch
	}
	found := false
	now := s.now()
	for i := range rc.CorrectiveActions {
		if rc.CorrectiveActions[i].ID == actionID {
			found = true
			if rc.CorrectiveActions[i].Status != "done" {
				rc.CorrectiveActions[i].Status = "done"
				rc.CorrectiveActions[i].CompletedAt = &now
			}
			break
		}
	}
	if !found {
		return RootCause{}, ErrInvalidRequest
	}
	return s.UpdateRootCause(ctx, rootCauseID, rowVersion, rc.CauseCategory, rc.Description, rc.CorrectiveActions, rc.RiskID)
}

// DeleteRootCause removes one root cause record.
func (s *Service) DeleteRootCause(ctx context.Context, incidentID, id uuid.UUID) error {
	if !has(ctx, PermUpdate) {
		return ErrForbidden
	}
	n, err := breachstore.New(pdb.MustTxFromContext(ctx)).DeleteRootCause(ctx, breachstore.DeleteRootCauseParams{ID: id, IncidentID: incidentID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "breach.root_cause.delete", RootCauseEntityType, id, nil, nil)
}

func pgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
