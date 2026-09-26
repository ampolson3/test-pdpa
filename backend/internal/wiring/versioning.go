package wiring

import (
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
)

// Versioning builds the versioning & approval framework (PLT-08). Modules register their versioned record
// types here (versioning.Service.Register: permissions, approval levels by role, publish hook).
func Versioning(n *notify.Service, a *audit.Service) *versioning.Service {
	return &versioning.Service{Notify: n, Audit: a}
}
