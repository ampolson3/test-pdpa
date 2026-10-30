package service

import (
	"fmt"
	"strings"

	"context"

	"github.com/google/uuid"
)

// RTG-05's own "คำถามภาษาง่าย → map เป็นฟิลด์ RoPA (rule mapping)": a fixed sequence of plain-language
// questions, each answer mapped by a small Go lookup table to a real RoPA field/code — not a generic
// PLT-06 form. The "questionnaire" form type (`internal/wiring/forms.go`) stays reserved-but-unused:
// RTG-05's acceptance criterion is a fixed, structural mapping (one question per RoPA sub-resource,
// same shape every time), not a branching Q&A that benefits from PLT-06's own conditional-section
// engine — the same reasoning PNG-01/PNG-04 already used for not reinstantiating a generic engine when
// direct composition covers the literal acceptance criterion. See docs/modules/RTG.md#rtg-05.
//
// Deliberately out of scope (a plain-language wizard doesn't ask about legal jargon it doesn't need to):
// role is always "controller" (the overwhelmingly common case for a department building its own entry;
// a processor-role activity still goes through ROPA-03/RTG-04 directly), one data subject type and one
// recipient at most, no security controls (ROPA-09 is its own screen), and the data source is always
// "direct". None of these block the resulting activity from being read, edited or completed further on
// its own detail page afterward — the wizard only has to get a real, reviewable first draft started.

// WizardLegalReason is RTG-05's plain-language stand-in for a lawful basis code (ม.24/26).
type WizardLegalReason string

const (
	WizardReasonConsent  WizardLegalReason = "consent"  // "ลูกค้า/เจ้าของข้อมูลยินยอมให้เก็บ"
	WizardReasonContract WizardLegalReason = "contract" // "จำเป็นต้องทำตามสัญญา"
	WizardReasonLaw      WizardLegalReason = "law"      // "กฎหมายกำหนดให้ต้องเก็บ"
	WizardReasonBenefit  WizardLegalReason = "benefit"  // "ประโยชน์โดยชอบด้วยกฎหมายของหน่วยงาน"
)

var wizardLawfulBasisCodes = map[WizardLegalReason]string{
	WizardReasonConsent:  "CONSENT",
	WizardReasonContract: "CONTRACT",
	WizardReasonLaw:      "LEGAL_OBLIGATION",
	WizardReasonBenefit:  "LEGITIMATE_INTEREST",
}

// WizardRetentionChoice is RTG-05's plain-language stand-in for a retention rule (ม.37(3)).
type WizardRetentionChoice string

const (
	WizardRetentionUntilWithdrawn WizardRetentionChoice = "until_withdrawn"  // consent-based: keep until withdrawn/deletion request
	WizardRetentionContractPlus1Y WizardRetentionChoice = "contract_plus_1y" // keep for the relationship + 1 year
	WizardRetentionFiveYears      WizardRetentionChoice = "five_years"
	WizardRetentionTenYears       WizardRetentionChoice = "ten_years" // e.g. accounting/tax records
)

type wizardRetentionPlan struct {
	Months  *int
	BasisTh string
	Trigger string
}

var wizardRetentionPlans = map[WizardRetentionChoice]wizardRetentionPlan{
	WizardRetentionUntilWithdrawn: {Months: nil, BasisTh: "เก็บจนกว่าเจ้าของข้อมูลจะถอนความยินยอมหรือร้องขอให้ลบ",
		Trigger: "เจ้าของข้อมูลถอนความยินยอมหรือร้องขอให้ลบ"},
	WizardRetentionContractPlus1Y: {Months: nil, BasisTh: "เก็บตามระยะเวลาความสัมพันธ์/สัญญา บวกอีก 1 ปีเพื่อรองรับข้อพิพาท",
		Trigger: "สิ้นสุดสัญญาหรือความสัมพันธ์"},
	WizardRetentionFiveYears: {Months: intPtr(60), BasisTh: "นโยบายเก็บข้อมูลทั่วไปของหน่วยงาน 5 ปี",
		Trigger: "วันที่เก็บข้อมูล"},
	WizardRetentionTenYears: {Months: intPtr(120), BasisTh: "นโยบายเก็บข้อมูล 10 ปี (เช่น ข้อมูลบัญชี/ภาษี)",
		Trigger: "วันที่เก็บข้อมูล"},
}

func intPtr(n int) *int { return &n }

// WizardRecipientKind is RTG-05's plain-language stand-in for a recipient role (ม.39).
type WizardRecipientKind string

const (
	WizardRecipientProcessor WizardRecipientKind = "processor"        // "ผู้ให้บริการที่เราจ้าง (ประมวลผลแทนเรา)"
	WizardRecipientOther     WizardRecipientKind = "other_controller" // "หน่วยงานอื่นที่ใช้ข้อมูลเพื่อจุดประสงค์ของตัวเอง"
	WizardRecipientGovt      WizardRecipientKind = "government"       // "หน่วยงานราชการ/กฎหมายกำหนด"
)

var wizardRecipientRoles = map[WizardRecipientKind]string{
	WizardRecipientProcessor: "processor",
	WizardRecipientOther:     "controller",
	WizardRecipientGovt:      "government",
}

