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
	// PLT-09 files
	"files.too_large": {
		Th: "ไฟล์มีขนาดใหญ่เกินกำหนด",
		En: "File too large",
	},
	"files.type_not_allowed": {
		Th: "ไม่รองรับไฟล์ประเภทนี้",
		En: "File type not allowed",
	},
	"files.empty": {
		Th: "ไฟล์ว่างเปล่า",
		En: "File is empty",
	},
	"files.invalid_name": {
		Th: "ชื่อไฟล์ไม่ถูกต้อง",
		En: "Invalid file name",
	},
	"files.scan_pending": {
		Th: "กำลังตรวจไวรัส กรุณารอสักครู่",
		En: "Virus scan not finished",
	},
	"files.infected": {
		Th: "ไฟล์ถูกปฏิเสธเนื่องจากตรวจพบไวรัส",
		En: "File rejected: malware found",
	},
	"files.scan_failed": {
		Th: "ตรวจไวรัสไม่สำเร็จ",
		En: "Virus scan failed",
	},
	// PLT-04 notifications
	"notify.template_exists": {
		Th: "มี template รหัส ช่องทาง และภาษานี้อยู่แล้ว",
		En: "Template already exists",
	},
	"notify.template_invalid": {
		Th: "template ไม่ถูกต้อง หรือใช้ตัวแปรที่ไม่ได้ประกาศ",
		En: "Template invalid or uses an undeclared variable",
	},
	"notify.invalid": {
		Th: "คำขอส่งการแจ้งเตือนไม่ถูกต้อง",
		En: "Invalid notification request",
	},
	// PLT-07 collaboration
	"collab.has_replies": {
		Th: "ลบความเห็นที่มีการตอบกลับแล้วไม่ได้",
		En: "Comment has replies",
	},
	"collab.invalid": {
		Th: "ความเห็นหรือไฟล์แนบไม่ถูกต้อง",
		En: "Invalid comment or attachment",
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
