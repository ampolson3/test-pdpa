package service_test

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/audit/service"
)

func (f *fixture) in(t *testing.T, tenant dbtest.Tenant, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), tenant.UserID.String(), fn); err != nil {
		t.Fatal(err)
	}
}

// Acceptance (ORG-19): who changed whose permissions, and when — found by the target user — and exported.
func TestSearch_WhoChangedWhosePermissions(t *testing.T) {
	f := setup(t)
	var bob uuid.UUID
	f.in(t, f.a, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET display_name = 'Alice Admin' WHERE id = $1`, f.a.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'bob@auditsearch.example', 'Bob', 'disabled') RETURNING id`, f.a.ID).Scan(&bob)
	})
	t.Cleanup(func() {
		f.in(t, f.a, func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, bob)
			return err
		})
	})
	alice := f.a.UserID
	f.write(t, f.a, service.Entry{ActorType: "user", ActorID: &alice, Action: "GET /admin/v1/me"})
	f.write(t, f.a, service.Entry{ActorType: "user", ActorID: &alice, Action: "iam.role_assignment.create", EntityType: "user", EntityID: &bob,
		Before: map[string]any{"roles": []string{"EMP"}}, After: map[string]any{"roles": []string{"EMP", "=DPO"}}})
	f.write(t, f.a, service.Entry{ActorType: "user", ActorID: &alice, Action: "org.calendar.update", EntityType: "business_calendar", EntityID: &alice})
	f.write(t, f.b, service.Entry{ActorType: "user", Action: "iam.role_assignment.create", EntityType: "user", EntityID: &bob})

	f.in(t, f.a, func(ctx context.Context) error {
		got, next, err := f.svc.Search(ctx, service.Filter{EntityType: "user", EntityID: &bob}, "", 50)
		if err != nil || next != "" || len(got) != 1 {
			t.Fatalf("by target user: %d %q %v", len(got), next, err)
		}
		e := got[0]
		if e.ActorName != "Alice Admin" || e.EntityName != "Bob" || e.Action != "iam.role_assignment.create" || e.OccurredAt.IsZero() || e.Before == nil || e.After == nil {
			t.Errorf("who/whose/when/what: %+v", e)
		}
		// Default kind hides per-request rows; module filter by action prefix; time range.
		changes, _, _ := f.svc.Search(ctx, service.Filter{}, "", 50)
		requests, _, _ := f.svc.Search(ctx, service.Filter{Kind: "requests"}, "", 50)
		all, _, _ := f.svc.Search(ctx, service.Filter{Kind: "all"}, "", 50)
		if len(changes) != 2 || len(requests) != 1 || len(all) != 3 {
			t.Errorf("kinds: changes %d requests %d all %d", len(changes), len(requests), len(all))
		}
		if iam, _, _ := f.svc.Search(ctx, service.Filter{ActionPrefix: "iam."}, "", 50); len(iam) != 1 {
			t.Errorf("module filter: %d", len(iam))
		}
		if pct, _, _ := f.svc.Search(ctx, service.Filter{ActionPrefix: "%"}, "", 50); len(pct) != 0 {
			t.Errorf("LIKE wildcard not escaped: %d", len(pct))
		}
		future := time.Now().Add(time.Hour)
		if later, _, _ := f.svc.Search(ctx, service.Filter{From: &future}, "", 50); len(later) != 0 {
			t.Errorf("time range: %d", len(later))
		}
		if _, _, err := f.svc.Search(ctx, service.Filter{From: &future, To: &future}, "", 50); !errors.Is(err, service.ErrBadFilter) {
			t.Errorf("empty range: %v", err)
		}
		if _, _, err := f.svc.Search(ctx, service.Filter{}, "garbage!", 50); !errors.Is(err, service.ErrBadCursor) {
			t.Errorf("bad cursor: %v", err)
		}
		// Paging: newest first, the cursor continues without overlap.
		p1, cur, err := f.svc.Search(ctx, service.Filter{Kind: "all"}, "", 2)
		if err != nil || len(p1) != 2 || cur == "" || p1[0].Action != "org.calendar.update" {
			t.Fatalf("page 1: %+v %q %v", p1, cur, err)
		}
		p2, cur2, _ := f.svc.Search(ctx, service.Filter{Kind: "all"}, cur, 2)
		if len(p2) != 1 || cur2 != "" || p2[0].Action != "GET /admin/v1/me" {
			t.Errorf("page 2: %+v %q", p2, cur2)
		}

		buf, n, err := f.svc.Export(ctx, service.Filter{EntityType: "user", EntityID: &bob})
		if err != nil || n != 1 {
			t.Fatalf("export: %d %v", n, err)
		}
		recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(buf.String(), "\uFEFF"))).ReadAll()
		if err != nil || len(recs) != 2 || recs[1][3] != "Alice Admin" || recs[1][7] != "Bob" || recs[1][4] != "iam.role_assignment.create" {
			t.Fatalf("csv: %v %v", recs, err)
		}
		if !strings.Contains(recs[1][9], `"=DPO"`) || strings.HasPrefix(recs[1][9], "=") {
			t.Errorf("after column: %q", recs[1][9])
		}
		return nil
	})
	// The export is itself on record; tenant B saw none of A's rows.
	f.in(t, f.a, func(ctx context.Context) error {
		got, _, err := f.svc.Search(ctx, service.Filter{ActionPrefix: "platform.audit.export"}, "", 5)
		if err != nil || len(got) != 1 || got[0].ActorName != "Alice Admin" {
			t.Errorf("export audited: %+v %v", got, err)
		}
		return nil
	})
	f.in(t, f.b, func(ctx context.Context) error {
		got, _, _ := f.svc.Search(ctx, service.Filter{Kind: "all"}, "", 50)
		if len(got) != 1 || got[0].EntityName != "" {
			t.Errorf("tenant B sees %d rows (want its own 1, no names from A): %+v", len(got), got)
		}
		return nil
	})
}

func TestExport_FormulaCellsAreText(t *testing.T) {
	f := setup(t)
	f.write(t, f.a, service.Entry{ActorType: "system", Action: "=HYPERLINK(\"x\")"})
	f.in(t, f.a, func(ctx context.Context) error {
		buf, _, err := f.svc.Export(ctx, service.Filter{})
		if err != nil {
			return err
		}
		if !strings.Contains(buf.String(), `'=HYPERLINK`) {
			t.Errorf("formula not neutralized: %s", buf.String())
		}
		return nil
	})
}
