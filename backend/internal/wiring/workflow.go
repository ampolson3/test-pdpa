package wiring

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	orgservice "pdpa-platform/internal/org/service"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/workflow"
)

// Workflow builds the workflow engine (PLT-05) the same way for cmd/api and cmd/worker: SLAs count on the
// org module's business calendars (ORG-20), and each module registers the policy of its record types
// here (workflow.Service.Register) so the API authorizes and the worker's SLA hooks run alike.
func Workflow(n *notify.Service, r *river.Client[pgx.Tx], a *audit.Service) *workflow.Service {
	return &workflow.Service{Calendars: &orgservice.Service{Audit: a}, Notify: n, River: r, Audit: a}
}
