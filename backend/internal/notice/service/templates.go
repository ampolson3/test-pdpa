package service

import (
	"context"
	"encoding/json"
	"fmt"

	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs/render"
)

// TemplateGroups are PNG-03's 8 fixed data-subject groups (module doc: ลูกค้า พนักงาน ผู้สมัครงาน คู่ค้า
// ผู้มาติดต่อ CCTV ผู้ถือหุ้น สมาชิก). Kept as a Go constant list, not a lookup table — the code itself is the
// whole label ("customer", "employee", ...); display names come from the UI's own i18n keys (rule 12), not
// this catalog, since notice.wizard_templates has no display-name column of its own.
var TemplateGroups = []string{"customer", "employee", "job_applicant", "vendor", "visitor", "cctv", "shareholder", "member"}

// ListTemplateGroups returns the groups that actually have a seeded (or tenant-authored) starter template,
// for the wizard's picker — a subset check against TemplateGroups rather than trusting the table blindly, in
// case a future tenant override adds a group this code doesn't know about yet (it's still listed, just
// unlabeled by the UI until a translation exists).
func (s *Service) ListTemplateGroups(ctx context.Context) ([]string, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	return q.ListWizardTemplateGroups(ctx)
}

// IndustryTemplates are PNG-11's 6 fixed industries (module doc: ค้าปลีก โรงพยาบาล การเงิน/ประกัน โรงแรม
// การศึกษา ภาครัฐ) -- the same "fixed Go list, display names come from UI i18n" convention TemplateGroups
// already uses, since notice.wizard_templates has no display-name column either.
var IndustryTemplates = []string{"retail", "hospital", "finance_insurance", "hotel", "education", "government"}

// ListIndustryTemplates returns the industries that actually have a seeded (or tenant-authored) starter
// template, for the wizard's industry filter -- the same ListTemplateGroups shape, on the other axis of the
// same notice.wizard_templates table (industry IS NOT NULL instead of IS NULL).
func (s *Service) ListIndustryTemplates(ctx context.Context) ([]string, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	return q.ListIndustryTemplates(ctx)
}

// templateContent loads the DRAFT starter content for one group (PNG-03's acceptance criterion: picking a
// group produces an immediate draft) — both languages, straight from platform.templates' stored ProseMirror
// tree (the seed migration's shape matches compose.go's own docNode output, so it needs no conversion, only
// JSON decoding).
func (s *Service) templateContent(ctx context.Context, group string) (render.Content, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.GetWizardTemplateContent(ctx, group)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: template_group", ErrInvalid)
	}
	out := make(render.Content, len(rows))
	for _, r := range rows {
		var node render.Node
		if err := json.Unmarshal(r.Content, &node); err != nil {
			return nil, err
		}
		out[r.Language] = node
	}
	return out, nil
}
