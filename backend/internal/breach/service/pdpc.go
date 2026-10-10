package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	breachstore "pdpa-platform/internal/breach/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
)

// PDPCDocType is the document composer type (PLT-16) a PDPC notification round is written on.
const PDPCDocType = "pdpc_form"

// PDPCEntityType is the audit/timeline entity type of a notification round.
const PDPCEntityType = "breach_pdpc_notification"

// PDPCNotification is one round of filing the PDPC notice for an incident (BRE-09): an initial notice within the
// legal window, and any supplementary or final rounds filed as the investigation progresses. The system prepares
// the document (PLT-16); the person files it through the PDPC's own channel and records the result here — that
// record, not the filing itself, is what this service manages (per the module note: "ระบบเตรียมเอกสาร ผู้ใช้ยื่นผ่าน
// ช่องทางของ สคส."). A round needs a second person's confirmation (maker-checker, matching subject notices) before
// it counts toward leaving "notifying" (ST-03).
type PDPCNotification struct {
	ID                uuid.UUID
	IncidentID        uuid.UUID
	SequenceNo        int16
	NotificationType  string // initial | supplementary | final
	DocumentVersionID uuid.UUID
	SubmittedAt       time.Time
	SubmissionRef     string
	IsLate            bool
	LateReason        string
	EvidenceFileID    *uuid.UUID
	CreatedBy         *uuid.UUID
	CreatedByName     string
	ApprovedBy        *uuid.UUID
	ApprovedByName    string
	RowVersion        int32
	CreatedAt         time.Time
}

// PDPCNotificationTypes are the values of notification_type, in filing order.
var PDPCNotificationTypes = []string{"initial", "supplementary", "final"}

func toPDPCNotification(r breachstore.GetPDPCNotificationRow) PDPCNotification {
	n := PDPCNotification{ID: r.ID, IncidentID: r.IncidentID, SequenceNo: r.SequenceNo, NotificationType: r.NotificationType,
		DocumentVersionID: r.DocumentVersionID, SubmittedAt: r.SubmittedAt.Time.UTC(), IsLate: r.IsLate, EvidenceFileID: uuidPtr(r.EvidenceFileID),
		CreatedBy: uuidPtr(r.CreatedBy), ApprovedBy: uuidPtr(r.ApprovedBy), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time.UTC()}
	n.SubmissionRef, n.LateReason = deref(r.SubmissionRef), deref(r.LateReason)
	return n
}

// CreatePDPCNotification records a round of filing the PDPC notice (BRE-08/09): documentVersionID must be a
// published version of a pdpc_form document (checked through the document composer's own service, which also
// confirms it is visible under RLS — CLAUDE.md rule 1, FK constraints don't). submittedAt is when it was actually
// filed with the PDPC (may be in the past); a round filed after the 72-hour window needs lateReason (BRE-08
// acceptance), the interpretation of the 15-day outer limit is pending legal confirmation (decisions.md Q-07), so
// it is not enforced as a hard cutoff here, only flagged.
func (s *Service) CreatePDPCNotification(ctx context.Context, incidentID uuid.UUID, notificationType string, documentVersionID uuid.UUID,
	submittedAt time.Time, submissionRef string, evidenceFileID *uuid.UUID, lateReason string) (PDPCNotification, error) {
	if !has(ctx, PermNoticeCreate) {
		return PDPCNotification{}, ErrForbidden
	}
	in, err := s.Get(ctx, incidentID)
	if err != nil {
		return PDPCNotification{}, err
	}
	if in.Status == StatusClosed {
		return PDPCNotification{}, ErrClosed
	}
	if in.Decision == DecisionNone {
		return PDPCNotification{}, ErrNoticeNotRequired
	}
	var errs []FieldError
	if !slices.Contains(PDPCNotificationTypes, notificationType) {
		errs = append(errs, FieldError{"notification_type", "invalid"})
	}
	if submittedAt.After(s.now()) {
		errs = append(errs, FieldError{"submitted_at", "future"})
	}
	submissionRef = strings.TrimSpace(submissionRef)
	if len(submissionRef) > 60 {
		errs = append(errs, FieldError{"submission_ref", "too_long"})
	}
	isLate := LateReasonRequired(in.AwareAt, submittedAt)
	lateReason = strings.TrimSpace(lateReason)
	if isLate && lateReason == "" {
		errs = append(errs, FieldError{"late_reason", "required"})
	}
	if len(lateReason) > 4000 {
		errs = append(errs, FieldError{"late_reason", "too_long"})
	}
	if len(errs) > 0 {
		return PDPCNotification{}, &ValidationError{Fields: errs}
	}
	if err := s.checkPDPCDocument(ctx, documentVersionID); err != nil {
		return PDPCNotification{}, err
	}
	if evidenceFileID != nil {
		f, err := s.Files.Get(ctx, *evidenceFileID)
		if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
			return PDPCNotification{}, ErrFileNotUsable
		}
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	seq, err := q.NextPDPCSequenceNo(ctx, incidentID)
	if err != nil {
		return PDPCNotification{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return PDPCNotification{}, err
	}
	p := breachstore.InsertPDPCNotificationParams{ID: id, IncidentID: incidentID, SequenceNo: seq, NotificationType: notificationType,
		DocumentVersionID: documentVersionID, SubmittedAt: ts(submittedAt), SubmissionRef: optText(submissionRef), IsLate: isLate,
		LateReason: optText(lateReason), EvidenceFileID: pgUUID(evidenceFileID), Actor: pgUUID(currentUser(ctx))}
	if _, err := q.InsertPDPCNotification(ctx, p); err != nil {
		return PDPCNotification{}, err
	}
	if evidenceFileID != nil {
		if err := s.Files.AttachSystem(ctx, *evidenceFileID, PDPCEntityType, id); err != nil {
			return PDPCNotification{}, err
		}
	}
	token := "pdpc_recorded:" + notificationType
	if isLate {
		token += ":late"
	}
	if err := s.timeline(ctx, incidentID, "communication", token, "", true); err != nil {
		return PDPCNotification{}, err
	}
	if err := s.audit(ctx, "breach.pdpc.create", PDPCEntityType, id, nil,
		map[string]any{"incident_id": incidentID, "notification_type": notificationType, "is_late": isLate}); err != nil {
		return PDPCNotification{}, err
	}
	return s.GetPDPCNotification(ctx, id)
}

// checkPDPCDocument confirms documentVersionID is a published version of a pdpc_form document, through the document
// composer's own service (module boundary, CLAUDE.md rule 9) — which also proves it is visible under RLS.
func (s *Service) checkPDPCDocument(ctx context.Context, documentVersionID uuid.UUID) error {
	if s.Docs == nil {
		return ErrBadDocument
	}
	_, docID, err := s.Docs.PublishedVersionOf(ctx, documentVersionID)
	if errors.Is(err, docsservice.ErrNotFound) {
		return ErrBadDocument
	}
	if err != nil {
		return err
	}
	doc, err := s.Docs.Get(ctx, docID)
	if err != nil {
		return ErrBadDocument
	}
	if doc.DocType != PDPCDocType {
		return ErrBadDocument
	}
	return nil
}

// GetPDPCNotification returns one round with its maker/approver names, for an incident the caller may see.
func (s *Service) GetPDPCNotification(ctx context.Context, id uuid.UUID) (PDPCNotification, error) {
	r, err := breachstore.New(pdb.MustTxFromContext(ctx)).GetPDPCNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return PDPCNotification{}, ErrNotFound
	}
	if err != nil {
		return PDPCNotification{}, err
	}
	n := toPDPCNotification(r)
	if _, err := s.Get(ctx, n.IncidentID); err != nil {
		return PDPCNotification{}, err
	}
	if err := s.namePDPC(ctx, &n); err != nil {
		return PDPCNotification{}, err
	}
	return n, nil
}

