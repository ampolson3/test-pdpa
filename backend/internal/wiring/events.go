package wiring

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/platform/events"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// Events registers this codebase's in-process event subscribers (PLT-11) on reg — call once in cmd/worker,
// after the modules it wires (ropaSvc, dpiaSvc, noticeSvc) are built. ROPA-10 logs a DSAR rejection against
// every processing activity the event names (dsar.rejected, published by DSAR-11's Transition), so the
// rejection shows up in that activity's RoPA automatically without dsar ever writing to the ropa schema
// itself (rule 9). DPIA-13 marks an activity's latest approved DPIA round needs_review when breach links an
// incident to it (breach.activity_linked, published by breach's own LinkActivity) — breach never writes to
// the assess schema directly either. PNG-10 marks every notice composed from an activity for review once
// that activity is (re-)approved (ropa.activity_approved, published by ropa's own ApproveActivity) — ropa
// never writes to the notice schema directly either.
func Events(reg *events.Registry, ropaSvc *ropaservice.Service, dpiaSvc *dpiaservice.Service, noticeSvc *noticeservice.Service, riskSvc *riskservice.Service) {
	reg.Subscribe("dsar.rejected", func(ctx context.Context, e events.Event) error {
		requestID := e.AggregateID // the dsar.requests row itself (rule 3: request_ref in Data is the human-facing number, not the id)
		reasonCode, _ := e.Data["reason_code"].(string)
		refs, _ := e.Data["activity_refs"].([]any)
		for _, ref := range refs {
			s, ok := ref.(string)
			if !ok {
				continue
			}
			activityID, err := uuid.Parse(s)
			if err != nil {
				continue
			}
			if err := ropaSvc.RecordRejection(ctx, activityID, requestID, reasonCode, e.OccurredAt); err != nil {
				return err
			}
		}
		return nil
	})

	reg.Subscribe("breach.activity_linked", func(ctx context.Context, e events.Event) error {
		ref, _ := e.Data["activity_ref"].(string)
		activityID, err := uuid.Parse(ref)
		if err != nil {
			return nil
		}
		incidentRef, _ := e.Data["incident_ref"].(string)
		reason := fmt.Sprintf("Linked to breach incident %s", incidentRef)
		return dpiaSvc.MarkActivityAssessmentForReview(ctx, activityID, reason)
	})

	reg.Subscribe("ropa.activity_approved", func(ctx context.Context, e events.Event) error {
		ref, _ := e.Data["activity_id"].(string)
		activityID, err := uuid.Parse(ref)
		if err != nil {
			return nil
		}
		if err := noticeSvc.MarkNoticesForReviewByActivity(ctx, activityID); err != nil {
			return err
		}
		// ROPA-19's own broader trigger: every real RoPA change, not just a breach link, marks the
		// activity's own latest-approved DPIA round needs_review (a harmless no-op with none) and opens
		// a review task per linked consent purpose (also a no-op with no Dpo wired, e.g. cmd/api).
		if err := dpiaSvc.MarkActivityAssessmentForReview(ctx, activityID, "RoPA activity changed"); err != nil {
			return err
		}
		if err := ropaSvc.NotifyConsentPurposesForReview(ctx, activityID, "RoPA activity changed"); err != nil {
			return err
		}
		// RRA-12: recompute the activity's own risk score from its current RoPA data (its own acceptance
		// criterion — within 1 minute of a RoPA change — is satisfied trivially, since this runs
		// synchronously in the same event-dispatch call the approval itself commits in).
		return riskSvc.RescoreOnActivityChange(ctx, activityID)
	})
}
