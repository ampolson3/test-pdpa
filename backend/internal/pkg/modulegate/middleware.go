// Package modulegate is IAM-03's own acceptance criterion as code: "ปิดโมดูลที่ไม่ได้ซื้อแล้วเมนูและ
// API ของโมดูลนั้นใช้ไม่ได้" (disabling a module the tenant hasn't purchased makes that module's menu
// and API unusable). It's a sibling of internal/pkg/authz.StrictMiddleware, not part of that package,
// because the two check different things against different data (a user's permission grants vs. a
// tenant's license) even though both hook in the same way — as an oapi-codegen strict-server
// middleware keyed by operationId, since chi.RouteContext(ctx).RoutePattern() is empty for anything
// mounted with r.Use() (see cmd/api's loadPermissions for the full explanation AuthZ already gives).
package modulegate

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
)

// coreModules are tagged x-module in the OpenAPI spec but aren't a purchasable epic module in
// platform.tenant_modules — iam (so a locked-out tenant can still sign in and see /me) and platform
// (the shared platform plumbing every module sits on, e.g. notifications/files/jobs). Neither is ever
// gated here; only a real purchasable module (ropa, consent, dsar, breach, ...) is.
var coreModules = map[string]bool{"iam": true, "platform": true}

// Middleware checks requiredModule(operationID) against the caller's tenant's own
// platform.tenant_modules row, using the request's own transaction (already open by the time this
// runs — Tx (#11) opens before AuthZ/module-gate, both oapi-codegen strict middlewares). No row for a
// tenant+module pair means the module is enabled (tenant_modules is opt-out, not opt-in, so an
// existing tenant with nothing explicit set keeps working); an explicit enabled = false blocks with
// 403 platform.module_disabled.
func Middleware[F ~func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error)](
	requiredModule func(operationID string) (code string, ok bool),
) func(f F, operationID string) F {
	return func(f F, operationID string) F {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			code, ok := requiredModule(operationID)
			if !ok || code == "" || coreModules[code] {
				return f(ctx, w, r, request)
			}

			tx, ok := pdb.TxFromContext(ctx)
			if !ok {
				return f(ctx, w, r, request)
			}

			var enabled bool
			err := tx.QueryRow(ctx, `SELECT enabled FROM platform.tenant_modules WHERE tenant_id = current_setting('app.tenant_id')::uuid AND module_code = $1`, code).Scan(&enabled)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return f(ctx, w, r, request)
				}
				httpx.WriteProblem(w, r, httpx.Internal())
				return nil, nil
			}
			if !enabled {
				httpx.WriteProblem(w, r, httpx.ModuleDisabled(code))
				return nil, nil
			}
			return f(ctx, w, r, request)
		}
	}
}
