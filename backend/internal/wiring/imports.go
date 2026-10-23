// Package wiring holds registrations that cmd/api and cmd/worker must share exactly — today the import
// types (PLT-14): the API creates and maps imports of a type, the worker validates and applies them.
package wiring

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	consentservice "pdpa-platform/internal/consent/service"
	orgservice "pdpa-platform/internal/org/service"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/importer"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// ImportTypes lists the import types modules offer. keyring/riverClient back CON-22's own legacy-consent
// import, which needs PLT-13 encryption (identifiers) and PLT-11 event publishing (consent.* events) —
// both already built by cmd/api/cmd/worker's own main.go before they call this.
func ImportTypes(keyring *crypto.Keyring, riverClient *river.Client[pgx.Tx]) importer.Registry {
	org := &orgservice.Service{Audit: audit.New()}
	ropa := &ropaservice.Service{Audit: audit.New(), Org: org}
	consent := &consentservice.Service{Audit: audit.New(), Org: org, Keyring: keyring, Events: &events.Publisher{River: riverClient}}
	return importer.Registry{
		orgservice.HolidayImportType:    org.HolidayImport(),
		orgservice.MasterDataImportType: org.MasterDataImport(),
		ropaservice.ActivityImportType:  ropa.ActivityImport(),
		consentservice.ImportType:       consent.LegacyImport(),
	}
}
