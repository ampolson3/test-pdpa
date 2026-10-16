package service

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/importer"
)

// ActivityImportType is ROPA-18's PLT-14 import type: one row per processing activity, carrying the
// core activity fields plus one purpose, one data/subject pair and one retention rule — the minimal
// slice BP-05's own Excel migration needs (an organization's existing RoPA spreadsheet typically has
// exactly one row per activity with these columns; an activity needing more than one purpose, data
// category or retention rule is edited by hand afterward on /ropa/activities/{id}, the same way a
// template-created activity's recipients are left for the OWNER to add, per RTG-01/ROPA-05's own
// reasoning). Legal entity, department, lawful basis, data category and subject type are free text
// in the sheet — fuzzyBestMatch resolves each against this tenant's own org.* master data (the
// "จับคู่ master data แบบ fuzzy" the module doc's own backend note calls for), so a column need not
// match an internal code or exact name character-for-character.
const ActivityImportType = "ropa.activity"

// ActivityImport describes the import. Error-report messages are codes (no personal data, rule 3):
// invalid_role, invalid_retention_months, invalid_disposal_method, unknown_legal_entity,
// unknown_org_unit, unknown_lawful_basis, unknown_data_category, unknown_subject_type (and the
// framework's own required/duplicate-code handling via SaveActivity's existing ErrInvalid).
func (s *Service) ActivityImport() importer.Type {
	return importer.Type{
		Permission: "ropa.activity.create",
		Columns: []importer.Column{
			{Key: "code", Required: true, Label: map[string]string{"th": "โค้ดกิจกรรม", "en": "Activity code"}},
			{Key: "name", Required: true, Label: map[string]string{"th": "ชื่อกิจกรรม", "en": "Activity name"}, Aliases: []string{"activity_name"}},
			{Key: "legal_entity", Required: true, Label: map[string]string{"th": "นิติบุคคล", "en": "Legal entity"}},
			{Key: "org_unit", Required: true, Label: map[string]string{"th": "หน่วยงาน", "en": "Department"}, Aliases: []string{"department"}},
			{Key: "role", Label: map[string]string{"th": "บทบาท (controller/processor)", "en": "Role (controller/processor)"}},
			{Key: "description", Label: map[string]string{"th": "คำอธิบาย", "en": "Description"}},
			{Key: "purpose", Required: true, Label: map[string]string{"th": "วัตถุประสงค์", "en": "Purpose"}},
			{Key: "lawful_basis", Required: true, Label: map[string]string{"th": "ฐานทางกฎหมาย", "en": "Lawful basis"}, Aliases: []string{"lawful_basis_code"}},
			{Key: "data_category", Required: true, Label: map[string]string{"th": "หมวดข้อมูล", "en": "Data category"}},
			{Key: "subject_type", Required: true, Label: map[string]string{"th": "กลุ่มเจ้าของข้อมูล", "en": "Data subject type"}},
			{Key: "retention_months", Required: true, Label: map[string]string{"th": "ระยะเวลาเก็บ (เดือน)", "en": "Retention (months)"}},
			{Key: "retention_basis", Required: true, Label: map[string]string{"th": "เหตุผลการเก็บ", "en": "Retention basis"}},
			{Key: "trigger_event", Required: true, Label: map[string]string{"th": "เริ่มนับจาก", "en": "Trigger event"}},
			{Key: "disposal_method", Required: true, Label: map[string]string{"th": "วิธีทำลาย/ลบ", "en": "Disposal method"}, Aliases: []string{"disposal"}},
		},
		Validate: func(ctx context.Context, _ int, r importer.Row) []importer.FieldError {
			var errs []importer.FieldError
			role := strings.ToLower(strings.TrimSpace(r["role"]))
			if role != "" && role != "controller" && role != "processor" {
				errs = append(errs, importer.FieldError{Column: "role", Message: "invalid_role"})
			}
			if _, err := s.resolveImportOrgUnit(ctx, r["legal_entity"], r["org_unit"]); err != nil {
				if err == ErrNotFound {
					errs = append(errs, importer.FieldError{Column: "org_unit", Message: "unknown_org_unit"})
				} else {
					errs = append(errs, importer.FieldError{Column: "legal_entity", Message: "unknown_legal_entity"})
				}
			}
			if basis, err := s.resolveImportLawfulBasis(ctx, r["lawful_basis"]); err != nil {
				errs = append(errs, importer.FieldError{Column: "lawful_basis", Message: "unknown_lawful_basis"})
			} else if basis.RequiresConsent {
				// ROPA-06: a consent-requiring basis needs a real consent.purposes link, which this
				// single-row import has no column for — flagged so OWNER links one by hand afterward
				// on /ropa/activities/{id} rather than Apply failing opaquely mid-batch.
				errs = append(errs, importer.FieldError{Column: "lawful_basis", Message: "requires_consent_link"})
			}
			if _, err := s.resolveImportMaster(ctx, "data_categories", r["data_category"]); err != nil {
				errs = append(errs, importer.FieldError{Column: "data_category", Message: "unknown_data_category"})
			}
			if _, err := s.resolveImportMaster(ctx, "data_subject_types", r["subject_type"]); err != nil {
				errs = append(errs, importer.FieldError{Column: "subject_type", Message: "unknown_subject_type"})
			}
			if months, err := strconv.Atoi(strings.TrimSpace(r["retention_months"])); err != nil || months <= 0 {
				errs = append(errs, importer.FieldError{Column: "retention_months", Message: "invalid_retention_months"})
			}
			if !slices.Contains(disposalMethods, strings.ToLower(strings.TrimSpace(r["disposal_method"]))) {
				errs = append(errs, importer.FieldError{Column: "disposal_method", Message: "invalid_disposal_method"})
			}
			return errs
		},
		Apply: func(ctx context.Context, _ int, r importer.Row) error {
			legalEntityID, orgUnitID, err := s.resolveImportLegalEntityAndUnit(ctx, r["legal_entity"], r["org_unit"])
			if err != nil {
				return err
			}
			role := strings.ToLower(strings.TrimSpace(r["role"]))
			if role == "" {
				role = "controller"
			}
			created, err := s.SaveActivity(ctx, Activity{
				LegalEntityID: legalEntityID,
				OrgUnitID:     orgUnitID,
				Code:          r["code"],
				Name:          r["name"],
				Description:   r["description"],
				Role:          role,
			}, 0)
			if err != nil {
				return err
			}

			basis, err := s.resolveImportLawfulBasis(ctx, r["lawful_basis"])
			if err != nil {
				return err
			}
			if _, err := s.AddActivityPurpose(ctx, ActivityPurpose{ActivityID: created.ID, PurposeText: r["purpose"], LawfulBasisCode: basis.Code}); err != nil {
				return err
			}

			categoryID, err := s.resolveImportMaster(ctx, "data_categories", r["data_category"])
			if err != nil {
				return err
			}
			subjectTypeID, err := s.resolveImportMaster(ctx, "data_subject_types", r["subject_type"])
			if err != nil {
				return err
			}
			if _, err := s.AddActivityData(ctx, ActivityData{ActivityID: created.ID, DataCategoryID: categoryID, SubjectTypeID: subjectTypeID, Source: "direct"}); err != nil {
				return err
			}

			months, _ := strconv.Atoi(strings.TrimSpace(r["retention_months"]))
			_, err = s.AddRetentionRule(ctx, RetentionRule{
				ActivityID:      created.ID,
				RetentionMonths: &months,
				RetentionBasis:  r["retention_basis"],
				TriggerEvent:    r["trigger_event"],
				DisposalMethod:  strings.ToLower(strings.TrimSpace(r["disposal_method"])),
			})
			return err
		},
	}
}