// WizardInput is every plain-language answer RTG-05's wizard collects.
type WizardInput struct {
	LegalEntityID uuid.UUID
	OrgUnitID     uuid.UUID
	Code          string
	Name          string // "เรียกกิจกรรมนี้ว่าอะไร"
	OwnerUserID   *uuid.UUID

	Purpose          string            // "เก็บข้อมูลนี้ไปทำอะไร"
	LegalReason      WizardLegalReason // "ทำไมถึงเก็บได้ (ไม่ต้องรู้ชื่อกฎหมาย)"
	ConsentPurposeID *uuid.UUID        // required when LegalReason == consent (ROPA-06)

	SubjectTypeID   uuid.UUID   // "ข้อมูลนี้เป็นของใคร"
	DataCategoryIDs []uuid.UUID // "เก็บข้อมูลอะไรบ้าง" (at least one)

	Retention WizardRetentionChoice // "เก็บไว้นานแค่ไหน"

	RecipientKind    WizardRecipientKind // optional: "" means no recipient this round
	RecipientPartyID *uuid.UUID
	RecipientNote    string // plain note on what's shared, stored as disclosure_basis
}

// CreateActivityFromWizard is RTG-05's own acceptance criterion: a plain-language Q&A, not RoPA/legal
// jargon, that an ordinary process owner can answer to get a real, reviewable processing-activity entry
// covering every topic the wizard itself asks about (purpose + lawful basis, who the data is about,
// what's collected, how long it's kept, and — optionally — who it's shared with). FK visibility for
// legal_entity_id/org_unit_id/owner_user_id is SaveActivity's own job; data_category_id/subject_type_id/
// party_id are each child-call's own job (AddActivityData/AddActivityRecipient), the same way
// CreateActivityFromTemplate already composes several child writes under SaveActivity.
func (s *Service) CreateActivityFromWizard(ctx context.Context, in WizardInput) (Activity, error) {
	in.Purpose = strings.TrimSpace(in.Purpose)
	if in.Purpose == "" {
		return Activity{}, fmt.Errorf("%w: purpose", ErrInvalid)
	}
	lawCode, ok := wizardLawfulBasisCodes[in.LegalReason]
	if !ok {
		return Activity{}, fmt.Errorf("%w: legal_reason", ErrInvalid)
	}
	if in.LegalReason == WizardReasonConsent && in.ConsentPurposeID == nil {
		return Activity{}, fmt.Errorf("%w: consent_purpose_id", ErrInvalid)
	}
	plan, ok := wizardRetentionPlans[in.Retention]
	if !ok {
		return Activity{}, fmt.Errorf("%w: retention", ErrInvalid)
	}
	if len(in.DataCategoryIDs) == 0 {
		return Activity{}, fmt.Errorf("%w: data_category_ids", ErrInvalid)
	}
	if in.SubjectTypeID == uuid.Nil {
		return Activity{}, fmt.Errorf("%w: subject_type_id", ErrInvalid)
	}
	var recipientRole string
	if in.RecipientKind != "" {
		role, ok := wizardRecipientRoles[in.RecipientKind]
		if !ok {
			return Activity{}, fmt.Errorf("%w: recipient_kind", ErrInvalid)
		}
		recipientRole = role
		if in.RecipientPartyID == nil {
			return Activity{}, fmt.Errorf("%w: recipient_party_id", ErrInvalid)
		}
	}

	out, err := s.SaveActivity(ctx, Activity{
		LegalEntityID: in.LegalEntityID, OrgUnitID: in.OrgUnitID, Code: in.Code, Name: in.Name,
		Role: "controller", OwnerUserID: in.OwnerUserID, RightsAndAccess: defaultRightsAndAccess,
	}, 0)
	if err != nil {
		return Activity{}, err
	}

	if _, err := s.AddActivityPurpose(ctx, ActivityPurpose{
		ActivityID: out.ID, PurposeText: in.Purpose, LawfulBasisCode: lawCode, ConsentPurposeID: in.ConsentPurposeID,
	}); err != nil {
		return Activity{}, err
	}

	for _, catID := range in.DataCategoryIDs {
		if _, err := s.AddActivityData(ctx, ActivityData{
			ActivityID: out.ID, DataCategoryID: catID, SubjectTypeID: in.SubjectTypeID, Source: "direct",
		}); err != nil {
			return Activity{}, err
		}
	}

	if _, err := s.AddRetentionRule(ctx, RetentionRule{
		ActivityID: out.ID, RetentionMonths: plan.Months, RetentionBasis: plan.BasisTh, TriggerEvent: plan.Trigger, DisposalMethod: "delete",
	}); err != nil {
		return Activity{}, err
	}

	if in.RecipientKind != "" {
		if _, err := s.AddActivityRecipient(ctx, ActivityRecipient{
			ActivityID: out.ID, PartyID: *in.RecipientPartyID, RecipientRole: recipientRole, DisclosureBasis: strings.TrimSpace(in.RecipientNote),
		}); err != nil {
			return Activity{}, err
		}
	}

	return s.GetActivity(ctx, out.ID)
}
