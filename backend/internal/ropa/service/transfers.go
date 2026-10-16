package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
	ropastore "pdpa-platform/internal/ropa/store"
)

var transferBases = []string{"adequacy", "bcr", "standard_clauses", "certification", "exemption", "consent"}

// ActivityTransfer is a cross-border transfer (ROPA-08, ม.28/29): the destination country and the
// legal mechanism relied on, optionally tied to one of the activity's recipients. TiaAssessmentID is
// DPX-06's own addition — set once a transfer impact assessment has been recorded for this route.
type ActivityTransfer struct {
	ID              uuid.UUID
	ActivityID      uuid.UUID
	RecipientID     *uuid.UUID
	CountryCode     string
	TransferBasis   string // adequacy | bcr | standard_clauses | certification | exemption | consent
	Safeguards      string
	TiaAssessmentID *uuid.UUID
	RowVersion      int32
	CreatedAt       time.Time
}

// DpiaTemplate mirrors dpiaservice.Template — just the fields DPX-06 needs. ropa/service cannot import
// dpia/service directly: dpia/service already imports ropa/service (for its own Ropa interface's return
// types), so the reverse would cycle. internal/wiring adapts the real *dpiaservice.Service to this
// interface instead (the same pattern RiskRopa already uses for risk/service).
type DpiaTemplate struct {
	ID     uuid.UUID
	Code   string
	Status string
	Name   string
}

// TiaResult mirrors dpiaservice.SubjectAssessmentResult — same reasoning as DpiaTemplate above.
type TiaResult struct {
	ID    uuid.UUID
	Score float64
}

// Dpia is what ropa reads from the dpia module (rule 9) to run a transfer impact assessment (DPX-06)
// against one of VEN-04/DPIA-03's own assessment_type='tia' templates without ever touching assess.* itself.
type Dpia interface {
	ListTemplates(ctx context.Context, assessmentType string) ([]DpiaTemplate, error)
	RecordSubjectAssessment(ctx context.Context, templateID uuid.UUID, subjectType string, subjectID uuid.UUID, title string, answers forms.Answers) (TiaResult, error)
}

func (tr *ActivityTransfer) normalize() error {
	tr.CountryCode = strings.ToUpper(strings.TrimSpace(tr.CountryCode))
	if len(tr.CountryCode) != 2 {
		return fmt.Errorf("%w: country_code", ErrInvalid)
	}
	if !slices.Contains(transferBases, tr.TransferBasis) {
		return fmt.Errorf("%w: transfer_basis", ErrInvalid)
	}
	tr.Safeguards = strings.TrimSpace(tr.Safeguards)
	return nil
}

