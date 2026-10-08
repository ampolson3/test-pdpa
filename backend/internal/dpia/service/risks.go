package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	riskservice "pdpa-platform/internal/risk/service"
)

// editableRiskStatuses mirrors RecordOpinion's own guard (DPIA-10): risks are identified/scored while the
// round is still under active work, never once it has moved to a decision or closed.
var editableRiskStatuses = map[string]bool{"in_progress": true, "in_review": true}

// RiskCatalogItem is one entry of DPIA-06's own "คลังความเสี่ยงสำเร็จรูป" (ready-made risk catalog) — a
// starting point the "add risk" form can prefill from, not a fixed enum: the DPO can still edit the title/
// description before saving, and any risk not matching an item here is just as valid. A plain Go constant,
// not a seeded table, because nothing here is tenant-editable or referenced by id (unlike ORG-07/RTG-01's own
// seeded master data) — picking one only fills in two text fields. Flagged draft pending legal review, the
// same decisions.md pattern ORG-07/ROPA-09/PNG-03/DPIA-01/RTG-01 already used for seeded domain content
// (docs/decisions.md Q-33).
type RiskCatalogItem struct {
	Code          string
	TitleTh       string
	TitleEn       string
	DescriptionTh string
}

// RiskCatalog is DPIA-06's own starter list — common PDPA risk scenarios a DPIA screening round surfaces.
func RiskCatalog() []RiskCatalogItem {
	return []RiskCatalogItem{
		{"reidentification", "ข้อมูลที่แฝง/ลบตัวบ่งชี้แล้วถูกระบุตัวบุคคลซ้ำได้", "Re-identification of de-identified data", "เทคนิค anonymization ไม่เพียงพอเมื่อรวมกับข้อมูลอื่น อาจระบุตัวเจ้าของข้อมูลได้"},
		{"excessive_retention", "เก็บข้อมูลนานกว่าที่จำเป็นตามวัตถุประสงค์", "Retention beyond necessity", "ไม่มีกำหนดเวลาทำลาย/ลบข้อมูลที่ชัดเจน หรือไม่ปฏิบัติตามที่กำหนด"},
		{"unauthorized_access", "การเข้าถึงข้อมูลโดยไม่ได้รับอนุญาตจากภายในองค์กร", "Unauthorized internal access", "สิทธิ์เข้าถึงกว้างเกินความจำเป็นของหน้าที่งาน (least privilege ไม่เพียงพอ)"},
		{"third_party_leak", "ข้อมูลรั่วไหลผ่านผู้ประมวลผล/คู่ค้าภายนอก", "Third-party/processor data leak", "ไม่มีข้อตกลงหรือมาตรการความปลอดภัยที่เพียงพอกับผู้รับข้อมูลภายนอก"},
		{"profiling_bias", "การวิเคราะห์ข้อมูล/จัดกลุ่มที่ส่งผลกระทบอย่างไม่เป็นธรรม", "Profiling leading to unfair treatment", "การทำ profiling หรือการตัดสินใจอัตโนมัติที่ไม่มีการตรวจสอบความเป็นธรรม"},
		{"no_lawful_basis", "ประมวลผลข้อมูลโดยไม่มีฐานทางกฎหมายที่ชัดเจน", "Processing without a clear lawful basis", "ไม่ได้ระบุหรือตรวจสอบฐานการประมวลผลตาม ม.24/26 ก่อนเริ่มเก็บข้อมูล"},
		{"cross_border_no_safeguard", "โอนข้อมูลไปต่างประเทศโดยไม่มีมาตรการ/ฐานการโอนรองรับ", "Cross-border transfer without a safeguard", "ประเทศปลายทางไม่มีมาตรฐานความคุ้มครองที่เพียงพอและไม่มีมาตรการทดแทน"},
		{"rights_not_fulfilled", "ไม่สามารถดำเนินการตามคำขอใช้สิทธิของเจ้าของข้อมูลได้ทันเวลา", "Data subject rights not fulfilled in time", "ไม่มีกระบวนการหรือระยะเวลาที่ชัดเจนในการตอบคำขอใช้สิทธิ"},
		{"insecure_storage", "จัดเก็บข้อมูลโดยไม่มีการเข้ารหัสหรือการควบคุมที่เหมาะสม", "Insecure storage", "ไม่มีการเข้ารหัสข้อมูลอ่อนไหวทั้งขณะจัดเก็บและส่งผ่าน"},
		{"vendor_breach", "เหตุละเมิดข้อมูลที่ต้นเหตุมาจากผู้ประมวลผลภายนอก", "Vendor-caused breach", "ผู้ประมวลผลภายนอกไม่มีมาตรการความปลอดภัยที่เพียงพอหรือไม่แจ้งเหตุตามเวลา"},
		{"automated_decision_no_review", "การตัดสินใจอัตโนมัติที่ส่งผลกระทบโดยไม่มีการทบทวนโดยมนุษย์", "Automated decision without human review", "ระบบตัดสินใจอัตโนมัติ (รวม AI) ที่มีผลกระทบสำคัญต่อเจ้าของข้อมูลโดยไม่มีช่องทางให้ทบทวน"},
		{"vulnerable_subjects", "ประมวลผลข้อมูลของกลุ่มเปราะบางโดยไม่มีมาตรการเพิ่มเติม", "Vulnerable data subjects without extra safeguards", "เด็ก ผู้สูงอายุ หรือกลุ่มเปราะบางอื่น ที่ไม่มีมาตรการคุ้มครองเพิ่มเติมจากมาตรฐานทั่วไป"},
	}
}

