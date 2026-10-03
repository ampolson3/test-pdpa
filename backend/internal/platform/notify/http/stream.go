package notifyhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/notify"
)

// Stream serves GET /admin/v1/platform/inbox/stream (operation platformStreamInbox): server-sent events
// with the signed-in user's unread in-app count, sent on connect and whenever it changes. It is mounted
// outside the Tx middleware — that one buffers the whole response until COMMIT, which a stream never
// reaches — and instead opens one short WithTenantTx per check, so no transaction stays open while the
// connection idles. Connections end after MaxAge; EventSource reconnects on its own.
type Stream struct {
	Service  *notify.Service
	Pool     *pgxpool.Pool
	Interval time.Duration // how often the count is checked; default 3 s
	MaxAge   time.Duration // default 30 min
}

func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, httpx.AuthnRequired())
		return
	}
	user, err := uuid.Parse(p.UserID)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.AuthnRequired())
		return
	}
	interval, maxAge := s.Interval, s.MaxAge
	if interval <= 0 {
		interval = 3 * time.Second
	}
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx-style proxies: don't buffer the stream
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithTimeout(r.Context(), maxAge)
	defer cancel()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	last := int64(-1)
	quiet := 0
	for {
		n, err := s.unread(ctx, p.TenantID, user)
		if err != nil {
			return // the client reconnects
		}
		if n != last {
			fmt.Fprintf(w, "event: unread\ndata: {\"unread\":%d}\n\n", n)
			last, quiet = n, 0
		} else if quiet++; quiet*int(interval/time.Second) >= 25 {
			fmt.Fprint(w, ": keep-alive\n\n")
			quiet = 0
		}
		if rc.Flush() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Stream) unread(ctx context.Context, tenant string, user uuid.UUID) (int64, error) {
	var n int64
	err := pdb.WithTenantTx(ctx, s.Pool, tenant, user.String(), func(ctx context.Context) error {
		var err error
		n, err = s.Service.Unread(ctx, user)
		return err
	})
	return n, err
}
