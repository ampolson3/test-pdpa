package service_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	auditjobs "pdpa-platform/internal/platform/audit/jobs"
	"pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/jobs"
)

type fixture struct {
	app, owner *pgxpool.Pool
	a, b       dbtest.Tenant
	svc        *service.Service
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t), owner: dbtest.OwnerPool(t), svc: service.New()}
	platform := dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "audit-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "audit-b")
	// audit_log is append-only for the app roles; the owner removes the test rows (before
	// SeedTenant's cleanup deletes the tenants they reference).
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			f.asOwner(t, tenant, `DELETE FROM platform.audit_log WHERE tenant_id = $1`, tenant.ID)
		}
	})
	return f
}

// asOwner runs one statement as pdpa_owner with RLS scoped to tenant — the out-of-band tamperer.
func (f *fixture) asOwner(t *testing.T, tenant dbtest.Tenant, sql string, args ...any) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.owner, tenant.ID.String(), "", func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, sql, args...)
		return err
	}); err != nil {
		t.Fatalf("owner exec %q: %v", sql, err)
	}
}

func (f *fixture) write(t *testing.T, tenant dbtest.Tenant, e service.Entry) {
	t.Helper()
	e.TenantID = tenant.ID
	if err := pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return f.svc.Write(ctx, e)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func (f *fixture) verify(t *testing.T, tenant dbtest.Tenant) service.VerifyResult {
	t.Helper()
	var res service.VerifyResult
	if err := pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
		var err error
		res, err = f.svc.Verify(ctx, tenant.ID)
		return err
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	return res
}

// ids returns the tenant's audit row ids in chain order.
func (f *fixture) ids(t *testing.T, tenant dbtest.Tenant) []int64 {
	t.Helper()
	var out []int64
	_ = pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
		rows, err := pdb.MustTxFromContext(ctx).Query(ctx, `SELECT id FROM platform.audit_log WHERE tenant_id = $1 ORDER BY occurred_at, id`, tenant.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out
}

func sampleEntry(t *testing.T, tenant dbtest.Tenant, i int) service.Entry {
	t.Helper()
	ip := netip.MustParseAddr("203.0.113.7")
	entity := uuid.New()
	before, after, err := service.Changes(
		map[string]any{"status": "draft", "title": "RoPA", "version": i},
		map[string]any{"status": "submitted", "title": "RoPA", "version": i},
	)
	if err != nil {
		t.Fatal(err)
	}
	return service.Entry{
		ActorType: "user", ActorID: &tenant.UserID, Action: fmt.Sprintf("ropa.activity.submit#%d", i),
		EntityType: "ropa_activity", EntityID: &entity, Before: before, After: after,
		IP: &ip, UserAgent: "Mozilla/5.0 (test)",
	}
}

func TestWrite_BuildsAVerifiableChain(t *testing.T) {
	f := setup(t)
	for i := 0; i < 5; i++ {
		f.write(t, f.a, sampleEntry(t, f.a, i))
	}
	f.write(t, f.a, service.Entry{ActorType: "system", Action: "GET /admin/v1/me"}) // all optional fields empty

	if res := f.verify(t, f.a); !res.OK || res.Checked != 6 {
		t.Fatalf("verify = %+v, want OK over 6 rows", res)
	}
}

// Acceptance criterion: "แก้ log ย้อนหลังแล้วการตรวจ hash chain ล้มเหลว" — changing any column of a
// past row, deleting a row or slipping one in makes verification fail at that row.
func TestVerify_DetectsRetroactiveEdits(t *testing.T) {
	tampers := []struct {
		name, sql, reason string
		target            int // index of the row the tamper hits (and verification must stop at)
	}{
		{"action", `UPDATE platform.audit_log SET action = 'ropa.activity.read' WHERE id = $1`, "hash_mismatch", 2},
		{"actor_id", `UPDATE platform.audit_log SET actor_id = gen_random_uuid() WHERE id = $1`, "hash_mismatch", 2},
		{"entity_id", `UPDATE platform.audit_log SET entity_id = NULL WHERE id = $1`, "hash_mismatch", 2},
		{"before", `UPDATE platform.audit_log SET before = '{"status":"approved"}' WHERE id = $1`, "hash_mismatch", 2},
		{"after", `UPDATE platform.audit_log SET after = NULL WHERE id = $1`, "hash_mismatch", 2},
		{"ip", `UPDATE platform.audit_log SET ip = '198.51.100.1' WHERE id = $1`, "hash_mismatch", 2},
		{"user_agent", `UPDATE platform.audit_log SET user_agent = 'curl' WHERE id = $1`, "hash_mismatch", 2},
		{"occurred_at", `UPDATE platform.audit_log SET occurred_at = occurred_at + interval '1 microsecond' WHERE id = $1`, "hash_mismatch", 2},
		{"delete middle row", `DELETE FROM platform.audit_log WHERE id = $1`, "prev_hash_mismatch", 3},
		{"recompute own hash", `UPDATE platform.audit_log SET action = 'x', hash = repeat('0', 64) WHERE id = $1`, "hash_mismatch", 2},
	}
	for _, tc := range tampers {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			for i := 0; i < 5; i++ {
				f.write(t, f.a, sampleEntry(t, f.a, i))
			}
			ids := f.ids(t, f.a)
			if res := f.verify(t, f.a); !res.OK {
				t.Fatalf("chain broken before tampering: %+v", res)
			}

			f.asOwner(t, f.a, tc.sql, ids[2])

			res := f.verify(t, f.a)
			if res.OK || res.Reason != tc.reason || res.BrokenAt != ids[tc.target] {
				t.Errorf("after tampering %s: %+v, want %s at row %d", tc.name, res, tc.reason, ids[tc.target])
			}
		})
	}
}

