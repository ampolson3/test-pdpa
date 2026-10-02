package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	templatesservice "pdpa-platform/internal/ropa/templates"
)

// defaultRightsAndAccess is the generic starting text ROPA-05 fills in for a template-created
// activity's rights-and-access note — the RTG-01 catalog's own `defaults` jsonb has no such field (it
// describes what data is processed, not how a specific tenant handles a data-subject request for it),
// so unlike purposes/data/retention/security_controls this one isn't copied from the template. It is
// internal RoPA record text, not the outward-facing legal wording rule 8 governs, so a plain Go
// constant (not a migration-seeded template) is enough — the completeness check still requires it be
// non-empty, and leaving it blank would make every template-created activity start incomplete on an
// item the template was never meant to answer.
const defaultRightsAndAccess = "ดำเนินการตามกระบวนการคำขอใช้สิทธิของเจ้าของข้อมูลมาตรฐานขององค์กร (DSAR) — โปรดทบทวนและปรับรายละเอียดให้ตรงกับกิจกรรมนี้"

// templateDefaults mirrors the shape RTG-01's migration seeded into ropa.activity_templates.defaults —
// see CLAUDE.md's RTG-01 section: field names match ROPA-03/06/08/09's own child-table columns exactly,
// keyed by *code* (not id) since a global template can't FK-reference a tenant's own org.* rows.
type templateDefaults struct {
	DescriptionTh string `json:"description_th"`
	Purposes      []struct {
		PurposeTextTh   string `json:"purpose_text_th"`
		LawfulBasisCode string `json:"lawful_basis_code"`
	} `json:"purposes"`
	Data []struct {
		DataCategoryCode string `json:"data_category_code"`
		SubjectTypeCode  string `json:"subject_type_code"`
		Source           string `json:"source"`
	} `json:"data"`
	Retention []struct {
		RetentionMonths  *int   `json:"retention_months"`
		RetentionBasisTh string `json:"retention_basis_th"`
		TriggerEventTh   string `json:"trigger_event_th"`
		DisposalMethod   string `json:"disposal_method"`
	} `json:"retention"`
	SecurityControls []string `json:"security_controls"`
}

// CreateActivityFromTemplate is ROPA-05's own half ("หรือเลือกจากคลังกิจกรรมมาตรฐานในโมดูล ROPA Template
// Generator"): it creates a new processing activity and populates it from an RTG-01 standard-activity
// template's defaults — purposes, data (with category/subject-type codes resolved to this tenant's own
// visible org.* rows), retention rules and security controls, covering every ม.39 topic the template
// itself carries (the acceptance criterion). Recipients are deliberately not auto-created: the
// template only has a role + free-text note (e.g. "ผู้ให้บริการระบบสรรหาบุคลากรภายนอก"), never a real
// org.external_parties id — a global template can't know which of *this* tenant's parties that is, the
// same "no automatic cross-schema FK reassignment" limit ORG-06's own merge already documented — so
// recipients stays an item the OWNER adds by hand afterward from the template's own note (surfaced on
// GetActivityTemplate already, ROPA-05 doesn't duplicate it). The same applies to a processor-role
// template's "controller" item (ม.39's required controller_party_id), which the completeness check
// itself only treats as conditional on role, so an otherwise-complete controller-role activity (the
// overwhelming majority of the 51 seeded activities) is already at 100% right after creation.
func (s *Service) CreateActivityFromTemplate(ctx context.Context, templateID, legalEntityID, orgUnitID uuid.UUID, code string, ownerUserID *uuid.UUID) (Activity, error) {
	if s.Templates == nil {
		return Activity{}, fmt.Errorf("%w: activity_template_id", ErrInvalid)
	}
	tpl, err := s.Templates.GetActivityTemplate(ctx, templateID)
	if err != nil {
		if errors.Is(err, templatesservice.ErrNotFound) {
			return Activity{}, fmt.Errorf("%w: activity_template_id", ErrInvalid)
		}
		return Activity{}, err
	}
	var d templateDefaults
	if err := json.Unmarshal(tpl.Defaults, &d); err != nil {
		return Activity{}, err
	}

	out, err := s.SaveActivity(ctx, Activity{
		LegalEntityID:   legalEntityID,
		OrgUnitID:       orgUnitID,
		Code:            code,
		Name:            tpl.NameTh,
		Description:     d.DescriptionTh,
		Role:            tpl.Role,
		OwnerUserID:     ownerUserID,
		RightsAndAccess: defaultRightsAndAccess,
	}, 0)
	if err != nil {
		return Activity{}, err
	}

	for _, p := range d.Purposes {
		if _, err := s.AddActivityPurpose(ctx, ActivityPurpose{ActivityID: out.ID, PurposeText: p.PurposeTextTh, LawfulBasisCode: p.LawfulBasisCode}); err != nil {
			return Activity{}, fmt.Errorf("template purpose %q: %w", p.LawfulBasisCode, err)
		}
	}

	for _, item := range d.Data {
		catID, err := s.masterIDByCode(ctx, "data_categories", item.DataCategoryCode)
		if err != nil {
			return Activity{}, err
		}
		subID, err := s.masterIDByCode(ctx, "data_subject_types", item.SubjectTypeCode)
		if err != nil {
			return Activity{}, err
		}
		if _, err := s.AddActivityData(ctx, ActivityData{ActivityID: out.ID, DataCategoryID: catID, SubjectTypeID: subID, Source: item.Source}); err != nil {
			return Activity{}, fmt.Errorf("template data %q/%q: %w", item.DataCategoryCode, item.SubjectTypeCode, err)
		}
	}

	for _, r := range d.Retention {
		if _, err := s.AddRetentionRule(ctx, RetentionRule{ActivityID: out.ID, RetentionMonths: r.RetentionMonths,
			RetentionBasis: r.RetentionBasisTh, TriggerEvent: r.TriggerEventTh, DisposalMethod: r.DisposalMethod}); err != nil {
			return Activity{}, fmt.Errorf("template retention: %w", err)
		}
	}

	for _, code := range d.SecurityControls {
		ctrlID, err := s.controlIDByCode(ctx, code)
		if err != nil {
			return Activity{}, err
		}
		if _, err := s.AddActivityControl(ctx, ActivityControl{ActivityID: out.ID, ControlID: ctrlID}); err != nil {
			return Activity{}, fmt.Errorf("template control %q: %w", code, err)
		}
	}

	final, err := s.GetActivity(ctx, out.ID)
	if err != nil {
		return Activity{}, err
	}
	return final, s.audit(ctx, "ropa.activity.create_from_template", out.ID, nil, map[string]any{"activity_template_id": templateID, "code": tpl.Code})
}

func (s *Service) masterIDByCode(ctx context.Context, kind, code string) (uuid.UUID, error) {
	items, err := s.Org.ListMaster(ctx, kind)
	if err != nil {
		return uuid.Nil, err
	}
	for _, it := range items {
		if it.Code == code && it.ID != nil {
			return *it.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("%w: %s %q", ErrInvalid, kind, code)
}

func (s *Service) controlIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	controls, err := s.Risk.ListControls(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	for _, c := range controls {
		if strings.EqualFold(c.Code, code) {
			return c.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("%w: security_control %q", ErrInvalid, code)
}
