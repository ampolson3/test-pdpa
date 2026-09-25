// Package i18n is the Go message catalog CLAUDE.md rule 12 asks for ("no user-facing string
// literals: keys in packages/i18n (th + en) and Go message catalogs") — the backend half of
// PLT-03. Domain content (consent text, notice templates, ...) doesn't get a separate translation
// table (docs/modules/PLT.md#plt-03's SA note): it's th/en columns or a {th, en} jsonb value next
// to the data, per the ERD. This package is only for the fixed strings the backend itself writes —
// today, RFC 9457 problem titles.
package i18n

import "strings"

type Lang string

const (
	Th Lang = "th"
	En Lang = "en"
)

// Default matches decisions.md Q-17 ("TH (ค่าเริ่มต้น) + EN").
const Default = Th

// FromAcceptLanguage resolves the Accept-Language header to a supported Lang, falling back to
// Default for anything else — including empty, unsupported, or a full RFC 7231 header
// ("en-US,en;q=0.9") a real browser might send even though api/openapi/openapi.yaml's
// AcceptLanguage parameter only declares the literal values "th" and "en" (apps/admin's BFF is
// responsible for sending one of those two, not forwarding the raw browser header — see
// apps/admin/src/lib/api.ts).
func FromAcceptLanguage(header string) Lang {
	if strings.EqualFold(strings.TrimSpace(header), string(En)) {
		return En
	}
	return Default
}
