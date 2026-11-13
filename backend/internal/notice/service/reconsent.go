package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/notify"
)

// consentPurposeIDs resolves every consent.purposes id reachable from this notice's linked RoPA processing
// activities (notice_activity_links -> ropa.activity_purposes.consent_purpose_id), deduplicated. A notice
// composed without any linked activity (PNG-03's template-group path, or a plain manual notice) simply has
// none — PNG-07's purpose-change job is only ever about a purpose the notice is actually tied to.
func (s *Service) consentPurposeIDs(ctx context.Context, noticeID uuid.UUID) ([]uuid.UUID, error) {
	n, err := s.GetNotice(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, activityID := range n.ActivityIDs {
		purposes, err := s.Ropa.ListActivityPurposes(ctx, activityID)
		if err != nil {
			return nil, err
		}
		for _, p := range purposes {
			if p.ConsentPurposeID == nil || seen[*p.ConsentPurposeID] {
				continue
			}
			seen[*p.ConsentPurposeID] = true
			out = append(out, *p.ConsentPurposeID)
		}
	}
	return out, nil
}

// openReconsentTasks is PNG-07's own acceptance criterion ("การเปลี่ยนวัตถุประสงค์สร้างงานขอความยินยอมใหม่
// อัตโนมัติ"): one dpo.tasks job per consent purpose this notice's RoPA activities reference, so a human DPO
// goes and authors/publishes the actual new consent text in CON (decisions.md Q-29) — never generated here.
func (s *Service) openReconsentTasks(ctx context.Context, n Notice, versionNo int32) error {
	if s.Dpo == nil || s.Consent == nil {
		return nil
	}
	ids, err := s.consentPurposeIDs(ctx, n.ID)
	if err != nil {
		return err
	}
	for _, purposeID := range ids {
		p, err := s.Consent.GetPurpose(ctx, purposeID)
		if err != nil {
			return err
		}
		name := p.Live.Name.Th
		if name == "" {
			name = p.Code
		}
		title := fmt.Sprintf("ขอความยินยอมใหม่: %s", name)
		description := fmt.Sprintf("ประกาศ \"%s\" เผยแพร่เวอร์ชัน %d ที่เปลี่ยนวัตถุประสงค์ \"%s\" — โปรดพิจารณาจัดทำข้อความยินยอมฉบับใหม่ (requires_reconsent) ใน /consent/purposes", n.Title, versionNo, name)
		if _, err := s.Dpo.OpenConsentTask(ctx, purposeID, title, description); err != nil {
			return err
		}
	}
	return nil
}

// alertMaterialChange is PNG-07's "แจ้งเจ้าของข้อมูล" half: decisions.md Q-29 routes it to role DPO (the same
// "default recipients until real routing exists" fallback BRE-07/PNG-04 use), since no generic, addressable
// data-subject audience or acknowledgement list (PNG-09, not built) exists yet to notify directly.
func (s *Service) alertMaterialChange(ctx context.Context, n Notice, versionNo int32) error {
	to, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	vars := map[string]any{"notice_title": n.Title, "version_no": versionNo}
	for _, u := range to {
		uid := u
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "notice.material_change", Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: "notice", EntityID: &n.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}
