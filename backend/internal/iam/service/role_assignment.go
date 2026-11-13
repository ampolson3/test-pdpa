package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	store "pdpa-platform/internal/iam/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// RoleAssignmentRequestType is the PLT-08 (platform/versioning) record type IAM-06 registers: a request
// to grant the ORGADMIN role, whose one SEC approval step gates the real effect (ApplyRoleAssignment, the
// module's own OnPublish hook) — the acceptance criterion itself, "action ไม่มีผลจนกว่าจะอนุมัติ": the
// authz grants loader (loader.go's ListEffectiveRoleAssignments) reads iam.role_assignments directly with
// no approval gate of its own, so nothing may write that row before PLT-08 actually publishes this
// request's one version.
const RoleAssignmentRequestType = "iam_role_assignment_request"

// GrantableRole is the only role this feature lets ORGADMIN request dual control for — "เปลี่ยน role
// ระดับ admin" read literally as the tenant's own admin-level role; SUPER is platform-only (IAM-03's own
// provider console, never requested through a tenant endpoint) and every other role has no gated path at
// all yet (ordinary role assignment stays IAM-01/02's own job, blocked on decisions.md Q-18).
const GrantableRole = "ORGADMIN"

var (
	ErrRoleNotGrantable   = errors.New("iam: only the ORGADMIN role can be requested through dual control")
	ErrTargetUserNotFound = errors.New("iam: target user not found or not active")
	ErrRequestNotFound    = errors.New("iam: role assignment request not found")
)

// VersionRef is the minimal shape iam/service needs back from platform/versioning.Service.SaveDraft —
// platform/versioning itself imports iam/service (to resolve a requester's/approver's display name), so
// this package can't import it back (a cycle); internal/wiring.IamRoleVersioning adapts the real
// *versioning.Service to this interface, the same pattern already established for Auditor/Notifier above.
type VersionRef struct {
	ID         uuid.UUID
	RowVersion int32
}

type Versioning interface {
	SaveDraft(ctx context.Context, entityType string, entityID uuid.UUID, snapshot any) (VersionRef, error)
	Submit(ctx context.Context, id uuid.UUID, rowVersion int32) (VersionRef, error)
}

// RoleAssignmentSnapshot is the version's own content — what the SEC approver is actually deciding.
type RoleAssignmentSnapshot struct {
	TargetUserID uuid.UUID `json:"target_user_id"`
	RoleID       uuid.UUID `json:"role_id"`
	RoleCode     string    `json:"role_code"`
}

// RoleAssignmentRequest is the read side (list/get) of a request, naming the target and role directly so
// the approval inbox and history never need a second decode of the version's own snapshot.
type RoleAssignmentRequest struct {
	ID               uuid.UUID
	TargetUserID     uuid.UUID
	TargetUserName   string
	RoleCode         string
	RequestedBy      uuid.UUID
	RequestedByName  string
	RoleAssignmentID *uuid.UUID
	AppliedAt        *time.Time
	CreatedAt        time.Time
	VersionID        uuid.UUID
	RowVersion       int32
}

// RequestRoleAssignment is the acceptance criterion's own first half: ORGADMIN (admin.user.update, checked
// at the HTTP layer) asks that target be granted ORGADMIN. It opens a PLT-08 version and submits it in the
// same transaction — the request is immediately in_review, not left as an editable draft, since there is
// nothing else to draft-edit about a single role grant. ErrSelfApproval (versioning's own, "the requester
// can't approve") is what then stops the same person from granting themselves the role.
func (s *Service) RequestRoleAssignment(ctx context.Context, targetUserID uuid.UUID) (RoleAssignmentRequest, error) {
	g, ok := authz.FromContext(ctx)
	if !ok {
		return RoleAssignmentRequest{}, ErrNoGrants
	}
	requestedBy, err := uuid.Parse(g.UserID)
	if err != nil {
		return RoleAssignmentRequest{}, ErrNoGrants
	}
	q := store.New(pdb.MustTxFromContext(ctx))

	names, err := q.ActiveUserNames(ctx, []uuid.UUID{targetUserID})
	if err != nil {
		return RoleAssignmentRequest{}, fmt.Errorf("iam: check target user: %w", err)
	}
	if len(names) == 0 {
		return RoleAssignmentRequest{}, ErrTargetUserNotFound
	}

	roleID, err := q.GetRoleID(ctx, GrantableRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleAssignmentRequest{}, fmt.Errorf("iam: %s role not seeded", GrantableRole)
	}
	if err != nil {
		return RoleAssignmentRequest{}, err
	}

	id := uuid.New()
	if _, err := q.InsertRoleAssignmentRequest(ctx, store.InsertRoleAssignmentRequestParams{
		ID: id, TargetUserID: targetUserID, RoleID: roleID, RequestedBy: requestedBy,
	}); err != nil {
		return RoleAssignmentRequest{}, fmt.Errorf("iam: insert role assignment request: %w", err)
	}

	snapshot := RoleAssignmentSnapshot{TargetUserID: targetUserID, RoleID: roleID, RoleCode: GrantableRole}
	v, err := s.Versioning.SaveDraft(ctx, RoleAssignmentRequestType, id, snapshot)
	if err != nil {
		return RoleAssignmentRequest{}, fmt.Errorf("iam: open approval request: %w", err)
	}
	v, err = s.Versioning.Submit(ctx, v.ID, v.RowVersion)
	if err != nil {
		return RoleAssignmentRequest{}, fmt.Errorf("iam: submit approval request: %w", err)
	}

	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return RoleAssignmentRequest{ID: id, TargetUserID: targetUserID, TargetUserName: names[0].DisplayName,
		RoleCode: GrantableRole, RequestedBy: requestedBy, CreatedAt: now,
		VersionID: v.ID, RowVersion: v.RowVersion}, nil
}

