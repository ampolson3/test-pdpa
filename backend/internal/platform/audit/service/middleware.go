package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"regexp"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
)

// TxMiddleware is #11 (Tx) and #13 (Audit) from docs/architecture/code-structure.md fused into one
// step, because the doc requires them to share a transaction: "Tx (#11) ครอบ Handler (#12) และ
// Audit (#13): audit ระดับ request เขียนใน transaction เดียวกันก่อน COMMIT". It opens the one
// transaction for this request via db.WithTenantTx, runs next inside it buffered through an
// httpx.Recorder (so nothing reaches the client until the transaction actually commits), appends
// one audit_log row, and only then flushes the buffered response. A handler that answered >= 500
// rolls the transaction back and the client gets a fresh 500 instead of whatever was buffered.
func (s *Service) TxMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := httpx.PrincipalFromContext(r.Context())
			if !ok {
				httpx.WriteProblem(w, r, httpx.AuthnRequired())
				return
			}

			rec := httpx.NewRecorder()

			err := pdb.WithTenantTx(r.Context(), pool, principal.TenantID, principal.UserID, func(ctx context.Context) error {
				next.ServeHTTP(rec, r.WithContext(ctx))
				if rec.Status() >= 500 {
					return fmt.Errorf("handler returned %d", rec.Status())
				}

				// r.URL.Path (not a chi route pattern): chi.RouteContext(ctx).RoutePattern() is
				// empty for middleware mounted with r.Use() — see cmd/api's loadPermissions.
				entry := Entry{ActorType: principal.ActorType, Action: RequestAction(r.Method, r.URL.Path)}
				if tid, err := uuid.Parse(principal.TenantID); err == nil {
					entry.TenantID = tid
				}
				if uid, err := uuid.Parse(principal.UserID); err == nil {
					entry.ActorID = &uid
				}
				// The TCP peer. Behind a load balancer this is the proxy's address until trusted
				// X-Forwarded-For handling is configured (open item, see PLT-12 in CLAUDE.md).
				if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
					ip := ap.Addr().Unmap()
					entry.IP = &ip
				}
				entry.UserAgent = r.UserAgent()
				return s.Write(ctx, entry)
			})

			if err != nil {
				// A handler's own 500 was already logged by it; this covers commit and audit failures.
				slog.ErrorContext(r.Context(), "request transaction failed", "request_id", middleware.GetReqID(r.Context()), "path", r.URL.Path, "error", err.Error())
				httpx.WriteProblem(w, r, httpx.Internal())
				return
			}
			rec.Flush(w)
		})
	}
}

var uuidInPath = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// RequestAction is the audit action of a request: method and path with ids replaced by {id}, so the same
// operation reads the same across records, cut to fit platform.audit_log.action (varchar(80)).
func RequestAction(method, path string) string {
	a := method + " " + uuidInPath.ReplaceAllString(path, "{id}")
	if r := []rune(a); len(r) > 80 {
		a = string(r[:79]) + "…"
	}
	return a
}
