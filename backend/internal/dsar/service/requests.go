package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dsarstore "pdpa-platform/internal/dsar/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
)

var channels = []string{"web", "email", "phone", "branch", "letter", "line", "api"}
var outcomes = []string{"fulfilled", "partially_fulfilled", "rejected", "withdrawn"}

const requesterNameContext = "dsar.requests.requester_name_enc"
const requesterContactContext = "dsar.requests.requester_contact_enc"
const cryptoClass = "dsar"

// Request is a DSAR request (ST-02, dsar.requests.status). The requester's name and contact are write-only
// here (PII, rule 3) — encrypted via PLT-13 at CreateRequest and only ever decrypted again by
// generateResponseLetter, to address the letter to them.
type Request struct {
	ID                  uuid.UUID
	RequestNo           string
	RequestTypeID       uuid.UUID
	LegalEntityID       uuid.UUID
	Channel             string
	OnBehalf            bool
	Status              string // received | verifying | in_review | in_progress | awaiting_info | completed | rejected | withdrawn (ST-02)
	ReceivedAt          time.Time
	DueAt               time.Time
	VerifiedAt          *time.Time
	ClosedAt            *time.Time
	Outcome             *string
	RejectionReasonCode *string
	AssigneeUserID      *uuid.UUID
	RowVersion          int32
	UpdatedAt           time.Time
}

type CreateRequestInput struct {
	RequestTypeID    uuid.UUID
	LegalEntityID    uuid.UUID
	Channel          string
	OnBehalf         bool
	RequesterName    string
	RequesterContact string
	ContactKind      crypto.IdentifierKind
}

