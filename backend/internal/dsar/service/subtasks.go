package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dsarstore "pdpa-platform/internal/dsar/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/notify"
)

// DSAR-08 (BP-06 "ดำเนินการ" step): once a request reaches in_review and is accepted, the DPO/Privacy team
// breaks the work into subtasks against whichever system/team/processor owns the data, and assigns each one
// out. The acceptance criterion — "คำขอเดินครบทุกขั้นตอนและปิดได้เมื่อ subtask เสร็จทั้งหมด" — is the close
// guard added to Transition's in_progress → completed edge below: a request with any subtask still open or
// in_progress cannot be completed. "ตั้ง rule อัตโนมัติตามประเภทและบริษัท" (an automatic default-subtask rule
// per request type) has no consuming screen yet and no column to configure it from — deferred the same way
// ROPA-01 deferred discovered_by_finding_id — subtasks here are always created by hand.
var subtaskActions = []string{"search", "export", "delete", "rectify", "restrict", "stop_marketing", "review"}

// subtaskEdges is dsar.subtasks.status's own small machine (open → in_progress → done | not_applicable, or
// straight from open to done|not_applicable for a subtask small enough to finish in one step) — local to this
// one table, not part of ST-02, so it has no docs/states/state-machines.yaml entry of its own the way a
// dsar.requests transition would.
var subtaskEdges = map[string][]string{
	"open":        {"in_progress", "done", "not_applicable"},
	"in_progress": {"done", "not_applicable"},
}

// Subtask is one unit of work against a DSAR request (dsar.subtasks).
type Subtask struct {
	ID              uuid.UUID
	RequestID       uuid.UUID
	AssetID         *uuid.UUID
	Action          string
	AssigneeUserID  *uuid.UUID
	AssigneeGroupID *uuid.UUID
	Status          string // open | in_progress | done | not_applicable
	DueAt           *time.Time
	CompletedAt     *time.Time
	EvidenceFileID  *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RowVersion      int32
}

type CreateSubtaskInput struct {
	AssetID         *uuid.UUID
	Action          string
	AssigneeUserID  *uuid.UUID
	AssigneeGroupID *uuid.UUID
	DueAt           *time.Time
}

// CreateSubtask opens one unit of work against a request and, when assigned, notifies whoever it was given
// to (PLT-04) — "มอบหมายงานย่อยให้เจ้าของระบบหรือทีม".
func (s *Service) CreateSubtask(ctx context.Context, requestID uuid.UUID, in CreateSubtaskInput) (Subtask, error) {
	if !slices.Contains(subtaskActions, in.Action) {
		return Subtask{}, fmt.Errorf("%w: action", ErrInvalid)
	}
	req, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return Subtask{}, err
	}
	if isClosed(req.Status) {
		return Subtask{}, fmt.Errorf("%w: request is closed", ErrInvalid)
	}
	if in.AssetID != nil {
		if _, err := s.Ropa.GetAsset(ctx, *in.AssetID); err != nil {
			return Subtask{}, fmt.Errorf("%w: asset_id", ErrInvalid)
		}
	}
	if in.AssigneeUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*in.AssigneeUserID})
		if err != nil {
			return Subtask{}, err
		}
		if _, ok := names[*in.AssigneeUserID]; !ok {
			return Subtask{}, fmt.Errorf("%w: assignee_user_id", ErrInvalid)
		}
	}
	if in.AssigneeGroupID != nil {
		names, err := iamservice.GroupNames(ctx, []uuid.UUID{*in.AssigneeGroupID})
		if err != nil {
			return Subtask{}, err
		}
		if _, ok := names[*in.AssigneeGroupID]; !ok {
			return Subtask{}, fmt.Errorf("%w: assignee_group_id", ErrInvalid)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Subtask{}, err
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).InsertSubtask(ctx, dsarstore.InsertSubtaskParams{
		ID: id, RequestID: requestID, AssetID: pgUUID(in.AssetID), Action: in.Action,
		AssigneeUserID: pgUUID(in.AssigneeUserID), AssigneeGroupID: pgUUID(in.AssigneeGroupID), DueAt: pgTimestamptz(in.DueAt),
	})
	if err != nil {
		return Subtask{}, err
	}
	out := toSubtask(dsarstore.GetSubtaskRow(row))
	if err := s.notifySubtaskAssignee(ctx, req, out); err != nil {
		return Subtask{}, err
	}
	return out, s.audit(ctx, "dsar.subtask.create", requestID, nil, map[string]any{"subtask_id": out.ID, "action": out.Action})
}

