// Package service implements DFG-01/DFG-03/DFG-04: a data flow diagram computed live from RoPA on every
// read. dataflow owns no table of its own for this feature — dataflow.layouts/snapshots exist for the later
// DFG-05 (drag-to-arrange) and DFG-07 (publish) features, which this pass doesn't build.
package service

import (
	"context"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
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

// Vendor is what dataflow reads from the vendormgmt module (rule 9) — DFG-04's own way to tell whether a
// RoPA recipient is tracked as a vendor at all.
type Vendor interface {
	GetVendorByPartyID(ctx context.Context, partyID uuid.UUID) (vendorservice.Vendor, error)
}

// Agreement is what dataflow reads from the agreement module (rule 9) — DFG-04's own "does this processor
// have a DPA" check, the same existence test VEN-11's own VendorContractStatus already established.
type Agreement interface {
	ListAgreements(ctx context.Context, f agreementservice.AgreementFilter) ([]agreementservice.Agreement, *agreementservice.AgreementCursor, error)
}

// Service runs the dataflow module for the tenant of the transaction in ctx. Vendor/Agreement are optional
// (nil skips DFG-04's own DPA check entirely) so existing callers/tests that only need DFG-01/03 need no
// changes.
type Service struct {
	Ropa      Ropa
	Org       Org
	Vendor    Vendor
	Agreement Agreement
}
