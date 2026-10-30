package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	breachservice "pdpa-platform/internal/breach/service"
	dposervice "pdpa-platform/internal/dpo/service"
	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
)

type centerEnv struct {
	app         *pgxpool.Pool
	tenant      dbtest.Tenant
	legalEntity uuid.UUID
	svc         *dposervice.Service
	dsar        *dsarservice.Service
	breach      *breachservice.Service
}

var centerPerms = []string{"dpo.report.read", "dsar.request.read", "dsar.request.create",
	"breach.incident.read", "breach.incident.create", "org.structure.read", "org.structure.update"}

func centerSetup(t *testing.T, suffix string) centerEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.requests`, `DELETE FROM breach.timeline_events`, `DELETE FROM breach.incidents`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	dsarSvc := &dsarservice.Service{Audit: audit.New(), Org: orgSvc, Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}}
	breachSvc := &breachservice.Service{Audit: audit.New(), Org: orgSvc}
	dpoSvc := &dposervice.Service{Audit: audit.New(), Org: orgSvc, Dsar: dsarSvc, Breach: breachSvc}

	var legalEntity uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"org.structure.update"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return centerEnv{app: app, tenant: tenant, legalEntity: legalEntity, svc: dpoSvc, dsar: dsarSvc, breach: breachSvc}
}

func (e centerEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	e.inAs(t, centerPerms, fn)
}

func (e centerEnv) inAs(t *testing.T, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// makeDSARRequest creates a request and forces its due_at directly (deterministic, independent of any
// request type's own sla_days).
func (e centerEnv) makeDSARRequest(t *testing.T, ctx context.Context, dueAt time.Time) dsarservice.Request {
	t.Helper()
	rts, err := e.dsar.ListRequestTypes(ctx)
	if err != nil || len(rts) == 0 {
		t.Fatalf("list request types: %v", err)
	}
	req, err := e.dsar.CreateRequest(ctx, dsarservice.CreateRequestInput{
		RequestTypeID: rts[0].ID, LegalEntityID: e.legalEntity, Channel: "web",
		RequesterName: "Somchai Test", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail,
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE dsar.requests SET due_at = $1 WHERE id = $2`, dueAt, req.ID); err != nil {
		t.Fatalf("force due_at: %v", err)
	}
	req.DueAt = dueAt
	return req
}

// makeIncident creates an open breach incident and forces its 72-hour deadline directly.
func (e centerEnv) makeIncident(t *testing.T, ctx context.Context, dueAt time.Time) breachservice.Incident {
	t.Helper()
	inc, err := e.breach.Create(ctx, breachservice.Input{
		LegalEntityID: e.legalEntity, ReportedVia: "system", Title: "Test incident", Description: "A test incident",
		BreachTypes: []string{"confidentiality"}, AwareAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE breach.incidents SET pdpc_due_at = $1 WHERE id = $2`, dueAt, inc.ID); err != nil {
		t.Fatalf("force pdpc_due_at: %v", err)
	}
	inc.DueAt = dueAt
	return inc
}

// TestDeadlines_AggregatesNearDeadlineAcrossModules is DPO-05's acceptance criterion: near-deadline and
// overdue work from DSAR and breach shows on one page, sorted by due date, while an on-track item from
// either module is left out.
func TestDeadlines_AggregatesNearDeadlineAcrossModules(t *testing.T) {
	e := centerSetup(t, "dpocenter1")
	now := time.Now().UTC()

	var overdueReq, atRiskReq dsarservice.Request
	var overdueInc, atRiskInc breachservice.Incident
	e.in(t, func(ctx context.Context) error {
		overdueReq = e.makeDSARRequest(t, ctx, now.Add(-2*time.Hour))
		atRiskReq = e.makeDSARRequest(t, ctx, now.Add(dsarservice.AtRiskWindow-time.Hour))
		e.makeDSARRequest(t, ctx, now.Add(20*24*time.Hour)) // on_track, must not appear

		overdueInc = e.makeIncident(t, ctx, now.Add(-time.Hour))
		atRiskInc = e.makeIncident(t, ctx, now.Add(3*time.Hour)) // within the 6h at_risk window
		e.makeIncident(t, ctx, now.Add(48*time.Hour))            // on_track, must not appear
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		items, err := e.svc.Deadlines(ctx)
		if err != nil {
			return err
		}
		if len(items) != 4 {
			t.Fatalf("got %d items, want 4: %+v", len(items), items)
		}
		// Sorted by due_at ascending.
		for i := 1; i < len(items); i++ {
			if items[i].DueAt.Before(items[i-1].DueAt) {
				t.Errorf("not sorted by due_at ascending: %+v", items)
			}
		}
		want := map[uuid.UUID]string{
			overdueReq.ID: "overdue", atRiskReq.ID: "at_risk",
			overdueInc.ID: "overdue", atRiskInc.ID: "at_risk",
		}
		for _, it := range items {
			status, ok := want[it.ReferenceID]
			if !ok {
				t.Errorf("unexpected item: %+v", it)
				continue
			}
			if it.Status != status {
				t.Errorf("item %v: status = %q, want %q", it.ReferenceID, it.Status, status)
			}
			delete(want, it.ReferenceID)
		}
		if len(want) != 0 {
			t.Errorf("missing items: %+v", want)
		}
		return nil
	})
}

// TestDeadlines_SkipsSourceWithoutModuleReadPermission: a caller holding only dpo.report.read (not the
// source module's own read permission) sees that source silently left out, not a 403 for the whole call.
func TestDeadlines_SkipsSourceWithoutModuleReadPermission(t *testing.T) {
	e := centerSetup(t, "dpocenter2")
	now := time.Now().UTC()
	e.in(t, func(ctx context.Context) error {
		e.makeDSARRequest(t, ctx, now.Add(-time.Hour))
		e.makeIncident(t, ctx, now.Add(-time.Hour))
		return nil
	})

	e.inAs(t, []string{"dpo.report.read"}, func(ctx context.Context) error {
		items, err := e.svc.Deadlines(ctx)
		if err != nil {
			return err
		}
		if len(items) != 0 {
			t.Errorf("got %d items without module read permissions, want 0: %+v", len(items), items)
		}
		return nil
	})
}

// TestDeadlines_TwoTenantIsolation proves tenant B never sees tenant A's deadlines.
func TestDeadlines_TwoTenantIsolation(t *testing.T) {
	a := centerSetup(t, "dpocenteraA")
	b := centerSetup(t, "dpocenteraB")
	now := time.Now().UTC()
	a.in(t, func(ctx context.Context) error {
		a.makeDSARRequest(t, ctx, now.Add(-time.Hour))
		a.makeIncident(t, ctx, now.Add(-time.Hour))
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		items, err := b.svc.Deadlines(ctx)
		if err != nil {
			return err
		}
		if len(items) != 0 {
			t.Errorf("tenant B saw tenant A's deadlines: %+v", items)
		}
		return nil
	})
}
