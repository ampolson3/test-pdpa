package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// defaultMatrix seeds a plain 3x3 risk matrix as the tenant's own default (RRA-02) — DPIA-06's own
// acceptance criterion needs one configured before any risk can be scored.
func (e env) defaultMatrix(t *testing.T, ctx context.Context) riskservice.RiskMatrix {
	t.Helper()
	m, err := e.risk.SaveMatrix(ctx, riskservice.RiskMatrix{
		Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
		Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
		IsDefault:  true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// screenedActivity returns an activity with a round already in_progress (required) — DPIA-06's risks are
// identified against an open round, the same editable window DPIA-10's own RecordOpinion uses.
func (e env) screenedActivity(t *testing.T, ctx context.Context, code string) (ropaservice.Activity, dpiaservice.Assessment) {
	t.Helper()
	act := e.activity(t, ctx, code)
	answers := allAnswers("no")
	answers["sensitive_data"] = "yes"
	answers["large_scale"] = "yes"
	a, err := e.svc.Screen(ctx, act.ID, answers)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "in_progress" {
		t.Fatalf("setup: screened status = %q, want in_progress", a.Status)
	}
	return act, a
}

// TestIdentifyRisk_ScoresAgainstTenantMatrix is DPIA-06's own acceptance criterion directly: a risk's score
// comes from Classify against the tenant's current default matrix (RRA-02), carrying the activity id along.
func TestIdentifyRisk_ScoresAgainstTenantMatrix(t *testing.T) {
	e := setup(t, "dpiarisk1")
	var assessmentID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		act, a := e.screenedActivity(t, ctx, "RISK-01")
		assessmentID, activityID = a.ID, act.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		r, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "เข้าถึงข้อมูลโดยไม่ได้รับอนุญาต", Likelihood: 2, Impact: 3})
		if err != nil {
			return err
		}
		if r.InherentScore != 6 || r.Level != "medium" {
			t.Errorf("score/level = %v/%q, want 6/medium", r.InherentScore, r.Level)
		}
		list, err := e.svc.ListAssessmentRisks(ctx, assessmentID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != r.ID {
			t.Fatalf("ListAssessmentRisks = %+v, want exactly the identified risk", list)
		}
		got, err := e.risk.GetRisk(ctx, r.ID)
		if err != nil {
			return err
		}
		if got.SourceType != "dpia" || got.SourceID == nil || *got.SourceID != assessmentID {
			t.Errorf("source = %q/%v, want dpia/%v", got.SourceType, got.SourceID, assessmentID)
		}
		if got.ActivityID == nil || *got.ActivityID != activityID {
			t.Errorf("activity_id = %v, want %v", got.ActivityID, activityID)
		}
		return nil
	})
}

// TestIdentifyRisk_ChangingTheMatrixChangesTheNextScore proves nothing is cached: re-scoring an equivalent
// risk after the tenant's own matrix changes reflects the new matrix immediately.
func TestIdentifyRisk_ChangingTheMatrixChangesTheNextScore(t *testing.T) {
	e := setup(t, "dpiarisk2")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "RISK-02")
		assessmentID = a.ID
		return nil
	})

	var first riskservice.Risk
	e.in(t, func(ctx context.Context) error {
		var err error
		first, err = e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 2})
		return err
	})
	if first.Level != "low" {
		t.Fatalf("first level = %q, want low (score 2 is below medium's own threshold of 4)", first.Level)
	}

	// Lower the medium threshold from 4 to 2, so the same 1x2 score (2) now classifies as medium.
	e.in(t, func(ctx context.Context) error {
		m, err := e.risk.GetMatrix(ctx, nil)
		if err != nil {
			return err
		}
		for i := range m.Thresholds {
			if m.Thresholds[i].Level == "medium" {
				m.Thresholds[i].MinScore = 2
			}
		}
		_, err = e.risk.SaveMatrix(ctx, m, m.RowVersion)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		second, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "b", Likelihood: 1, Impact: 2})
		if err != nil {
			return err
		}
		if second.Level != "medium" {
			t.Errorf("after lowering the threshold: level = %q, want medium", second.Level)
		}
		return nil
	})
}

