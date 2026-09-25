// Package httpx holds the RFC 9457 problem+json helpers shared by every module's http layer.
// See api/openapi/README.md "Error" for the status/code table this file implements.
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"pdpa-platform/internal/pkg/i18n"
)

// Problem is an RFC 9457 problem detail. Code is the stable, machine-readable discriminator
// clients and tests switch on; the rest is for humans.
type Problem struct {
	Type      string       `json:"type,omitempty"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func (p Problem) Error() string { return p.Code + ": " + p.Title }

// WriteProblem writes p as application/problem+json with p.Status as the HTTP status. p.Title is
// localized from p.Code against the caller's Accept-Language (PLT-03) when a translation exists;
// otherwise the title Problem was constructed with stands as the fallback.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.RequestID == "" {
		p.RequestID = middleware.GetReqID(r.Context())
	}
	lang := i18n.FromAcceptLanguage(r.Header.Get("Accept-Language"))
	if title, ok := i18n.ProblemTitle(p.Code, lang); ok {
		p.Title = title
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteJSON writes v as application/json with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Constructors for the problems in api/openapi/README.md's error table. Handlers use these instead
// of Problem literals so the status/code pairing can't drift from the contract.

func RequestInvalid(detail string, errs ...FieldError) Problem {
	return Problem{Title: "Malformed request", Status: http.StatusBadRequest, Code: "request.invalid", Detail: detail, Errors: errs}
}

func AuthnRequired() Problem {
	return Problem{Title: "Authentication required", Status: http.StatusUnauthorized, Code: "authn.required"}
}

func AuthzDenied() Problem {
	return Problem{Title: "Not allowed", Status: http.StatusForbidden, Code: "authz.denied"}
}

func NotFound() Problem {
	return Problem{Title: "Not found", Status: http.StatusNotFound, Code: "not_found"}
}

func InvalidTransition(module string) Problem {
	return Problem{Title: "Invalid state transition", Status: http.StatusConflict, Code: module + ".invalid_transition"}
}

func IdempotencyInProgress() Problem {
	return Problem{Title: "Request already in progress", Status: http.StatusConflict, Code: "idempotency.in_progress"}
}

func IdempotencyKeyReused() Problem {
	return Problem{Title: "Idempotency-Key reused with a different body", Status: http.StatusUnprocessableEntity, Code: "idempotency.key_reused"}
}

func VersionMismatch() Problem {
	return Problem{Title: "If-Match does not match the current version", Status: http.StatusPreconditionFailed, Code: "conflict.version_mismatch"}
}

func PreconditionRequired(detail string) Problem {
	return Problem{Title: "Missing required header", Status: http.StatusPreconditionRequired, Code: "precondition.required", Detail: detail}
}

func UnprocessableEntity(code, detail string) Problem {
	return Problem{Title: "Business rule violation", Status: http.StatusUnprocessableEntity, Code: code, Detail: detail}
}

func RateLimited() Problem {
	return Problem{Title: "Too many requests", Status: http.StatusTooManyRequests, Code: "rate_limited"}
}

func Internal() Problem {
	return Problem{Title: "Internal error", Status: http.StatusInternalServerError, Code: "internal"}
}