// ListRoleAssignmentRequests is the acceptance criterion's own history ("กล่องคำขอรออนุมัติ + ประวัติ"
// reuses PLT-08's own generic /approvals inbox and RecordVersions panel for the pending/decided state —
// this is the request-identity side those generic views don't carry).
func (s *Service) ListRoleAssignmentRequests(ctx context.Context) ([]RoleAssignmentRequest, error) {
	q := store.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListRoleAssignmentRequests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoleAssignmentRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, RoleAssignmentRequest{
			ID: r.ID, TargetUserID: r.TargetUserID, TargetUserName: r.TargetUserName, RoleCode: r.RoleCode,
			RequestedBy: r.RequestedBy, RequestedByName: r.RequestedByName,
			RoleAssignmentID: uuidPtrVerify(r.RoleAssignmentID), AppliedAt: timePtrVerify(r.AppliedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func (s *Service) GetRoleAssignmentRequest(ctx context.Context, id uuid.UUID) (RoleAssignmentRequest, error) {
	q := store.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetRoleAssignmentRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleAssignmentRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return RoleAssignmentRequest{}, err
	}
	return RoleAssignmentRequest{ID: r.ID, TargetUserID: r.TargetUserID, TargetUserName: r.TargetUserName,
		RoleCode: r.RoleCode, RequestedBy: r.RequestedBy, RoleAssignmentID: uuidPtrVerify(r.RoleAssignmentID),
		AppliedAt: timePtrVerify(r.AppliedAt), CreatedAt: r.CreatedAt.Time}, nil
}

// RoleAssignmentRequestTitle backs platform/versioning.Policy.Title for the inbox/history display —
// wired from internal/wiring, which builds the Policy literal (iam/service can't import the versioning
// package itself, the same cycle VersionRef's own doc comment explains).
func (s *Service) RoleAssignmentRequestTitle(ctx context.Context, entityID uuid.UUID) (string, error) {
	r, err := s.GetRoleAssignmentRequest(ctx, entityID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s → %s", r.RoleCode, r.TargetUserName), nil
}

// ApplyRoleAssignment is platform/versioning.Policy.OnPublish for RoleAssignmentRequestType — the one
// moment the action actually takes effect, inside the same transaction PLT-08 publishes the approved
// version in. The requester themselves becomes "granted_by" (who asked for it), distinct from who decided
// it (SEC, already recorded on the version's own approval step).
func (s *Service) ApplyRoleAssignment(ctx context.Context, entityID uuid.UUID, snapshot json.RawMessage) error {
	var sn RoleAssignmentSnapshot
	if err := json.Unmarshal(snapshot, &sn); err != nil {
		return fmt.Errorf("iam: decode role assignment snapshot: %w", err)
	}
	req, err := s.GetRoleAssignmentRequest(ctx, entityID)
	if err != nil {
		return err
	}
	q := store.New(pdb.MustTxFromContext(ctx))
	return q.ApplyRoleAssignmentRequest(ctx, store.ApplyRoleAssignmentRequestParams{
		RequestID: entityID, TargetUserID: pgUUIDVerify(&sn.TargetUserID), RoleID: sn.RoleID, GrantedBy: pgUUIDVerify(&req.RequestedBy),
	})
}
