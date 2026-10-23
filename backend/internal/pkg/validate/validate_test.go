package validate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"pdpa-platform/internal/pkg/validate"
)

// Runs the middleware against the real contract with a Host that isn't the spec's `servers` host —
// the situation in every deployment and in local dev.
func TestMiddleware_ValidatesAgainstSpecRegardlessOfHost(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromFile("../../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	mw, err := validate.Middleware(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	cases := []struct {
		name, target, lang string
		want               int
	}{
		{"valid list", "/admin/v1/platform/jobs?state=running,retryable&limit=10", "", http.StatusNoContent},
		{"unknown state", "/admin/v1/platform/jobs?state=bogus", "", http.StatusBadRequest},
		{"limit above max", "/admin/v1/platform/jobs?limit=999", "", http.StatusBadRequest},
		{"kind too long", "/admin/v1/platform/jobs?kind=" + longKind, "", http.StatusBadRequest},
		{"raw browser Accept-Language", "/admin/v1/me", "en-US,en;q=0.9", http.StatusBadRequest},
		{"th Accept-Language", "/admin/v1/me", "th", http.StatusNoContent},
		{"unknown route passes through", "/nope", "", http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost:8080"+tc.target, nil)
			if tc.lang != "" {
				req.Header.Set("Accept-Language", tc.lang)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

var longKind = func() string {
	b := make([]byte, 101)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}()
