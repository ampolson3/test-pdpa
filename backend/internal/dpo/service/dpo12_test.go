package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
)

var kbManagerPerms = []string{"dpo.kb.read", "dpo.kb.create", "dpo.kb.update", "dpo.kb.publish", "dpo.kb.delete"}

func (e env) inKb(t *testing.T, tenant dbtest.Tenant, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			_, _ = pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM dpo.kb_articles WHERE tenant_id IS NOT NULL`)
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	if err := pdb.WithTenantTx(context.Background(), e.app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func hasArticle(list []dposervice.KbArticle, id uuid.UUID) bool {
	for _, a := range list {
		if a.ID == id {
			return true
		}
	}
	return false
}

// TestKnowledgeBase_ThaiSearchAndLifecycle is DPO-12's acceptance criterion ("ค้นหาเอกสารภาษาไทยได้")
// plus the draft → published → archived lifecycle and versioning.
func TestKnowledgeBase_ThaiSearchAndLifecycle(t *testing.T) {
	e := setup(t, "dpo12")
	in := dposervice.KbInput{Category: "policy", Title: "นโยบายการจัดเก็บเอกสารพนักงาน", Source: "policy",
		Body: map[string]any{"text": "แฟ้มประวัติพนักงานต้องเก็บในตู้ที่ล็อกกุญแจเสมอ"}, Tags: []string{"ทรัพยากรบุคคล"}}
	var art dposervice.KbArticle

	e.inKb(t, e.tenant, kbManagerPerms, func(ctx context.Context) error {
		// the seeded global starter content is searchable in Thai from day one
		seeded, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "72 ชั่วโมง"})
		if err != nil {
			return err
		}
		if len(seeded) == 0 || !seeded[0].IsGlobal {
			t.Errorf("expected the seeded breach article for a Thai query, got %+v", seeded)
		}
		if _, err := e.svc.UpdateKbArticle(ctx, seeded[0].ID, in, seeded[0].RowVersion); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("editing a global article: %v, want ErrInvalid", err)
		}

		if art, err = e.svc.CreateKbArticle(ctx, in); err != nil {
			return err
		}
		if art.Status != "draft" || art.VersionNo != 1 {
			t.Errorf("new article = %+v, want draft v1", art)
		}
		readers, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "ล็อกกุญแจ"})
		if err != nil {
			return err
		}
		if hasArticle(readers, art.ID) {
			t.Error("a draft must not appear in the reader search")
		}
		all, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "ล็อกกุญแจ", IncludeAll: true})
		if err != nil || !hasArticle(all, art.ID) {
			t.Errorf("manager search should include the draft: %v", err)
		}
		if _, err := e.svc.SetKbArticleStatus(ctx, art.ID, "published", art.RowVersion+7); !errors.Is(err, dposervice.ErrVersionMismatch) {
			t.Errorf("stale publish: %v, want ErrVersionMismatch", err)
		}
		if art, err = e.svc.SetKbArticleStatus(ctx, art.ID, "published", art.RowVersion); err != nil {
			return err
		}
		// body text, title fragment and tag all match in Thai
		for _, q := range []string{"ล็อกกุญแจ", "เอกสารพนักงาน", "ทรัพยากรบุคคล"} {
			got, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: q})
			if err != nil {
				return err
			}
			if !hasArticle(got, art.ID) {
				t.Errorf("Thai query %q did not find the published article", q)
			}
		}
		if got, _ := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "ล็อกกุญแจ", Category: "faq"}); hasArticle(got, art.ID) {
			t.Error("category filter should exclude a policy article")
		}
		// editing a published article starts version 2
		in.Title = "นโยบายการจัดเก็บเอกสารพนักงาน (ปรับปรุง)"
		if art, err = e.svc.UpdateKbArticle(ctx, art.ID, in, art.RowVersion); err != nil {
			return err
		}
		if art.VersionNo != 2 {
			t.Errorf("version_no = %d, want 2", art.VersionNo)
		}
		if art, err = e.svc.SetKbArticleStatus(ctx, art.ID, "archived", art.RowVersion); err != nil {
			return err
		}
		if got, _ := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "ล็อกกุญแจ"}); hasArticle(got, art.ID) {
			t.Error("an archived article must leave the reader search")
		}
		bad := in
		bad.Source = "blog"
		if _, err := e.svc.CreateKbArticle(ctx, bad); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("bad source: %v, want ErrInvalid", err)
		}
		return nil
	})

	// a reader can neither author nor list drafts
	e.inKb(t, e.tenant, []string{"dpo.kb.read"}, func(ctx context.Context) error {
		if _, err := e.svc.CreateKbArticle(ctx, in); !errors.Is(err, dposervice.ErrForbidden) {
			t.Errorf("reader create: %v, want ErrForbidden", err)
		}
		if _, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{IncludeAll: true}); !errors.Is(err, dposervice.ErrForbidden) {
			t.Errorf("reader include_all: %v, want ErrForbidden", err)
		}
		if _, err := e.svc.GetKbArticle(ctx, art.ID); !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("reader get archived: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestKnowledgeBase_TwoTenantIsolation(t *testing.T) {
	e := setup(t, "dpo12iso")
	other := dbtest.SeedTenant(t, context.Background(), e.app, dbtest.PlatformPool(t), "dpo12isob")
	var art dposervice.KbArticle
	e.inKb(t, e.tenant, kbManagerPerms, func(ctx context.Context) error {
		var err error
		art, err = e.svc.CreateKbArticle(ctx, dposervice.KbInput{Category: "internal", Title: "บันทึกภายในของผู้เช่า A", Source: "internal", Body: map[string]any{"text": "ลับเฉพาะ"}})
		if err != nil {
			return err
		}
		art, err = e.svc.SetKbArticleStatus(ctx, art.ID, "published", art.RowVersion)
		return err
	})
	e.inKb(t, other, kbManagerPerms, func(ctx context.Context) error {
		got, err := e.svc.ListKbArticles(ctx, dposervice.KbFilter{Q: "ผู้เช่า A", IncludeAll: true})
		if err != nil {
			return err
		}
		if hasArticle(got, art.ID) {
			t.Error("another tenant must not see this tenant's article")
		}
		if _, err := e.svc.GetKbArticle(ctx, art.ID); !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("cross-tenant get: %v, want ErrNotFound", err)
		}
		return nil
	})
}
