// Package consenttest is the consent module's test fixture: two tenants with a legal entity each, a maker and a
// DPO in tenant A, and helpers that take purposes and collection points live. Used by the service and HTTP tests.
package consenttest

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	consent "pdpa-platform/internal/consent/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
)

type Fixture struct {
	App, Owner *pgxpool.Pool
	A, B       dbtest.Tenant
	Alice, DPO uuid.UUID // Alice drafts (MKT), DPO approves
	EntityA    uuid.UUID
	EntityB    uuid.UUID
	Svc        *consent.Service
	Ver        *versioning.Service
}

var Maker = []string{"consent.purpose.read", "consent.purpose.create", "consent.purpose.update", "consent.purpose.publish",
	"consent.collectionpoint.read", "consent.collectionpoint.create", "consent.collectionpoint.update", "consent.collectionpoint.publish",
	"consent.record.read", "consent.onbehalf.create"}

func Setup(t *testing.T) *Fixture {
	t.Helper()
	ctx := context.Background()
	f := &Fixture{App: dbtest.Pool(t), Owner: dbtest.OwnerPool(t)}
	platform := dbtest.PlatformPool(t)
	f.A = dbtest.SeedTenant(t, ctx, f.App, platform, "consent-a")
	f.B = dbtest.SeedTenant(t, ctx, f.App, platform, "consent-b")
	f.Alice = f.A.UserID
	for _, tn := range []struct {
		t      dbtest.Tenant
		entity *uuid.UUID
	}{{f.A, &f.EntityA}, {f.B, &f.EntityB}} {
		if err := pdb.WithTenantTx(ctx, f.App, tn.t.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, tn.t.UserID); err != nil {
				return err
			}
			if tn.t.ID == f.A.ID {
				if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'dpo@consent.example', 'Dee DPO', 'active') RETURNING id`, tn.t.ID).Scan(&f.DPO); err != nil {
					return err
				}
			}
			return tx.QueryRow(ctx, `INSERT INTO org.legal_entities (tenant_id, name_th) VALUES ($1, 'บริษัท ทดสอบ จำกัด') RETURNING id`, tn.t.ID).Scan(tn.entity)
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tn := range []dbtest.Tenant{f.A, f.B} {
			_ = pdb.WithTenantTx(context.Background(), f.Owner, tn.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				for _, q := range []string{`DELETE FROM consent.consent_status`, `DELETE FROM consent.consent_transactions`, `DELETE FROM consent.consent_receipts`,
					`DELETE FROM consent.subject_identifiers`, `DELETE FROM consent.data_subjects`, `DELETE FROM consent.collection_point_purposes`,
					`DELETE FROM consent.collection_points`, `DELETE FROM consent.purpose_preferences`, `UPDATE consent.purposes SET current_version_id = NULL`,
					`DELETE FROM consent.purpose_versions`, `DELETE FROM consent.purposes`, `DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`,
					`DELETE FROM platform.public_keys`, `DELETE FROM platform.webhook_deliveries`, `DELETE FROM platform.outbox_events`, `DELETE FROM platform.notifications`,
					`DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`, `DELETE FROM org.legal_entities`} {
					if _, err := tx.Exec(ctx, q); err != nil {
						t.Logf("cleanup %q: %v", q, err)
					}
				}
				_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id <> $1`, tn.UserID)
				return err
			})
			_, _ = f.App.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tn.ID.String())
		}
	})
	client, _ := jobs.NewInsertClient(f.App)
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	n := &notify.Service{Keyring: keyring, River: client}
	f.Ver = &versioning.Service{Notify: n, Audit: audit.New()}
	f.Svc = &consent.Service{Versioning: f.Ver, Events: &events.Publisher{River: client}, Notify: n, Keyring: keyring, Audit: audit.New(), Org: &orgservice.Service{}}
	f.Svc.RegisterVersioning()
	return f
}

// as runs fn in one transaction of tenant as user with roles and permissions.
func (f *Fixture) As(t *testing.T, tenant dbtest.Tenant, user uuid.UUID, roles, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), f.App, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Roles: roles, Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// public runs fn as a web form of tenant a would (no user).
func (f *Fixture) Public(t *testing.T, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: f.A.ID.String()}))
	})
}

func Content(name, text string, cats ...string) consent.PurposeContent {
	return consent.PurposeContent{Name: consent.Text{Th: name, En: name + " (en)"}, ConsentText: consent.Text{Th: text, En: text + " (en)"}, DataCategoryCodes: cats}
}

// livePurpose creates a purpose and takes its draft through DPO approval to published.
func (f *Fixture) LivePurpose(t *testing.T, code string, c consent.PurposeContent) consent.Purpose {
	t.Helper()
	var p consent.Purpose
	f.As(t, f.A, f.Alice, nil, Maker, func(ctx context.Context) error {
		var err error
		p, err = f.Svc.CreatePurpose(ctx, code, f.EntityA, c)
		return err
	})
	f.ApproveAndPublish(t, p.ID)
	f.As(t, f.A, f.Alice, nil, Maker, func(ctx context.Context) error {
		var err error
		p, err = f.Svc.GetPurpose(ctx, p.ID)
		return err
	})
	return p
}

func (f *Fixture) ApproveAndPublish(t *testing.T, purpose uuid.UUID) error {
	t.Helper()
	var v versioning.Version
	f.As(t, f.A, f.Alice, nil, Maker, func(ctx context.Context) error {
		vs, err := f.Ver.List(ctx, consent.PurposeType, purpose)
		if err != nil {
			return err
		}
		v, err = f.Ver.Submit(ctx, vs[0].ID, vs[0].RowVersion)
		return err
	})
	f.As(t, f.A, f.DPO, []string{"DPO"}, []string{"consent.purpose.read"}, func(ctx context.Context) error {
		inbox, err := f.Ver.Inbox(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(inbox, func(it versioning.InboxItem) bool { return it.VersionID == v.ID })
		if i < 0 {
			t.Fatalf("not in the DPO's inbox: %+v", inbox)
		}
		v, err = f.Ver.Decide(ctx, inbox[i].ID, inbox[i].RowVersion, "approved", "")
		return err
	})
	var pubErr error
	f.As(t, f.A, f.Alice, nil, Maker, func(ctx context.Context) error {
		err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			_, err := f.Ver.Publish(ctx, v.ID, v.RowVersion)
			return err
		})
		pubErr = err
		return nil
	})
	return pubErr
}

var FullChecklist = consent.Checklist{SeparateText: true, NotBundled: true, PlainLanguage: true, WithdrawalInfo: true}

func (f *Fixture) LiveCP(t *testing.T, code string, purposes ...consent.CPPurposeInput) consent.CollectionPoint {
	t.Helper()
	var cp consent.CollectionPoint
	f.As(t, f.A, f.Alice, nil, Maker, func(ctx context.Context) error {
		var err error
		if cp, err = f.Svc.CreateCollectionPoint(ctx, consent.CollectionPointInput{Code: code, Name: "สมัครสมาชิก", Channel: "web", LegalEntityID: f.EntityA, Purposes: purposes}); err != nil {
			return err
		}
		cp, err = f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, FullChecklist)
		return err
	})
	return cp
}

func (f *Fixture) Record(t *testing.T, sub consent.Submission) (consent.Receipt, error) {
	t.Helper()
	var r consent.Receipt
	err := f.Public(t, func(ctx context.Context) error {
		var err error
		r, err = f.Svc.Record(ctx, sub)
		return err
	})
	return r, err
}

func Email(v string) []consent.Identifier { return []consent.Identifier{{Type: "email", Value: v}} }