func canEditRisks(status string) bool { return editableRiskStatuses[status] }

// IdentifyRisk is DPIA-06's own write path: it creates a new risk.risks row through risk/service (rule 9 —
// dpia never writes the risk schema itself) scored against the tenant's own matrix, then links it to this
// DPIA round via assess.assessment_risks, the schema's own intended join table for this purpose.
func (s *Service) IdentifyRisk(ctx context.Context, assessmentID uuid.UUID, r riskservice.Risk) (riskservice.Risk, error) {
	a, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return riskservice.Risk{}, err
	}
	if !canEditRisks(a.Status) {
		return riskservice.Risk{}, fmt.Errorf("%w: status", ErrInvalidTransition)
	}
	r.SourceType = "dpia"
	r.SourceID = &assessmentID
	if a.ActivityID != uuid.Nil {
		r.ActivityID = &a.ActivityID
	}
	risk, err := s.Risk.IdentifyRisk(ctx, r)
	if err != nil {
		return riskservice.Risk{}, err
	}
	if err := dpiastore.New(pdb.MustTxFromContext(ctx)).LinkAssessmentRisk(ctx, dpiastore.LinkAssessmentRiskParams{
		AssessmentID: assessmentID, RiskID: risk.ID,
	}); err != nil {
		return riskservice.Risk{}, err
	}
	if err := s.audit(ctx, "dpia.risk.identify", AssessmentEntityType, assessmentID, nil,
		map[string]any{"risk_id": risk.ID, "title": risk.Title, "level": risk.Level}); err != nil {
		return riskservice.Risk{}, err
	}
	return risk, nil
}

// UpdateRisk edits a risk already linked to this assessment — refuses a risk id that isn't actually linked
// here, so one assessment can never silently edit another's risk through a guessed id.
func (s *Service) UpdateRisk(ctx context.Context, assessmentID, riskID uuid.UUID, r riskservice.Risk, version int32) (riskservice.Risk, error) {
	a, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return riskservice.Risk{}, err
	}
	if !canEditRisks(a.Status) {
		return riskservice.Risk{}, fmt.Errorf("%w: status", ErrInvalidTransition)
	}
	if err := s.requireLinkedRisk(ctx, assessmentID, riskID); err != nil {
		return riskservice.Risk{}, err
	}
	return s.Risk.UpdateRisk(ctx, riskID, r, version)
}

// ListAssessmentRisks returns every risk linked to this DPIA round.
func (s *Service) ListAssessmentRisks(ctx context.Context, assessmentID uuid.UUID) ([]riskservice.Risk, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return nil, err
	}
	ids, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListAssessmentRiskIDs(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.Risk.ListRisksByIDs(ctx, ids)
}

// RemoveAssessmentRisk unlinks a risk from this round (the risk.risks row itself is left alone — it may
// still be referenced elsewhere, e.g. a later re-screening round).
func (s *Service) RemoveAssessmentRisk(ctx context.Context, assessmentID, riskID uuid.UUID) error {
	a, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return err
	}
	if !canEditRisks(a.Status) {
		return fmt.Errorf("%w: status", ErrInvalidTransition)
	}
	n, err := dpiastore.New(pdb.MustTxFromContext(ctx)).UnlinkAssessmentRisk(ctx, dpiastore.UnlinkAssessmentRiskParams{
		AssessmentID: assessmentID, RiskID: riskID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "dpia.risk.remove", AssessmentEntityType, assessmentID, nil, map[string]any{"risk_id": riskID})
}

func (s *Service) requireLinkedRisk(ctx context.Context, assessmentID, riskID uuid.UUID) error {
	ids, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListAssessmentRiskIDs(ctx, assessmentID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == riskID {
			return nil
		}
	}
	return ErrNotFound
}
