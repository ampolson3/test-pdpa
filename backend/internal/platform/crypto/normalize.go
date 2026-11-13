package crypto

import (
	"errors"
	"strings"
)

// IdentifierKind is the kind of identifier a blind index is computed over; the values match
// consent.subject_identifiers.identifier_type.
type IdentifierKind string

const (
	KindEmail      IdentifierKind = "email"
	KindPhone      IdentifierKind = "phone"
	KindNationalID IdentifierKind = "national_id"
	KindCustomerID IdentifierKind = "customer_id"
	KindPassport   IdentifierKind = "passport"
	KindLineUID    IdentifierKind = "line_uid"
	KindOther      IdentifierKind = "other"
)

// ErrInvalidIdentifier means the value can't be normalized for its kind (e.g. a phone number with no
// digits); callers should reject the input rather than index a value no lookup will ever match.
var ErrInvalidIdentifier = errors.New("crypto: identifier cannot be normalized")

// Normalize puts an identifier in the one form both writes and lookups index (security.md: lower-case
// e-mail, E.164 phone). Thai numbers are the default region for phones without a country code.
func Normalize(kind IdentifierKind, value string) (string, error) {
	v := strings.TrimSpace(value)
	switch kind {
	case KindEmail:
		v = strings.ToLower(v)
		if !strings.Contains(v, "@") {
			return "", ErrInvalidIdentifier
		}
		return v, nil
	case KindPhone:
		plus := strings.HasPrefix(v, "+")
		digits := onlyDigits(v)
		switch {
		case digits == "":
			return "", ErrInvalidIdentifier
		case plus:
			return "+" + digits, nil
		case strings.HasPrefix(digits, "00"):
			return "+" + digits[2:], nil
		case strings.HasPrefix(digits, "0") && (len(digits) == 9 || len(digits) == 10):
			return "+66" + digits[1:], nil // 0XX-XXX-XXXX (mobile) / 0X-XXX-XXXX (landline)
		case strings.HasPrefix(digits, "66") && len(digits) >= 10:
			return "+" + digits, nil
		default:
			return "", ErrInvalidIdentifier
		}
	case KindNationalID:
		digits := onlyDigits(v)
		if len(digits) != 13 {
			return "", ErrInvalidIdentifier
		}
		return digits, nil
	case KindPassport:
		v = strings.ToUpper(strings.Join(strings.Fields(v), ""))
	case KindCustomerID, KindLineUID, KindOther:
	default:
		return "", ErrInvalidIdentifier
	}
	if v == "" {
		return "", ErrInvalidIdentifier
	}
	return v, nil
}

// onlyDigits keeps ASCII digits and maps Thai digits (๐–๙) to ASCII, so a number typed with Thai
// numerals indexes the same as one typed with Arabic numerals.
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '๐' && r <= '๙':
			b.WriteRune('0' + (r - '๐'))
		}
	}
	return b.String()
}
