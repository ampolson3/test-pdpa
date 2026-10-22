package wiring

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/platform/events"
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
func Events(reg *events.Registry, ropaSvc *ropaservice.Service, dpiaSvc *dpiaservice.Service, noticeSvc *noticeservice.Service) {
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
		return noticeSvc.MarkNoticesForReviewByActivity(ctx, activityID)
	})
}
