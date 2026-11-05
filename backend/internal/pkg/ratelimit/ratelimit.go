// Package ratelimit is the fixed-window Valkey limiter behind middleware #6 in
// docs/architecture/code-structure.md ("Rate limit | Valkey").
package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/clientip"
	"pdpa-platform/internal/pkg/httpx"
)

// Limiter counts requests per key in fixed windows of length Window, allowing at most Limit per
// window. It's an approximation of the token-bucket api/openapi/README.md describes — precise
// enough to stop abuse without a second moving part.
type Limiter struct {
	rdb    *redis.Client
	Limit  int
	Window time.Duration
}

func New(rdb *redis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, Limit: limit, Window: window}
}

// Allow increments the counter for key and reports whether the caller is still within budget, plus
// how long until the window resets (for the Retry-After header).
func (l *Limiter) Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error) {
	windowKey := fmt.Sprintf("ratelimit:%s:%d", key, time.Now().Unix()/int64(l.Window/time.Second))

	count, err := l.rdb.Incr(ctx, windowKey).Result()
	if err != nil {
		return false, 0, fmt.Errorf("ratelimit: incr: %w", err)
	}
	if count == 1 {
		l.rdb.Expire(ctx, windowKey, l.Window)
	}

	if count > int64(l.Limit) {
		ttl, _ := l.rdb.TTL(ctx, windowKey).Result()
		return false, ttl, nil
	}
	return true, 0, nil
}

// Middleware rate-limits by client IP — the only identity available this early in the chain (#6
// runs before AuthN #7). A per-user or per-API-client limit can layer on top once that identity is
// known, inside the AuthZ or a module's own handler.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The client address clientip.Middleware resolved (trusted proxies, D-23), else the TCP peer.
		var ip string
		if a, ok := clientip.From(r.Context()); ok {
			ip = a.String()
		} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}

		allowed, retryAfter, err := l.Allow(r.Context(), ip)
		if err != nil {
			// Fail open: a Valkey outage must not take the API down.
			next.ServeHTTP(w, r)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			httpx.WriteProblem(w, r, httpx.RateLimited())
			return
		}
		next.ServeHTTP(w, r)
	})
}
