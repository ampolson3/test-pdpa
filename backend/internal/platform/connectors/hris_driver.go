package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// hrisMaxBody caps how much of an HRIS response is read (a roster of a few hundred thousand people).
const hrisMaxBody = 64 << 20

// HRISDriver is DPX-13's connector type for an HR system with a JSON REST API. Config:
//
//	base_url           required
//	employees_path     required; GET returns a JSON array, or an object holding one under items_field
//	items_field        optional name of the array inside an object response (e.g. "data")
//	email_field        default "email"
//	id_field           default "id"
//	status_field       default "status"
//	terminated_values  comma-separated status values meaning "has left"; default
//	                   "terminated,inactive,resigned"
//
// The secret, when set, is sent as a Bearer token.
func HRISDriver() Driver {
	fetch := func(ctx context.Context, cfg Config) ([]map[string]any, error) {
		base, path := cfg.get("base_url"), cfg.get("employees_path")
		if base == "" || path == "" {
			return nil, fmt.Errorf("connectors: missing config.base_url/employees_path")
		}
		url := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if cfg.Secret != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Secret)
		}
		resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("connectors: hris returned status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, hrisMaxBody))
		if err != nil {
			return nil, err
		}
		var items []map[string]any
		if field := cfg.get("items_field"); field != "" {
			var wrapper map[string]json.RawMessage
			if err := json.Unmarshal(body, &wrapper); err != nil {
				return nil, fmt.Errorf("connectors: hris response is not a JSON object")
			}
			if err := json.Unmarshal(wrapper[field], &items); err != nil {
				return nil, fmt.Errorf("connectors: hris response has no array at config.items_field")
			}
			return items, nil
		}
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("connectors: hris response is not a JSON array")
		}
		return items, nil
	}

	orDefault := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}

	return Driver{
		Capabilities: []string{CapTest, CapListEmployees},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := fetch(ctx, cfg)
			return err
		},
		ListEmployees: func(ctx context.Context, cfg Config) ([]Employee, error) {
			items, err := fetch(ctx, cfg)
			if err != nil {
				return nil, err
			}
			emailField := orDefault(cfg.get("email_field"), "email")
			idField := orDefault(cfg.get("id_field"), "id")
			statusField := orDefault(cfg.get("status_field"), "status")
			terminated := map[string]bool{}
			for _, v := range strings.Split(orDefault(cfg.get("terminated_values"), "terminated,inactive,resigned"), ",") {
				if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
					terminated[v] = true
				}
			}
			out := make([]Employee, 0, len(items))
			for _, it := range items {
				email := strings.ToLower(strings.TrimSpace(fmt.Sprint(orNil(it[emailField]))))
				if email == "" {
					continue
				}
				out = append(out, Employee{
					ExternalID: fmt.Sprint(orNil(it[idField])),
					Email:      email,
					Terminated: terminated[strings.ToLower(strings.TrimSpace(fmt.Sprint(orNil(it[statusField]))))],
				})
			}
			return out, nil
		},
	}
}

// orNil turns a missing JSON value into "" instead of fmt's "<nil>".
func orNil(v any) any {
	if v == nil {
		return ""
	}
	return v
}
