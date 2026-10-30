package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
)

var advisoryPerms = []string{"dpo.advisory.create", "dpo.advisory.read", "dpo.advisory.update", "dpo.kb.read", "dpo.kb.create"}

func (e env) inAdvisory(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.advisories`)
			_, err := tx.Exec(ctx, `DELETE FROM dpo.kb_articles`)
			return err
		})
	})
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: advisoryPerms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAdvisoryLifecycle_AnsweredQuestionIsSearchableInTheKnowledgeBase is DPO-07's own acceptance
// criterion directly: submitting, then answering, a question promotes it into the searchable knowledge
// base in the same step — nothing left for a separate "publish" call.
func TestAdvisoryLifecycle_AnsweredQuestionIsSearchableInTheKnowledgeBase(t *testing.T) {
	e := setup(t, "dpo07")

	var adv dposervice.Advisory
	e.inAdvisory(t, func(ctx context.Context) error {
		var err error
		adv, err = e.svc.SubmitAdvisory(ctx, nil, "Cookie consent on the marketing site", "Do we need a separate consent for analytics cookies?")
		if err != nil {
			t.Fatalf("SubmitAdvisory: %v", err)
		}
		if adv.Status != "open" || adv.AdvisoryNo == "" {
			t.Errorf("expected an open, numbered advisory, got %+v", adv)
		}
		return nil
	})

	e.inAdvisory(t, func(ctx context.Context) error {
		before, err := e.svc.SearchKbArticles(ctx, "", "analytics cookies")
		if err != nil {
			return err
		}
		if len(before) != 0 {
			t.Errorf("an unanswered advisory must not already be in the knowledge base, got %d", len(before))
		}
		return nil
	})

	e.inAdvisory(t, func(ctx context.Context) error {
		var err error
		adv, err = e.svc.AnswerAdvisory(ctx, adv.ID, "Yes — analytics cookies need their own consent category.", adv.RowVersion)
		if err != nil {
			t.Fatalf("AnswerAdvisory: %v", err)
		}
		if adv.Status != "answered" || adv.KbArticleID == nil {
			t.Errorf("expected answered with a kb_article_id, got %+v", adv)
		}
		return nil
	})

	e.inAdvisory(t, func(ctx context.Context) error {
		found, err := e.svc.SearchKbArticles(ctx, "", "analytics cookies")
		if err != nil {
			return err
		}
		if len(found) != 1 {
			t.Fatalf("expected exactly one matching kb article, got %d", len(found))
		}
		if !strings.Contains(found[0].Title, "Cookie consent") {
			t.Errorf("unexpected article: %+v", found[0])
		}
		if found[0].Body["answer"] != "Yes — analytics cookies need their own consent category." {
			t.Errorf("expected the real answer in the article body, got %+v", found[0].Body)
		}
		return nil
	})
}

// TestAnswerAdvisory_RefusesAlreadyAnsweredAndStaleVersion covers the guard rails.
func TestAnswerAdvisory_RefusesAlreadyAnsweredAndStaleVersion(t *testing.T) {
	e := setup(t, "dpo07guard")

	var adv dposervice.Advisory
	e.inAdvisory(t, func(ctx context.Context) error {
		var err error
		adv, err = e.svc.SubmitAdvisory(ctx, nil, "Subject", "Question")
		return err
	})
	e.inAdvisory(t, func(ctx context.Context) error {
		var err error
		adv, err = e.svc.AnswerAdvisory(ctx, adv.ID, "Answer", adv.RowVersion)
		return err
	})
	e.inAdvisory(t, func(ctx context.Context) error {
		if _, err := e.svc.AnswerAdvisory(ctx, adv.ID, "Second answer", adv.RowVersion); err != dposervice.ErrVersionMismatch {
			t.Errorf("answering an already-answered advisory: got %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestSubmitAdvisory_ValidatesOrgUnitAndRequiredFields.
func TestSubmitAdvisory_ValidatesOrgUnitAndRequiredFields(t *testing.T) {
	e := setup(t, "dpo07validate")
	e.inAdvisory(t, func(ctx context.Context) error {
		if _, err := e.svc.SubmitAdvisory(ctx, nil, "", "question"); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("empty subject: got %v, want ErrInvalid", err)
		}
		unknown := uuid.New()
		if _, err := e.svc.SubmitAdvisory(ctx, &unknown, "Subject", "Question"); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("unknown org_unit_id: got %v, want ErrInvalid", err)
		}
		return nil
	})

	var unit orgservice.OrgUnit
	e.inAdvisory(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT07", NameTh: "IT", UnitType: "department"})
		return err
	})
	e.inAdvisory(t, func(ctx context.Context) error {
		adv, err := e.svc.SubmitAdvisory(ctx, &unit.ID, "Subject", "Question")
		if err != nil {
			t.Fatalf("SubmitAdvisory with a real org unit: %v", err)
		}
		if adv.OrgUnitID == nil || *adv.OrgUnitID != unit.ID {
			t.Errorf("expected org_unit_id to round-trip, got %+v", adv)
		}
		return nil
	})
}

// TestListAdvisories_TwoTenantIsolation proves RLS scopes the list to the caller's own tenant.
func TestListAdvisories_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpo07isoa")
	b := setup(t, "dpo07isob")

	a.inAdvisory(t, func(ctx context.Context) error {
		_, err := a.svc.SubmitAdvisory(ctx, nil, "A's own question", "Question")
		return err
	})

	b.inAdvisory(t, func(ctx context.Context) error {
		list, err := b.svc.ListAdvisories(ctx, nil, "")
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B must not see tenant A's own advisories, got %d", len(list))
		}
		return nil
	})
}
