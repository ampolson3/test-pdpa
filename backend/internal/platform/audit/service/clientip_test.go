package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pdpa-platform/internal/pkg/clientip"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
)

// Decision D-23: behind a trusted load balancer the request audit records the client's address from
// X-Forwarded-For; from anyone else the header is ignored and the TCP peer is recorded.
func TestRequestAudit_RecordsClientBehindTrustedProxy(t *testing.T) {
	f := setup(t)
	res, err := clientip.Parse("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	h := res.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(httpx.WithPrincipal(r.Context(), httpx.Principal{TenantID: f.a.ID.String(), UserID: f.a.UserID.String(), ActorType: "user"}))
		f.svc.TxMiddleware(f.app)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(w, r)
	}))
	lastIP := func() string {
		t.Helper()
		var ip string
		if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
			return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT host(ip) FROM platform.audit_log ORDER BY occurred_at DESC, id DESC LIMIT 1`).Scan(&ip)
		}); err != nil {
			t.Fatal(err)
		}
		return ip
	}
	for _, c := range []struct{ peer, xff, want string }{
		{"10.0.0.2:443", "6.6.6.6, 198.51.100.9", "198.51.100.9"},
		{"203.0.113.7:5000", "198.51.100.9", "203.0.113.7"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/admin/v1/me", nil)
		req.RemoteAddr = c.peer
		req.Header.Set("X-Forwarded-For", c.xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status %d", rec.Code)
		}
		if got := lastIP(); got != c.want {
			t.Errorf("peer %s: audited %s, want %s", c.peer, got, c.want)
		}
	}
	if r := f.verify(t, f.a); !r.OK {
		t.Errorf("chain: %+v", r)
	}
}
