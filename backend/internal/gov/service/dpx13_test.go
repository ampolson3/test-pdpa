package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/connectors"
)

// fakeHris stands in for the connector framework: one roster per connector id, or an error.
type fakeHris struct {
	conns   []connectors.Connector
	rosters map[uuid.UUID][]connectors.Employee
}

func (f *fakeHris) ListActiveWithCapability(_ context.Context, capability string) ([]connectors.Connector, error) {
	if capability != connectors.CapListEmployees {
		return nil, nil
	}
	return f.conns, nil
}

func (f *fakeHris) ListEmployees(_ context.Context, id uuid.UUID) ([]connectors.Employee, error) {
	r, ok := f.rosters[id]
	if !ok {
		return nil, errors.New("hris unreachable")
	}
	return r, nil
}

func seedUser(t *testing.T, e govEnv, email, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`INSERT INTO iam.users (id, tenant_id, email, display_name, status) VALUES ($1, $2, $3, $4, $5)`,
			id, e.tenant.ID, email, "DPX-13 "+email, status)
		return err
	})
	return id
}

func userStatus(t *testing.T, e govEnv, id uuid.UUID) (status string, disabledAtSet bool) {
	t.Helper()
	e.in(t, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT status, disabled_at IS NOT NULL FROM iam.users WHERE id = $1`, id).Scan(&status, &disabledAtSet)
	})
	return
}

// DPX-13 acceptance criterion: an employee who has left in the HRIS has their account closed.
func TestSyncHRIS_DisablesLeaversOnly(t *testing.T) {
	e := govSetup(t, "dpx13")
	leaver := seedUser(t, e, "leaver-"+uuid.NewString()[:8]+"@example.com", "active")
	stayer := seedUser(t, e, "stayer-"+uuid.NewString()[:8]+"@example.com", "active")
	var leaverEmail, stayerEmail string
	e.in(t, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `SELECT email FROM iam.users WHERE id = $1`, leaver).Scan(&leaverEmail); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT email FROM iam.users WHERE id = $1`, stayer).Scan(&stayerEmail)
	})
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), "", func(ctx context.Context) error {
			_, _ = pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM iam.users WHERE id = ANY($1)`, []uuid.UUID{leaver, stayer})
			return nil
		})
	})

	good, broken := uuid.New(), uuid.New()
	e.svc.Connectors = &fakeHris{
		conns: []connectors.Connector{{ID: broken}, {ID: good}},
		rosters: map[uuid.UUID][]connectors.Employee{good: {
			{ExternalID: "1", Email: leaverEmail, Terminated: true},
			{ExternalID: "2", Email: stayerEmail, Terminated: false},
			{ExternalID: "3", Email: "nobody-here@example.com", Terminated: true},
		}},
	}

	e.in(t, func(ctx context.Context) error {
		res, err := e.svc.SyncHRIS(ctx)
		if err != nil {
			return err
		}
		if res.ConnectorsChecked != 2 || res.ConnectorsFailed != 1 || res.EmployeesSeen != 3 || res.AccountsDisabled != 1 {
			t.Errorf("first run = %+v", res)
		}
		return nil
	})
	if st, at := userStatus(t, e, leaver); st != "disabled" || !at {
		t.Errorf("leaver status = %q disabled_at set = %v, want disabled/true", st, at)
	}
	if st, _ := userStatus(t, e, stayer); st != "active" {
		t.Errorf("stayer status = %q, want active", st)
	}

	// The closed account loads no grants at all.
	grants, err := iamservice.NewLoader(e.app)(context.Background(), e.tenant.ID.String(), leaver.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(grants.Roles) != 0 || len(grants.Permissions) != 0 {
		t.Errorf("disabled account still has grants: %+v", grants)
	}

	// The audit row names the user id and carries no e-mail address.
	e.in(t, func(ctx context.Context) error {
		var n int
		var after string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*), coalesce(max(after::text), '') FROM platform.audit_log WHERE action = 'iam.user.disable' AND entity_id = $1`,
			leaver).Scan(&n, &after); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("audit rows = %d, want 1", n)
		}
		if after == "" || strings.Contains(after, leaverEmail) {
			t.Errorf("audit after = %q: must exist and hold no e-mail", after)
		}
		return nil
	})

	// A second run closes nothing more.
	e.in(t, func(ctx context.Context) error {
		res, err := e.svc.SyncHRIS(ctx)
		if err != nil {
			return err
		}
		if res.AccountsDisabled != 0 {
			t.Errorf("second run disabled %d accounts, want 0", res.AccountsDisabled)
		}
		return nil
	})
}

// A leaver in tenant A's HRIS never closes the same e-mail's account in tenant B (RLS).
func TestSyncHRIS_TwoTenantIsolation(t *testing.T) {
	a, b := govSetup(t, "dpx13a"), govSetup(t, "dpx13b")
	email := "shared-" + uuid.NewString()[:8] + "@example.com"
	inB := seedUser(t, b, email, "active")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), b.app, b.tenant.ID.String(), "", func(ctx context.Context) error {
			_, _ = pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, inB)
			return nil
		})
	})
	conn := uuid.New()
	a.svc.Connectors = &fakeHris{conns: []connectors.Connector{{ID: conn}},
		rosters: map[uuid.UUID][]connectors.Employee{conn: {{Email: email, Terminated: true}}}}
	a.in(t, func(ctx context.Context) error {
		res, err := a.svc.SyncHRIS(ctx)
		if err != nil {
			return err
		}
		if res.AccountsDisabled != 0 {
			t.Errorf("tenant A closed %d accounts, want 0", res.AccountsDisabled)
		}
		return nil
	})
	if st, _ := userStatus(t, b, inB); st != "active" {
		t.Errorf("tenant B user status = %q, want active", st)
	}
}
