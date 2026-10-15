// Package wiring holds registrations that cmd/api and cmd/worker must share exactly — today the import
// types (PLT-14): the API creates and maps imports of a type, the worker validates and applies them.
package wiring

import (
	orgservice "pdpa-platform/internal/org/service"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/importer"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// ImportTypes lists the import types modules offer.
func ImportTypes() importer.Registry {
	org := &orgservice.Service{Audit: audit.New()}
	ropa := &ropaservice.Service{Audit: audit.New(), Org: org}
	return importer.Registry{
		orgservice.HolidayImportType:   org.HolidayImport(),
		ropaservice.ActivityImportType: ropa.ActivityImport(),
	}
}
