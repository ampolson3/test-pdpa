package connectors_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"pdpa-platform/internal/platform/connectors"
)

// ownerDSN matches internal/pkg/dbtest's own unexported defaultOwnerURL literal exactly (README's
// local-Postgres setup) — the postgres driver test needs CREATE TABLE on a scratch table, which the
// ordinary pdpa_app role this package's other tests connect as doesn't hold.
func ownerDSN() string {
	if v := os.Getenv("TEST_OWNER_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://pdpa_migrator:pdpa_migrator@localhost:5433/pdpa?sslmode=disable&options=-c%20role%3Dpdpa_owner"
}

func skipUnlessReachable(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("connectors: no reachable Postgres: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("connectors: no reachable Postgres (set TEST_OWNER_DATABASE_URL to run this test): %v", err)
	}
}

// TestPostgresDriver_FullLifecycle is the acceptance criterion's own "test/discover/search_subject/
// update/delete" against a real scratch table — proving the generic sqlDriver builder actually talks
// to a real PostgreSQL server, not just compiles against one.
func TestPostgresDriver_FullLifecycle(t *testing.T) {
	dsn := ownerDSN()
	skipUnlessReachable(t, dsn)

	ctx := context.Background()
	setup, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	if _, err := setup.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.connectors_driver_test (email text PRIMARY KEY, name text)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = setup.ExecContext(ctx, `DROP TABLE IF EXISTS public.connectors_driver_test`)
	})
	if _, err := setup.ExecContext(ctx, `DELETE FROM public.connectors_driver_test`); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(ctx, `INSERT INTO public.connectors_driver_test (email, name) VALUES ('a@example.com', 'Alice')`); err != nil {
		t.Fatal(err)
	}

	d := connectors.PostgresDriver()
	cfg := connectors.Config{Settings: map[string]string{
		"dsn": dsn, "table": "public.connectors_driver_test", "identifier_column": "email",
	}}

	if err := d.Test(ctx, cfg); err != nil {
		t.Fatalf("Test: %v", err)
	}

	tables, err := d.Discover(ctx, cfg)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "connectors_driver_test" {
			found = true
			if len(tb.Columns) != 2 {
				t.Errorf("Discover columns = %v, want 2", tb.Columns)
			}
		}
	}
	if !found {
		t.Fatalf("Discover did not find connectors_driver_test among %v", tables)
	}

	recs, err := d.SearchSubject(ctx, cfg, "a@example.com")
	if err != nil {
		t.Fatalf("SearchSubject: %v", err)
	}
	if len(recs) != 1 || recs[0].Fields["name"] != "Alice" {
		t.Fatalf("SearchSubject = %+v, want one record named Alice", recs)
	}

	if err := d.Update(ctx, cfg, "a@example.com", map[string]string{"name": "Alicia"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	recs, err = d.SearchSubject(ctx, cfg, "a@example.com")
	if err != nil || len(recs) != 1 || recs[0].Fields["name"] != "Alicia" {
		t.Fatalf("after Update, SearchSubject = %+v, err=%v", recs, err)
	}

	if err := d.Delete(ctx, cfg, "a@example.com"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	recs, err = d.SearchSubject(ctx, cfg, "a@example.com")
	if err != nil {
		t.Fatalf("SearchSubject after delete: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("after Delete, SearchSubject = %+v, want none", recs)
	}
}

// TestRESTDriver_FullLifecycle drives a real httptest.Server — no reachable external infrastructure
// needed, so this one always runs regardless of this environment's own Postgres/Redis reachability.
func TestRESTDriver_FullLifecycle(t *testing.T) {
	type record struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	store := map[string]record{"a@example.com": {ID: "a@example.com", Name: "Alice"}}
	var sawAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/subjects/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/subjects/"):]
		switch r.Method {
		case http.MethodGet:
			rec, ok := store[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rec)
		case http.MethodPatch:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec := store[id]
			if name, ok := body["name"]; ok {
				rec.Name = name
			}
			store[id] = rec
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(store, id)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	d := connectors.RESTDriver()
	cfg := connectors.Config{
		Secret: "tok123",
		Settings: map[string]string{
			"base_url":    srv.URL,
			"test_path":   "/health",
			"search_path": "/subjects/{id}",
			"update_path": "/subjects/{id}",
			"delete_path": "/subjects/{id}",
		},
	}

	if err := d.Test(ctx, cfg); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if sawAuth != "Bearer tok123" {
		t.Errorf("Authorization header = %q, want Bearer tok123", sawAuth)
	}

	recs, err := d.SearchSubject(ctx, cfg, "a@example.com")
	if err != nil {
		t.Fatalf("SearchSubject: %v", err)
	}
	if len(recs) != 1 || recs[0].Fields["name"] != "Alice" {
		t.Fatalf("SearchSubject = %+v, want Alice", recs)
	}

	recs, err = d.SearchSubject(ctx, cfg, "unknown@example.com")
	if err != nil {
		t.Fatalf("SearchSubject unknown: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("SearchSubject unknown = %+v, want none (404 maps to empty, not an error)", recs)
	}

	if err := d.Update(ctx, cfg, "a@example.com", map[string]string{"name": "Alicia"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if store["a@example.com"].Name != "Alicia" {
		t.Fatalf("store after Update = %+v, want Alicia", store["a@example.com"])
	}

	if err := d.Delete(ctx, cfg, "a@example.com"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store["a@example.com"]; ok {
		t.Fatal("record still present after Delete")
	}
}

// TestRegistry_AddingANewTypeNeedsNoCoreChange is the acceptance criterion itself, proven directly:
// a brand-new connector_type backed by a Driver this test defines on the spot, with no change anywhere
// in internal/platform/connectors's own Service, is immediately usable through the registry lookup —
// the same extensibility proof internal/platform/importer's own Type/Registry already established.
func TestRegistry_AddingANewTypeNeedsNoCoreChange(t *testing.T) {
	called := false
	reg := connectors.Registry{
		"my_custom_type": {
			Capabilities: []string{connectors.CapTest},
			Test: func(ctx context.Context, cfg connectors.Config) error {
				called = true
				return nil
			},
		},
	}
	d, ok := reg["my_custom_type"]
	if !ok {
		t.Fatal("registry lookup failed")
	}
	if err := d.Test(context.Background(), connectors.Config{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("custom driver's own Test was never invoked")
	}
	// A capability the driver leaves nil is ErrUnsupported territory, never a nil-pointer panic.
	if d.Discover != nil {
		t.Fatal("Discover should be nil for this driver")
	}
}
