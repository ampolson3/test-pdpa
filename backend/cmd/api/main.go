// Command api is the PDPA platform's HTTP API (backend/cmd/api), serving /admin/v1, /portal/v1,
// /public/v1, /api/v1, /scim/v2 and /webhooks per api/openapi/README.md. Only /admin/v1/me is wired
// so far — the P0 reference slice from .claude/commands/scaffold.md.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	iamhttp "pdpa-platform/internal/iam/http"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authn"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/idempotency"
	"pdpa-platform/internal/pkg/otelx"
	"pdpa-platform/internal/pkg/ratelimit"
	"pdpa-platform/internal/pkg/validate"
	auditservice "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/collab"
	collabhttp "pdpa-platform/internal/platform/collab/http"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/files"
	fileshttp "pdpa-platform/internal/platform/files/http"
	"pdpa-platform/internal/platform/importer"
	importerhttp "pdpa-platform/internal/platform/importer/http"
	"pdpa-platform/internal/platform/jobs"
	jobshttp "pdpa-platform/internal/platform/jobs/http"
	"pdpa-platform/internal/platform/notify"
	notifyhttp "pdpa-platform/internal/platform/notify/http"
	"pdpa-platform/internal/wiring"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownOtel, err := otelx.Setup(ctx, "pdpa-api", cfg.OTelEndpoint)
	if err != nil {
		return err
	}
	defer shutdownOtel(context.Background())

	pool, err := pdb.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.ValkeyAddr})
	defer rdb.Close()

	spec, err := openapi3.NewLoader().LoadFromFile(cfg.OpenAPISpecPath)
	if err != nil {
		return err
	}
	if err := spec.Validate(ctx); err != nil {
		return err
	}
	permissions, err := loadPermissions(spec)
	if err != nil {
		return err
	}
	requiredPermission := func(operationID string) (string, bool) {
		code, ok := permissions[operationID]
		return code, ok
	}

	validateMw, err := validate.Middleware(spec, nil)
	if err != nil {
		return err
	}

	iamSvc := iamservice.New()
	authzCache := authz.NewCachedLoader(rdb, iamservice.NewLoader(pool))
	auditSvc := auditservice.New()
	jwks := authn.NewJWKS(cfg.OIDCJWKSURL)
	verifier := authn.NewVerifier(jwks, cfg.OIDCIssuer)
	limiter := ratelimit.New(rdb, 100, time.Minute)
	idemMw := idempotency.New(rdb)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(otelx.Middleware("pdpa-api"))
	r.Use(middleware.Recoverer)
	r.Use(accessLog)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", "If-Match", "Accept-Language"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(limiter.Middleware)
	r.Use(maxBody(files.DefaultConfig().MaxBytes))
	// Structural request validation isn't one of the 13 named middlewares in
	// docs/architecture/code-structure.md, but oapi-codegen's strict server assumes the request
	// already matches the spec by the time a handler runs, so it sits here, before AuthN.
	r.Use(validateMw)
	r.Use(verifier.Middleware) // AuthN (#7) — every mounted route today requires adminJwt
	// AuthZ (#9) is NOT here: chi.RouteContext(ctx).RoutePattern() is empty for anything mounted
	// with r.Use() (only populated once the mux is dispatching to the matched route), so it runs
	// as an oapi-codegen strict middleware instead, keyed by operationId — see
	// internal/pkg/authz.StrictMiddleware and loadPermissions in permissions.go. That does mean it
	// runs after Tx opens its transaction rather than strictly before, as the numbered list implies;
	// that's fine because AuthZ's cache loader always opens its own separate read-only transaction
	// regardless of when it's called, so it never touches the request's own transaction.

	requestError := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
	}
	// A handler may return an httpx.Problem as its error to have it written (and localized) here;
	// anything else is an internal error whose details never reach the client.
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		var p httpx.Problem
		if errors.As(err, &p) {
			httpx.WriteProblem(w, r, p)
			return
		}
		// Logged so a 500 can be traced (request_id is in the client's problem too). Handler errors come
		// from our own code and the database driver; they carry no request data (CLAUDE.md rule 3).
		slog.ErrorContext(r.Context(), "handler error", "request_id", middleware.GetReqID(r.Context()), "path", r.URL.Path, "error", err.Error())
		httpx.WriteProblem(w, r, httpx.Internal())
	}

	// Insert-only River client: enqueues jobs on the request transaction (jobs.Enqueue) and lists
	// them for the job-status page; cmd/worker is the only process that works them.
	riverClient, err := jobs.NewInsertClient(pool)
	if err != nil {
		return err
	}
	kek, err := crypto.KEKFromEnv()
	if err != nil {
		return err
	}
	keyring := &crypto.Keyring{KEK: kek}
	notifySvc := &notify.Service{Keyring: keyring, River: riverClient, Quiet: notify.DefaultQuietHours()}
	store, err := files.NewS3Store(files.S3ConfigFromEnv())
	if err != nil {
		return err
	}
	fileSvc := &files.Service{Store: store, River: riverClient, Config: files.DefaultConfig()}

	// Record types that take comments, attachments and an activity feed (PLT-07). Each module registers
	// its own here as it is built; attachments of a record are downloadable with its read permission.
	collabSvc := &collab.Service{Notify: notifySvc, Files: fileSvc, Audit: auditSvc}
	collabSvc.Register("notification_template", collab.Policy{
		ReadPermission: "admin.notification.read", WritePermission: "admin.notification.update",
		Exists: func(ctx context.Context, id uuid.UUID) (bool, error) {
			_, err := notifySvc.GetTemplate(ctx, id)
			if errors.Is(err, notify.ErrNotFound) {
				return false, nil
			}
			return err == nil, err
		},
	})
	fileSvc.EntityPermissions = collabSvc.FilePermissions()

	// Bulk import (PLT-14): the same registry as cmd/worker's (importTypes in imports.go).
	importSvc := &importer.Service{Types: wiring.ImportTypes(), Files: fileSvc, River: riverClient, Audit: auditSvc}

	// The inbox stream (SSE) is the one route outside Idempotency + Tx: the Tx middleware buffers the
	// response until COMMIT, which a stream never reaches. It opens a short transaction per check itself.
	r.Method(http.MethodGet, "/admin/v1/platform/inbox/stream", &notifyhttp.Stream{Service: notifySvc, Pool: pool})

	r.Group(func(g chi.Router) {
		g.Use(idemMw.Handler)              // Idempotency (#10)
		g.Use(auditSvc.TxMiddleware(pool)) // Tx (#11) + Audit (#13)

		strictIam := iamhttp.NewStrictHandlerWithOptions(iamhttp.NewStrict(iamSvc),
			[]iamhttp.StrictMiddlewareFunc{authz.StrictMiddleware[iamhttp.StrictHandlerFunc](authzCache, requiredPermission)},
			iamhttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		iamhttp.HandlerWithOptions(strictIam, iamhttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})

		strictFiles := fileshttp.NewStrictHandlerWithOptions(fileshttp.NewStrict(fileSvc),
			[]fileshttp.StrictMiddlewareFunc{authz.StrictMiddleware[fileshttp.StrictHandlerFunc](authzCache, requiredPermission)},
			fileshttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		fileshttp.HandlerWithOptions(strictFiles, fileshttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})

		strictJobs := jobshttp.NewStrictHandlerWithOptions(jobshttp.NewStrict(riverClient),
			[]jobshttp.StrictMiddlewareFunc{authz.StrictMiddleware[jobshttp.StrictHandlerFunc](authzCache, requiredPermission)},
			jobshttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		jobshttp.HandlerWithOptions(strictJobs, jobshttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})

		strictCollab := collabhttp.NewStrictHandlerWithOptions(collabhttp.NewStrict(collabSvc),
			[]collabhttp.StrictMiddlewareFunc{authz.StrictMiddleware[collabhttp.StrictHandlerFunc](authzCache, requiredPermission)},
			collabhttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		collabhttp.HandlerWithOptions(strictCollab, collabhttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})

		strictImports := importerhttp.NewStrictHandlerWithOptions(importerhttp.NewStrict(importSvc),
			[]importerhttp.StrictMiddlewareFunc{authz.StrictMiddleware[importerhttp.StrictHandlerFunc](authzCache, requiredPermission)},
			importerhttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		importerhttp.HandlerWithOptions(strictImports, importerhttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})

		strictNotify := notifyhttp.NewStrictHandlerWithOptions(notifyhttp.NewStrict(notifySvc),
			[]notifyhttp.StrictMiddlewareFunc{authz.StrictMiddleware[notifyhttp.StrictHandlerFunc](authzCache, requiredPermission)},
			notifyhttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		notifyhttp.HandlerWithOptions(strictNotify, notifyhttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("api: listening", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
