package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
)

// SuggestedPurpose is one RTG-06 recommendation for a purpose/lawful-basis pair, decorated with the
// lawful basis's own org.lawful_bases.section_ref — a real article citation (ม.24/26), not the template's
// own generic rationale text (which only ever names "มาตรา 24/26" as a pair, never the specific one).
type SuggestedPurpose struct {
	PurposeText       string
	LawfulBasisCode   string
	LawfulBasisNameTh string
	Rationale         string
}

// SuggestedData is one recommended data-category/subject-type pair, cited to ม.26 when the category is
// sensitive (the same ORG-07 is_sensitive flag ROPA-01/03's own completeness checks already read) or
// ม.39(2) — the RoPA's own "ประเภทข้อมูลส่วนบุคคล" item — otherwise.
type SuggestedData struct {
	DataCategoryCode string
	DataCategoryName string
	SubjectTypeCode  string
	SubjectTypeName  string
	IsSensitive      bool
	Source           string
	Rationale        string
}

// SuggestedRetention is one recommended retention rule, cited to ม.39(3) and carrying the template's own
// plain-language basis text (`retention_basis_th`) as the human-readable half of the rationale.
type SuggestedRetention struct {
	RetentionMonths *int
	RetentionBasis  string
	TriggerEvent    string
	DisposalMethod  string
	Rationale       string
}

// TemplateSuggestions is RTG-06's own read: one RTG-01 standard-activity template's purposes/data/
// retention items, each decorated with a rationale that cites a real legal article, for the OWNER to
// review one at a time. Nothing here is written to any activity — ApplySuggestedItems is the only path
// that persists any of it, and only the items the caller actually selects (the acceptance criterion:
// suggestions are never saved until the user confirms them).
type TemplateSuggestions struct {
	TemplateID   uuid.UUID
	TemplateCode string
	TemplateName string
	Purposes     []SuggestedPurpose
	Data         []SuggestedData
	Retention    []SuggestedRetention
}

func masterByCode(items []orgservice.MasterItem, code string) orgservice.MasterItem {
	for _, it := range items {
		if it.Code == code {
			return it
		}
	}
	return orgservice.MasterItem{Code: code}
}

// Suggest resolves one RTG-01 template into RTG-06's own per-item recommendations. The template itself
// (`ropa.activity_templates`) is unchanged by RTG-01/ROPA-05/RTG-04 — this only reads it and the tenant's
// own visible org.* master data (rule 1) to build each item's rationale.
func (s *Service) Suggest(ctx context.Context, templateID uuid.UUID) (TemplateSuggestions, error) {
	tpl, d, err := s.loadTemplateDefaults(ctx, templateID)
	if err != nil {
		return TemplateSuggestions{}, err
	}

	bases, err := s.Org.ListMaster(ctx, orgservice.KindLawfulBases)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	cats, err := s.Org.ListMaster(ctx, orgservice.KindDataCategories)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	subs, err := s.Org.ListMaster(ctx, orgservice.KindSubjectTypes)
	if err != nil {
		return TemplateSuggestions{}, err
	}

	out := TemplateSuggestions{TemplateID: templateID, TemplateCode: tpl.Code, TemplateName: tpl.NameTh}
	for _, p := range d.Purposes {
		basis := masterByCode(bases, p.LawfulBasisCode)
		ref := basis.SectionRef
		if ref == "" {
			ref = "ม.24"
		}
		out.Purposes = append(out.Purposes, SuggestedPurpose{
			PurposeText: p.PurposeTextTh, LawfulBasisCode: p.LawfulBasisCode, LawfulBasisNameTh: basis.NameTh,
			Rationale: fmt.Sprintf("ฐานทางกฎหมาย \"%s\" ตาม%s — เลือกตามลักษณะความสัมพันธ์และวัตถุประสงค์ของกิจกรรมนี้", basis.NameTh, ref),
		})
	}
	for _, item := range d.Data {
		cat := masterByCode(cats, item.DataCategoryCode)
		sub := masterByCode(subs, item.SubjectTypeCode)
		ref := "ม.39(2)"
		sensitiveNote := ""
		if cat.IsSensitive {
			ref = "ม.26"
			sensitiveNote = " (ข้อมูลอ่อนไหวตามมาตรา 26)"
		}
		out.Data = append(out.Data, SuggestedData{
			DataCategoryCode: item.DataCategoryCode, DataCategoryName: cat.NameTh, SubjectTypeCode: item.SubjectTypeCode,
			SubjectTypeName: sub.NameTh, IsSensitive: cat.IsSensitive, Source: item.Source,
			Rationale: fmt.Sprintf("ประเภทข้อมูล \"%s\" ของกลุ่มเจ้าของข้อมูล \"%s\" ตาม%s%s", cat.NameTh, sub.NameTh, ref, sensitiveNote),
		})
	}
	for _, r := range d.Retention {
		out.Retention = append(out.Retention, SuggestedRetention{
			RetentionMonths: r.RetentionMonths, RetentionBasis: r.RetentionBasisTh, TriggerEvent: r.TriggerEventTh,
			DisposalMethod: r.DisposalMethod,
			Rationale:      fmt.Sprintf("ระยะเวลาเก็บรักษาตามมาตรา 39(3) — %s", r.RetentionBasisTh),
		})
	}
	return out, nil
}