// The application role can't rewrite history in the first place (rule 4, deploy/db/10-grants.sql).
func TestAppRole_CannotUpdateOrDeleteAuditRows(t *testing.T) {
	f := setup(t)
	f.write(t, f.a, sampleEntry(t, f.a, 0))
	for _, sql := range []string{
		`UPDATE platform.audit_log SET action = 'x' WHERE tenant_id = $1`,
		`DELETE FROM platform.audit_log WHERE tenant_id = $1`,
	} {
		err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, sql, f.a.ID)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s as pdpa_app: got %v, want permission denied", sql, err)
		}
	}
}

// Concurrent requests of one tenant must not fork the chain (both linking to the same previous row).
func TestWrite_ConcurrentAppendsKeepOneChain(t *testing.T) {
	f := setup(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := sampleEntry(t, f.a, i)
			e.TenantID = f.a.ID
			errs <- pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
				return f.svc.Write(ctx, e)
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if res := f.verify(t, f.a); !res.OK || res.Checked != 20 {
		t.Fatalf("verify after concurrent writes = %+v, want OK over 20 rows", res)
	}
}

// Each tenant has its own chain, invisible to the other (RLS), and verifying one ignores the other.
func TestChains_ArePerTenant(t *testing.T) {
	f := setup(t)
	f.write(t, f.a, sampleEntry(t, f.a, 0))
	f.write(t, f.b, sampleEntry(t, f.b, 0))
	f.write(t, f.a, sampleEntry(t, f.a, 1))

	if res := f.verify(t, f.a); !res.OK || res.Checked != 2 {
		t.Errorf("tenant A = %+v, want OK over its 2 rows", res)
	}
	if res := f.verify(t, f.b); !res.OK || res.Checked != 1 {
		t.Errorf("tenant B = %+v, want OK over its 1 row", res)
	}
	var visible int
	_ = pdb.WithTenantTx(context.Background(), f.app, f.b.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE tenant_id = $1`, f.a.ID).Scan(&visible)
	})
	if visible != 0 {
		t.Errorf("tenant B sees %d of tenant A's audit rows, want 0", visible)
	}
}

func TestChanges_KeepsOnlyDifferingFields(t *testing.T) {
	type activity struct {
		Status string `json:"status"`
		Title  string `json:"title"`
		Owner  string `json:"owner,omitempty"`
	}
	b, a, err := service.Changes(activity{Status: "draft", Title: "HR"}, activity{Status: "approved", Title: "HR", Owner: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1 || b["status"] != "draft" {
		t.Errorf("before = %v, want only status", b)
	}
	if len(a) != 2 || a["status"] != "approved" || a["owner"] != "u1" {
		t.Errorf("after = %v, want status and owner", a)
	}
	if b, a, _ := service.Changes(nil, activity{Status: "draft"}); b != nil || a["status"] != "draft" {
		t.Errorf("create: before=%v after=%v", b, a)
	}
}

// The background verifier reports a broken chain as an alert naming the first bad row.
func TestVerifierJob_AlertsOnBrokenChain(t *testing.T) {
	f := setup(t)
	for i := 0; i < 3; i++ {
		f.write(t, f.a, sampleEntry(t, f.a, i))
	}
	ids := f.ids(t, f.a)
	f.asOwner(t, f.a, `UPDATE platform.audit_log SET action = 'forged' WHERE id = $1`, ids[1])

	var logs bytes.Buffer
	w := &auditjobs.Verifier{Audit: f.svc, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return w.Work(ctx, &river.Job[auditjobs.VerifyArgs]{Args: auditjobs.VerifyArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}}})
	}); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	if !strings.Contains(out, `"alert":"audit_chain_broken"`) || !strings.Contains(out, fmt.Sprintf(`"row_id":%d`, ids[1])) {
		t.Errorf("verifier log = %s, want audit_chain_broken alert at row %d", out, ids[1])
	}
}

func TestVerifySweeper_EnqueuesPerTenant(t *testing.T) {
	f := setup(t)
	client, err := jobs.NewInsertClient(f.app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'audit.verify'`) })
	if err := (&auditjobs.VerifySweeper{Pool: f.app, River: client}).Work(context.Background(), &river.Job[auditjobs.VerifySweepArgs]{}); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []dbtest.Tenant{f.a, f.b} {
		var n int
		if err := f.app.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind = 'audit.verify' AND args->>'tenant_id' = $1`, tenant.ID.String()).Scan(&n); err != nil || n != 1 {
			t.Errorf("tenant %s: %d verify jobs (err %v), want 1", tenant.ID, n, err)
		}
	}
	if !jobs.GlobalKinds[auditjobs.VerifySweepArgs{}.Kind()] {
		t.Error("audit.verify_sweep must be in jobs.GlobalKinds")
	}
}
