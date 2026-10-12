// Package service implements DFG-01/DFG-03: a data flow diagram computed live from RoPA on every read.
// dataflow owns no table of its own for this feature — dataflow.layouts/snapshots exist for the later
// DFG-05 (drag-to-arrange) and DFG-07 (publish) features, which this pass doesn't build.
package service

import (
	"context"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// Ropa is what dataflow reads from the ropa module (rule 9: no direct schema access). Every method here is
// already exported by ropaservice for ROPA-03/08's own screens, so this adds no new ropa code.
type Ropa interface {
	ListActivities(ctx context.Context, f ropaservice.ActivityFilter) ([]ropaservice.Activity, *ropaservice.ActivityCursor, error)
	ListActivityRecipients(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityRecipient, error)
	ListActivityTransfers(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityTransfer, error)
	ListRetentionRules(ctx context.Context, activityID uuid.UUID) ([]ropaservice.RetentionRule, error)
	ListActivityData(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityData, error)
}

// Org is what dataflow reads from the org module (rule 9): org-unit names for swimlanes, external-party
// names/countries for recipient and country nodes, and master-data names for data categories.
type Org interface {
	GetOrgUnit(ctx context.Context, id uuid.UUID) (orgservice.OrgUnit, error)
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
	GetMaster(ctx context.Context, kind string, id uuid.UUID) (orgservice.MasterItem, error)
}

// Service runs the dataflow module for the tenant of the transaction in ctx.
type Service struct {
	Ropa Ropa
	Org  Org
}
