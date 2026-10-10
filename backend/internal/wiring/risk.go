package wiring

import (
	"context"
	"errors"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// RiskRopa adapts the real ropa + org services to risk/service's own local Ropa interface (RRA-01) — a
// direct import the other way is impossible, since internal/ropa/service already imports
// internal/risk/service for ROPA-09's security-controls catalog (rule 9: no cycle).
type RiskRopa struct {
	Ropa *ropaservice.Service
	Org  *orgservice.Service
}

var highVolumeBands = map[string]bool{"10k_100k": true, "gt_100k": true}

func (a RiskRopa) ActivityVisible(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.Ropa.GetActivity(ctx, id)
	if errors.Is(err, ropaservice.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// MissingItems is RRA-04's own read of ROPA-03's live completeness check (no_lawful_basis/no_retention/
// transfer_no_basis/sensitive_no_consent map straight onto these same codes) — rule 9: risk never reads
// ropa.processing_activities directly, only through GetActivity.
func (a RiskRopa) MissingItems(ctx context.Context, id uuid.UUID) ([]string, error) {
	act, err := a.Ropa.GetActivity(ctx, id)
	if err != nil {
		return nil, err
	}
	return act.MissingItems, nil
}

// ListActivityIDs is RRA-04's own sweep source — every activity visible to the caller's tenant, paged
// through ROPA-03's own cursor exactly as the admin UI's list does.
func (a RiskRopa) ListActivityIDs(ctx context.Context) ([]uuid.UUID, error) {
	var out []uuid.UUID
	f := ropaservice.ActivityFilter{}
	for {
		page, cursor, err := a.Ropa.ListActivities(ctx, f)
		if err != nil {
			return nil, err
		}
		for _, act := range page {
			out = append(out, act.ID)
		}
		if cursor == nil {
			return out, nil
		}
		f.After = cursor
	}
}

func (a RiskRopa) Signals(ctx context.Context, id uuid.UUID) (riskservice.ActivitySignals, error) {
	var sig riskservice.ActivitySignals

	data, err := a.Ropa.ListActivityData(ctx, id)
	if err != nil {
		return sig, err
	}
	for _, d := range data {
		if d.IsSensitive {
			sig.HasSensitiveData = true
		}
		if highVolumeBands[d.VolumeBand] {
			sig.HighVolume = true
		}
		item, err := a.Org.GetMaster(ctx, orgservice.KindSubjectTypes, d.SubjectTypeID)
		if err == nil && item.IsVulnerable {
			sig.HasVulnerableSubject = true
		}
	}

	recipients, err := a.Ropa.ListActivityRecipients(ctx, id)
	if err != nil {
		return sig, err
	}
	sig.RecipientCount = len(recipients)

	transfers, err := a.Ropa.ListActivityTransfers(ctx, id)
	if err != nil {
		return sig, err
	}
	sig.HasCrossBorderTransfer = len(transfers) > 0

	controls, err := a.Ropa.ListActivityControls(ctx, id)
	if err != nil {
		return sig, err
	}
	sig.ControlCount = len(controls)

	return sig, nil
}