func (s *Service) notifySubtaskAssignee(ctx context.Context, req Request, st Subtask) error {
	if s.Notify == nil || st.AssigneeUserID == nil {
		return nil
	}
	vars := map[string]any{"request_no": req.RequestNo, "action": st.Action}
	uid := *st.AssigneeUserID
	for _, ch := range []string{"in_app", "email"} {
		err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "dsar.subtask_assigned", Channel: ch, RecipientUserID: &uid,
				Vars: vars, EntityType: "dsar_request", EntityID: &req.ID, Urgent: false})
			return err
		})
		if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
			return err
		}
	}
	return nil
}

// ListSubtasks returns every subtask of a request, oldest first.
func (s *Service) ListSubtasks(ctx context.Context, requestID uuid.UUID) ([]Subtask, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListSubtasksForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]Subtask, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSubtask(dsarstore.GetSubtaskRow(r)))
	}
	return out, nil
}

func (s *Service) getSubtaskFor(ctx context.Context, requestID, subtaskID uuid.UUID) (Subtask, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetSubtask(ctx, subtaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subtask{}, ErrNotFound
	}
	if err != nil {
		return Subtask{}, err
	}
	if row.RequestID != requestID {
		return Subtask{}, ErrNotFound
	}
	return toSubtask(row), nil
}

