package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/versioning"
	"pdpa-platform/internal/wiring"
)

type roleAssignEnv struct {
	app        *pgxpool.Pool
	tenant     dbtest.Tenant
	svc        *iamservice.Service
	versioning *versioning.Service
	target     uuid.UUID
	sec        uuid.UUID
}

func roleAssignSetup(t *testing.T, suffix string) roleAssignEnv {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		owner := dbtest.OwnerPool(t)
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM iam.role_assignment_requests`, `DELETE FROM platform.approvals`,
				`DELETE FROM platform.record_versions`, `DELETE FROM iam.role_assignments`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	var target, sec uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Target User', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@iam06.example").Scan(&target); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Sam Sec', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@iam06.example").Scan(&sec); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'SEC' AND tenant_id IS NULL`, tenant.ID, sec)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	iamSvc := &iamservice.Service{}
	versioningSvc := &versioning.Service{}
	wiring.RegisterIamVersioning(iamSvc, versioningSvc)
	return roleAssignEnv{app: app, tenant: tenant, svc: iamSvc, versioning: versioningSvc, target: target, sec: sec}
}

func (e roleAssignEnv) as(userID uuid.UUID, roles, perms []string, fn func(ctx context.Context) error) error {
	return pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), userID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: userID.String(), Roles: roles, Permissions: perms}))
	})
}

var orgadminPerms = []string{"admin.user.update", "admin.user.read"}

// TestRequestRoleAssignment_RequesterCannotApproveOwnRequest is the acceptance criterion directly: the
// requester holding ORGADMIN cannot decide their own request (versioning.ErrSelfApproval, surfaced
// through the generic Decide call every maker-checker flow shares) even though they also hold every
// permission the step would otherwise need.
func TestRequestRoleAssignment_RequesterCannotApproveOwnRequest(t *testing.T) {
	e := roleAssignSetup(t, "iam06self")
	var req iamservice.RoleAssignmentRequest
	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		r, err := e.svc.RequestRoleAssignment(ctx, e.target)
		req = r
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if req.VersionID == (uuid.UUID{}) {
		t.Fatal("expected a version id on the fresh request")
	}

	// List the approvals for this version to find the step the requester themselves would try to decide.
	var approvalID uuid.UUID
	var approvalRowVersion int32
	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		v, err := e.versioning.Get(ctx, req.VersionID)
		if err != nil {
			return err
		}
		if len(v.Approvals) != 1 {
			t.Fatalf("expected exactly one approval step, got %d", len(v.Approvals))
		}
		approvalID, approvalRowVersion = v.Approvals[0].ID, v.Approvals[0].RowVersion
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err := e.as(e.tenant.UserID, []string{"SEC"}, orgadminPerms, func(ctx context.Context) error {
		_, err := e.versioning.Decide(ctx, approvalID, approvalRowVersion, "approved", "")
		return err
	})
	if !errors.Is(err, versioning.ErrSelfApproval) {
		t.Fatalf("decide by the requester = %v, want ErrSelfApproval", err)
	}

	// The role grant must not exist yet — no effect until approved.
	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		r, err := e.svc.GetRoleAssignmentRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if r.RoleAssignmentID != nil {
			t.Error("role was granted despite the decision being refused")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRequestRoleAssignment_DifferentApproverPublishesAndGrants is the acceptance criterion's other half:
// once a different person (SEC) decides and the version is published, the real iam.role_assignments row
// exists — never before.
func TestRequestRoleAssignment_DifferentApproverPublishesAndGrants(t *testing.T) {
	e := roleAssignSetup(t, "iam06grant")
	var req iamservice.RoleAssignmentRequest
	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		r, err := e.svc.RequestRoleAssignment(ctx, e.target)
		req = r
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var approvalID uuid.UUID
	var approvalRowVersion int32
	if err := e.as(e.sec, []string{"SEC"}, nil, func(ctx context.Context) error {
		v, err := e.versioning.Get(ctx, req.VersionID)
		if err != nil {
			return err
		}
		approvalID, approvalRowVersion = v.Approvals[0].ID, v.Approvals[0].RowVersion
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// SEC decides — read access comes purely from holding the step's own role, no extra permission needed.
	var afterDecide versioning.Version
	if err := e.as(e.sec, []string{"SEC"}, nil, func(ctx context.Context) error {
		v, err := e.versioning.Decide(ctx, approvalID, approvalRowVersion, "approved", "")
		afterDecide = v
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if afterDecide.Status != "approved" {
		t.Fatalf("status after decide = %q, want approved", afterDecide.Status)
	}

	// Still no grant before Publish.
	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		r, err := e.svc.GetRoleAssignmentRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if r.RoleAssignmentID != nil {
			t.Error("role was granted before publish")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		_, err := e.versioning.Publish(ctx, afterDecide.ID, afterDecide.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		r, err := e.svc.GetRoleAssignmentRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if r.RoleAssignmentID == nil {
			t.Fatal("expected the role to be granted after publish")
		}
		var code string
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `SELECT r.code FROM iam.role_assignments ra JOIN iam.roles r ON r.id = ra.role_id WHERE ra.id = $1`,
			*r.RoleAssignmentID).Scan(&code); err != nil {
			return err
		}
		if code != "ORGADMIN" {
			t.Errorf("granted role = %q, want ORGADMIN", code)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRoleAssignment_UnknownTargetRefused(t *testing.T) {
	e := roleAssignSetup(t, "iam06unknown")
	err := e.as(e.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		_, err := e.svc.RequestRoleAssignment(ctx, uuid.New())
		return err
	})
	if !errors.Is(err, iamservice.ErrTargetUserNotFound) {
		t.Fatalf("err = %v, want ErrTargetUserNotFound", err)
	}
}

func TestListRoleAssignmentRequests_TwoTenantIsolation(t *testing.T) {
	a := roleAssignSetup(t, "iam06isoa")
	b := roleAssignSetup(t, "iam06isob")
	if err := a.as(a.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		_, err := a.svc.RequestRoleAssignment(ctx, a.target)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.as(b.tenant.UserID, nil, orgadminPerms, func(ctx context.Context) error {
		rs, err := b.svc.ListRoleAssignmentRequests(ctx)
		if err != nil {
			return err
		}
		if len(rs) != 0 {
			t.Errorf("tenant B saw %d requests, want 0 (tenant A's own)", len(rs))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