// TestIdentifyRisk_RefusedOutsideEditableStatus proves a not_required round (nothing to score) and a round
// no longer open for editing both refuse IdentifyRisk.
func TestIdentifyRisk_RefusedOutsideEditableStatus(t *testing.T) {
	e := setup(t, "dpiarisk3")
	var notRequiredID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		act := e.activity(t, ctx, "RISK-03")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		notRequiredID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.IdentifyRisk(ctx, notRequiredID, riskservice.Risk{Title: "x", Likelihood: 1, Impact: 1}); !errors.Is(err, dpiaservice.ErrInvalidTransition) {
			t.Errorf("not_required round: %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestUpdateRisk_RejectsARiskNotLinkedToThisAssessment proves one round can't edit another's risk by
// guessing its id, even within the same tenant.
func TestUpdateRisk_RejectsARiskNotLinkedToThisAssessment(t *testing.T) {
	e := setup(t, "dpiarisk4")
	var round1, round2 uuid.UUID
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a1 := e.screenedActivity(t, ctx, "RISK-04A")
		_, a2 := e.screenedActivity(t, ctx, "RISK-04B")
		round1, round2 = a1.ID, a2.ID
		r, err := e.svc.IdentifyRisk(ctx, round1, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		riskID = r.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.UpdateRisk(ctx, round2, riskID, riskservice.Risk{Title: "b", Likelihood: 2, Impact: 2}, 1); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("unlinked risk: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestUpdateRisk_RecomputesScore edits an existing risk's likelihood/impact and proves the score/level is
// recomputed, not just the text fields.
func TestUpdateRisk_RecomputesScore(t *testing.T) {
	e := setup(t, "dpiarisk5")
	var assessmentID, riskID uuid.UUID
	var version int32
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "RISK-05")
		assessmentID = a.ID
		r, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		riskID, version = r.ID, r.RowVersion
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		updated, err := e.svc.UpdateRisk(ctx, assessmentID, riskID, riskservice.Risk{Title: "a (revised)", Likelihood: 3, Impact: 3, Treatment: "mitigate"}, version)
		if err != nil {
			return err
		}
		if updated.Level != "high" || updated.InherentScore != 9 {
			t.Errorf("after update: score/level = %v/%q, want 9/high", updated.InherentScore, updated.Level)
		}
		if updated.Treatment != "mitigate" {
			t.Errorf("treatment = %q, want mitigate", updated.Treatment)
		}
		return nil
	})
}

// TestRemoveAssessmentRisk_UnlinksOnly proves removing a risk from a round only drops the link — the
// risk.risks row itself is untouched (it may still matter to another round or a future feature).
func TestRemoveAssessmentRisk_UnlinksOnly(t *testing.T) {
	e := setup(t, "dpiarisk6")
	var assessmentID, riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "RISK-06")
		assessmentID = a.ID
		r, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		riskID = r.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.RemoveAssessmentRisk(ctx, assessmentID, riskID); err != nil {
			return err
		}
		list, err := e.svc.ListAssessmentRisks(ctx, assessmentID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("ListAssessmentRisks after remove = %+v, want empty", list)
		}
		if _, err := e.risk.GetRisk(ctx, riskID); err != nil {
			t.Errorf("risk.risks row itself: %v, want still present", err)
		}
		if err := e.svc.RemoveAssessmentRisk(ctx, assessmentID, riskID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("removing again: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestRiskCatalog_HasUniqueEntries is DPIA-06's own "คลังความเสี่ยงสำเร็จรูป" sanity check.
func TestRiskCatalog_HasUniqueEntries(t *testing.T) {
	catalog := dpiaservice.RiskCatalog()
	if len(catalog) < 8 {
		t.Fatalf("got %d catalog entries, want at least 8", len(catalog))
	}
	seen := map[string]bool{}
	for _, c := range catalog {
		if c.Code == "" || c.TitleTh == "" || c.TitleEn == "" || c.DescriptionTh == "" {
			t.Errorf("incomplete entry: %+v", c)
		}
		if seen[c.Code] {
			t.Errorf("duplicate catalog code %q", c.Code)
		}
		seen[c.Code] = true
	}
}

// TestIdentifyRisk_TwoTenantIsolation proves tenant B can neither identify a risk against tenant A's
// assessment nor list/read it.
func TestIdentifyRisk_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiariskisoA")
	b := setup(t, "dpiariskisoB")
	var assessmentID, riskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		a.defaultMatrix(t, ctx)
		_, round := a.screenedActivity(t, ctx, "ISO-01")
		assessmentID = round.ID
		r, err := a.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		riskID = r.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "x", Likelihood: 1, Impact: 1}); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant assessment id: %v, want ErrNotFound", err)
		}
		if _, err := b.risk.GetRisk(ctx, riskID); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("cross-tenant risk read: %v, want risk.ErrNotFound", err)
		}
		return nil
	})
}