// UpdateSubtaskStatus moves a subtask along its own small machine. "แก้ได้เฉพาะงานที่ได้รับมอบหมาย"
// (docs/security/permissions.md): a caller without dsar.subtask.execute may only change a subtask assigned
// to them directly, or to a group they belong to. evidence_file_id, when given, must be the caller's own
// clean, still-unattached PLT-09 upload — attached here the same way DSAR-06 attaches its own evidence.
func (s *Service) UpdateSubtaskStatus(ctx context.Context, requestID, subtaskID uuid.UUID, rowVersion int32, status string, evidenceFileID *uuid.UUID) (Subtask, error) {
	st, err := s.getSubtaskFor(ctx, requestID, subtaskID)
	if err != nil {
		return Subtask{}, err
	}
	allowed, ok := subtaskEdges[st.Status]
	if !ok || !slices.Contains(allowed, status) {
		return Subtask{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, st.Status, status)
	}
	if err := s.requireSubtaskAccess(ctx, st); err != nil {
		return Subtask{}, err
	}
	if evidenceFileID != nil {
		f, err := s.Files.Get(ctx, *evidenceFileID)
		if err != nil || f.EntityType != "" {
			return Subtask{}, fmt.Errorf("%w: evidence_file_id", ErrInvalid)
		}
		if err := s.Files.Attach(ctx, *evidenceFileID, "dsar_request", requestID); err != nil {
			return Subtask{}, err
		}
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).UpdateSubtaskStatus(ctx, dsarstore.UpdateSubtaskStatusParams{
		ID: subtaskID, Status: status, EvidenceFileID: pgUUID(evidenceFileID), RowVersion: rowVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Subtask{}, ErrVersionMismatch
	}
	if err != nil {
		return Subtask{}, err
	}
	out := toSubtask(dsarstore.GetSubtaskRow(row))
	return out, s.audit(ctx, "dsar.subtask.update", requestID, map[string]any{"status": st.Status}, map[string]any{"subtask_id": out.ID, "status": out.Status})
}

func (s *Service) requireSubtaskAccess(ctx context.Context, st Subtask) error {
	g, _ := authz.FromContext(ctx)
	if g.Has("dsar.subtask.execute") {
		return nil
	}
	callerID, err := uuid.Parse(g.UserID)
	if err != nil {
		return ErrForbidden
	}
	if st.AssigneeUserID != nil && *st.AssigneeUserID == callerID {
		return nil
	}
	if st.AssigneeGroupID != nil {
		groups, err := iamservice.GroupIDsOf(ctx, callerID)
		if err != nil {
			return err
		}
		if slices.Contains(groups, *st.AssigneeGroupID) {
			return nil
		}
	}
	return ErrForbidden
}

// DeleteSubtask removes a subtask (dsar.subtask.delete — a permanent change, DPO/PRIVACY only, unlike the
// plain .update any assignee holds).
func (s *Service) DeleteSubtask(ctx context.Context, requestID, subtaskID uuid.UUID) error {
	st, err := s.getSubtaskFor(ctx, requestID, subtaskID)
	if err != nil {
		return err
	}
	n, err := dsarstore.New(pdb.MustTxFromContext(ctx)).DeleteSubtask(ctx, subtaskID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "dsar.subtask.delete", requestID, map[string]any{"subtask_id": st.ID, "action": st.Action}, nil)
}

// rightSubtaskAction is DSAR-03's own mapping from a request's right type (ม.19, 30-36) to the subtask action
// its per-system work item should carry — "ขั้นตอนเฉพาะของแต่ละสิทธิ" generalized across every type via the
// subtask action vocabulary DSAR-08 already seeded (search/export/delete/rectify/restrict/stop_marketing/
// review), rather than the single erasure-only example the module doc's own backend note gives.
var rightSubtaskAction = map[string]string{
	"access":           "search",         // ม.30 — locate the subject's data in each system before disclosure
	"portability":      "export",         // ม.31 — machine-readable package
	"objection":        "stop_marketing", // ม.32 — stop processing (direct marketing) immediately
	"erasure":          "delete",         // ม.33 — delete / destroy / de-identify
	"restriction":      "restrict",       // ม.34 — flag restricted
	"rectification":    "rectify",        // ม.35-36 — correct the record
	"withdraw_consent": "restrict",       // ม.19 — stop processing under the withdrawn consent basis
	"complaint":        "review",
	"inquiry":          "review",
}

// openRightSubtasks is DSAR-03's own acceptance criterion in code: entering in_progress with one or more
// linked RoPA assets (the systems that actually hold the subject's data, per the RoPA/data map — ropa.assets,
// not ropa.processing_activities) opens one subtask per asset, so the request "เดินครบทุกขั้นตอน" against
// every system involved, not just a single generic task. An unmapped right type (future CHECK-constraint
// addition) falls back to "review" rather than failing the transition outright.
func (s *Service) openRightSubtasks(ctx context.Context, req Request, rt RequestType, assetIDs []uuid.UUID) error {
	action, ok := rightSubtaskAction[rt.Code]
	if !ok {
		action = "review"
	}
	for _, aid := range assetIDs {
		assetID := aid
		if _, err := s.CreateSubtask(ctx, req.ID, CreateSubtaskInput{AssetID: &assetID, Action: action, DueAt: &req.DueAt}); err != nil {
			return err
		}
	}
	return nil
}

// allSubtasksDone is DSAR-08's own close guard on Transition's in_progress → completed edge: a request with
// zero subtasks is vacuously "all done" (not every request needs subtasks — DSAR-13's own completed tests
// never created any), one with any subtask still open or in_progress is not.
func (s *Service) allSubtasksDone(ctx context.Context, requestID uuid.UUID) (bool, error) {
	n, err := dsarstore.New(pdb.MustTxFromContext(ctx)).CountOpenSubtasksForRequest(ctx, requestID)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func toSubtask(r dsarstore.GetSubtaskRow) Subtask {
	st := Subtask{ID: r.ID, RequestID: r.RequestID, Action: r.Action, Status: r.Status,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
	if r.AssetID.Valid {
		u := uuid.UUID(r.AssetID.Bytes)
		st.AssetID = &u
	}
	if r.AssigneeUserID.Valid {
		u := uuid.UUID(r.AssigneeUserID.Bytes)
		st.AssigneeUserID = &u
	}
	if r.AssigneeGroupID.Valid {
		u := uuid.UUID(r.AssigneeGroupID.Bytes)
		st.AssigneeGroupID = &u
	}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		st.DueAt = &t
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		st.CompletedAt = &t
	}
	if r.EvidenceFileID.Valid {
		u := uuid.UUID(r.EvidenceFileID.Bytes)
		st.EvidenceFileID = &u
	}
	return st
}

func pgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
