package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	riskstore "pdpa-platform/internal/risk/store"
)

// RiskControl is one risk.risk_controls link — a mitigation measure (from the ROPA-09 catalog) chosen
// against a risk (DPIA-07's own "เลือกมาตรการจากคลัง control").
type RiskControl struct {
	RiskID          uuid.UUID
	ControlID       uuid.UUID
	ControlCode     string
	ControlName     string
	ControlCategory string
	Status          string // existing | planned | implemented | not_effective
	OwnerUserID     *uuid.UUID
	DueAt           *time.Time
	TaskID          *uuid.UUID
	RowVersion      int32
	CreatedAt       time.Time
}

var validControlStatuses = map[string]bool{"existing": true, "planned": true, "implemented": true, "not_effective": true}

// DpoTasks is what risk reads from the dpo module (rule 9) to open a remediation task when a control gets
// an owner/due date — a local interface, not a direct import of dpo/service: dpo already imports dsar,
// which imports ropa, which imports this package, so risk/service -> dpo/service would cycle. dpo's own
// concrete Service satisfies this structurally (see internal/dpo/service/risk_controls.go), no adapter
// needed — the same shape as RRA-03's own DpiaTrigger, just pointed the other way.
type DpoTasks interface {
	OpenRiskControlTask(ctx context.Context, riskID uuid.UUID, title, description string, assigneeUserID *uuid.UUID, dueAt *time.Time) (uuid.UUID, error)
	// OpenGapRemediationTask is RRA-07's own task-opening call: a legal-gap finding, given an assignee,
	// due date and priority, opens one dpo.tasks job ("ropa_gap" source) instead of DPIA-07's "risk" one.
	OpenGapRemediationTask(ctx context.Context, findingID uuid.UUID, title, description string, assigneeUserID *uuid.UUID, dueAt *time.Time, priority string) (uuid.UUID, error)
	// HasTaskForRisk is DPIA-08's own dedup check (EnsureRemediationTask) — true once any dpo.tasks row
	// already names this risk as its own source, from any caller (an earlier DPIA-07 control or an earlier
	// EnsureRemediationTask call), so a risk is never given a second remediation task.
	HasTaskForRisk(ctx context.Context, riskID uuid.UUID) (bool, error)
	// TaskStatusForRisk is RRA-14's own executive report: this risk's own remediation task status, if one
	// exists (found=false otherwise) — the report's "ความคืบหน้า" (progress) column.
	TaskStatusForRisk(ctx context.Context, riskID uuid.UUID) (status string, found bool, err error)
}

// AddRiskControl links a control from the catalog to a risk, optionally with an owner and due date — giving
// both opens a dpo.tasks remediation job (DPIA-07's own "ผู้รับผิดชอบ/วันเสร็จ → task"), and either way the
// risk's own residual likelihood/impact/score/level is recomputed from the now-updated control set.
func (s *Service) AddRiskControl(ctx context.Context, riskID, controlID uuid.UUID, ownerUserID *uuid.UUID, dueAt *time.Time) (RiskControl, error) {
	risk, err := s.GetRisk(ctx, riskID)
	if err != nil {
		return RiskControl{}, err
	}
	if _, err := s.GetControl(ctx, controlID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RiskControl{}, fmt.Errorf("%w: control_id", ErrInvalid)
		}
		return RiskControl{}, err
	}
	if ownerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*ownerUserID})
		if err != nil {
			return RiskControl{}, err
		}
		if _, ok := names[*ownerUserID]; !ok {
			return RiskControl{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}

	var taskID *uuid.UUID
	if ownerUserID != nil && dueAt != nil && s.Dpo != nil {
		id, err := s.Dpo.OpenRiskControlTask(ctx, riskID, "มาตรการลดความเสี่ยง: "+risk.Title,
			fmt.Sprintf("ดำเนินมาตรการลดความเสี่ยงสำหรับ \"%s\"", risk.Title), ownerUserID, dueAt)
		if err != nil {
			return RiskControl{}, err
		}
		taskID = &id
	}

	var row riskstore.RiskRiskControl
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = riskstore.New(pdb.MustTxFromContext(ctx)).InsertRiskControl(ctx, riskstore.InsertRiskControlParams{
			RiskID: riskID, ControlID: controlID, OwnerUserID: pgUUID(ownerUserID), DueAt: pgDate(dueAt), TaskID: pgUUID(taskID),
		})
		return err
	})
	if err != nil {
		return RiskControl{}, fmt.Errorf("%w: control already linked to this risk", ErrInvalid)
	}

	if err := s.recomputeResidual(ctx, riskID); err != nil {
		return RiskControl{}, err
	}
	if err := s.audit(ctx, "risk.risk_control.add", "risk", riskID, nil, map[string]any{"control_id": controlID}); err != nil {
		return RiskControl{}, err
	}
	control, err := s.GetControl(ctx, controlID)
	if err != nil {
		return RiskControl{}, err
	}
	return toRiskControl(riskstore.ListRiskControlsRow{
		RiskID: row.RiskID, ControlID: row.ControlID, Status: row.Status, OwnerUserID: row.OwnerUserID, DueAt: row.DueAt,
		TaskID: row.TaskID, RowVersion: row.RowVersion, CreatedAt: row.CreatedAt,
		ControlCode: control.Code, ControlName: control.Name, ControlCategory: control.Category,
	}), nil
}

