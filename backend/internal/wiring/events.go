package wiring

import (
	"context"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/events"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// Events registers this codebase's in-process event subscribers (PLT-11) on reg — call once in cmd/worker,
// after the modules it wires (ropaSvc) are built. ROPA-10 is the first (and, for now, only) subscriber: it
// logs a DSAR rejection against every processing activity the event names (dsar.rejected, published by
// DSAR-11's Transition), so the rejection shows up in that activity's RoPA automatically without dsar ever
// writing to the ropa schema itself (rule 9).
func Events(reg *events.Registry, ropaSvc *ropaservice.Service) {
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
}
