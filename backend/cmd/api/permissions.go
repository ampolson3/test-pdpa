package main

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// loadPermissions reads x-permission off every operation in spec and returns it keyed by path and
// HTTP method, for the AuthZ middleware (api/openapi/README.md: "ทุก operation ต้องมี x-permission").
func loadPermissions(spec *openapi3.T) (map[string]map[string]string, error) {
	out := make(map[string]map[string]string)

	for path, item := range spec.Paths.Map() {
		methods := map[string]*openapi3.Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put,
			"PATCH": item.Patch, "DELETE": item.Delete,
		}
		for method, op := range methods {
			if op == nil {
				continue
			}
			code, ok := stringExtension(op.Extensions, "x-permission")
			if !ok {
				return nil, fmt.Errorf("permissions: %s %s is missing x-permission", method, path)
			}
			if out[path] == nil {
				out[path] = make(map[string]string)
			}
			out[path][method] = code
		}
	}
	return out, nil
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
