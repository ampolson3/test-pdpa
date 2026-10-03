package fileshttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/files"
	fileshttp "pdpa-platform/internal/platform/files/http"
	"pdpa-platform/internal/platform/jobs"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the file endpoints (CLAUDE.md rule 2): 401 without a principal, 201 upload,
// 409 files.scan_pending before the scan, 404 for another user's unattached file, 302 once clean,
// 415 for a disallowed type — all through the generated strict handler and the AuthZ middleware.
func TestFileEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	endpoint := envOr("TEST_S3_ENDPOINT", "127.0.0.1:8333")
	if c, err := net.DialTimeout("tcp", endpoint, time.Second); err != nil {
		t.Skipf("no S3 endpoint at %s", endpoint)
	} else {
		c.Close()
	}
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "fileshttp")
	store, err := files.NewS3Store(files.S3Config{Endpoint: endpoint, AccessKey: envOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: envOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"), Bucket: "pdpa-files-test", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	svc := &files.Service{Store: store, River: client, Config: files.DefaultConfig(), EntityPermissions: map[string]string{}}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), "", func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.files`)
			return err
		})
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE kind LIKE 'files.%' AND args->>'tenant_id' = $1`, tenant.ID.String())
	})

	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tenantID, userID string) (authz.Grants, error) {
		return authz.Grants{TenantID: tenantID, UserID: userID}, nil
	})
	other := uuid.New()
	t.Cleanup(func() {
		_ = cache.Invalidate(context.Background(), tenant.ID.String(), tenant.UserID.String())
		_ = cache.Invalidate(context.Background(), tenant.ID.String(), other.String())
	})
	perms := func(op string) (string, bool) {
		switch op {
		case "PlatformUploadFile", "PlatformGetFile", "PlatformDownloadFile":
			return "authenticated", true
		}
		return "", false
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler { // stand-in for AuthN (#7) + Tx (#11)
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var p httpx.Principal
			if raw := req.Header.Get("X-Test-Principal"); raw != "" && json.Unmarshal([]byte(raw), &p) == nil {
				req = req.WithContext(httpx.WithPrincipal(req.Context(), p))
				_ = pdb.WithTenantTx(req.Context(), app, p.TenantID, p.UserID, func(ctx context.Context) error {
					next.ServeHTTP(w, req.WithContext(ctx))
					return nil
				})
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	strict := fileshttp.NewStrictHandlerWithOptions(fileshttp.NewStrict(svc),
		[]fileshttp.StrictMiddlewareFunc{authz.StrictMiddleware[fileshttp.StrictHandlerFunc](cache, perms)},
		fileshttp.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) { // as cmd/api
				if p, ok := err.(httpx.Problem); ok {
					httpx.WriteProblem(w, r, p)
					return
				}
				httpx.WriteProblem(w, r, httpx.Internal())
			},
		})
	fileshttp.HandlerFromMux(strict, r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	do := func(method, path string, user *uuid.UUID, body *bytes.Buffer, contentType string) *http.Response {
		t.Helper()
		if body == nil {
			body = &bytes.Buffer{}
		}
		req, _ := http.NewRequest(method, srv.URL+path, body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if user != nil {
			raw, _ := json.Marshal(httpx.Principal{TenantID: tenant.ID.String(), UserID: user.String(), ActorType: "user"})
			req.Header.Set("X-Test-Principal", string(raw))
		}
		res, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	multipartBody := func(name string, content []byte) (*bytes.Buffer, string) {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		fw, _ := mw.CreateFormFile("file", name)
		fw.Write(content)
		mw.Close()
		return &b, mw.FormDataContentType()
	}
	pdf := []byte("%PDF-1.4\n%%EOF\n")

	body, ct := multipartBody("a.pdf", pdf)
	if res := do("POST", "/admin/v1/platform/files", nil, body, ct); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("upload without principal: %d, want 401", res.StatusCode)
	}
	uploader := tenant.UserID
	body, ct = multipartBody("a.pdf", pdf)
	res := do("POST", "/admin/v1/platform/files", &uploader, body, ct)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d, want 201", res.StatusCode)
	}
	var stored fileshttp.StoredFile
	if err := json.NewDecoder(res.Body).Decode(&stored); err != nil || stored.AvStatus != "pending" {
		t.Fatalf("upload body: %+v %v", stored, err)
	}

	download := "/admin/v1/platform/files/" + stored.Id.String() + "/download"
	if res := do("GET", download, &uploader, nil, ""); res.StatusCode != http.StatusConflict {
		t.Errorf("download before scan: %d, want 409", res.StatusCode)
	} else {
		var p httpx.Problem
		_ = json.NewDecoder(res.Body).Decode(&p)
		if p.Code != "files.scan_pending" {
			t.Errorf("problem code %q, want files.scan_pending", p.Code)
		}
	}
	if res := do("GET", "/admin/v1/platform/files/"+stored.Id.String(), &other, nil, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("another user's unattached file: %d, want 404", res.StatusCode)
	}

	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), "", func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE platform.files SET av_status = 'clean' WHERE id = $1`, stored.Id)
		return err
	})
	res = do("GET", download, &uploader, nil, "")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") == "" {
		t.Errorf("download when clean: %d Location=%q, want 302 to a signed URL", res.StatusCode, res.Header.Get("Location"))
	}

	body, ct = multipartBody("tool.exe", []byte("MZ\x90\x00"))
	if res := do("POST", "/admin/v1/platform/files", &uploader, body, ct); res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("exe upload: %d, want 415", res.StatusCode)
	}
}
