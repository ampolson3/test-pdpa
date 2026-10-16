package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
)

// DpiaGetAssessmentDescription is DPIA-04: the assessment's processing description, composed live from its
// RoPA activity on every call (dpiaservice.Service.ActivityDescription never persists it).
func (h *Strict) DpiaGetAssessmentDescription(ctx context.Context, req DpiaGetAssessmentDescriptionRequestObject) (DpiaGetAssessmentDescriptionResponseObject, error) {
	d, err := h.svc.ActivityDescription(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaGetAssessmentDescription200JSONResponse(toDescriptionWire(d)), nil
}

func toDescriptionWire(d dpiaservice.ActivityDescription) DpiaActivityDescription {
	w := DpiaActivityDescription{
		ActivityId:          d.ActivityID,
		ActivityCode:        d.ActivityCode,
		ActivityName:        d.ActivityName,
		ActivityDescription: ptr(d.ActivityDescription),
		Role:                ActivityRole(d.Role),
	}
	for _, p := range d.Purposes {
		w.Purposes = append(w.Purposes, struct {
			LawfulBasisCode   string  `json:"lawful_basis_code"`
			LawfulBasisNameEn *string `json:"lawful_basis_name_en,omitempty"`
			LawfulBasisNameTh string  `json:"lawful_basis_name_th"`
			Text              string  `json:"text"`
		}{
			Text: p.Text, LawfulBasisCode: p.LawfulBasisCode,
			LawfulBasisNameTh: p.LawfulBasisNameTh, LawfulBasisNameEn: ptr(p.LawfulBasisNameEn),
		})
	}
	for _, item := range d.Data {
		w.Data = append(w.Data, struct {
			CategoryNameEn    *string                           `json:"category_name_en,omitempty"`
			CategoryNameTh    string                            `json:"category_name_th"`
			IsSensitive       bool                              `json:"is_sensitive"`
			Source            DpiaActivityDescriptionDataSource `json:"source"`
			SubjectTypeNameEn *string                           `json:"subject_type_name_en,omitempty"`
			SubjectTypeNameTh string                            `json:"subject_type_name_th"`
		}{
			CategoryNameTh: item.CategoryNameTh, CategoryNameEn: ptr(item.CategoryNameEn),
			SubjectTypeNameTh: item.SubjectTypeNameTh, SubjectTypeNameEn: ptr(item.SubjectTypeNameEn),
			IsSensitive: item.IsSensitive, Source: DpiaActivityDescriptionDataSource(item.Source),
		})
	}
	for _, r := range d.Recipients {
		w.Recipients = append(w.Recipients, struct {
			DisclosureBasis *string `json:"disclosure_basis,omitempty"`
			PartyNameEn     *string `json:"party_name_en,omitempty"`
			PartyNameTh     string  `json:"party_name_th"`
			RecipientRole   string  `json:"recipient_role"`
		}{
			PartyNameTh: r.PartyNameTh, PartyNameEn: ptr(r.PartyNameEn),
			RecipientRole: r.RecipientRole, DisclosureBasis: ptr(r.DisclosureBasis),
		})
	}
	for _, tr := range d.Transfers {
		w.Transfers = append(w.Transfers, struct {
			CountryNameEn *string `json:"country_name_en,omitempty"`
			CountryNameTh string  `json:"country_name_th"`
			Safeguards    *string `json:"safeguards,omitempty"`
			TransferBasis string  `json:"transfer_basis"`
		}{
			CountryNameTh: tr.CountryNameTh, CountryNameEn: ptr(tr.CountryNameEn),
			TransferBasis: tr.TransferBasis, Safeguards: ptr(tr.Safeguards),
		})
	}
	for _, rr := range d.Retention {
		w.Retention = append(w.Retention, struct {
			CategoryNameEn  *string `json:"category_name_en,omitempty"`
			CategoryNameTh  *string `json:"category_name_th,omitempty"`
			DisposalMethod  string  `json:"disposal_method"`
			RetentionBasis  string  `json:"retention_basis"`
			RetentionMonths *int    `json:"retention_months,omitempty"`
			TriggerEvent    string  `json:"trigger_event"`
		}{
			CategoryNameTh: ptr(rr.CategoryNameTh), CategoryNameEn: ptr(rr.CategoryNameEn),
			DisposalMethod: rr.DisposalMethod, RetentionBasis: rr.RetentionBasis,
			RetentionMonths: rr.RetentionMonths, TriggerEvent: rr.TriggerEvent,
		})
	}
	return w
}
