package service

import (
	"context"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
)

// ProcessingSchedule is DPA-04's own acceptance criterion: the module doc's own topic list — data
// categories, data subject groups, purposes, retention periods and security measures — pulled straight
// from every RoPA activity this agreement covers. Computed live on every call, never persisted (same
// reasoning DPIA-04's own ActivityDescription already used: there is nothing to go stale, so there is no
// separate "sync when RoPA changes" step to build).
type ProcessingSchedule struct {
	Activities []ScheduleActivity
}

type ScheduleActivity struct {
	ActivityID       uuid.UUID
	ActivityCode     string
	ActivityName     string
	Purposes         []SchedulePurpose
	Data             []ScheduleData
	Retention        []ScheduleRetention
	SecurityMeasures []ScheduleControl
}

type SchedulePurpose struct {
	Text              string
	LawfulBasisCode   string
	LawfulBasisNameTh string
	LawfulBasisNameEn string
}

type ScheduleData struct {
	CategoryNameTh    string
	CategoryNameEn    string
	SubjectTypeNameTh string
	SubjectTypeNameEn string
	IsSensitive       bool
}

type ScheduleRetention struct {
	CategoryNameTh  string
	CategoryNameEn  string
	RetentionMonths *int
	RetentionBasis  string
	TriggerEvent    string
	DisposalMethod  string
}

type ScheduleControl struct {
	Code        string
	Name        string
	Category    string
	Description string
}

// ProcessingSchedule composes the annex content for an agreement's own ภาคผนวกรายละเอียดการประมวลผล, one
// section per linked RoPA activity, reading ropa/org only through their already-exported services (rule 9 —
// agreement never reads ropa's or org's schema directly).
func (s *Service) ProcessingSchedule(ctx context.Context, agreementID uuid.UUID) (ProcessingSchedule, error) {
	a, err := s.GetAgreement(ctx, agreementID)
	if err != nil {
		return ProcessingSchedule{}, err
	}

	controls, err := s.Ropa.ListControls(ctx)
	if err != nil {
		return ProcessingSchedule{}, err
	}
	controlByID := make(map[uuid.UUID]ScheduleControl, len(controls))
	for _, c := range controls {
		controlByID[c.ID] = ScheduleControl{Code: c.Code, Name: c.Name, Category: c.Category}
	}
	lawfulBases, err := s.Org.ListMaster(ctx, orgservice.KindLawfulBases)
	if err != nil {
		return ProcessingSchedule{}, err
	}
	basisByCode := make(map[string]orgservice.MasterItem, len(lawfulBases))
	for _, b := range lawfulBases {
		basisByCode[b.Code] = b
	}

	out := ProcessingSchedule{Activities: make([]ScheduleActivity, 0, len(a.ActivityIDs))}
	for _, activityID := range a.ActivityIDs {
		act, err := s.Ropa.GetActivity(ctx, activityID)
		if err != nil {
			return ProcessingSchedule{}, err
		}
		sa := ScheduleActivity{ActivityID: act.ID, ActivityCode: act.Code, ActivityName: act.Name}

		purposes, err := s.Ropa.ListActivityPurposes(ctx, act.ID)
		if err != nil {
			return ProcessingSchedule{}, err
		}
		for _, p := range purposes {
			basis := basisByCode[p.LawfulBasisCode]
			sa.Purposes = append(sa.Purposes, SchedulePurpose{
				Text: p.PurposeText, LawfulBasisCode: p.LawfulBasisCode,
				LawfulBasisNameTh: basis.NameTh, LawfulBasisNameEn: basis.NameEn,
			})
		}

		data, err := s.Ropa.ListActivityData(ctx, act.ID)
		if err != nil {
			return ProcessingSchedule{}, err
		}
		for _, d := range data {
			cat, err := s.Org.GetMaster(ctx, orgservice.KindDataCategories, d.DataCategoryID)
			if err != nil {
				return ProcessingSchedule{}, err
			}
			subj, err := s.Org.GetMaster(ctx, orgservice.KindSubjectTypes, d.SubjectTypeID)
			if err != nil {
				return ProcessingSchedule{}, err
			}
			sa.Data = append(sa.Data, ScheduleData{
				CategoryNameTh: cat.NameTh, CategoryNameEn: cat.NameEn,
				SubjectTypeNameTh: subj.NameTh, SubjectTypeNameEn: subj.NameEn, IsSensitive: d.IsSensitive,
			})
		}

		retention, err := s.Ropa.ListRetentionRules(ctx, act.ID)
		if err != nil {
			return ProcessingSchedule{}, err
		}
		for _, rr := range retention {
			var cat orgservice.MasterItem
			if rr.DataCategoryID != nil {
				cat, err = s.Org.GetMaster(ctx, orgservice.KindDataCategories, *rr.DataCategoryID)
				if err != nil {
					return ProcessingSchedule{}, err
				}
			}
			sa.Retention = append(sa.Retention, ScheduleRetention{
				CategoryNameTh: cat.NameTh, CategoryNameEn: cat.NameEn,
				RetentionMonths: rr.RetentionMonths, RetentionBasis: rr.RetentionBasis,
				TriggerEvent: rr.TriggerEvent, DisposalMethod: rr.DisposalMethod,
			})
		}

		measures, err := s.Ropa.ListActivityControls(ctx, act.ID)
		if err != nil {
			return ProcessingSchedule{}, err
		}
		for _, m := range measures {
			c := controlByID[m.ControlID]
			c.Description = m.Description
			sa.SecurityMeasures = append(sa.SecurityMeasures, c)
		}

		out.Activities = append(out.Activities, sa)
	}
	return out, nil
}
