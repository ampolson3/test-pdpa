package authz

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pdpa-platform/internal/pkg/httpx"
)

// RequiredPermission resolves the x-permission value declared for a route (chi's matched pattern,
// e.g. "/admin/v1/me") and HTTP method. Built from the loaded OpenAPI spec at startup — see
// cmd/api's loadPermissions. ok is false when the route isn't in the map (fails closed).
type RequiredPermission func(pattern, method string) (code string, ok bool)

// Middleware is AuthZ, #9 in docs/architecture/code-structure.md: it resolves the grants for the
// principal AuthN (#7) attached to the context (via cache, loading through Loader on a miss) and
// enforces the matched route's x-permission. "public" skips both principal and grants entirely;
// "authenticated" requires a principal and loads grants (later handlers may need them, e.g.
// GET /admin/v1/me) but does not check a specific code.
func Middleware(cache *CachedLoader, required RequiredPermission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pattern := chi.RouteContext(r.Context()).RoutePattern()
			code, ok := required(pattern, r.Method)
			if !ok {
				httpx.WriteProblem(w, r, httpx.NotFound())
				return
			}
			if code == "public" {
				next.ServeHTTP(w, r)
				return
			}

			principal, ok := httpx.PrincipalFromContext(r.Context())
			if !ok {
				httpx.WriteProblem(w, r, httpx.AuthnRequired())
				return
			}

			grants, err := cache.Load(r.Context(), principal.TenantID, principal.UserID)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.Internal())
				return
			}

			if code != "authenticated" && code != "scim" && code != "webhook" && !grants.Has(code) {
				httpx.WriteProblem(w, r, httpx.AuthzDenied())
				return
			}

			next.ServeHTTP(w, r.WithContext(WithGrants(r.Context(), grants)))
		})
	}
}