// resolveImportLegalEntityAndUnit resolves both together so Apply doesn't re-resolve the legal
// entity twice (once for itself, once via the org unit's own lookup).
func (s *Service) resolveImportLegalEntityAndUnit(ctx context.Context, legalEntityName, unitName string) (uuid.UUID, uuid.UUID, error) {
	entities, err := s.Org.ListLegalEntities(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	leIdx, ok := fuzzyBestMatch(legalEntityName, legalEntityNames(entities))
	if !ok {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	legalEntityID := entities[leIdx].ID

	units, err := s.Org.ListOrgUnits(ctx, &legalEntityID, false)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	unitIdx, ok := fuzzyBestMatch(unitName, orgUnitNames(units))
	if !ok {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return legalEntityID, units[unitIdx].ID, nil
}

func (s *Service) resolveImportOrgUnit(ctx context.Context, legalEntityName, unitName string) (uuid.UUID, error) {
	_, unitID, err := s.resolveImportLegalEntityAndUnit(ctx, legalEntityName, unitName)
	return unitID, err
}

func (s *Service) resolveImportLawfulBasis(ctx context.Context, text string) (orgservice.MasterItem, error) {
	items, err := s.Org.ListMaster(ctx, "lawful_bases")
	if err != nil {
		return orgservice.MasterItem{}, err
	}
	idx, ok := fuzzyBestMatch(text, masterNames(items))
	if !ok {
		return orgservice.MasterItem{}, ErrNotFound
	}
	return items[idx], nil
}

func (s *Service) resolveImportMaster(ctx context.Context, kind, text string) (uuid.UUID, error) {
	items, err := s.Org.ListMaster(ctx, kind)
	if err != nil {
		return uuid.Nil, err
	}
	idx, ok := fuzzyBestMatch(text, masterNames(items))
	if !ok || items[idx].ID == nil {
		return uuid.Nil, ErrNotFound
	}
	return *items[idx].ID, nil
}

func legalEntityNames(entities []orgservice.LegalEntity) []string {
	out := make([]string, len(entities))
	for i, e := range entities {
		out[i] = strings.TrimSpace(e.NameTh + " " + e.NameEn)
	}
	return out
}

func orgUnitNames(units []orgservice.OrgUnit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = strings.TrimSpace(u.NameTh + " " + u.NameEn + " " + u.Code)
	}
	return out
}

func masterNames(items []orgservice.MasterItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = strings.TrimSpace(it.Code + " " + it.NameTh + " " + it.NameEn)
	}
	return out
}
