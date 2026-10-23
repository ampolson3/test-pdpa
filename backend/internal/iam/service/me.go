// Package service holds iam's business rules: reading the acting user's profile, resolving their
// effective roles/permissions/data scopes, and (later) everything else in docs/modules/IAM.md.
// Stores are read only through this package's own transaction from context — see CLAUDE.md rule 1.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	store "pdpa-platform/internal/iam/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
)

// AuditEntry mirrors platform/audit/service.Entry's fields (TenantID/ActorType/ActorID/EntityType/
// EntityID/Action/Before/After) without importing that package — audit/service itself imports iam/service
// (ORG-19's actor-name resolution), so importing it back here would be a cycle. cmd/api and cmd/worker adapt
// the real *audit.Service to this Auditor interface (internal/wiring.IamAuditor).
type AuditEntry struct {
	TenantID   uuid.UUID
	ActorType  string
	ActorID    *uuid.UUID
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	Before     any
	After      any
}

type Auditor interface {
	Write(ctx context.Context, e AuditEntry) error
}

// NotifyRequest mirrors the fields of platform/notify/service.Request that IAM-05 needs — notify itself
// imports iam/service (Contact resolution for a user recipient), so it can't be imported back here either;
// internal/wiring.IamVerification adapts the real *notify.Service to this Notifier interface.
type NotifyRequest struct {
	TemplateCode     string
	Channel          string
	RecipientAddress string
	Vars             map[string]any
	Urgent           bool
}

type Notifier interface {
	Send(ctx context.Context, req NotifyRequest) (uuid.UUID, error)
}

// ErrNoGrants is returned by Me when it is called outside the AuthZ middleware (#9), which is the
// only place authz.Grants gets attached to the context — a programmer error, not a runtime one.
var ErrNoGrants = fmt.Errorf("iam: no grants in context — AuthZ middleware must run before this handler")

type Tenant struct {
	ID   uuid.UUID
	Code string
	Name string
}

// Me is the signed-in user's profile plus their effective permission set for this tenant — the
// domain result behind GET /admin/v1/me (SEQ-01).
type Me struct {
	ID          uuid.UUID
	DisplayName string
	EmailMasked string
	Locale      string
	Tenant      Tenant
	Roles       []string
	Permissions []string
	Scopes      []authz.DataScope
	MFAEnrolled bool
}

// Keyring, Notify and Audit are IAM-05's own dependencies (verification.go); nil in code paths (like Me)
// that don't need them.
type Service struct {
	Keyring *crypto.Keyring
	Notify  Notifier
	Audit   Auditor
	// Now is the clock (injectable for tests); nil means time.Now.
	Now func() time.Time
}

func New() *Service { return &Service{} }

// Me loads the user's profile and tenant, and combines them with the effective grants the AuthZ
// middleware (#9) already resolved (via CachedLoader, backed by NewLoader) into context — so this
// call adds exactly one profile query on top of what authorization already paid for. Must run
// inside a transaction opened by db.WithTenantTx so RLS already limits every row read here to the
// caller's tenant.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (Me, error) {
	grants, ok := authz.FromContext(ctx)
	if !ok {
		return Me{}, ErrNoGrants
	}

	q := store.New(pdb.MustTxFromContext(ctx))

	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return Me{}, fmt.Errorf("iam: load user: %w", err)
	}

	tenant, err := q.GetTenantByID(ctx, user.TenantID)
	if err != nil {
		return Me{}, fmt.Errorf("iam: load tenant: %w", err)
	}

	return Me{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		EmailMasked: maskEmail(user.Email),
		Locale:      user.Locale,
		Tenant:      Tenant{ID: tenant.ID, Code: tenant.Code, Name: tenant.Name},
		Roles:       grants.Roles,
		Permissions: grants.Permissions,
		Scopes:      grants.Scopes,
		MFAEnrolled: user.MfaRequired,
	}, nil
}

// maskEmail keeps the first two characters of the local part, per the Me example in
// api/openapi/openapi.yaml (somsri@example.co.th -> so****@example.co.th). Full values are never
// logged or returned without going through the unmask endpoint (CLAUDE.md rule 3).
func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "****"
	}
	local, domain := email[:at], email[at:]
	keep := min(2, len(local))
	return local[:keep] + "****" + domain
}

func uuidPtr(v pgtype.UUID) *string {
	if !v.Valid {
		return nil
	}
	s := uuid.UUID(v.Bytes).String()
	return &s
}
