// Package publickeys issues and resolves the keys in platform.public_keys (decisions.md D-10): /public/v1 and the
// portal have no login, so the key a form or SDK carries is what names the tenant — resolved before the tenant
// transaction opens, then the request runs under that tenant like any other.
package publickeys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
	pkstore "pdpa-platform/internal/platform/publickeys/store"
)

// Entity types a key can stand for (platform.public_keys.entity_type).
const (
	EntityCollectionPoint = "collection_point"
)

// Key is a resolved public key.
type Key struct {
	Key            string
	TenantID       uuid.UUID
	EntityType     string
	EntityID       uuid.UUID
	AllowedOrigins []string
}

// Issue creates a key for an entity of the transaction's tenant: 24 random bytes, base64url (192 bits).
func Issue(ctx context.Context, entityType string, entityID uuid.UUID, allowedOrigins []string) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := base64.RawURLEncoding.EncodeToString(b)
	if allowedOrigins == nil {
		allowedOrigins = []string{}
	}
	err := pkstore.New(pdb.MustTxFromContext(ctx)).InsertPublicKey(ctx, pkstore.InsertPublicKeyParams{Key: key, EntityType: entityType, EntityID: entityID, AllowedOrigins: allowedOrigins})
	return key, err
}

// Revoke stops a key of the transaction's tenant from resolving.
func Revoke(ctx context.Context, key string) error {
	_, err := pkstore.New(pdb.MustTxFromContext(ctx)).RevokePublicKey(ctx, key)
	return err
}

// SetOrigins replaces the origins a browser may call with this key from (empty: any).
func SetOrigins(ctx context.Context, key string, origins []string) error {
	if origins == nil {
		origins = []string{}
	}
	_, err := pkstore.New(pdb.MustTxFromContext(ctx)).SetPublicKeyOrigins(ctx, pkstore.SetPublicKeyOriginsParams{Key: key, AllowedOrigins: origins})
	return err
}

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)

// Resolve finds an active key without a tenant transaction (the table is readable by every request).
func Resolve(ctx context.Context, pool *pgxpool.Pool, key string) (Key, bool, error) {
	if !keyRE.MatchString(key) {
		return Key{}, false, nil
	}
	r, err := pkstore.New(pool).ResolvePublicKey(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Key{}, false, nil
	}
	if err != nil {
		return Key{}, false, err
	}
	return Key{Key: r.Key, TenantID: r.TenantID, EntityType: r.EntityType, EntityID: r.EntityID, AllowedOrigins: r.AllowedOrigins}, true, nil
}

type ctxKey struct{}

// FromContext returns the key the Middleware resolved for this request.
func FromContext(ctx context.Context) (Key, bool) {
	k, ok := ctx.Value(ctxKey{}).(Key)
	return k, ok
}

// WithKey puts a resolved key in ctx (tests, and callers that resolved it themselves).
func WithKey(ctx context.Context, k Key) context.Context { return context.WithValue(ctx, ctxKey{}, k) }

var pathKeyRE = regexp.MustCompile(`^/public/v1/collection-points/([^/]+)`)

// Middleware resolves the public key of a /public/v1 request — from the path (/public/v1/collection-points/{key})
// or the X-Public-Key header — refuses browsers from origins the key doesn't allow, and sets a data-subject
// principal of the key's tenant so the ordinary Tx + audit middleware run the request. Unknown or revoked keys
// are 404, like a missing record.
func Middleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("X-Public-Key")
			if m := pathKeyRE.FindStringSubmatch(r.URL.Path); m != nil {
				raw = m[1]
			}
			k, ok, err := Resolve(r.Context(), pool, raw)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.Internal())
				return
			}
			if !ok {
				httpx.WriteProblem(w, r, httpx.NotFound())
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && len(k.AllowedOrigins) > 0 && !slices.ContainsFunc(k.AllowedOrigins, func(o string) bool {
				return strings.EqualFold(strings.TrimRight(o, "/"), origin)
			}) {
				httpx.WriteProblem(w, r, httpx.AuthzDenied())
				return
			}
			ctx := httpx.WithPrincipal(r.Context(), httpx.Principal{TenantID: k.TenantID.String(), ActorType: "data_subject"})
			next.ServeHTTP(w, r.WithContext(WithKey(ctx, k)))
		})
	}
}
