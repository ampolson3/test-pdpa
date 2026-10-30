package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/wiring"
)

// dpx03Setup wires govEnv's own svc (govSetup, retention_test.go) with a minimal dpiaservice.Service for
// Dpia — ScreenSubject only needs Forms, and the global "org_maturity" template migration 00093 seeded
// already covers this test with no local fixture form to create or publish, the exact same move DPO-02's
// own withDpiaScreening already established for its "dpo_requirement_check" template.
func dpx03Setup(t *testing.T, suffix string) govEnv {
	t.Helper()
	e := govSetup(t, suffix)
	formsSvc := wiring.Forms(nil, audit.New())
	e.svc.Dpia = &dpiaservice.Service{Forms: formsSvc, Audit: audit.New()}
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM gov.audit_findings`)
			_, _ = tx.Exec(ctx, `DELETE FROM gov.audits`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.answers`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessments`)
			return nil
		})
	})
	return e
}

var dpx03Perms = []string{"dpx.audit.read", "dpx.audit.create", "dpx.audit.update", "dpx.audit.delete", "dpx.audit.approve"}

func (e govEnv) dpx03In(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: dpx03Perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func allYesAnswers() forms.Answers {
	return forms.Answers{
		"gov_dpo_appointed": "yes", "gov_policy_published": "yes",
		"ropa_maintained": "yes", "ropa_reviewed_annually": "yes",
		"notices_published": "yes", "consent_records_kept": "yes",
		"dsar_channel_exists": "yes", "dsar_sla_tracked": "yes",
		"breach_plan_exists": "yes", "security_measures_reviewed": "yes",
	}
}

// TestRunAudit_ScoresPerDimensionAndOpensFindings is the acceptance criterion directly: the result shows
// a score per dimension, and every yes_no "no" answer opens a remediation finding.
func TestRunAudit_ScoresPerDimensionAndOpensFindings(t *testing.T) {
	e := dpx03Setup(t, "dpx03score")
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		return err
	})

	answers := allYesAnswers()
	answers["dsar_sla_tracked"] = "no"
	answers["security_measures_reviewed"] = "no"

	var a govservice.Audit
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: le.ID, Period: "2026-Q4", TemplateCode: "org_maturity", Answers: answers})
		return err
	})

	if len(a.SectionScores) != 5 {
		t.Fatalf("expected 5 dimension scores, got %d: %+v", len(a.SectionScores), a.SectionScores)
	}
	for _, s := range a.SectionScores {
		if s.Key == "dsar" && s.Score != 1 {
			t.Errorf("dsar dimension score: %+v, want 1 (one of two answered yes)", s)
		}
		if s.Key == "governance" && s.Score != 2 {
			t.Errorf("governance dimension score: %+v, want 2 (both answered yes)", s)
		}
	}
	if len(a.Findings) != 2 {
		t.Fatalf("expected 2 findings (one per failed question), got %d: %+v", len(a.Findings), a.Findings)
	}
	if a.OverallScore == nil || *a.OverallScore != 8 {
		t.Errorf("overall score: %v, want 8", a.OverallScore)
	}

	// GetAudit re-reads the same findings and section scores.
	e.dpx03In(t, func(ctx context.Context) error {
		got, err := e.svc.GetAudit(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(got.Findings) != 2 {
			t.Errorf("GetAudit findings: %+v", got.Findings)
		}
		return nil
	})
}

// TestRunAudit_FullyPassingHasNoFindings is the fully-passing complement.
func TestRunAudit_FullyPassingHasNoFindings(t *testing.T) {
	e := dpx03Setup(t, "dpx03pass")
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		return err
	})
	e.dpx03In(t, func(ctx context.Context) error {
		a, err := e.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: le.ID, Period: "2026-Q4", TemplateCode: "org_maturity", Answers: allYesAnswers()})
		if err != nil {
			return err
		}
		if len(a.Findings) != 0 {
			t.Errorf("expected no findings, got %+v", a.Findings)
		}
		if a.Band != "mature" {
			t.Errorf("band: %q, want mature", a.Band)
		}
		return nil
	})
}

// TestRunAudit_Validation covers the FK-visibility and bad-template cases.
func TestRunAudit_Validation(t *testing.T) {
	e := dpx03Setup(t, "dpx03valid")
	e.dpx03In(t, func(ctx context.Context) error {
		bogus := uuid.Must(uuid.NewV7())
		if _, err := e.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: bogus, Period: "2026-Q4", TemplateCode: "org_maturity", Answers: allYesAnswers()}); !errors.Is(err, govservice.ErrInvalid) {
			t.Errorf("unknown legal entity: %v, want ErrInvalid", err)
		}
		return nil
	})
	var le orgservice.LegalEntity
	e.dpx03In(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})
	e.dpx03In(t, func(ctx context.Context) error {
		if _, err := e.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: le.ID, Period: "2026-Q4", TemplateCode: "no_such_template", Answers: allYesAnswers()}); !errors.Is(err, govservice.ErrBadMaturityTemplate) {
			t.Errorf("unknown template: %v, want ErrBadMaturityTemplate", err)
		}
		return nil
	})
}

// TestAudit_TwoTenantIsolation is rule 1's own two-tenant isolation proof.
func TestAudit_TwoTenantIsolation(t *testing.T) {
	eA := dpx03Setup(t, "dpx03isoA")
	eB := dpx03Setup(t, "dpx03isoB")
	var auditA govservice.Audit
	eA.dpx03In(t, func(ctx context.Context) error {
		le, err := eA.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		auditA, err = eA.svc.RunAudit(ctx, govservice.RunAuditInput{LegalEntityID: le.ID, Period: "2026-Q4", TemplateCode: "org_maturity", Answers: allYesAnswers()})
		return err
	})
	eB.dpx03In(t, func(ctx context.Context) error {
		if _, err := eB.svc.GetAudit(ctx, auditA.ID); !errors.Is(err, govservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's audit: %v", err)
		}
		list, err := eB.svc.ListAudits(ctx, nil)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's audit list should be empty, got %+v", list)
		}
		return nil
	})
}