// ListRiskControls returns every control linked to a risk, with the catalog's own display fields joined in.
func (s *Service) ListRiskControls(ctx context.Context, riskID uuid.UUID) ([]RiskControl, error) {
	if _, err := s.GetRisk(ctx, riskID); err != nil {
		return nil, err
	}
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListRiskControls(ctx, riskID)
	if err != nil {
		return nil, err
	}
	out := make([]RiskControl, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRiskControl(r))
	}
	return out, nil
}

// UpdateRiskControlStatus changes a linked control's own status (e.g. planned -> implemented) and
// recomputes the risk's residual score, since only "implemented" controls count toward it.
func (s *Service) UpdateRiskControlStatus(ctx context.Context, riskID, controlID uuid.UUID, status string, version int32) (RiskControl, error) {
	if !validControlStatuses[status] {
		return RiskControl{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	if _, err := s.GetRisk(ctx, riskID); err != nil {
		return RiskControl{}, err
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).UpdateRiskControlStatus(ctx, riskstore.UpdateRiskControlStatusParams{
		RiskID: riskID, ControlID: controlID, Status: status, RowVersion: version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return RiskControl{}, ErrVersionMismatch
	}
	if err != nil {
		return RiskControl{}, err
	}
	if err := s.recomputeResidual(ctx, riskID); err != nil {
		return RiskControl{}, err
	}
	if err := s.audit(ctx, "risk.risk_control.status", "risk", riskID, nil, map[string]any{"control_id": controlID, "status": status}); err != nil {
		return RiskControl{}, err
	}
	control, err := s.GetControl(ctx, controlID)
	if err != nil {
		return RiskControl{}, err
	}
	return toRiskControl(riskstore.ListRiskControlsRow{
		RiskID: row.RiskID, ControlID: row.ControlID, Status: row.Status, OwnerUserID: row.OwnerUserID, DueAt: row.DueAt,
		TaskID: row.TaskID, RowVersion: row.RowVersion, CreatedAt: row.CreatedAt,
		ControlCode: control.Code, ControlName: control.Name, ControlCategory: control.Category,
	}), nil
}

// RemoveRiskControl unlinks a control and recomputes the risk's residual score accordingly.
func (s *Service) RemoveRiskControl(ctx context.Context, riskID, controlID uuid.UUID) error {
	if _, err := s.GetRisk(ctx, riskID); err != nil {
		return err
	}
	n, err := riskstore.New(pdb.MustTxFromContext(ctx)).DeleteRiskControl(ctx, riskstore.DeleteRiskControlParams{RiskID: riskID, ControlID: controlID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := s.recomputeResidual(ctx, riskID); err != nil {
		return err
	}
	return s.audit(ctx, "risk.risk_control.remove", "risk", riskID, nil, map[string]any{"control_id": controlID})
}

// recomputeResidual is DPIA-07's own acceptance criterion: every time the implemented-control count for a
// risk could have changed (added, removed, or status flipped to/from "implemented"), the residual
// likelihood is reduced by one level per implemented control (floored at 1 — more mitigation never raises
// risk), impact is left as the inherent one (the catalog's controls reduce how likely harm is, not how
// severe it would be if it still happened — a config default, not a legal rule), and both are classified
// against the tenant's current matrix exactly like IdentifyRisk/UpdateRisk already do, never cached.
func (s *Service) recomputeResidual(ctx context.Context, riskID uuid.UUID) error {
	risk, err := s.GetRisk(ctx, riskID)
	if err != nil {
		return err
	}
	q := riskstore.New(pdb.MustTxFromContext(ctx))
	implemented, err := q.CountImplementedControls(ctx, riskID)
	if err != nil {
		return err
	}
	matrix, err := s.GetMatrix(ctx, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: no default risk matrix configured for this tenant (RRA-02)", ErrInvalid)
		}
		return err
	}
	residualLikelihood := risk.Likelihood - int(implemented)
	if residualLikelihood < 1 {
		residualLikelihood = 1
	}
	residualImpact := risk.Impact
	if residualImpact > len(matrix.ImpactLevels) {
		residualImpact = len(matrix.ImpactLevels)
	}
	if residualLikelihood > len(matrix.LikelihoodLevels) {
		residualLikelihood = len(matrix.LikelihoodLevels)
	}
	score, level, err := Classify(matrix, residualLikelihood, residualImpact)
	if err != nil {
		return err
	}
	l16, i16 := int16(residualLikelihood), int16(residualImpact)
	_, err = q.SetResidualRisk(ctx, riskstore.SetResidualRiskParams{
		ID: riskID, ResidualLikelihood: &l16, ResidualImpact: &i16, ResidualScore: numeric(score), ResidualLevel: &level,
	})
	return err
}

// highRemediationLevels is "ยังเหลือ" (still significant) for DPIA-08's own "ความเสี่ยงคงเหลือ...สร้างงานแก้ไข
// อัตโนมัติ": a risk whose effective level (residual once at least one control has been linked, else
// inherent) reads high or very_high still needs active work.
var highRemediationLevels = map[string]bool{"high": true, "very_high": true}

// EffectiveLevel is the residual level once computed, else the inherent one — what a caller means by "this
// risk's current level" once mitigation may have started.
func (r Risk) EffectiveLevel() string {
	if r.ResidualLevel != nil {
		return *r.ResidualLevel
	}
	return r.Level
}

// EnsureRemediationTask is DPIA-08's own acceptance criterion ("residual risk ... creates a remediation
// task automatically"): opens one dpo.tasks job for this risk if it's still significant (effective level
// high/very_high) and not yet accepted/closed — but only once per risk, ever (s.Dpo.HasTaskForRisk, rule 9:
// risk never queries dpo.tasks itself). A risk that's already accepted/closed, whose level doesn't warrant
// one, or that already has a task (from an earlier DPIA-07 control or an earlier call here) is a harmless
// no-op, never a duplicate — returns whether this call actually opened one.
func (s *Service) EnsureRemediationTask(ctx context.Context, riskID uuid.UUID) (bool, error) {
	risk, err := s.GetRisk(ctx, riskID)
	if err != nil {
		return false, err
	}
	if risk.Status == "accepted" || risk.Status == "closed" {
		return false, nil
	}
	if !highRemediationLevels[risk.EffectiveLevel()] {
		return false, nil
	}
	if s.Dpo == nil {
		return false, nil
	}
	exists, err := s.Dpo.HasTaskForRisk(ctx, riskID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := s.Dpo.OpenRiskControlTask(ctx, riskID, "ความเสี่ยงคงเหลือต้องจัดการ: "+risk.Title, risk.Description, nil, nil); err != nil {
		return false, err
	}
	return true, nil
}

func toRiskControl(r riskstore.ListRiskControlsRow) RiskControl {
	rc := RiskControl{RiskID: r.RiskID, ControlID: r.ControlID, ControlCode: r.ControlCode, ControlName: r.ControlName,
		ControlCategory: r.ControlCategory, Status: r.Status, OwnerUserID: uuidPtr(r.OwnerUserID), TaskID: uuidPtr(r.TaskID),
		RowVersion: r.RowVersion}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		rc.DueAt = &t
	}
	if r.CreatedAt.Valid {
		rc.CreatedAt = r.CreatedAt.Time
	}
	return rc
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
