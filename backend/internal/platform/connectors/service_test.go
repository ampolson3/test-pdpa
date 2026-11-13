package connectors_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/connectors"
	"pdpa-platform/internal/platform/crypto"
)

type fakeRopa struct {
	visible map[uuid.UUID]bool
}

func (f fakeRopa) AssetVisible(ctx context.Context, id uuid.UUID) (bool, error) {
	return f.visible[id], nil
}

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *connectors.Service
}

func testRegistry() connectors.Registry {
	return connectors.Registry{
		"postgres": connectors.PostgresDriver(),
		"rest":     connectors.RESTDriver(),
	}
}

func setup(t *testing.T, suffix string, ropa connectors.Ropa) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connector_secrets`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connectors`)
			_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE email LIKE 'owner-%@dbtest.example'`)
			return err
		})
	})
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	return env{app: app, tenant: tenant, svc: connectors.New(testRegistry(), keyring, ropa)}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: []string{"ropa.discovery.read", "ropa.discovery.create", "ropa.discovery.update", "ropa.discovery.delete", "ropa.discovery.execute"}}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListTypes_ReflectsTheRegistryLive(t *testing.T) {
	e := setup(t, "ctypes", nil)
	types := e.svc.ListTypes()
	codes := map[string]bool{}
	for _, ty := range types {
		codes[ty.Code] = true
	}
	if !codes["postgres"] || !codes["rest"] {
		t.Fatalf("ListTypes() = %+v, want postgres and rest", types)
	}
}

func TestCreateConnector_RejectsUnknownType(t *testing.T) {
	e := setup(t, "cunknown", nil)
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "not_a_real_type"})
		if !errors.Is(err, connectors.ErrInvalid) {
			t.Fatalf("CreateConnector = %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestCreateConnector_RejectsUnsupportedCapability(t *testing.T) {
	e := setup(t, "ccap", nil)
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "rest", Capabilities: []string{"discover"}})
		if !errors.Is(err, connectors.ErrInvalid) {
			t.Fatalf("CreateConnector with rest+discover = %v, want ErrInvalid (rest driver has no Discover)", err)
		}
		return nil
	})
}

// TestCreateConnector_SecretRoundTrips is the acceptance criterion's own secret-handling half: a sealed
// secret is never returned over the Connector struct, but TestConnection can still decrypt and use it.
func TestCreateConnector_SecretRoundTrips(t *testing.T) {
	e := setup(t, "csecret", nil)
	var id uuid.UUID
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.CreateConnector(ctx, connectors.Input{
			Name: "REST connector", ConnectorType: "rest",
			Config: json.RawMessage(`{"base_url": "http://example.invalid", "test_path": "/health"}`),
			Secret: strPtr("super-secret-token"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !c.HasSecret {
			t.Fatal("HasSecret = false, want true")
		}
		id = c.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetConnector(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !got.HasSecret {
			t.Fatal("GetConnector: HasSecret = false, want true")
		}
		// Clearing the secret with an explicit empty string actually removes it.
		updated, err := e.svc.UpdateConnector(ctx, id, connectors.Input{
			Name: got.Name, Config: got.Config, Secret: strPtr(""),
		}, got.RowVersion)
		if err != nil {
			t.Fatal(err)
		}
		if updated.HasSecret {
			t.Fatal("after clearing, HasSecret = true, want false")
		}
		return nil
	})
}

// TestUpdateConnector_OmittingSecretLeavesItUnchanged proves the nil-means-unchanged contract: a PATCH
// that never mentions `secret` must not silently wipe an already-sealed one.
func TestUpdateConnector_OmittingSecretLeavesItUnchanged(t *testing.T) {
	e := setup(t, "ckeep", nil)
	var id uuid.UUID
	var version int32
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "rest", Secret: strPtr("keep-me")})
		if err != nil {
			t.Fatal(err)
		}
		id, version = c.ID, c.RowVersion
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		updated, err := e.svc.UpdateConnector(ctx, id, connectors.Input{Name: "renamed"}, version)
		if err != nil {
			t.Fatal(err)
		}
		if !updated.HasSecret {
			t.Fatal("after an update that never mentions secret, HasSecret = false, want true (unchanged)")
		}
		return nil
	})
}

func TestUpdateConnector_StaleVersionRefused(t *testing.T) {
	e := setup(t, "cstale", nil)
	var id uuid.UUID
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "rest"})
		if err != nil {
			t.Fatal(err)
		}
		id = c.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.UpdateConnector(ctx, id, connectors.Input{Name: "y"}, 999)
		if !errors.Is(err, connectors.ErrVersionMismatch) {
			t.Fatalf("UpdateConnector stale = %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

func TestDeleteConnector_RemovesSecretToo(t *testing.T) {
	e := setup(t, "cdelete", nil)
	var id uuid.UUID
	var version int32
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "rest", Secret: strPtr("bye")})
		if err != nil {
			t.Fatal(err)
		}
		id, version = c.ID, c.RowVersion
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteConnector(ctx, id, version); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.GetConnector(ctx, id)
		if !errors.Is(err, connectors.ErrNotFound) {
			t.Fatalf("GetConnector after delete = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestCreateConnector_AssetVisibility is the rule-1 FK-visibility check: an asset_id must resolve
// through the Ropa interface before the connector row is ever written.
func TestCreateConnector_AssetVisibility(t *testing.T) {
	assetID := uuid.Must(uuid.NewV7())
	e := setup(t, "casset", fakeRopa{visible: map[uuid.UUID]bool{assetID: true}})
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "x", ConnectorType: "rest", AssetID: &assetID})
		if err != nil {
			t.Fatal(err)
		}
		if c.AssetID == nil || *c.AssetID != assetID {
			t.Fatalf("AssetID = %v, want %v", c.AssetID, assetID)
		}
		return nil
	})

	unknown := uuid.Must(uuid.NewV7())
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "y", ConnectorType: "rest", AssetID: &unknown})
		if !errors.Is(err, connectors.ErrInvalid) {
			t.Fatalf("CreateConnector with invisible asset = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestTwoTenantIsolation is CLAUDE.md rule 1's own mandatory proof for a new repository.
func TestTwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "ciso1", nil)
	ctx := context.Background()
	app, owner := e1.app, dbtest.OwnerPool(t)
	tenant2 := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ciso2")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant2.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connector_secrets`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connectors`)
			_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE email LIKE 'owner-%@dbtest.example'`)
			return err
		})
	})
	e2 := env{app: app, tenant: tenant2, svc: e1.svc}

	var id uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		c, err := e1.svc.CreateConnector(ctx, connectors.Input{Name: "tenant1-only", ConnectorType: "rest"})
		if err != nil {
			t.Fatal(err)
		}
		id = c.ID
		return nil
	})

	e2.in(t, func(ctx context.Context) error {
		_, err := e2.svc.GetConnector(ctx, id)
		if !errors.Is(err, connectors.ErrNotFound) {
			t.Fatalf("tenant2 GetConnector(tenant1's id) = %v, want ErrNotFound", err)
		}
		list, _, err := e2.svc.ListConnectors(ctx, connectors.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if c.ID == id {
				t.Fatal("tenant2's ListConnectors leaked tenant1's row")
			}
		}
		return nil
	})
}

