package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/connectors"
	"pdpa-platform/internal/platform/crypto"
)

// thaiID builds a checksum-valid 13-digit Thai national id from a 12-digit prefix.
func thaiID(prefix string) string {
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(prefix[i]-'0') * (13 - i)
	}
	return prefix + fmt.Sprint((11-sum%11)%10)
}

// TestMatchValue_DetectsThaiNationalIDs is DFG-09's acceptance criterion directly: at least 95% of a
// synthetic set of valid ids is detected (pure, no database needed).
func TestMatchValue_DetectsThaiNationalIDs(t *testing.T) {
	const n = 200
	hit := 0
	for i := 0; i < n; i++ {
		id := thaiID(fmt.Sprintf("%012d", 100000000000+i*7919))
		v := id
		if i%4 == 0 { // formatted variants
			v = id[:1] + "-" + id[1:5] + "-" + id[5:10] + "-" + id[10:12] + "-" + id[12:]
		}
		if dataflowservice.MatchValue("checksum", nil, v) {
			hit++
		}
	}
	if float64(hit)/n < 0.95 {
		t.Fatalf("detected %d/%d valid ids, want >= 95%%", hit, n)
	}
	if dataflowservice.MatchValue("checksum", nil, "1234567890123") && !strings.HasSuffix(thaiID("123456789012"), "3") {
		t.Fatal("an id with a wrong check digit must not match")
	}
}

func TestMatchValue_PhoneAndName(t *testing.T) {
	re := regexp.MustCompile(`^(0[689][0-9]{8}|\+66[689][0-9]{8})$`)
	if !dataflowservice.MatchValue("regex", re, "081-234-5678") || dataflowservice.MatchValue("regex", re, "12345") {
		t.Fatal("phone classifier wrong")
	}
	if !dataflowservice.MatchValue("dictionary", nil, "สมชาย ใจดี") || dataflowservice.MatchValue("dictionary", nil, "John Smith") || dataflowservice.MatchValue("dictionary", nil, "สมชาย") {
		t.Fatal("thai name heuristic wrong")
	}
}

func discoveryDSN() string {
	if v := os.Getenv("TEST_OWNER_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://pdpa_migrator:pdpa_migrator@localhost:5433/pdpa?sslmode=disable&options=-c%20role%3Dpdpa_owner"
}

// TestRunScan_FindsThaiPIIAndStoresNoValues scans a real scratch table: the id/phone/name columns are
// proposed as findings, an unrelated column is not, and no sampled value appears in any stored row.
func TestRunScan_FindsThaiPIIAndStoresNoValues(t *testing.T) {
	e := setup(t, "dfgdisc1")
	dsn := discoveryDSN()
	scratch, err := sql.Open("pgx", dsn)
	if err != nil || scratch.Ping() != nil {
		t.Skip("dataflow: no reachable owner Postgres for the scratch table")
	}
	defer scratch.Close()
	if _, err := scratch.Exec(`CREATE TABLE IF NOT EXISTS public.dfg09_scan_test (national_id text, phone text, full_name text, note text)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = scratch.Exec(`DROP TABLE IF EXISTS public.dfg09_scan_test`) })
	_, _ = scratch.Exec(`DELETE FROM public.dfg09_scan_test`)
	secretID := thaiID("110170000001")
	for i := 0; i < 10; i++ {
		id := thaiID(fmt.Sprintf("1101700000%02d", i+1))
		if i == 0 {
			id = secretID
		}
		if _, err := scratch.Exec(`INSERT INTO public.dfg09_scan_test VALUES ($1,$2,$3,$4)`, id, fmt.Sprintf("0812345%03d", i), "สมชาย ใจดี", "hello"); err != nil {
			t.Fatal(err)
		}
	}

	connSvc := connectors.New(connectors.Registry{"postgres": connectors.PostgresDriver()}, &crypto.Keyring{KEK: crypto.NewLocalKEK()}, nil)
	e.svc.Connectors = connSvc
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), dbtest.OwnerPool(t), e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{`DELETE FROM dataflow.discovery_findings`, `DELETE FROM dataflow.discovery_scans`, `DELETE FROM platform.connector_secrets`, `DELETE FROM platform.connectors`} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	e.in(t, func(ctx context.Context) error {
		cfg, _ := json.Marshal(map[string]string{"dsn": dsn})
		c, err := connSvc.CreateConnector(ctx, connectors.Input{Name: "src", ConnectorType: "postgres", Config: cfg,
			Capabilities: []string{connectors.CapTest, connectors.CapDiscover, connectors.CapSample}})
		if err != nil {
			return err
		}
		if _, _, _, _, err := connSvc.TestConnection(ctx, c.ID); err != nil {
			return err
		}
		sc, err := e.svc.RunScan(ctx, c.ID, nil)
		if err != nil {
			return err
		}
		if sc.Status != "done" {
			t.Errorf("status = %s, want done", sc.Status)
		}
		fs, err := e.svc.ListDiscoveryFindings(ctx, sc.ID)
		if err != nil {
			return err
		}
		got := map[string]string{}
		for _, f := range fs {
			if strings.HasPrefix(f.ObjectPath, "dfg09_scan_test.") {
				got[f.ObjectPath] = f.ClassifierCode
			}
		}
		want := map[string]string{"dfg09_scan_test.national_id": "thai_national_id", "dfg09_scan_test.phone": "thai_phone", "dfg09_scan_test.full_name": "thai_name"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("finding for %s = %q, want %q (all: %v)", k, got[k], v, got)
			}
		}
		if _, ok := got["dfg09_scan_test.note"]; ok {
			t.Error("unrelated column must not be flagged")
		}
		// review: accept one, a stale version is refused
		f := fs[0]
		if _, err := e.svc.ReviewDiscoveryFinding(ctx, f.ID, "accepted", f.RowVersion); err != nil {
			return err
		}
		if _, err := e.svc.ReviewDiscoveryFinding(ctx, f.ID, "rejected", f.RowVersion); err != dataflowservice.ErrDiscoveryVersion {
			t.Errorf("stale review err = %v, want ErrDiscoveryVersion", err)
		}
		// no real value is stored anywhere in the findings rows
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM dataflow.discovery_findings f WHERE row_to_json(f)::text LIKE '%' || $1 || '%' OR row_to_json(f)::text LIKE '%สมชาย%'`, secretID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("%d findings rows contain a raw sampled value", n)
		}
		return nil
	})
}
