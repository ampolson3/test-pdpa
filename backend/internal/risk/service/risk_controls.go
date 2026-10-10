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
