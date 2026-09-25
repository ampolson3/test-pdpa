// Package authz holds the mechanics of x-permission enforcement (context carrier + cache) with no
// business logic: computing the actual grants is internal/iam's job, since it is the only module
// allowed to read iam.role_assignments / iam.role_permissions (CLAUDE.md rule 9). The AuthZ
// middleware (#9 in docs/architecture/code-structure.md) calls a Loader supplied by internal/iam.
package authz

import "context"

// DataScope narrows a grant to less than the whole tenant. See docs/security/permissions.md.
type DataScope struct {
	ScopeType          string  `json:"scope_type"` // tenant | legal_entity | org_unit | self
	LegalEntityID      *string `json:"legal_entity_id,omitempty"`
	OrgUnitID          *string `json:"org_unit_id,omitempty"`
	IncludeDescendants bool    `json:"include_descendants"`
}

// Grants is the effective permission set for one user in one tenant, computed once per request (or
// on a cache miss) and reused by every module's service layer during that request.
type Grants struct {
	UserID      string      `json:"user_id"`
	TenantID    string      `json:"tenant_id"`
	Roles       []string    `json:"roles"`
	Permissions []string    `json:"permissions"`
	Scopes      []DataScope `json:"scopes"`
	MFAEnrolled bool        `json:"mfa_enrolled"`
}

// Has reports whether code (an x-permission value, or "authenticated") is in the grant set.
// "authenticated" and "public" are not real permission codes — callers check those separately.
func (g Grants) Has(code string) bool {
	for _, p := range g.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

type ctxKey struct{}

// WithGrants attaches g to ctx. Called once by the AuthZ middleware.
func WithGrants(ctx context.Context, g Grants) context.Context {
	return context.WithValue(ctx, ctxKey{}, g)
}

// FromContext returns the Grants attached by the AuthZ middleware.
func FromContext(ctx context.Context) (Grants, bool) {
	g, ok := ctx.Value(ctxKey{}).(Grants)
	return g, ok
}

// Loader computes the effective Grants for a user, reading iam.role_assignments,
// iam.role_permissions and iam.roles under its own read-only transaction. Implemented by
// internal/iam and injected into the AuthZ middleware and CachedLoader.
type Loader func(ctx context.Context, tenantID, userID string) (Grants, error)
