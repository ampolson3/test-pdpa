package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RESTDriver is the fourth member of PLT-20's own "ชุดแรก": a generic connector for any customer system
// reachable over a plain JSON REST API, driven entirely by this connector's own config — base_url,
// test_path (health check), search_path (a {id} placeholder substituted with the search identifier),
// update_path ({id}, PATCH body = the given fields as JSON), delete_path ({id}). Config.Secret, when set,
// is sent as a Bearer token — the one authentication scheme common enough across customer APIs to build in
// generically; anything else is this connector_type's own future variant, not this driver's job.
func RESTDriver() Driver {
	client := func() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

	doRequest := func(ctx context.Context, cfg Config, method, url string, body []byte) (*http.Response, error) {
		var r io.Reader
		if body != nil {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, r)
		if err != nil {
			return nil, err
		}
		if cfg.Secret != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Secret)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return client().Do(req)
	}

	withID := func(pathTemplate, id string) string {
		return strings.ReplaceAll(pathTemplate, "{id}", id)
	}

	joinURL := func(baseURL, path string) string {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}

	return Driver{
		Capabilities: []string{CapTest, CapSearchSubject, CapUpdate, CapDelete},
		Test: func(ctx context.Context, cfg Config) error {
			base, testPath := cfg.get("base_url"), cfg.get("test_path")
			if base == "" {
				return fmt.Errorf("connectors: missing config.base_url")
			}
			if testPath == "" {
				testPath = "/"
			}
			resp, err := doRequest(ctx, cfg, http.MethodGet, joinURL(base, testPath), nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				return fmt.Errorf("connectors: rest test returned status %d", resp.StatusCode)
			}
			return nil
		},
		SearchSubject: func(ctx context.Context, cfg Config, identifier string) ([]SubjectRecord, error) {
			base, searchPath := cfg.get("base_url"), cfg.get("search_path")
			if base == "" || searchPath == "" {
				return nil, fmt.Errorf("connectors: missing config.base_url/search_path")
			}
			resp, err := doRequest(ctx, cfg, http.MethodGet, joinURL(base, withID(searchPath, identifier)), nil)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				return nil, nil
			}
			if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("connectors: rest search returned status %d", resp.StatusCode)
			}
			var decoded any
			if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
				return nil, err
			}
			return decodeSubjectRecords(decoded), nil
		},
		Update: func(ctx context.Context, cfg Config, identifier string, fields map[string]string) error {
			base, updatePath := cfg.get("base_url"), cfg.get("update_path")
			if base == "" || updatePath == "" {
				return fmt.Errorf("connectors: missing config.base_url/update_path")
			}
			body, err := json.Marshal(fields)
			if err != nil {
				return err
			}
			resp, err := doRequest(ctx, cfg, http.MethodPatch, joinURL(base, withID(updatePath, identifier)), body)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				return fmt.Errorf("connectors: rest update returned status %d", resp.StatusCode)
			}
			return nil
		},
		Delete: func(ctx context.Context, cfg Config, identifier string) error {
			base, deletePath := cfg.get("base_url"), cfg.get("delete_path")
			if base == "" || deletePath == "" {
				return fmt.Errorf("connectors: missing config.base_url/delete_path")
			}
			resp, err := doRequest(ctx, cfg, http.MethodDelete, joinURL(base, withID(deletePath, identifier)), nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
				return fmt.Errorf("connectors: rest delete returned status %d", resp.StatusCode)
			}
			return nil
		},
	}
}

// decodeSubjectRecords accepts either a single JSON object or an array of objects — most REST APIs
// return one shape or the other for a "search" endpoint, and this driver shouldn't force a customer's API
// into a specific envelope convention to be usable.
func decodeSubjectRecords(decoded any) []SubjectRecord {
	switch v := decoded.(type) {
	case map[string]any:
		return []SubjectRecord{{Table: "rest", Fields: stringifyFields(v)}}
	case []any:
		out := make([]SubjectRecord, 0, len(v))
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				out = append(out, SubjectRecord{Table: "rest", Fields: stringifyFields(obj)})
			}
		}
		return out
	default:
		return nil
	}
}

func stringifyFields(obj map[string]any) map[string]string {
	fields := make(map[string]string, len(obj))
	for k, v := range obj {
		fields[k] = fmt.Sprint(v)
	}
	return fields
}
