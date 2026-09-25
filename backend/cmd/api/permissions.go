package main

import (
	"encoding/json"
	"fmt"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// loadPermissions reads x-permission off every operation in spec and returns it keyed by the
// Go-exported form of its operationId — e.g. "iamGetMe" -> "IamGetMe" — because that's the literal
// string oapi-codegen's generated strict-server wrapper passes to StrictMiddlewareFunc
// (backend/internal/iam/http/me.gen.go: `middleware(handler, "IamGetMe")`), not the chi route
// pattern. chi.RouteContext(ctx).RoutePattern() is empty for middleware mounted with r.Use()
// because it's only populated once the mux has matched and is dispatching to the route's own
// handler chain — by design this can't be read any earlier, so AuthZ has to hook in here instead
// (see internal/pkg/authz.StrictMiddleware) rather than as a chi-level middleware.
func loadPermissions(spec *openapi3.T) (map[string]string, error) {
	out := make(map[string]string)

	for path, item := range spec.Paths.Map() {
		methods := map[string]*openapi3.Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put,
			"PATCH": item.Patch, "DELETE": item.Delete,
		}
		for method, op := range methods {
			if op == nil {
				continue
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("permissions: %s %s is missing operationId", method, path)
			}
			code, ok := stringExtension(op.Extensions, "x-permission")
			if !ok {
				return nil, fmt.Errorf("permissions: %s %s is missing x-permission", method, path)
			}
			out[exportedName(op.OperationID)] = code
		}
	}
	return out, nil
}

func exportedName(operationID string) string {
	r := []rune(operationID)
	if len(r) == 0 {
		return operationID
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func stringExtension(ext map[string]any, key string) (string, bool) {
	v, ok := ext[key]
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.RawMessage:
		var s string
		if json.Unmarshal(t, &s) == nil {
			return s, true
		}
	}
	return "", false
}