// ApplySuggestedItemsInput selects, by index into the matching Suggest() slice, exactly which
// recommendations the user confirmed — anything not listed here is simply never written.
type ApplySuggestedItemsInput struct {
	TemplateID uuid.UUID
	Purposes   []int
	Data       []int
	Retention  []int
}

// ApplySuggestedItems writes only the suggested items the caller selected onto an existing activity,
// through the exact same AddActivityPurpose/AddActivityData/AddRetentionRule calls ROPA-03's own manual
// entry and ROPA-05's bulk template-copy already use — a selectively-confirmed item is otherwise
// indistinguishable from one a person typed in by hand. GetActivity proves the activity is the caller's
// own (rule 1) before anything is written.
func (s *Service) ApplySuggestedItems(ctx context.Context, activityID uuid.UUID, in ApplySuggestedItemsInput) (Activity, error) {
	if _, err := s.GetActivity(ctx, activityID); err != nil {
		return Activity{}, err
	}
	_, d, err := s.loadTemplateDefaults(ctx, in.TemplateID)
	if err != nil {
		return Activity{}, err
	}

	for _, idx := range in.Purposes {
		if idx < 0 || idx >= len(d.Purposes) {
			return Activity{}, fmt.Errorf("%w: purposes[%d]", ErrInvalid, idx)
		}
		p := d.Purposes[idx]
		if _, err := s.AddActivityPurpose(ctx, ActivityPurpose{ActivityID: activityID, PurposeText: p.PurposeTextTh, LawfulBasisCode: p.LawfulBasisCode}); err != nil {
			return Activity{}, fmt.Errorf("suggested purpose %d: %w", idx, err)
		}
	}
	for _, idx := range in.Data {
		if idx < 0 || idx >= len(d.Data) {
			return Activity{}, fmt.Errorf("%w: data[%d]", ErrInvalid, idx)
		}
		item := d.Data[idx]
		catID, err := s.masterIDByCode(ctx, orgservice.KindDataCategories, item.DataCategoryCode)
		if err != nil {
			return Activity{}, err
		}
		subID, err := s.masterIDByCode(ctx, orgservice.KindSubjectTypes, item.SubjectTypeCode)
		if err != nil {
			return Activity{}, err
		}
		if _, err := s.AddActivityData(ctx, ActivityData{ActivityID: activityID, DataCategoryID: catID, SubjectTypeID: subID, Source: item.Source}); err != nil {
			return Activity{}, fmt.Errorf("suggested data %d: %w", idx, err)
		}
	}
	for _, idx := range in.Retention {
		if idx < 0 || idx >= len(d.Retention) {
			return Activity{}, fmt.Errorf("%w: retention[%d]", ErrInvalid, idx)
		}
		r := d.Retention[idx]
		if _, err := s.AddRetentionRule(ctx, RetentionRule{ActivityID: activityID, RetentionMonths: r.RetentionMonths,
			RetentionBasis: r.RetentionBasisTh, TriggerEvent: r.TriggerEventTh, DisposalMethod: r.DisposalMethod}); err != nil {
			return Activity{}, fmt.Errorf("suggested retention %d: %w", idx, err)
		}
	}

	final, err := s.GetActivity(ctx, activityID)
	if err != nil {
		return Activity{}, err
	}
	return final, s.audit(ctx, "ropa.activity.apply_suggested_items", activityID, nil,
		map[string]any{"activity_template_id": in.TemplateID, "purposes": in.Purposes, "data": in.Data, "retention": in.Retention})
}
