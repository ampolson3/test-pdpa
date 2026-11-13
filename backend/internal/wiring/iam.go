package wiring

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	auditservice "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
)

// iamAuditor adapts *audit/service.Service to iamservice.Auditor — iam/service cannot import
// platform/audit/service directly (that package itself imports iam/service for ORG-19's actor-name
// resolution, which would be a cycle), so it defines its own minimal Auditor interface and this is the glue,
// built where both concrete packages are already imported.
type iamAuditor struct{ svc *auditservice.Service }

func (a iamAuditor) Write(ctx context.Context, e iamservice.AuditEntry) error {
	return a.svc.Write(ctx, auditservice.Entry{TenantID: e.TenantID, ActorType: e.ActorType, ActorID: e.ActorID,
		EntityType: e.EntityType, EntityID: e.EntityID, Action: e.Action, Before: e.Before, After: e.After})
}

// iamNotifier adapts *platform/notify.Service to iamservice.Notifier — same reasoning as iamAuditor: notify
// itself imports iam/service (Contact resolution for a user recipient), so iam/service can't import notify.
type iamNotifier struct{ svc *notify.Service }

func (n iamNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	return n.svc.Send(ctx, notify.Request{TemplateCode: req.TemplateCode, Channel: req.Channel,
		RecipientAddress: req.RecipientAddress, Vars: req.Vars, Urgent: req.Urgent})
}

// IamVerification builds iam/service's Service configured for IAM-05 (subject verification, OTP) and /me.
// audit/notifySvc may be nil (a caller that doesn't need them, e.g. a test) — never the case in
// cmd/api/cmd/worker.
func IamVerification(keyring *crypto.Keyring, notifySvc *notify.Service, audit *auditservice.Service) *iamservice.Service {
	s := &iamservice.Service{Keyring: keyring}
	if notifySvc != nil {
		s.Notify = iamNotifier{svc: notifySvc}
	}
	if audit != nil {
		s.Audit = iamAuditor{svc: audit}
	}
	return s
}

// iamVersioning adapts *platform/versioning.Service to iamservice.Versioning — versioning itself imports
// iam/service (to resolve a requester's/approver's display name), so iam/service can't import it back.
type iamVersioning struct{ svc *versioning.Service }

func (v iamVersioning) SaveDraft(ctx context.Context, entityType string, entityID uuid.UUID, snapshot any) (iamservice.VersionRef, error) {
	ver, err := v.svc.SaveDraft(ctx, entityType, entityID, snapshot)
	if err != nil {
		return iamservice.VersionRef{}, err
	}
	return iamservice.VersionRef{ID: ver.ID, RowVersion: ver.RowVersion}, nil
}

func (v iamVersioning) Submit(ctx context.Context, id uuid.UUID, rowVersion int32) (iamservice.VersionRef, error) {
	ver, err := v.svc.Submit(ctx, id, rowVersion)
	if err != nil {
		return iamservice.VersionRef{}, err
	}
	return iamservice.VersionRef{ID: ver.ID, RowVersion: ver.RowVersion}, nil
}

// RegisterIamVersioning puts IAM-06's role-assignment-request record type under PLT-08 (one SEC approval
// step; publishing is this request's own real effect — iamSvc.ApplyRoleAssignment) and gives iamSvc the
// adapter it needs to open/submit a request itself (RequestRoleAssignment).
func RegisterIamVersioning(iamSvc *iamservice.Service, versioningSvc *versioning.Service) {
	iamSvc.Versioning = iamVersioning{svc: versioningSvc}
	versioningSvc.Register(iamservice.RoleAssignmentRequestType, versioning.Policy{
		ReadPermission: "admin.role.read", EditPermission: "admin.user.update", PublishPermission: "admin.role.approve",
		Steps: []versioning.Step{{Role: "SEC"}},
		Title: iamSvc.RoleAssignmentRequestTitle,
		OnPublish: func(ctx context.Context, entityID uuid.UUID, snapshot json.RawMessage) error {
			return iamSvc.ApplyRoleAssignment(ctx, entityID, snapshot)
		},
	})
}
