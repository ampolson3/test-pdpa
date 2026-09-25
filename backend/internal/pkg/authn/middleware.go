package authn

import (
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"pdpa-platform/internal/pkg/httpx"
)

// Claims is the subset of a Keycloak access token this service reads. TenantID comes from the
// `tid` protocol mapper decided in docs/decisions.md Q-18 (Keycloak Organizations, one org per
// tenant); Acr reflects whether step-up/MFA was performed for this session.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tid"`
	Acr      string `json:"acr,omitempty"`
}

// Verifier validates adminJwt bearer tokens (api/openapi/openapi.yaml) against a tenant's Keycloak
// realm. It only proves who is asking (#7) and, for this surface, which tenant (#8) — it enforces
// no permission itself.
type Verifier struct {
	jwks   *JWKS
	issuer string
}

func NewVerifier(jwks *JWKS, issuer string) *Verifier {
	return &Verifier{jwks: jwks, issuer: issuer}
}

// Middleware requires a valid, unexpired Bearer token issued by v.issuer and attaches an
// httpx.Principal to the context. Do not put it in front of routes whose x-permission is "public".
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString, ok := bearerToken(r)
		if !ok {
			httpx.WriteProblem(w, r, httpx.AuthnRequired())
			return
		}

		claims := &Claims{}
		_, err := jwt.ParseWithClaims(tokenString, claims, v.keyfunc,
			jwt.WithIssuer(v.issuer),
			jwt.WithValidMethods([]string{"RS256"}),
		)
		if err != nil || claims.Subject == "" || claims.TenantID == "" {
			httpx.WriteProblem(w, r, httpx.AuthnRequired())
			return
		}

		principal := httpx.Principal{
			UserID:     claims.Subject,
			TenantID:   claims.TenantID,
			ActorType:  "user",
			MFAAcrHigh: claims.Acr != "",
		}
		next.ServeHTTP(w, r.WithContext(httpx.WithPrincipal(r.Context(), principal)))
	})
}

func (v *Verifier) keyfunc(token *jwt.Token) (any, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("authn: token missing kid header")
	}
	return v.jwks.Key(kid)
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	return strings.TrimPrefix(h, prefix), true
}
