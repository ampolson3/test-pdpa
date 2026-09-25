package iamstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	iamstore "pdpa-platform/internal/iam/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
)

// TestTenantIsolation_Users is PLT-02's acceptance criterion for the iam.users repository:
// "ผู้ใช้ tenant A เข้าถึงข้อมูล tenant B ไม่ได้แม้เรียก API ตรง" (docs/modules/PLT.md#plt-02) — proven here
// one layer below the API, directly against the store, where every future repository's own
// isolation test (CLAUDE.md rule 1) should follow the same shape via internal/pkg/dbtest.
func TestTenantIsolation_Users(t *testing.T) {
	ctx := context.Background()
	appPool := dbtest.Pool(t)
	platformPool := dbtest.PlatformPool(t)

	tenantA := dbtest.SeedTenant(t, ctx, appPool, platformPool, "plt02-a")
	tenantB := dbtest.SeedTenant(t, ctx, appPool, platformPool, "plt02-b")

	// Tenant B's own transaction must not be able to read tenant A's user by id, even though the
	// query has no tenant filter of its own — RLS (backend/db/migrations, policy on iam.users)
	// enforces the boundary at the database, not in application code.
	err := pdb.WithTenantTx(ctx, appPool, tenantB.ID.String(), tenantB.UserID.String(), func(ctx context.Context) error {
		q := iamstore.New(pdb.MustTxFromContext(ctx))
		_, err := q.GetUserByID(ctx, tenantA.UserID)
		if err == nil {
			t.Fatalf("tenant B read tenant A's user %s — RLS isolation is broken", tenantA.UserID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows (row filtered by RLS), got: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant B transaction: %v", err)
	}

	// Sanity check: a tenant can still read its own user — rules out a query that's simply broken
	// (which would make the assertion above pass for the wrong reason).
	err = pdb.WithTenantTx(ctx, appPool, tenantA.ID.String(), tenantA.UserID.String(), func(ctx context.Context) error {
		q := iamstore.New(pdb.MustTxFromContext(ctx))
		u, err := q.GetUserByID(ctx, tenantA.UserID)
		if err != nil {
			return err
		}
		if u.ID != tenantA.UserID {
			t.Fatalf("GetUserByID returned a different user than requested")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant A transaction: %v", err)
	}
}
