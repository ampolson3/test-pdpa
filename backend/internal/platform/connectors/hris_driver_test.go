package connectors_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pdpa-platform/internal/platform/connectors"
)

// DPX-13: the HRIS driver reads a roster and marks leavers, with the default and custom field names.
func TestHRISDriver_ListEmployees(t *testing.T) {
	var sawAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/employees", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "email": " Alice@Example.com ", "status": "active"},
			{"id": 2, "email": "bob@example.com", "status": "Terminated"},
			{"id": 3, "status": "terminated"}
		]`))
	})
	mux.HandleFunc("/wrapped", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": [{"empNo": "E9", "mail": "c@example.com", "state": "LEFT"}]}`))
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := connectors.HRISDriver()
	ctx := context.Background()
	cfg := connectors.Config{Settings: map[string]string{"base_url": srv.URL, "employees_path": "/employees"}, Secret: "s3cret"}

	if err := d.Test(ctx, cfg); err != nil {
		t.Fatalf("test: %v", err)
	}
	got, err := d.ListEmployees(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer s3cret" {
		t.Errorf("authorization header = %q", sawAuth)
	}
	if len(got) != 2 {
		t.Fatalf("employees = %d, want 2 (the one without an e-mail is skipped)", len(got))
	}
	if got[0].Email != "alice@example.com" || got[0].Terminated || got[0].ExternalID != "1" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Email != "bob@example.com" || !got[1].Terminated {
		t.Errorf("second = %+v", got[1])
	}

	custom := connectors.Config{Settings: map[string]string{
		"base_url": srv.URL, "employees_path": "wrapped", "items_field": "data",
		"email_field": "mail", "id_field": "empNo", "status_field": "state", "terminated_values": "left, gone",
	}}
	got, err = d.ListEmployees(ctx, custom)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Terminated || got[0].ExternalID != "E9" || got[0].Email != "c@example.com" {
		t.Errorf("custom fields = %+v", got)
	}

	if _, err := d.ListEmployees(ctx, connectors.Config{Settings: map[string]string{"base_url": srv.URL, "employees_path": "/broken"}}); err == nil {
		t.Error("a 500 from the HRIS must be an error")
	}
	if err := d.Test(ctx, connectors.Config{Settings: map[string]string{"base_url": srv.URL}}); err == nil {
		t.Error("missing employees_path must be an error")
	}
}
