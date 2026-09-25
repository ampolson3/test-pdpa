package i18n

import "strings"

// problemTitles covers the fixed, code-stable problems from api/openapi/README.md's error table
// (internal/pkg/httpx's Problem constructors). Module-specific business-rule codes
// (httpx.UnprocessableEntity, httpx.InvalidTransition) aren't cataloged here — those get their own
// entries as each module that raises them is implemented, alongside its own domain vocabulary.
var problemTitles = map[string]map[Lang]string{
	"request.invalid": {
		Th: "คำขอไม่ถูกต้อง",
		En: "Malformed request",
	},
	"authn.required": {
		Th: "กรุณาเข้าสู่ระบบ",
		En: "Authentication required",
	},
	"authz.denied": {
		Th: "ไม่มีสิทธิ์เข้าถึง",
		En: "Not allowed",
	},
	"not_found": {
		Th: "ไม่พบข้อมูล",
		En: "Not found",
	},
	"idempotency.in_progress": {
		Th: "คำขอนี้กำลังดำเนินการอยู่",
		En: "Request already in progress",
	},
	"idempotency.key_reused": {
		Th: "Idempotency-Key นี้ถูกใช้กับเนื้อหาอื่นแล้ว",
		En: "Idempotency-Key reused with a different body",
	},
	"conflict.version_mismatch": {
		Th: "ข้อมูลถูกแก้ไขโดยผู้อื่นแล้ว กรุณาโหลดใหม่",
		En: "If-Match does not match the current version",
	},
	"precondition.required": {
		Th: "ขาด header ที่จำเป็น",
		En: "Missing required header",
	},
	"rate_limited": {
		Th: "มีการเรียกใช้งานถี่เกินไป กรุณาลองใหม่ภายหลัง",
		En: "Too many requests",
	},
	"internal": {
		Th: "เกิดข้อผิดพลาดภายในระบบ",
		En: "Internal error",
	},
}

// ProblemTitle returns the title for code in lang, falling back through: the requested
// language -> Default -> ok=false (caller keeps whatever title it already had — the fallback
// PLT-03 asks for, so an unrecognized code never ends up with an empty title).
func ProblemTitle(code string, lang Lang) (string, bool) {
	if strings.HasSuffix(code, ".invalid_transition") {
		return invalidTransitionTitle[lang], true
	}
	byLang, ok := problemTitles[code]
	if !ok {
		return "", false
	}
	if title, ok := byLang[lang]; ok {
		return title, true
	}
	return byLang[Default], true
}

var invalidTransitionTitle = map[Lang]string{
	Th: "ไม่สามารถเปลี่ยนสถานะนี้ได้",
	En: "Invalid state transition",
}
