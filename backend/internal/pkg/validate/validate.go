// Package validate wraps kin-openapi's request validator as a chi middleware so every request is
// checked against api/openapi/openapi.yaml before it reaches a handler. Per api/openapi/README.md,
// a required header that's missing (Idempotency-Key, If-Match) must map to 428 precondition.required,
// not the generic 400 request.invalid that every other validation failure gets.
package validate

import (
	"errors"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"pdpa-platform/internal/pkg/httpx"
)

// Middleware validates each request's path, parameters, headers and body against spec. authFunc, if
// non-nil, runs as part of validation (kin-openapi calls it once per declared security requirement);
// leave nil to validate schema only and let the AuthN middleware (#7) handle authentication.
func Middleware(spec *openapi3.T, authFunc openapi3filter.AuthenticationFunc) (func(http.Handler) http.Handler, error) {
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				// Unknown route: let the router 404 rather than failing validation.
				next.ServeHTTP(w, r)
				return
			}

			input := &openapi3filter.RequestValidationInput{
				Request:     r,
				PathParams:  pathParams,
				Route:       route,
				QueryParams: r.URL.Query(),
				Options: &openapi3filter.Options{
					AuthenticationFunc: authFunc,
				},
			}

			if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				writeValidationError(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	if reqErr, ok := err.(*openapi3filter.RequestError); ok && reqErr.Parameter != nil {
		p := reqErr.Parameter
		if p.In == openapi3.ParameterInHeader && p.Required && errors.Is(reqErr.Err, openapi3filter.ErrInvalidRequired) {
			httpx.WriteProblem(w, r, httpx.PreconditionRequired("missing required header: "+p.Name))
			return
		}
	}

	httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
}
