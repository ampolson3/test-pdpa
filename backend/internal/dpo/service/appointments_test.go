package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/wiring"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	// user is a real, active iam.users row (dbtest.SeedTenant's own tenant user defaults to status
	// 'invited', which iamservice.Names — and so an "internal" appointment — doesn't count as usable).
	user  uuid.UUID
	svc   *dposervice.Service
	org   *orgservice.Service
	forms *forms.Service
}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	var user uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'DPO Person', 'active') RETURNING id`,
			tenant.ID, suffix+"-dpo@dbtest.example").Scan(&user)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.security_assessments`)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.tasks`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_submissions`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_versions`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_definitions`)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.appointments`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, user)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	formsSvc := wiring.Forms(nil, audit.New())
	svc := &dposervice.Service{Audit: audit.New(), Org: org, Forms: formsSvc}
	return env{app: app, tenant: tenant, user: user, svc: svc, org: org, forms: formsSvc}
}

var securityPerms = []string{"dpo.profile.read", "dpo.profile.create", "dpo.profile.update",
	"dpo.risk.read", "dpo.risk.create", "dpo.risk.update", "dpo.risk.approve",
	// only needed by TestAssess_RejectsNonSecurityForm, to publish a form of a different type
	"ropa.template.read", "ropa.template.create", "ropa.template.update", "ropa.template.publish"}

// in runs fn as the tenant's admin in one transaction (as the Tx middleware would).
func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(),
			Permissions: securityPerms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppointments_CRUDAndValidation(t *testing.T) {
	e := setup(t, "dpoappt")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		return err
	})

	appointedAt := date(2026, 1, 1)
	var created dposervice.Appointment
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "internal",
			UserID: &e.user, ContactEmail: "DPO@Example.com", AppointedAt: appointedAt}, 0)
		if err != nil {
			return err
		}
		if created.ContactEmail != "dpo@example.com" || created.RowVersion != 1 {
			t.Errorf("create did not stick / email not normalized: %+v", created)
		}
		got, err := e.svc.GetAppointment(ctx, created.ID)
		if err != nil {
			return err
		}
		if got.LegalEntityID != le.ID {
			t.Errorf("read back mismatch: %+v", got)
		}
		bogus := uuid.New()
		for name, in := range map[string]dposervice.Appointment{
			"bad dpo_type":         {LegalEntityID: le.ID, DPOType: "bogus", ContactEmail: "x@example.com", AppointedAt: appointedAt},
			"no email":             {LegalEntityID: le.ID, DPOType: "internal", UserID: &e.user, AppointedAt: appointedAt},
			"internal no user":     {LegalEntityID: le.ID, DPOType: "internal", ContactEmail: "x@example.com", AppointedAt: appointedAt},
			"external no name":     {LegalEntityID: le.ID, DPOType: "external", ContactEmail: "x@example.com", AppointedAt: appointedAt},
			"unknown legal entity": {LegalEntityID: bogus, DPOType: "internal", UserID: &e.user, ContactEmail: "x@example.com", AppointedAt: appointedAt},
			"unknown user":         {LegalEntityID: le.ID, DPOType: "internal", UserID: &bogus, ContactEmail: "x@example.com", AppointedAt: appointedAt},
		} {
			if _, err := e.svc.SaveAppointment(ctx, in, 0); !errors.Is(err, dposervice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveAppointment(ctx, dposervice.Appointment{ID: created.ID, LegalEntityID: le.ID, DPOType: "internal",
			UserID: &e.user, ContactEmail: "dpo@example.com", AppointedAt: appointedAt}, 0); !errors.Is(err, dposervice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		ended := date(2026, 6, 1)
		updated, err := e.svc.SaveAppointment(ctx, dposervice.Appointment{ID: created.ID, LegalEntityID: le.ID, DPOType: "external",
			ExternalName: "สมชาย ใจดี", ExternalCompany: "ที่ปรึกษา จำกัด", ContactEmail: "consultant@example.com",
			AppointedAt: appointedAt, EndedAt: &ended}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.DPOType != "external" || updated.RowVersion != 2 || updated.EndedAt == nil {
			t.Errorf("update did not stick: %+v", updated)
		}
		return nil
	})
}

// TestCurrentContact_MergeFields is DPO-01's acceptance criterion: a legal entity's active appointment
// resolves to a contact channel (for PLT-16's merge fields), the internal appointee's name comes from iam,
// an ended appointment stops being current, and a legal entity with no appointment at all resolves to none.
func TestCurrentContact_MergeFields(t *testing.T) {
	e := setup(t, "dpocontact")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		_, ok, err := e.svc.CurrentContact(ctx, le.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Error("expected no current contact before any appointment")
		}
		return nil
	})

	var first dposervice.Appointment
	e.in(t, func(ctx context.Context) error {
		var err error
		first, err = e.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "internal",
			UserID: &e.user, ContactEmail: "dpo1@example.com", AppointedAt: date(2026, 1, 1)}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		c, ok, err := e.svc.CurrentContact(ctx, le.ID)
		if err != nil {
			return err
		}
		if !ok || c.Email != "dpo1@example.com" || c.Name == "" {
			t.Errorf("expected the internal appointee's contact, got %+v ok=%v", c, ok)
		}
		fields, err := e.svc.MergeFields(ctx, le.ID)
		if err != nil {
			return err
		}
		if fields["dpo_email"] != "dpo1@example.com" || fields["dpo_name"] == "" {
			t.Errorf("merge fields: %+v", fields)
		}
		return nil
	})

	// Ending the appointment: no current contact any more.
	e.in(t, func(ctx context.Context) error {
		ended := date(2026, 2, 1)
		_, err := e.svc.SaveAppointment(ctx, dposervice.Appointment{ID: first.ID, LegalEntityID: le.ID, DPOType: "internal",
			UserID: &e.user, ContactEmail: "dpo1@example.com", AppointedAt: date(2026, 1, 1), EndedAt: &ended}, first.RowVersion)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, ok, err := e.svc.CurrentContact(ctx, le.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Error("expected no current contact after the appointment ended")
		}
		fields, err := e.svc.MergeFields(ctx, le.ID)
		if err != nil {
			return err
		}
		if fields != nil {
			t.Errorf("expected no merge fields after the appointment ended: %+v", fields)
		}
		return nil
	})

	// A second, external appointment becomes current.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "external",
			ExternalName: "สมชาย ใจดี", ContactEmail: "dpo2@example.com", AppointedAt: date(2026, 2, 15)}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		c, ok, err := e.svc.CurrentContact(ctx, le.ID)
		if err != nil {
			return err
		}
		if !ok || c.Email != "dpo2@example.com" || c.Name != "สมชาย ใจดี" {
			t.Errorf("expected the external appointee's contact, got %+v ok=%v", c, ok)
		}
		return nil
	})
}

// TestAppointments_Isolation is CLAUDE.md rule 1's per-repository requirement: tenant B's own transaction
// cannot read tenant A's appointment, whether by id or in its list.
func TestAppointments_Isolation(t *testing.T) {
	a := setup(t, "dpoisoa")
	b := setup(t, "dpoisob")
	var created dposervice.Appointment
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		created, err = a.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "internal",
			UserID: &a.user, ContactEmail: "a@example.com", AppointedAt: date(2026, 1, 1)}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetAppointment(ctx, created.ID); !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's appointment: %v", err)
		}
		list, _, err := b.svc.ListAppointments(ctx, dposervice.AppointmentFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
