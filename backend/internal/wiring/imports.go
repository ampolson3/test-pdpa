// Package wiring holds registrations that cmd/api and cmd/worker must share exactly — today the import
// types (PLT-14): the API creates and maps imports of a type, the worker validates and applies them.
package wiring

import "pdpa-platform/internal/platform/importer"

// ImportTypes lists the import types modules offer.
func ImportTypes() importer.Registry {
	return importer.Registry{}
}
