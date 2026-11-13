package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/connectors"
)

// Connectors is what DPX-13 needs from the connector framework (PLT-20).
type Connectors interface {
	ListActiveWithCapability(ctx context.Context, capability string) ([]connectors.Connector, error)
	ListEmployees(ctx context.Context, id uuid.UUID) ([]connectors.Employee, error)
}

// HrisSyncResult summarises one run of SyncHRIS.
type HrisSyncResult struct {
	ConnectorsChecked int
	ConnectorsFailed  int
	EmployeesSeen     int
	AccountsDisabled  int
}

// SyncHRIS is DPX-13's acceptance criterion: every employee an active HR connector reports as having
// left has their account here closed. It runs daily (gov.hris_sync_check) and on demand. A connector
// that cannot be read is counted and skipped, never aborting the others; closing an account that is
// already closed is a no-op, so the run is safe to repeat. E-mail addresses are compared in memory only
// and never logged or audited — the audit row names the user id.
func (s *Service) SyncHRIS(ctx context.Context) (HrisSyncResult, error) {
	var out HrisSyncResult
	if s.Connectors == nil {
		return out, nil
	}
	conns, err := s.Connectors.ListActiveWithCapability(ctx, connectors.CapListEmployees)
	if err != nil {
		return out, fmt.Errorf("gov: hris sync: %w", err)
	}
	for _, c := range conns {
		out.ConnectorsChecked++
		var employees []connectors.Employee
		// A savepoint keeps one connector's failure from poisoning the request transaction.
		err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			var err error
			employees, err = s.Connectors.ListEmployees(ctx, c.ID)
			return err
		})
		if err != nil {
			out.ConnectorsFailed++
			continue
		}
		out.EmployeesSeen += len(employees)
		for _, e := range employees {
			if !e.Terminated || e.Email == "" {
				continue
			}
			userID, disabled, err := iamservice.DisableUserByEmail(ctx, e.Email)
			if err != nil {
				return out, err
			}
			if !disabled {
				continue
			}
			out.AccountsDisabled++
			if err := s.audit(ctx, "iam.user.disable", "user", userID, nil,
				map[string]any{"status": "disabled", "reason": "hris_termination", "connector_id": c.ID}); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}