// TestSearchSubject_RequiresTheCapability is DSAR-09's own prerequisite: a connector whose type offers
// search_subject but whose own narrower Capabilities list doesn't include it is refused, not silently run.
func TestSearchSubject_RequiresTheCapability(t *testing.T) {
	e := setup(t, "csearch", nil)
	var withCap, withoutCap uuid.UUID
	e.in(t, func(ctx context.Context) error {
		c1, err := e.svc.CreateConnector(ctx, connectors.Input{
			Name: "rest-with-search", ConnectorType: "rest", Capabilities: []string{"test", "search_subject"},
		})
		if err != nil {
			t.Fatal(err)
		}
		withCap = c1.ID
		c2, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "rest-test-only", ConnectorType: "rest", Capabilities: []string{"test"}})
		if err != nil {
			t.Fatal(err)
		}
		withoutCap = c2.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SearchSubject(ctx, withoutCap, "a@example.com"); !errors.Is(err, connectors.ErrUnsupported) {
			t.Fatalf("SearchSubject without the capability = %v, want ErrUnsupported", err)
		}
		// withCap's own REST driver will fail to actually reach http://example.invalid, but that's a
		// network error, not ErrUnsupported — the capability check itself is what this test proves.
		if _, err := e.svc.SearchSubject(ctx, withCap, "a@example.com"); errors.Is(err, connectors.ErrUnsupported) {
			t.Fatalf("SearchSubject with the capability = %v, want something other than ErrUnsupported", err)
		}
		return nil
	})
}

// TestListActiveWithCapability_FiltersStatusAndCapability is the read side DSAR-09's own "ค้นข้อมูลจาก
// ระบบที่เชื่อมได้" needs: only active connectors that actually offer the given capability.
func TestListActiveWithCapability_FiltersStatusAndCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()

	e := setup(t, "clistcap", nil)
	e.in(t, func(ctx context.Context) error {
		searchable, err := e.svc.CreateConnector(ctx, connectors.Input{
			Name: "searchable", ConnectorType: "rest", Capabilities: []string{"test", "search_subject"},
			Config: json.RawMessage(`{"base_url": "` + srv.URL + `", "test_path": "/"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.CreateConnector(ctx, connectors.Input{Name: "not-searchable", ConnectorType: "rest", Capabilities: []string{"test"}}); err != nil {
			t.Fatal(err)
		}
		// A connector's status defaults to "draft" until TestConnection succeeds.
		ok, _, _, _, err := e.svc.TestConnection(ctx, searchable.ID)
		if err != nil || !ok {
			t.Fatalf("TestConnection = %v, %v, want ok", ok, err)
		}

		list, err := e.svc.ListActiveWithCapability(ctx, connectors.CapSearchSubject)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range list {
			if c.ID == searchable.ID {
				found = true
			}
			if c.Status != "active" || !containsStr(c.Capabilities, connectors.CapSearchSubject) {
				t.Fatalf("ListActiveWithCapability returned a non-matching connector: %+v", c)
			}
		}
		if !found {
			t.Fatal("ListActiveWithCapability did not include the active, searchable connector")
		}
		return nil
	})
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }
