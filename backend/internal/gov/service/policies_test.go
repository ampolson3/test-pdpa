package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	govservice "pdpa-platform/internal/gov/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
	"pdpa-platform/internal/wiring"
)

// policyEnv is DPX-02's own fixture: a real PLT-16 "policy" document type (already registered by
// wiring.Docs, borrowing notice's own permission codes — see internal/wiring/docs.go) driven through the
// real submit → DPO approve → publish chain, exactly the way notice's own checklist_test.go already proves
// out a full PLT-08 cycle needs no S3/clamd in this environment.
type policyEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	dpo    uuid.UUID
	emp    uuid.UUID
	svc    *govservice.Service
	docs   *docs.Service
	ver    *versioning.Service
}

var (
	policyAuthorPerms = []string{"notice.document.read", "notice.document.create", "notice.document.update", "dpx.policy.publish", "dpx.policy.read"}
	policyDpoPerms    = []string{"notice.document.read", "notice.document.update", "notice.document.approve", "notice.document.publish", "dpx.policy.read", "dpx.policy.publish"}
	policyEmpPerms    = []string{"dpx.policy.execute"}
)

func policySetup(t *testing.T, suffix string) policyEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM gov.policy_attestations`, `DELETE FROM gov.policies`,
				`DELETE FROM platform.document_downloads`, `DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM iam.role_assignments`, `DELETE FROM platform.audit_log`,
				`DELETE FROM iam.users WHERE email LIKE '%@dpx02.example'`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &govservice.Service{Audit: audit.New(), Docs: docsSvc}

	var dpo, emp uuid.UUID
	if err := pdb.WithTenantTx(ctx, owner, tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Dee DPO', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@dpx02.example").Scan(&dpo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'DPO' AND tenant_id IS NULL`, tenant.ID, dpo); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Emp One', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@dpx02.example").Scan(&emp)
	}); err != nil {
		t.Fatal(err)
	}
	return policyEnv{app: app, tenant: tenant, dpo: dpo, emp: emp, svc: svc, docs: docsSvc, ver: versioningSvc}
}

func (e policyEnv) inAs(user uuid.UUID, permissions []string, fn func(ctx context.Context) error) error {
	return pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: user.String(), Permissions: permissions}))
	})
}

func (e policyEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := e.inAs(e.tenant.UserID, policyAuthorPerms, fn); err != nil {
		t.Fatal(err)
	}
}

// publishedPolicyDocument drives a brand-new "policy" document through the real PLT-08 submit → DPO
// approve → publish chain and returns its id — the same pattern notice's own checklist_test.go already
// established for proving this cycle needs no S3/clamd.
func publishedPolicyDocument(t *testing.T, e policyEnv, title string) uuid.UUID {
	t.Helper()
	var doc docs.Document
	if err := e.inAs(e.tenant.UserID, policyAuthorPerms, func(ctx context.Context) error {
		var err error
		doc, err = e.docs.Create(ctx, docs.CreateInput{DocType: "policy", Title: title})
		if err != nil {
			return err
		}
		doc, err = e.docs.SaveDraft(ctx, doc.ID, doc.RowVersion, docs.Draft{
			Title:   title,
			Content: render.Content{"th": {Type: "doc", Content: []render.Node{{Type: "paragraph", Content: []render.Node{{Type: "text", Text: "เนื้อหานโยบาย"}}}}}},
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var v versioning.Version
	if err := e.inAs(e.tenant.UserID, policyAuthorPerms, func(ctx context.Context) error {
		var err error
		v, err = e.ver.Submit(ctx, doc.Latest.ID, doc.Latest.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		inbox, err := e.ver.Inbox(ctx)
		if err != nil {
			return err
		}
		for _, it := range inbox {
			if it.VersionID == v.ID {
				v, err = e.ver.Decide(ctx, it.ID, it.RowVersion, "approved", "")
				return err
			}
		}
		t.Fatal("submitted version not in the DPO's inbox")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		_, err := e.ver.Publish(ctx, v.ID, v.RowVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return doc.ID
}

// TestPublishPolicy_TracksTheDocumentsCurrentVersion is the write-side half of the feature: publishing an
// already-published PLT-16 document records this feature's own tracked round at that document's current
// version — publishing the identical document again (no new version) is a harmless no-op, not a duplicate.
func TestPublishPolicy_TracksTheDocumentsCurrentVersion(t *testing.T) {
	e := policySetup(t, "dpx02pub")
	docID := publishedPolicyDocument(t, e, "นโยบายความเป็นส่วนตัวภายใน")

	var p1 govservice.Policy
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		var err error
		p1, err = e.svc.PublishPolicy(ctx, docID, "", true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p1.VersionNo != 1 || p1.Title != "นโยบายความเป็นส่วนตัวภายใน" || !p1.RequiresAttestation {
		t.Fatalf("unexpected policy: %+v", p1)
	}

	var p2 govservice.Policy
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		var err error
		p2, err = e.svc.PublishPolicy(ctx, docID, "", true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p2.ID != p1.ID {
		t.Fatalf("republishing the same version should be a no-op on the same row, got a different id")
	}

	if err := e.inAs(e.dpo, []string{"dpx.policy.read"}, func(ctx context.Context) error {
		list, err := e.svc.ListPolicies(ctx)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 tracked policy, got %d", len(list))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPublishPolicy_RefusesAnUnpublishedDocument is the guard: a draft/never-published document names no
// real version employees could attest to.
func TestPublishPolicy_RefusesAnUnpublishedDocument(t *testing.T) {
	e := policySetup(t, "dpx02draft")
	var doc docs.Document
	e.in(t, func(ctx context.Context) error {
		var err error
		doc, err = e.docs.Create(ctx, docs.CreateInput{DocType: "policy", Title: "ยังไม่เผยแพร่"})
		return err
	})
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		_, err := e.svc.PublishPolicy(ctx, doc.ID, "", true)
		return err
	}); err == nil {
		t.Fatal("expected PublishPolicy to refuse an unpublished document")
	}
}

// TestAttestPolicy_AcceptanceCriterion is the acceptance criterion itself: an employee attests the
// policy's current version, and MissingAttestations — the roster of every active user minus whoever has
// attested that exact version — clears for them and stays listed for everyone else; attesting again is a
// harmless no-op, not a duplicate row.
func TestAttestPolicy_AcceptanceCriterion(t *testing.T) {
	e := policySetup(t, "dpx02attest")
	docID := publishedPolicyDocument(t, e, "นโยบายการใช้งานระบบ")
	var p govservice.Policy
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		var err error
		p, err = e.svc.PublishPolicy(ctx, docID, "", true)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Before anyone attests: both the tenant's own seeded user and emp are missing.
	if err := e.inAs(e.dpo, []string{"dpx.policy.read"}, func(ctx context.Context) error {
		missing, err := e.svc.MissingAttestations(ctx, p.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range missing {
			if m.UserID == e.emp {
				found = true
			}
		}
		if !found {
			t.Fatal("expected emp to be listed as missing before attesting")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var a1 govservice.Attestation
	if err := e.inAs(e.emp, policyEmpPerms, func(ctx context.Context) error {
		var err error
		a1, err = e.svc.AttestPolicy(ctx, p.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if a1.VersionNo != p.VersionNo || a1.UserID != e.emp {
		t.Fatalf("unexpected attestation: %+v", a1)
	}

	// emp no longer appears in the missing list.
	if err := e.inAs(e.dpo, []string{"dpx.policy.read"}, func(ctx context.Context) error {
		missing, err := e.svc.MissingAttestations(ctx, p.ID)
		if err != nil {
			return err
		}
		for _, m := range missing {
			if m.UserID == e.emp {
				t.Fatal("emp should no longer be listed as missing once attested")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Attesting again is a harmless no-op: same row, not a second one.
	var a2 govservice.Attestation
	if err := e.inAs(e.emp, policyEmpPerms, func(ctx context.Context) error {
		var err error
		a2, err = e.svc.AttestPolicy(ctx, p.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if a2.ID != a1.ID {
		t.Fatal("a repeat attestation of the same version should be the same row")
	}

	if err := e.inAs(e.dpo, []string{"dpx.policy.read"}, func(ctx context.Context) error {
		list, err := e.svc.ListAttestations(ctx, p.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("expected exactly 1 attestation row, got %d", len(list))
		}
		if list[0].UserName == "" {
			t.Fatal("expected the attester's own name to be resolved")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAttestPolicy_RequiresPermission is the permission gate: an employee without dpx.policy.execute
// cannot attest.
func TestAttestPolicy_RequiresPermission(t *testing.T) {
	e := policySetup(t, "dpx02forbid")
	docID := publishedPolicyDocument(t, e, "นโยบาย")
	var p govservice.Policy
	if err := e.inAs(e.dpo, policyDpoPerms, func(ctx context.Context) error {
		var err error
		p, err = e.svc.PublishPolicy(ctx, docID, "", true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.inAs(e.emp, nil, func(ctx context.Context) error {
		_, err := e.svc.AttestPolicy(ctx, p.ID)
		return err
	}); err == nil {
		t.Fatal("expected AttestPolicy to be refused without dpx.policy.execute")
	}
}

// TestMissingAttestations_TwoTenantIsolation proves RLS alone already scopes a cross-tenant policy id to
// not-found.
func TestMissingAttestations_TwoTenantIsolation(t *testing.T) {
	e1 := policySetup(t, "dpx02isoa")
	e2 := policySetup(t, "dpx02isob")
	doc1 := publishedPolicyDocument(t, e1, "นโยบาย A")
	var p1 govservice.Policy
	if err := e1.inAs(e1.dpo, policyDpoPerms, func(ctx context.Context) error {
		var err error
		p1, err = e1.svc.PublishPolicy(ctx, doc1, "", true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e2.inAs(e2.dpo, []string{"dpx.policy.read"}, func(ctx context.Context) error {
		_, err := e2.svc.GetPolicy(ctx, p1.ID)
		return err
	}); err == nil {
		t.Fatal("expected tenant B to never see tenant A's policy")
	}
}