func (s *Service) ListActivityTransfers(ctx context.Context, activityID uuid.UUID) ([]ActivityTransfer, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityTransfers(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityTransfer, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityTransfer(ropastore.InsertActivityTransferRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityTransfer(ctx context.Context, tr ActivityTransfer) (ActivityTransfer, error) {
	if err := tr.normalize(); err != nil {
		return ActivityTransfer{}, err
	}
	if _, err := s.mustActivity(ctx, tr.ActivityID); err != nil {
		return ActivityTransfer{}, err
	}
	if err := s.validCountry(ctx, tr.CountryCode); err != nil {
		return ActivityTransfer{}, err
	}
	if tr.RecipientID != nil {
		recipients, err := s.ListActivityRecipients(ctx, tr.ActivityID)
		if err != nil {
			return ActivityTransfer{}, err
		}
		found := false
		for _, r := range recipients {
			if r.ID == *tr.RecipientID {
				found = true
				break
			}
		}
		if !found {
			return ActivityTransfer{}, fmt.Errorf("%w: recipient_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityTransfer{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertActivityTransfer(ctx, ropastore.InsertActivityTransferParams{ID: id, ActivityID: tr.ActivityID,
		RecipientID: pgUUID(tr.RecipientID), CountryCode: tr.CountryCode, TransferBasis: tr.TransferBasis, Safeguards: opt(tr.Safeguards)})
	if err != nil {
		return ActivityTransfer{}, err
	}
	out := toActivityTransfer(row)
	if _, _, err := s.recompute(ctx, tr.ActivityID); err != nil {
		return ActivityTransfer{}, err
	}
	return out, s.audit(ctx, "ropa.activity_transfer.create", tr.ActivityID, nil, transferAudit(out))
}

// RecordTransferAssessment is DPX-06's own acceptance criterion: run a published assessment_type='tia'
// template (by its own code, e.g. "transfer_tia") against one logged cross-border transfer, then link the
// resulting assess.assessments row back via ropa.activity_transfers.tia_assessment_id — the exact pattern
// VEN-07's RecordAssessment already established for vendor.vendor_assessments.
func (s *Service) RecordTransferAssessment(ctx context.Context, activityID, transferID uuid.UUID, templateCode string, answers forms.Answers) (ActivityTransfer, error) {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return ActivityTransfer{}, err
	}
	if s.Dpia == nil {
		return ActivityTransfer{}, errors.New("ropa: transfer assessment scoring not configured")
	}
	if _, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetActivityTransfer(ctx, ropastore.GetActivityTransferParams{ID: transferID, ActivityID: activityID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActivityTransfer{}, ErrNotFound
		}
		return ActivityTransfer{}, err
	}

	templates, err := s.Dpia.ListTemplates(ctx, "tia")
	if err != nil {
		return ActivityTransfer{}, err
	}
	var tpl *DpiaTemplate
	for i := range templates {
		if templates[i].Code == templateCode && templates[i].Status == "published" {
			tpl = &templates[i]
			break
		}
	}
	if tpl == nil {
		return ActivityTransfer{}, fmt.Errorf("%w: template_code", ErrInvalid)
	}

	result, err := s.Dpia.RecordSubjectAssessment(ctx, tpl.ID, "transfer", transferID, tpl.Name, answers)
	if err != nil {
		return ActivityTransfer{}, err
	}

	updated, err := ropastore.New(pdb.MustTxFromContext(ctx)).SetTransferTIA(ctx, ropastore.SetTransferTIAParams{
		ID: transferID, ActivityID: activityID, TiaAssessmentID: pgUUID(&result.ID)})
	if err != nil {
		return ActivityTransfer{}, err
	}
	out := toActivityTransfer(ropastore.InsertActivityTransferRow(updated))
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return ActivityTransfer{}, err
	}
	if err := s.audit(ctx, "ropa.activity_transfer.tia", activityID,
		map[string]any{"transfer_id": transferID, "tia_assessment_id": nil},
		map[string]any{"transfer_id": transferID, "tia_assessment_id": result.ID, "score": result.Score}); err != nil {
		return ActivityTransfer{}, err
	}
	return out, nil
}

func (s *Service) DeleteActivityTransfer(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityTransfer(ctx, ropastore.DeleteActivityTransferParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_transfer.delete", activityID, map[string]any{"id": id}, nil)
}

// validCountry checks a code against ORG-07's countries list — like validLawfulBasis, keyed by code
// not id, so it scans ListMaster rather than a by-id GetMaster lookup.
func (s *Service) validCountry(ctx context.Context, code string) error {
	items, err := s.Org.ListMaster(ctx, "countries")
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Code == code {
			return nil
		}
	}
	return fmt.Errorf("%w: country_code", ErrInvalid)
}

func transferAudit(tr ActivityTransfer) map[string]any {
	return map[string]any{"id": tr.ID, "country_code": tr.CountryCode, "transfer_basis": tr.TransferBasis, "recipient_id": tr.RecipientID}
}

func toActivityTransfer(r ropastore.InsertActivityTransferRow) ActivityTransfer {
	return ActivityTransfer{ID: r.ID, ActivityID: r.ActivityID, RecipientID: uuidPtr(r.RecipientID), CountryCode: r.CountryCode,
		TransferBasis: r.TransferBasis, Safeguards: deref(r.Safeguards), TiaAssessmentID: uuidPtr(r.TiaAssessmentID),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
}
