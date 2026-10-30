package service

import (
	"context"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
)

// ActivityDescription is DPIA-04's own acceptance criterion: everything ม.39 already asks for — purposes,
// data categories, data subject groups, recipients, cross-border transfers and retention — pulled straight
// from the assessment's linked RoPA activity. It is computed live on every call, never persisted, so it
// always matches the activity's current data ("sync เมื่อ RoPA เปลี่ยน" needs no separate sync step this
// way — there is nothing to go stale).
type ActivityDescription struct {
	ActivityID          uuid.UUID
	ActivityCode        string
	ActivityName        string
	ActivityDescription string
	Role                string
	Purposes            []DescriptionPurpose
	Data                []DescriptionData
	Recipients          []DescriptionRecipient
	Transfers           []DescriptionTransfer
	Retention           []DescriptionRetention
}

type DescriptionPurpose struct {
	Text               string
	LawfulBasisCode    string
	LawfulBasisNameTh  string
	LawfulBasisNameEn  string
}

type DescriptionData struct {
	CategoryNameTh    string
	CategoryNameEn    string
	SubjectTypeNameTh string
	SubjectTypeNameEn string
	IsSensitive       bool
	Source            string
}

type DescriptionRecipient struct {
	PartyNameTh     string
	PartyNameEn     string
	RecipientRole   string
	DisclosureBasis string
}

type DescriptionTransfer struct {
	CountryNameTh string
	CountryNameEn string
	TransferBasis string
	Safeguards    string
}

type DescriptionRetention struct {
	CategoryNameTh  string
	CategoryNameEn  string
	RetentionMonths *int
	RetentionBasis  string
	TriggerEvent    string
	DisposalMethod  string
}

// ActivityDescription resolves an assessment to its RoPA activity's own ม.39 fields (rule 9: dpia never
// reads ropa's schema directly, only its already-exported service — the exact methods ROPA-03/08's own
// completeness checks already call). The activity itself is resolved through Ropa.GetActivity, which is
// already RLS-scoped, so a cross-tenant assessment id fails at GetAssessment before this is ever reached.
func (s *Service) ActivityDescription(ctx context.Context, assessmentID uuid.UUID) (ActivityDescription, error) {
	a, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return ActivityDescription{}, err
	}
	act, err := s.Ropa.GetActivity(ctx, a.ActivityID)
	if err != nil {
		return ActivityDescription{}, err
	}

	out := ActivityDescription{
		ActivityID: act.ID, ActivityCode: act.Code, ActivityName: act.Name,
		ActivityDescription: act.Description, Role: act.Role,
	}

	purposes, err := s.Ropa.ListActivityPurposes(ctx, act.ID)
	if err != nil {
		return ActivityDescription{}, err
	}
	for _, p := range purposes {
		basis, err := s.lawfulBasisNames(ctx, p.LawfulBasisCode)
		if err != nil {
			return ActivityDescription{}, err
		}
		out.Purposes = append(out.Purposes, DescriptionPurpose{
			Text: p.PurposeText, LawfulBasisCode: p.LawfulBasisCode,
			LawfulBasisNameTh: basis.NameTh, LawfulBasisNameEn: basis.NameEn,
		})
	}

	data, err := s.Ropa.ListActivityData(ctx, act.ID)
	if err != nil {
		return ActivityDescription{}, err
	}
	for _, d := range data {
		cat, err := s.Org.GetMaster(ctx, orgservice.KindDataCategories, d.DataCategoryID)
		if err != nil {
			return ActivityDescription{}, err
		}
		subj, err := s.Org.GetMaster(ctx, orgservice.KindSubjectTypes, d.SubjectTypeID)
		if err != nil {
			return ActivityDescription{}, err
		}
		out.Data = append(out.Data, DescriptionData{
			CategoryNameTh: cat.NameTh, CategoryNameEn: cat.NameEn,
			SubjectTypeNameTh: subj.NameTh, SubjectTypeNameEn: subj.NameEn,
			IsSensitive: d.IsSensitive, Source: d.Source,
		})
	}

	recipients, err := s.Ropa.ListActivityRecipients(ctx, act.ID)
	if err != nil {
		return ActivityDescription{}, err
	}
	for _, r := range recipients {
		party, err := s.Org.GetExternalParty(ctx, r.PartyID)
		if err != nil {
			return ActivityDescription{}, err
		}
		out.Recipients = append(out.Recipients, DescriptionRecipient{
			PartyNameTh: party.NameTh, PartyNameEn: party.NameEn,
			RecipientRole: r.RecipientRole, DisclosureBasis: r.DisclosureBasis,
		})
	}

	transfers, err := s.Ropa.ListActivityTransfers(ctx, act.ID)
	if err != nil {
		return ActivityDescription{}, err
	}
	countries, err := s.countryNames(ctx)
	if err != nil {
		return ActivityDescription{}, err
	}
	for _, tr := range transfers {
		c := countries[tr.CountryCode]
		out.Transfers = append(out.Transfers, DescriptionTransfer{
			CountryNameTh: c.NameTh, CountryNameEn: c.NameEn,
			TransferBasis: tr.TransferBasis, Safeguards: tr.Safeguards,
		})
	}

	retention, err := s.Ropa.ListRetentionRules(ctx, act.ID)
	if err != nil {
		return ActivityDescription{}, err
	}
	for _, rr := range retention {
		var cat orgservice.MasterItem
		if rr.DataCategoryID != nil {
			cat, err = s.Org.GetMaster(ctx, orgservice.KindDataCategories, *rr.DataCategoryID)
			if err != nil {
				return ActivityDescription{}, err
			}
		}
		out.Retention = append(out.Retention, DescriptionRetention{
			CategoryNameTh: cat.NameTh, CategoryNameEn: cat.NameEn,
			RetentionMonths: rr.RetentionMonths, RetentionBasis: rr.RetentionBasis,
			TriggerEvent: rr.TriggerEvent, DisposalMethod: rr.DisposalMethod,
		})
	}

	return out, nil
}

func (s *Service) lawfulBasisNames(ctx context.Context, code string) (orgservice.MasterItem, error) {
	items, err := s.Org.ListMaster(ctx, orgservice.KindLawfulBases)
	if err != nil {
		return orgservice.MasterItem{}, err
	}
	for _, it := range items {
		if it.Code == code {
			return it, nil
		}
	}
	return orgservice.MasterItem{}, nil
}

func (s *Service) countryNames(ctx context.Context) (map[string]orgservice.MasterItem, error) {
	items, err := s.Org.ListMaster(ctx, orgservice.KindCountries)
	if err != nil {
		return nil, err
	}
	out := make(map[string]orgservice.MasterItem, len(items))
	for _, it := range items {
		out[it.Code] = it
	}
	return out, nil
}
