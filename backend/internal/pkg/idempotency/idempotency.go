// Package idempotency implements middleware #10 in docs/architecture/code-structure.md, following
// api/openapi/README.md "Concurrency and idempotency" exactly: a request carrying an
// Idempotency-Key is recorded in Valkey for 24h; a concurrent replay gets 409
// idempotency.in_progress, a replay with a different body gets 422 idempotency.key_reused, and an
// identical replay gets back the original status and body with Idempotent-Replayed: true.
package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/httpx"
)

const ttl = 24 * time.Hour

type record struct {
	Status   string `json:"status"` // in_progress | done
	BodyHash string `json:"body_hash"`
	HTTPCode int    `json:"http_code,omitempty"`
	Body     string `json:"body,omitempty"`
}

type Middleware struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Middleware {
	return &Middleware{rdb: rdb}
}

// Handler wraps next. Requests without an Idempotency-Key header pass straight through — the
// request validator (#[pkg/validate]) already rejects a missing key where the spec requires one.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		principal, _ := httpx.PrincipalFromContext(r.Context())
		redisKey := fmt.Sprintf("idem:%s:%s:%s", principal.TenantID, principal.UserID, key)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.RequestInvalid("could not read request body"))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		bodyHash := sha256Hex(body)

		ctx := r.Context()
		claimed, err := m.rdb.SetNX(ctx, redisKey, mustJSON(record{Status: "in_progress", BodyHash: bodyHash}), ttl).Result()
		if err != nil {
			// Fail open: a Valkey outage must not block writes; idempotency is best-effort then.
			next.ServeHTTP(w, r)
			return
		}

		if !claimed {
			existing, err := m.rdb.Get(ctx, redisKey).Result()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			var rec record
			if json.Unmarshal([]byte(existing), &rec) != nil {
				next.ServeHTTP(w, r)
				return
			}
			switch rec.Status {
			case "in_progress":
				httpx.WriteProblem(w, r, httpx.IdempotencyInProgress())
			case "done":
				if rec.BodyHash != bodyHash {
					httpx.WriteProblem(w, r, httpx.IdempotencyKeyReused())
					return
				}
				w.Header().Set("Idempotent-Replayed", "true")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(rec.HTTPCode)
				_, _ = w.Write([]byte(rec.Body))
			default:
				next.ServeHTTP(w, r)
			}
			return
		}

		rec := httpx.NewRecorder()
		next.ServeHTTP(rec, r)

		_ = m.rdb.Set(ctx, redisKey, mustJSON(record{
			Status:   "done",
			BodyHash: bodyHash,
			HTTPCode: rec.Status(),
			Body:     rec.BodyString(),
		}), ttl).Err()

		rec.Flush(w)
	})
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
