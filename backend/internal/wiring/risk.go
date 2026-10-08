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
