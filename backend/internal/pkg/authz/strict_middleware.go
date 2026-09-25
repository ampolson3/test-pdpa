package authz

import (
	"context"
	"net/http"

	"pdpa-platform/internal/pkg/httpx"
)

// StrictMiddleware is AuthZ (#9 in docs/architecture/code-structure.md) implemented as an
// oapi-codegen strict-server middleware, keyed by operationId, rather than a chi-level
// http.Handler middleware keyed by route pattern: chi.RouteContext(ctx).RoutePattern() is empty
// for anything mounted with r.Use() (it's only populated once the mux is dispatching to the
// matched route's own handler chain, which is after every top-level Use() middleware has already
// run) — see cmd/api's loadPermissions for the full explanation. This still runs the same way
// AuthZ always does: it resolves the caller's grants (via cache, loading through Loader — its own
// read-only db.WithTenantTx — on a miss) and enforces the operation's x-permission.
//
// F is a type parameter rather than this package's own concrete function type because every
// module's oapi-codegen output declares its own StrictHandlerFunc / StrictMiddlewareFunc types;
// they're structurally identical (same underlying function signature) but distinct named types, so
// a type parameter constrained by that shared underlying signature lets this same middleware value
// be used as any module's StrictMiddlewareFunc without a manual per-module adapter.
func StrictMiddleware[F ~func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error)](
	cache *CachedLoader,
	requiredPermission func(operationID string) (code string, ok bool),
) func(f F, operationID string) F {
	return func(f F, operationID string) F {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			code, ok := requiredPermission(operationID)
			if !ok {
				httpx.WriteProblem(w, r, httpx.NotFound())
				return nil, nil
			}
			if code == "public" {
				return f(ctx, w, r, request)
			}

			principal, ok := httpx.PrincipalFromContext(ctx)
			if !ok {
				httpx.WriteProblem(w, r, httpx.AuthnRequired())
				return nil, nil
			}

			grants, err := cache.Load(ctx, principal.TenantID, principal.UserID)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.Internal())
				return nil, nil
			}

			if code != "authenticated" && code != "scim" && code != "webhook" && !grants.Has(code) {
				httpx.WriteProblem(w, r, httpx.AuthzDenied())
				return nil, nil
			}

			return f(WithGrants(ctx, grants), w, r, request)
		}
	}
}