// CreateRequest is the minimal slice of DSAR-01/02's job this feature needs: receive a request and start its
// SLA clock. due_at here is received_at plus the request type's sla_days as *calendar* days — a placeholder;
// the real business-day countdown (Songkran-aware, like every other deadline in this codebase) is DSAR-07's
// job and replaces this once built.
func (s *Service) CreateRequest(ctx context.Context, in CreateRequestInput) (Request, error) {
	if !slices.Contains(channels, in.Channel) {
		return Request{}, fmt.Errorf("%w: channel", ErrInvalid)
	}
	in.RequesterName = strings.TrimSpace(in.RequesterName)
	if in.RequesterName == "" {
		return Request{}, fmt.Errorf("%w: requester_name", ErrInvalid)
	}
	if strings.TrimSpace(in.RequesterContact) == "" {
		return Request{}, fmt.Errorf("%w: requester_contact", ErrInvalid)
	}
	if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
		return Request{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	rt, err := s.GetRequestType(ctx, in.RequestTypeID)
	if err != nil {
		return Request{}, fmt.Errorf("%w: request_type_id", ErrInvalid)
	}

	normalizedContact, err := crypto.Normalize(in.ContactKind, in.RequesterContact)
	if err != nil {
		return Request{}, fmt.Errorf("%w: requester_contact", ErrInvalid)
	}
	nameEnc, err := s.Keyring.Encrypt(ctx, cryptoClass, requesterNameContext, []byte(in.RequesterName))
	if err != nil {
		return Request{}, err
	}
	contactEnc, err := s.Keyring.Encrypt(ctx, cryptoClass, requesterContactContext, []byte(normalizedContact))
	if err != nil {
		return Request{}, err
	}
	blindIndex, err := s.Keyring.BlindIndex(ctx, in.ContactKind, in.RequesterContact)
	if err != nil {
		return Request{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Request{}, err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(s.now().Year())
	if err := q.LockRequestNumbering(ctx, year); err != nil {
		return Request{}, err
	}
	prefix := "DSAR-" + year + "-"
	n, err := q.CountRequestsInYear(ctx, prefix)
	if err != nil {
		return Request{}, err
	}

	receivedAt := s.now()
	dueAt := receivedAt.AddDate(0, 0, int(rt.SLADays))
	row, err := q.InsertRequest(ctx, dsarstore.InsertRequestParams{
		ID: id, RequestNo: fmt.Sprintf("%s%04d", prefix, n+1), RequestTypeID: in.RequestTypeID,
		LegalEntityID: in.LegalEntityID, Channel: in.Channel, OnBehalf: in.OnBehalf, Details: []byte("{}"),
		RequesterNameEnc: nameEnc, RequesterContactEnc: contactEnc, RequesterBlindIndex: blindIndex,
		ReceivedAt: pgtype.Timestamptz{Time: receivedAt, Valid: true}, DueAt: pgtype.Timestamptz{Time: dueAt, Valid: true},
	})
	if err != nil {
		return Request{}, err
	}
	out := toRequest(dsarstore.GetRequestRow(row))
	if err := s.audit(ctx, "dsar.request.create", out.ID, nil, map[string]any{"request_no": out.RequestNo, "request_type_id": out.RequestTypeID, "channel": out.Channel}); err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Service) GetRequest(ctx context.Context, id uuid.UUID) (Request, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, err
	}
	return toRequest(row), nil
}

type RequestCursor struct {
	ReceivedAt time.Time
	ID         uuid.UUID
}

type RequestFilter struct {
	Status string
	After  *RequestCursor
	Limit  int
}

const requestPageSize = 50

func (s *Service) ListRequests(ctx context.Context, f RequestFilter) ([]Request, *RequestCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > requestPageSize {
		limit = requestPageSize
	}
	p := dsarstore.ListRequestsParams{Lim: int32(limit + 1)}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.ReceivedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListRequests(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Request, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &RequestCursor{ReceivedAt: last.ReceivedAt, ID: last.ID}, nil
		}
		out = append(out, toRequest(dsarstore.GetRequestRow(r)))
	}
	return out, nil, nil
}

// ST-02's real transition graph (docs/states/state-machines.yaml#ST-02) — the full 8-state, 17-edge machine
// the SA spec already fully defines, even though this feature only needs a subset of it: encoding the whole
// thing here (it's cheap, just a table) means DSAR-06/08/11 build on a correct machine later, not a
// reinvented one. No verification (DSAR-06), subtask-completeness (DSAR-08) or SLA-overdue (DSAR-07) guard is
// enforced yet on the intermediate edges — those modules aren't built; only the two data-carrying guards below
// (outcome required to enter completed, a reason required to enter rejected) are checked here.
var st02Edges = map[string][]string{
	"received":      {"verifying", "withdrawn"},
	"verifying":     {"in_review", "awaiting_info", "withdrawn"},
	"in_review":     {"in_progress", "awaiting_info", "rejected", "withdrawn"},
	"in_progress":   {"completed", "awaiting_info", "withdrawn"},
	"awaiting_info": {"in_review", "rejected", "withdrawn"},
}

type TransitionInput struct {
	To                  string
	Outcome             *string
	RejectionReasonCode *string
	// ActivityIDs are the RoPA processing activities this rejection concerns (DSAR-11, ม.39(7)) — logged in
	// the dsar.rejected event for ROPA-10 to record against each one; optional, since not every request maps
	// to a specific processing activity.
	ActivityIDs []uuid.UUID
}

// Transition moves a request along ST-02. Entering awaiting_info, completed or rejected auto-generates the
// matching response letter draft (DSAR-13's own acceptance criterion) — request_info / result / rejection
// respectively; the returned document id is what the frontend opens next ("เลือก template + แก้ก่อนส่ง").
// Entering rejected additionally needs a reason and an approver (DSAR-11, ม.30/ม.39(7)): the caller must hold
// dsar.request.approve, not just dsar.request.execute, and the rejection is published as `dsar.rejected` for
// ROPA-10 to log against the referenced activities — DSAR-11 itself never writes to the ropa schema (rule 9).
func (s *Service) Transition(ctx context.Context, id uuid.UUID, rowVersion int32, in TransitionInput) (Request, *uuid.UUID, error) {
	req, err := s.GetRequest(ctx, id)
	if err != nil {
		return Request{}, nil, err
	}
	allowed, ok := st02Edges[req.Status]
	if !ok || !slices.Contains(allowed, in.To) {
		return Request{}, nil, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, req.Status, in.To)
	}
	var outcome *string
	switch in.To {
	case "completed":
		if in.Outcome == nil || !slices.Contains(outcomes[:2], *in.Outcome) {
			return Request{}, nil, fmt.Errorf("%w: outcome (fulfilled or partially_fulfilled required)", ErrInvalid)
		}
		outcome = in.Outcome
	case "rejected":
		if in.RejectionReasonCode == nil || strings.TrimSpace(*in.RejectionReasonCode) == "" {
			return Request{}, nil, fmt.Errorf("%w: rejection_reason_code", ErrInvalid)
		}
		if g, _ := authz.FromContext(ctx); !g.Has("dsar.request.approve") {
			return Request{}, nil, ErrForbidden
		}
		for _, aid := range in.ActivityIDs {
			if _, err := s.Ropa.GetActivity(ctx, aid); err != nil {
				return Request{}, nil, fmt.Errorf("%w: activity_ids", ErrInvalid)
			}
		}
	case "withdrawn":
		w := "withdrawn"
		outcome = &w
	}
	var rejectionReason *string
	if in.To == "rejected" {
		rejectionReason = in.RejectionReasonCode
	}

	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateRequestStatus(ctx, dsarstore.UpdateRequestStatusParams{ID: id, Status: in.To, Outcome: outcome, RejectionReasonCode: rejectionReason, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, nil, ErrVersionMismatch
	}
	if err != nil {
		return Request{}, nil, err
	}
	out := toRequest(dsarstore.GetRequestRow(row))
	if err := s.audit(ctx, "dsar.request.transition", out.ID, map[string]any{"status": req.Status}, map[string]any{"status": out.Status, "outcome": out.Outcome}); err != nil {
		return Request{}, nil, err
	}

	if in.To == "rejected" && s.Events != nil {
		rt, err := s.GetRequestType(ctx, out.RequestTypeID)
		if err != nil {
			return Request{}, nil, err
		}
		activityRefs := make([]string, len(in.ActivityIDs))
		for i, aid := range in.ActivityIDs {
			activityRefs[i] = aid.String()
		}
		if _, err := s.Events.Publish(ctx, events.Event{Type: "dsar.rejected", AggregateType: "dsar_request", AggregateID: out.ID,
			Data: map[string]any{"request_ref": out.RequestNo, "request_type": rt.Code, "due_at": out.DueAt.Format(time.RFC3339),
				"status": out.Status, "reason_code": *out.RejectionReasonCode, "activity_refs": activityRefs}}); err != nil {
			return Request{}, nil, err
		}
	}

	var purpose string
	switch in.To {
	case "awaiting_info":
		purpose = "request_info"
	case "completed":
		purpose = "result"
	case "rejected":
		purpose = "rejection"
	}
	var docID *uuid.UUID
	if purpose != "" {
		rt, err := s.GetRequestType(ctx, out.RequestTypeID)
		if err != nil {
			return Request{}, nil, err
		}
		id, err := s.generateResponseLetter(ctx, out, rt, purpose)
		if err != nil {
			return Request{}, nil, err
		}
		docID = &id
	}
	return out, docID, nil
}

func toRequest(r dsarstore.GetRequestRow) Request {
	req := Request{ID: r.ID, RequestNo: r.RequestNo, RequestTypeID: r.RequestTypeID, LegalEntityID: r.LegalEntityID,
		Channel: r.Channel, OnBehalf: r.OnBehalf, Status: r.Status, ReceivedAt: r.ReceivedAt.Time, DueAt: r.DueAt.Time,
		Outcome: r.Outcome, RejectionReasonCode: r.RejectionReasonCode, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	if r.VerifiedAt.Valid {
		t := r.VerifiedAt.Time
		req.VerifiedAt = &t
	}
	if r.ClosedAt.Valid {
		t := r.ClosedAt.Time
		req.ClosedAt = &t
	}
	if r.AssigneeUserID.Valid {
		u := uuid.UUID(r.AssigneeUserID.Bytes)
		req.AssigneeUserID = &u
	}
	return req
}
