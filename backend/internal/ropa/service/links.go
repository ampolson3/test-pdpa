// ROPA-19 (cross-module links): "เมื่อ RoPA เปลี่ยน แจ้งให้ทบทวนประกาศ DPIA แผนผัง และ Purpose ที่เกี่ยวข้อง" —
// the event side of this is already wired (internal/wiring/events.go subscribes ropa.activity_approved,
// published by ApproveActivity in versioning.go): PNG-10 already marks every linked notice for review and
// DPIA-13 already marks an activity's own DPIA round needs_review, both reused here unconditionally for
// this feature's own broader "any RoPA change" trigger, not just a breach link. This file adds the one
// genuinely new consumer, consent purposes, plus the read side (GetActivityLinks) backing the "linkage"
// tab on the activity page. Data Flow needs no code at all: dataflow.Graph (DFG-01) is always computed
// live off the current ropa/org/vendor rows, the same "nothing to go stale" reasoning DPIA-04's own
// description composition already established — there is no snapshot an edit could ever leave stale.
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ActivityLinks is the read side of ROPA-19's own "linkage" tab: every other module's record that already
// points back at this one activity.
type ActivityLinks struct {
	Notices         []LinkedNotice
	ConsentPurposes []LinkedConsentPurpose
}

// LinkedConsentPurpose is one of the activity's own purposes that cites a real consent.purposes row
// (ROPA-06's own consent_purpose_id link).
type LinkedConsentPurpose struct {
	PurposeID uuid.UUID // the ropa.activity_purposes row
	ConsentID uuid.UUID
	Code      string
	Status    string
}

// GetActivityLinks proves the activity is the caller's own (rule 1, via GetActivity) before reading either
// cross-module link — a cross-tenant activity id is ErrNotFound before this ever reaches Notice/Consent.
func (s *Service) GetActivityLinks(ctx context.Context, activityID uuid.UUID) (ActivityLinks, error) {
	if _, err := s.GetActivity(ctx, activityID); err != nil {
		return ActivityLinks{}, err
	}
	out := ActivityLinks{}
	if s.Notice != nil {
		notices, err := s.Notice.ListNoticesByActivity(ctx, activityID)
		if err != nil {
			return ActivityLinks{}, err
		}
		out.Notices = notices
	}
	purposes, err := s.ListActivityPurposes(ctx, activityID)
	if err != nil {
		return ActivityLinks{}, err
	}
	for _, p := range purposes {
		if p.ConsentPurposeID == nil {
			continue
		}
		cp, err := s.Consent.GetPurpose(ctx, *p.ConsentPurposeID)
		if err != nil {
			continue // a dangling/unreadable link is skipped, not fatal to the whole read
		}
		out.ConsentPurposes = append(out.ConsentPurposes, LinkedConsentPurpose{
			PurposeID: p.ID, ConsentID: cp.ID, Code: cp.Code, Status: cp.Status,
		})
	}
	return out, nil
}

// NotifyConsentPurposesForReview is ROPA-19's own "Purpose" consumer — called from the ropa.activity_approved
// subscriber (internal/wiring/events.go) for every real RoPA change: each of the activity's own purposes
// that cites a consent.purposes row gets one dpo.tasks review job, deduped the same way DPIA-08/DPA-12's
// own sibling consumers already dedupe (HasTaskForConsentPurpose) — a repeat approval of an
// already-flagged purpose opens nothing more. A nil Dpo (a context that never needs this, e.g. a test
// fixture) is a harmless no-op, the same convention Events/Collab already use.
func (s *Service) NotifyConsentPurposesForReview(ctx context.Context, activityID uuid.UUID, reason string) error {
	if s.Dpo == nil {
		return nil
	}
	purposes, err := s.ListActivityPurposes(ctx, activityID)
	if err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range purposes {
		if p.ConsentPurposeID == nil || seen[*p.ConsentPurposeID] {
			continue
		}
		seen[*p.ConsentPurposeID] = true
		cp, err := s.Consent.GetPurpose(ctx, *p.ConsentPurposeID)
		if err != nil {
			continue
		}
		has, err := s.Dpo.HasTaskForConsentPurpose(ctx, *p.ConsentPurposeID)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		title := fmt.Sprintf("Review consent purpose %s for RoPA change", cp.Code)
		if _, err := s.Dpo.OpenConsentReviewTask(ctx, *p.ConsentPurposeID, title, reason); err != nil {
			return err
		}
	}
	return nil
}
