package agreementhttp

import (
	"context"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// AgreementProcessingSchedule is DPA-04: the ม.40 annex, composed live from every RoPA activity this
// agreement covers (agreementservice.Service.ProcessingSchedule never persists it).
func (h *Strict) AgreementProcessingSchedule(ctx context.Context, req AgreementProcessingScheduleRequestObject) (AgreementProcessingScheduleResponseObject, error) {
	sched, err := h.svc.ProcessingSchedule(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementProcessingSchedule200JSONResponse(toScheduleWire(sched)), nil
}

func scheduleStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toScheduleWire(s agreementservice.ProcessingSchedule) AgreementProcessingSchedule {
	w := AgreementProcessingSchedule{Activities: []struct {
		ActivityCode string `json:"activity_code"`
		ActivityId   Uuid   `json:"activity_id"`
		ActivityName string `json:"activity_name"`
		Data         []struct {
			CategoryNameEn    *string `json:"category_name_en,omitempty"`
			CategoryNameTh    string  `json:"category_name_th"`
			IsSensitive       bool    `json:"is_sensitive"`
			SubjectTypeNameEn *string `json:"subject_type_name_en,omitempty"`
			SubjectTypeNameTh string  `json:"subject_type_name_th"`
		} `json:"data"`
		Purposes []struct {
			LawfulBasisCode   string  `json:"lawful_basis_code"`
			LawfulBasisNameEn *string `json:"lawful_basis_name_en,omitempty"`
			LawfulBasisNameTh string  `json:"lawful_basis_name_th"`
			Text              string  `json:"text"`
		} `json:"purposes"`
		Retention []struct {
			CategoryNameEn  *string `json:"category_name_en,omitempty"`
			CategoryNameTh  *string `json:"category_name_th,omitempty"`
			DisposalMethod  string  `json:"disposal_method"`
			RetentionBasis  string  `json:"retention_basis"`
			RetentionMonths *int    `json:"retention_months,omitempty"`
			TriggerEvent    string  `json:"trigger_event"`
		} `json:"retention"`
		SecurityMeasures []struct {
			Category    string  `json:"category"`
			Code        string  `json:"code"`
			Description *string `json:"description,omitempty"`
			Name        string  `json:"name"`
		} `json:"security_measures"`
	}{}}
	for _, a := range s.Activities {
		wa := struct {
			ActivityCode string `json:"activity_code"`
			ActivityId   Uuid   `json:"activity_id"`
			ActivityName string `json:"activity_name"`
			Data         []struct {
				CategoryNameEn    *string `json:"category_name_en,omitempty"`
				CategoryNameTh    string  `json:"category_name_th"`
				IsSensitive       bool    `json:"is_sensitive"`
				SubjectTypeNameEn *string `json:"subject_type_name_en,omitempty"`
				SubjectTypeNameTh string  `json:"subject_type_name_th"`
			} `json:"data"`
			Purposes []struct {
				LawfulBasisCode   string  `json:"lawful_basis_code"`
				LawfulBasisNameEn *string `json:"lawful_basis_name_en,omitempty"`
				LawfulBasisNameTh string  `json:"lawful_basis_name_th"`
				Text              string  `json:"text"`
			} `json:"purposes"`
			Retention []struct {
				CategoryNameEn  *string `json:"category_name_en,omitempty"`
				CategoryNameTh  *string `json:"category_name_th,omitempty"`
				DisposalMethod  string  `json:"disposal_method"`
				RetentionBasis  string  `json:"retention_basis"`
				RetentionMonths *int    `json:"retention_months,omitempty"`
				TriggerEvent    string  `json:"trigger_event"`
			} `json:"retention"`
			SecurityMeasures []struct {
				Category    string  `json:"category"`
				Code        string  `json:"code"`
				Description *string `json:"description,omitempty"`
				Name        string  `json:"name"`
			} `json:"security_measures"`
		}{ActivityId: a.ActivityID, ActivityCode: a.ActivityCode, ActivityName: a.ActivityName}

		for _, p := range a.Purposes {
			wa.Purposes = append(wa.Purposes, struct {
				LawfulBasisCode   string  `json:"lawful_basis_code"`
				LawfulBasisNameEn *string `json:"lawful_basis_name_en,omitempty"`
				LawfulBasisNameTh string  `json:"lawful_basis_name_th"`
				Text              string  `json:"text"`
			}{Text: p.Text, LawfulBasisCode: p.LawfulBasisCode, LawfulBasisNameTh: p.LawfulBasisNameTh, LawfulBasisNameEn: scheduleStrPtr(p.LawfulBasisNameEn)})
		}
		for _, d := range a.Data {
			wa.Data = append(wa.Data, struct {
				CategoryNameEn    *string `json:"category_name_en,omitempty"`
				CategoryNameTh    string  `json:"category_name_th"`
				IsSensitive       bool    `json:"is_sensitive"`
				SubjectTypeNameEn *string `json:"subject_type_name_en,omitempty"`
				SubjectTypeNameTh string  `json:"subject_type_name_th"`
			}{CategoryNameTh: d.CategoryNameTh, CategoryNameEn: scheduleStrPtr(d.CategoryNameEn),
				SubjectTypeNameTh: d.SubjectTypeNameTh, SubjectTypeNameEn: scheduleStrPtr(d.SubjectTypeNameEn), IsSensitive: d.IsSensitive})
		}
		for _, rr := range a.Retention {
			wa.Retention = append(wa.Retention, struct {
				CategoryNameEn  *string `json:"category_name_en,omitempty"`
				CategoryNameTh  *string `json:"category_name_th,omitempty"`
				DisposalMethod  string  `json:"disposal_method"`
				RetentionBasis  string  `json:"retention_basis"`
				RetentionMonths *int    `json:"retention_months,omitempty"`
				TriggerEvent    string  `json:"trigger_event"`
			}{CategoryNameTh: scheduleStrPtr(rr.CategoryNameTh), CategoryNameEn: scheduleStrPtr(rr.CategoryNameEn),
				DisposalMethod: rr.DisposalMethod, RetentionBasis: rr.RetentionBasis, RetentionMonths: rr.RetentionMonths, TriggerEvent: rr.TriggerEvent})
		}
		for _, m := range a.SecurityMeasures {
			wa.SecurityMeasures = append(wa.SecurityMeasures, struct {
				Category    string  `json:"category"`
				Code        string  `json:"code"`
				Description *string `json:"description,omitempty"`
				Name        string  `json:"name"`
			}{Code: m.Code, Name: m.Name, Category: m.Category, Description: scheduleStrPtr(m.Description)})
		}
		w.Activities = append(w.Activities, wa)
	}
	return w
}
