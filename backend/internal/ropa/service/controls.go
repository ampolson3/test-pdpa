package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	pdb "pdpa-platform/internal/pkg/db"
	riskservice "pdpa-platform/internal/risk/service"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ActivityControl links a processing activity to one ม.37(1) security measure (ROPA-09), optionally with
// a free-text description of how it's applied to this specific activity.
type ActivityControl struct {
	ActivityID  uuid.UUID
	ControlID   uuid.UUID
	Description string
}

// ListControls returns the ม.37(1) security-measures catalog (global + this tenant's own) for the
// activity-control picker — a thin pass-through to the risk module (rule 9).
func (s *Service) ListControls(ctx context.Context) ([]riskservice.Control, error) {
	return s.Risk.ListControls(ctx)
}

func (s *Service) ListActivityControls(ctx context.Context, activityID uuid.UUID) ([]ActivityControl, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityControls(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityControl, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityControl(ropastore.InsertActivityControlRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityControl(ctx context.Context, c ActivityControl) (ActivityControl, error) {
	c.Description = strings.TrimSpace(c.Description)
	if _, err := s.mustActivity(ctx, c.ActivityID); err != nil {
		return ActivityControl{}, err
	}
	if _, err := s.Risk.GetControl(ctx, c.ControlID); err != nil {
		return ActivityControl{}, fmt.Errorf("%w: control_id", ErrInvalid)
	}
	var out ActivityControl
	err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		row, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertActivityControl(ctx, ropastore.InsertActivityControlParams{
			ActivityID: c.ActivityID, ControlID: c.ControlID, Description: opt(c.Description)})
		if err != nil {
			return err
		}
		out = toActivityControl(row)
		return nil
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return ActivityControl{}, fmt.Errorf("%w: control_id already linked", ErrInvalid)
	case err != nil:
		return ActivityControl{}, err
	}
	if _, _, err := s.recompute(ctx, c.ActivityID); err != nil {
		return ActivityControl{}, err
	}
	return out, s.audit(ctx, "ropa.activity_control.create", c.ActivityID, nil, controlAudit(out))
}

func (s *Service) DeleteActivityControl(ctx context.Context, activityID, controlID uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityControl(ctx, ropastore.DeleteActivityControlParams{
		ActivityID: activityID, ControlID: controlID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_control.delete", activityID, map[string]any{"control_id": controlID}, nil)
}

func controlAudit(c ActivityControl) map[string]any {
	return map[string]any{"control_id": c.ControlID}
}

func toActivityControl(r ropastore.InsertActivityControlRow) ActivityControl {
	return ActivityControl{ActivityID: r.ActivityID, ControlID: r.ControlID, Description: deref(r.Description)}
}
