// Package authn verifies the Keycloak-issued JWTs the admin BFF attaches server-side
// (adminJwt in api/openapi/openapi.yaml) — AuthN, middleware #7 in
// docs/architecture/code-structure.md. No business logic: it only proves who is asking.
package authn

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// JWKS fetches and caches a JSON Web Key Set, refreshing it at most every minRefresh.
type JWKS struct {
	url        string
	minRefresh time.Duration
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewJWKS(url string) *JWKS {
	return &JWKS{url: url, minRefresh: time.Minute, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

type jwksDoc struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
		Use string `json:"use"`
	} `json:"keys"`
}

// Key returns the RSA public key for kid, fetching (or refreshing) the JWKS document if kid is
// unknown and the cache is older than minRefresh — covers Keycloak rotating its signing key.
func (j *JWKS) Key(kid string) (*rsa.PublicKey, error) {
	j.mu.RLock()
	key, ok := j.keys[kid]
	stale := time.Since(j.fetchedAt) > j.minRefresh
	j.mu.RUnlock()
	if ok && !stale {
		return key, nil
	}

	if err := j.refresh(); err != nil {
		if ok {
			return key, nil // serve the stale key rather than fail a request over a transient fetch error
		}
		return nil, err
	}

	j.mu.RLock()
	defer j.mu.RUnlock()
	key, ok = j.keys[kid]
	if !ok {
		return nil, fmt.Errorf("authn: no JWKS key for kid %q", kid)
	}
	return key, nil
}

func (j *JWKS) refresh() error {
	resp, err := j.httpClient.Get(j.url)
	if err != nil {
		return fmt.Errorf("authn: fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authn: fetch JWKS: status %d", resp.StatusCode)
	}

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("authn: decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	j.mu.Lock()
	j.keys = keys
	j.fetchedAt = time.Now()
	j.mu.Unlock()
	return nil
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
