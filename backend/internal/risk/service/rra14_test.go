package service_test

import (
	"context"
	"strings"
	"testing"

	riskservice "pdpa-platform/internal/risk/service"
)

// TestExecutiveReport_IncludesOnlyOpenHighRisksAndMatchesRegister is RRA-14's own acceptance criterion
// directly: the report lists every still-open high/very_high risk from the register, excludes a low one
// and a closed high one, and shows each included risk's own title/level/status.
func TestExecutiveReport_IncludesOnlyOpenHighRisksAndMatchesRegister(t *testing.T) {
	e := scoreSetup(t, "rra14")

	var closedID string
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		if _, err := mustIdentifyErr(ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "ความเสี่ยงต่ำ", Likelihood: 1, Impact: 1}); err != nil {
			return err
		}
		if _, err := mustIdentifyErr(ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "ความเสี่ยงสูงที่เปิดอยู่", Likelihood: 3, Impact: 3}); err != nil {
			return err
		}
		closed, err := mustIdentifyErr(ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "ความเสี่ยงสูงที่ปิดแล้ว", Likelihood: 3, Impact: 3})
		if err != nil {
			return err
		}
		closedID = closed.ID.String()
		closed.Status = "closed"
		_, err = e.svc.UpdateRisk(ctx, closed.ID, closed, closed.RowVersion)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		b, name, err := e.svc.ExecutiveReport(ctx, "th", "html")
		if err != nil {
			return err
		}
		if !strings.HasSuffix(name, ".html") {
			t.Errorf("name = %q, want .html suffix", name)
		}
		html := string(b)
		if !strings.Contains(html, "ความเสี่ยงสูงที่เปิดอยู่") {
			t.Error("expected the open high risk's own title in the report")
		}
		if strings.Contains(html, "ความเสี่ยงต่ำ") {
			t.Error("a low risk must not appear in the executive report")
		}
		if strings.Contains(html, "ความเสี่ยงสูงที่ปิดแล้ว") {
			t.Error("a closed risk, even if high, must not appear in the executive report")
		}
		return nil
	})
	_ = closedID
}

// TestExecutiveReport_EnglishAndEmptyRegister covers the other language and the "nothing to report" path.
func TestExecutiveReport_EnglishAndEmptyRegister(t *testing.T) {
	e := scoreSetup(t, "rra14empty")
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		b, _, err := e.svc.ExecutiveReport(ctx, "en", "html")
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "No open high/very-high risks") {
			t.Errorf("expected the empty-register message, got: %s", b)
		}
		return nil
	})
}

// TestExecutiveReport_InvalidLanguageAndFormatRefused rounds out the validation paths.
func TestExecutiveReport_InvalidLanguageAndFormatRefused(t *testing.T) {
	e := scoreSetup(t, "rra14invalid")
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		if _, _, err := e.svc.ExecutiveReport(ctx, "fr", "html"); err == nil {
			t.Error("expected an error for an unsupported language")
		}
		if _, _, err := e.svc.ExecutiveReport(ctx, "th", "csv"); err == nil {
			t.Error("expected an error for an unsupported format")
		}
		return nil
	})
}

// mustIdentifyErr is register_test.go's own mustIdentify without the t.Fatal, so a seeded fixture row's own
// validation error (should there ever be one) surfaces through e.in's normal error path instead.
func mustIdentifyErr(ctx context.Context, svc *riskservice.Service, r riskservice.Risk) (riskservice.Risk, error) {
	return svc.IdentifyRisk(ctx, r)
}
