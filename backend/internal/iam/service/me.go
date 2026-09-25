// Package service holds iam's business rules: reading the acting user's profile, resolving their
// effective roles/permissions/data scopes, and (later) everything else in docs/modules/IAM.md.
// Stores are read only through this package's own transaction from context — see CLAUDE.md rule 1.
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	store "pdpa-platform/internal/iam/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

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

type Service struct{}

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
