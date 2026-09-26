package wiring

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	breach "pdpa-platform/internal/breach/service"
	orgservice "pdpa-platform/internal/org/service"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/notify"
)

// Breach is the breach module (BRE) as both binaries run it: the API records incidents and schedules the 72-hour
// timers; the worker fires them and sends data-subject notices.
func Breach(n *notify.Service, f *files.Service, r *river.Client[pgx.Tx], a *audit.Service, k *crypto.Keyring) *breach.Service {
	return &breach.Service{Events: &events.Publisher{River: r}, Notify: n, Forms: Forms(n, a), Files: f, Keyring: k, Audit: a,
		Org: &orgservice.Service{Audit: a}, River: r}
}
