package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/platform/connectors"
	"pdpa-platform/internal/platform/crypto"
)

func connectorsEnv() *connectors.Service {
	return connectors.New(connectors.Registry{"rest": connectors.RESTDriver()}, &crypto.Keyring{KEK: crypto.NewLocalKEK()}, nil)
}

// TestSearchAcrossSystems_OneCommandAcrossEveryConnector is the acceptance criterion directly: one call
// reaches every active, search-capable connector — a connector that finds a match is recorded as such, one
// whose own search endpoint errors is recorded as not-found rather than aborting the run, and a connector
// with no search_subject capability (or one still draft) is never queried at all.
func TestSearchAcrossSystems_OneCommandAcrossEveryConnector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/erroring-subjects/somchai@example.com":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"email": "somchai@example.com", "name": "สมชาย"})
		}
	}))
	defer srv.Close()

	e := setup(t, "dsarsearch")
	connSvc := connectorsEnv()
	e.svc.Connectors = connSvc

	var reqID uuid.UUID
	var foundConnID, erroringConnID, uncapableConnID, draftConnID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		r, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		reqID = r.ID

		found, err := connSvc.CreateConnector(ctx, connectors.Input{
			Name: "found-crm", ConnectorType: "rest", Capabilities: []string{"test", "search_subject"},
			Config: json.RawMessage(`{"base_url": "` + srv.URL + `", "test_path": "/health", "search_path": "/subjects/{id}"}`),
		})
		if err != nil {
			return err
		}
		foundConnID = found.ID
		if ok, _, _, _, err := connSvc.TestConnection(ctx, found.ID); err != nil || !ok {
			t.Fatalf("TestConnection(found) = %v, %v", ok, err)
		}

		// A real connector-level failure: the connector itself is reachable (Test hits /health and
		// succeeds, so it's genuinely active), but its own search endpoint returns an error — proving a
		// single connector's own failure never aborts the rest of the run.
		erroring, err := connSvc.CreateConnector(ctx, connectors.Input{
			Name: "erroring-pos", ConnectorType: "rest", Capabilities: []string{"test", "search_subject"},
			Config: json.RawMessage(`{"base_url": "` + srv.URL + `", "test_path": "/health", "search_path": "/erroring-subjects/{id}"}`),
		})
		if err != nil {
			return err
		}
		erroringConnID = erroring.ID
		if ok, _, _, _, err := connSvc.TestConnection(ctx, erroring.ID); err != nil || !ok {
			t.Fatalf("TestConnection(erroring) = %v, %v", ok, err)
		}

		noCap, err := connSvc.CreateConnector(ctx, connectors.Input{Name: "no-search-cap", ConnectorType: "rest", Capabilities: []string{"test"},
			Config: json.RawMessage(`{"base_url": "` + srv.URL + `", "test_path": "/health"}`)})
		if err != nil {
			return err
		}
		uncapableConnID = noCap.ID
		if ok, _, _, _, err := connSvc.TestConnection(ctx, noCap.ID); err != nil || !ok {
			t.Fatalf("TestConnection(noCap) = %v, %v", ok, err)
		}

		draft, err := connSvc.CreateConnector(ctx, connectors.Input{Name: "still-draft", ConnectorType: "rest", Capabilities: []string{"search_subject"}})
		if err != nil {
			return err
		}
		draftConnID = draft.ID
		return nil
	})

	var results []dsarservice.SearchResult
	e.in(t, func(ctx context.Context) error {
		var err error
		results, err = e.svc.SearchAcrossSystems(ctx, reqID)
		return err
	})

	byConn := map[uuid.UUID]dsarservice.SearchResult{}
	for _, r := range results {
		byConn[r.ConnectorID] = r
	}
	if r, ok := byConn[foundConnID]; !ok || !r.Found || r.RecordCount == nil || *r.RecordCount != 1 {
		t.Fatalf("found connector result = %+v, ok=%v, want found with 1 record", byConn[foundConnID], ok)
	}
	if r, ok := byConn[erroringConnID]; !ok || r.Found {
		t.Fatalf("erroring connector result = %+v, ok=%v, want recorded as not-found, not missing", byConn[erroringConnID], ok)
	}
	if _, ok := byConn[uncapableConnID]; ok {
		t.Fatal("a connector without search_subject should never be queried")
	}
	if _, ok := byConn[draftConnID]; ok {
		t.Fatal("a draft (non-active) connector should never be queried")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want exactly 2 (found + erroring, both active and search-capable)", len(results))
	}

	// Re-reading via ListSearchResults returns the same, persisted rows.
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListSearchResults(ctx, reqID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("ListSearchResults = %+v, want 2 rows", list)
		}
		for _, r := range list {
			if r.PurgedAt != nil {
				t.Fatal("result should not be purged yet")
			}
		}
		return nil
	})
}

// TestSearchAcrossSystems_PurgedOnClose is the acceptance criterion's other half: entering a terminal
// status wipes every search_results row's own finding for that request, keeping the row itself.
func TestSearchAcrossSystems_PurgedOnClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"email": "somchai@example.com"})
	}))
	defer srv.Close()

	e := setup(t, "dsarpurge")
	connSvc := connectorsEnv()
	e.svc.Connectors = connSvc

	var reqID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		r, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		reqID = r.ID
		c, err := connSvc.CreateConnector(ctx, connectors.Input{
			Name: "crm", ConnectorType: "rest", Capabilities: []string{"test", "search_subject"},
			Config: json.RawMessage(`{"base_url": "` + srv.URL + `", "test_path": "/health", "search_path": "/subjects/{id}"}`),
		})
		if err != nil {
			return err
		}
		if ok, _, _, _, err := connSvc.TestConnection(ctx, c.ID); err != nil || !ok {
			return err
		}
		_, err = e.svc.SearchAcrossSystems(ctx, reqID)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListSearchResults(ctx, reqID)
		if err != nil {
			return err
		}
		if len(list) != 1 || !list[0].Found {
			t.Fatalf("precondition failed, got %+v", list)
		}
		return nil
	})

	// received -> withdrawn is the shortest path to a terminal status (st02Edges).
	e.in(t, func(ctx context.Context) error {
		req, err := e.svc.GetRequest(ctx, reqID)
		if err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, reqID, req.RowVersion, dsarservice.TransitionInput{To: "withdrawn"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListSearchResults(ctx, reqID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("len(list) = %d, want 1 (the row stays)", len(list))
		}
		if list[0].Found {
			t.Fatal("found should be purged away — row kept but no longer reporting a match")
		}
		if list[0].RecordCount != nil {
			t.Fatal("record_count should be cleared on purge")
		}
		if list[0].PurgedAt == nil {
			t.Fatal("purged_at should be set")
		}
		return nil
	})
}

// TestSearchAcrossSystems_NotConfiguredIsRefused is the nil-means-not-wired guard: a context (e.g.
// cmd/worker's own, which never needs this) with no Connectors wired is refused cleanly, not a panic.
func TestSearchAcrossSystems_NotConfiguredIsRefused(t *testing.T) {
	e := setup(t, "dsarsearchnil")
	var reqID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID := seedLegalEntityAndType(t, ctx, e)
		r, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.SearchAcrossSystems(ctx, reqID)
		if err == nil {
			t.Fatal("expected an error with no Connectors configured")
		}
		return nil
	})
}