// ListPDPCNotifications returns an incident's filing rounds, oldest first (BRE-09: the history of every round).
func (s *Service) ListPDPCNotifications(ctx context.Context, incidentID uuid.UUID) ([]PDPCNotification, error) {
	if _, err := s.Get(ctx, incidentID); err != nil {
		return nil, err
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListPDPCNotifications(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]PDPCNotification, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPDPCNotification(breachstore.GetPDPCNotificationRow(r)))
	}
	for i := range out {
		if err := s.namePDPC(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) namePDPC(ctx context.Context, n *PDPCNotification) error {
	ids := make([]uuid.UUID, 0, 2)
	if n.CreatedBy != nil {
		ids = append(ids, *n.CreatedBy)
	}
	if n.ApprovedBy != nil {
		ids = append(ids, *n.ApprovedBy)
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return err
	}
	if n.CreatedBy != nil {
		n.CreatedByName = names[*n.CreatedBy]
	}
	if n.ApprovedBy != nil {
		n.ApprovedByName = names[*n.ApprovedBy]
	}
	return nil
}

// ConfirmPDPCNotification is the maker-checker step (rule: "ผู้อนุมัติส่งแบบแจ้ง = DPO"): a second person with
// breach.notification.approve confirms the round was filed as recorded. Only a confirmed round counts toward
// leaving "notifying" (ST-03).
func (s *Service) ConfirmPDPCNotification(ctx context.Context, id uuid.UUID, version int32) (PDPCNotification, error) {
	if !has(ctx, PermNoticeSend) {
		return PDPCNotification{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockPDPCNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return PDPCNotification{}, ErrNotFound
	}
	if err != nil {
		return PDPCNotification{}, err
	}
	n := toPDPCNotification(breachstore.GetPDPCNotificationRow(r))
	if _, err := s.Get(ctx, n.IncidentID); err != nil {
		return PDPCNotification{}, err
	}
	if n.RowVersion != version {
		return PDPCNotification{}, ErrVersionMismatch
	}
	if n.ApprovedBy != nil {
		return PDPCNotification{}, ErrInvalidTransition
	}
	me := currentUser(ctx)
	if me == nil || (n.CreatedBy != nil && *n.CreatedBy == *me) {
		return PDPCNotification{}, ErrSelfApproval
	}
	rows, err := q.ApprovePDPCNotification(ctx, breachstore.ApprovePDPCNotificationParams{Actor: pgUUID(me), ID: id, RowVersion: version})
	if err != nil {
		return PDPCNotification{}, err
	}
	if rows == 0 {
		return PDPCNotification{}, ErrVersionMismatch
	}
	if err := s.timeline(ctx, n.IncidentID, "communication", "pdpc_confirmed:"+n.NotificationType, "", true); err != nil {
		return PDPCNotification{}, err
	}
	if err := s.audit(ctx, "breach.pdpc.confirm", PDPCEntityType, id, map[string]any{"approved_by": nil}, map[string]any{"approved_by": *me}); err != nil {
		return PDPCNotification{}, err
	}
	return s.GetPDPCNotification(ctx, id)
}

// pdpcRecorded reports whether at least one filing round has been confirmed (the ST-03 guard leaving "notifying").
func (s *Service) pdpcRecorded(ctx context.Context, incidentID uuid.UUID) (bool, error) {
	n, err := breachstore.New(pdb.MustTxFromContext(ctx)).CountConfirmedPDPCNotifications(ctx, incidentID)
	return n > 0, err
}
