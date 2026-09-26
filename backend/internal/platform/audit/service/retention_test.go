package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	auditjobs "pdpa-platform/internal/platform/audit/jobs"
)

// ageRows moves a tenant's oldest n rows into partition month (UPDATE across partitions, as the owner) —
// the stand-in for rows written five years ago. Their hashes no longer match, which doesn't matter: the
// purge drops them; what must survive is the link from the first remaining row.
func (f *fixture) ageRows(t *testing.T, tenant dbtest.Tenant, month time.Time, n int) {
	t.Helper()
	f.asOwner(t, tenant, `
		UPDATE platform.audit_log l SET occurred_at = $2::timestamptz + make_interval(secs => o.rn)
		FROM (SELECT id, row_number() OVER (ORDER BY occurred_at, id) AS rn FROM platform.audit_log WHERE tenant_id = $1
		      ORDER BY occurred_at, id LIMIT $3) o
		WHERE l.id = o.id`, tenant.ID, month, n)
}

// Decision D-22: months older than 5 years are dropped; each tenant's chain still verifies from the anchor
// the purge recorded, new rows keep chaining (also for a tenant whose every row was purged), and a forged
// anchor is caught.
func TestRetention_DropsExpiredMonthsAndKeepsChainsVerifiable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	month := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -70, 0)
	part := fmt.Sprintf("audit_log_y%04dm%02d", month.Year(), int(month.Month()))
	if _, err := f.owner.Exec(ctx, fmt.Sprintf(`CREATE TABLE platform.%s PARTITION OF platform.audit_log FOR VALUES FROM ('%s') TO ('%s')`,
		part, month.Format(time.RFC3339), month.AddDate(0, 1, 0).Format(time.RFC3339))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.owner.Exec(context.Background(), `DROP TABLE IF EXISTS platform.`+part)
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			f.asOwner(t, tenant, `DELETE FROM platform.audit_chain_anchors WHERE tenant_id = $1`, tenant.ID)
		}
	})
	for i := range 3 {
		f.write(t, f.a, sampleEntry(t, f.a, i))
	}
	for i := range 2 {
		f.write(t, f.b, sampleEntry(t, f.b, i))
	}
	f.ageRows(t, f.a, month, 2)
	f.ageRows(t, f.b, month, 2) // every row of tenant B expires

	// The floor is the decided retention: app credentials can't purge anything younger.
	if _, err := f.app.Exec(ctx, `SELECT * FROM platform.drop_expired_audit_partitions(59)`); err == nil || !strings.Contains(err.Error(), "at least 60 months") {
		t.Fatalf("keep 59 months: %v", err)
	}
	w := &auditjobs.Retention{Pool: f.app}
	if err := w.Work(ctx, &river.Job[auditjobs.RetentionArgs]{Args: auditjobs.RetentionArgs{}}); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := f.owner.QueryRow(ctx, `SELECT to_regclass('platform.'||$1) IS NOT NULL`, part).Scan(&exists); err != nil || exists {
		t.Fatalf("expired partition still there: %v %v", exists, err)
	}
	var current bool
	if err := f.owner.QueryRow(ctx, `SELECT to_regclass(format('platform.audit_log_y%s', to_char(now() AT TIME ZONE 'UTC', 'YYYY"m"MM'))) IS NOT NULL`).Scan(&current); err != nil || !current {
		t.Fatalf("current month dropped: %v %v", current, err)
	}

	anchors := func(tenant dbtest.Tenant) (n int, dropped int64) {
		t.Helper()
		if err := pdb.WithTenantTx(ctx, f.app, tenant.ID.String(), "", func(ctx context.Context) error {
			return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*), coalesce(sum(rows_dropped), 0) FROM platform.audit_chain_anchors`).Scan(&n, &dropped)
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if n, d := anchors(f.a); n != 1 || d != 2 {
		t.Errorf("tenant A anchors: %d rows_dropped %d (RLS shows only its own)", n, d)
	}
	if n, d := anchors(f.b); n != 1 || d != 2 {
		t.Errorf("tenant B anchors: %d rows_dropped %d", n, d)
	}
	if res := f.verify(t, f.a); !res.OK || res.Checked != 1 {
		t.Errorf("tenant A after purge: %+v", res)
	}
	if res := f.verify(t, f.b); !res.OK || res.Checked != 0 {
		t.Errorf("tenant B after purge: %+v", res)
	}
	f.write(t, f.a, sampleEntry(t, f.a, 9))
	f.write(t, f.b, sampleEntry(t, f.b, 9))
	if res := f.verify(t, f.a); !res.OK || res.Checked != 2 {
		t.Errorf("tenant A keeps chaining: %+v", res)
	}
	if res := f.verify(t, f.b); !res.OK || res.Checked != 1 {
		t.Errorf("tenant B chains its first new row to the anchor: %+v", res)
	}
	if _, err := f.app.Exec(ctx, `INSERT INTO platform.audit_chain_anchors (tenant_id, partition_name, dropped_through, last_id, last_occurred_at, last_hash, rows_dropped)
		VALUES ($1, 'x', now(), 1, now(), repeat('0', 64), 1)`, f.a.ID); err == nil {
		t.Error("the app role wrote an anchor")
	}
	// Someone with owner access forging a newer anchor (to hide removed rows) breaks verification.
	f.asOwner(t, f.a, `INSERT INTO platform.audit_chain_anchors (tenant_id, partition_name, dropped_through, last_id, last_occurred_at, last_hash, rows_dropped)
		VALUES ($1, 'forged', now(), 1, now(), repeat('0', 64), 1)`, f.a.ID)
	if res := f.verify(t, f.a); res.OK || res.Reason != "prev_hash_mismatch" {
		t.Errorf("forged anchor: %+v", res)
	}
}
