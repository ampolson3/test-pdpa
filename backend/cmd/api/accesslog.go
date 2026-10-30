package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/metrics"
)

// slowRequestThreshold is PLT-19's own "latency เกินเกณฑ์" cut-off for a single request counted into
// the per-minute "slow" bucket — a tunable, not a decisions.md question (it only shapes the dashboard,
// never a contract or legal behaviour).
const slowRequestThreshold = 1 * time.Second

// accessLog is #4 in docs/architecture/code-structure.md: "Access log | JSON · no PII". It logs
// only routing/timing facts — never headers, query strings or bodies, which may carry personal
// data or tokens (CLAUDE.md rule 3). It also feeds PLT-19's own health dashboard (internal/pkg/metrics)
// with the same status/duration it already computed for the log line, best-effort over Redis.
func accessLog(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			dur := time.Since(start)
			slog.Info("http_request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", dur.Milliseconds(),
			)
			metrics.Record(r.Context(), rdb, "api", ww.Status(), dur, slowRequestThreshold)
		})
	}
}
