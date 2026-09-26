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
	// PLT-06 form & assessment engine
	"forms.invalid_answers": {
		Th: "คำตอบบางข้อไม่ถูกต้อง",
		En: "Some answers are invalid",
	},
	"forms.invalid_state": {
		Th: "ทำรายการนี้ไม่ได้ในสถานะปัจจุบันของแบบฟอร์ม",
		En: "Not allowed in the form's current state",
	},
	"forms.unknown_type": {
		Th: "ไม่มีโมดูลที่ใช้แบบฟอร์มประเภทนี้",
		En: "No module offers this form type",
	},
	"forms.invalid_schema": {
		Th: "โครงสร้างแบบฟอร์มไม่ถูกต้อง",
		En: "Invalid form structure",
	},
	"forms.invalid_request": {
		Th: "คำขอไม่ถูกต้อง",
		En: "Invalid request",
	},
	"forms.draft_exists": {
		Th: "มีฉบับร่างอยู่แล้ว",
		En: "A draft already exists",
	},
	// PLT-08 versioning & approval
	"versioning.invalid_transition": {
		Th: "ทำรายการนี้ไม่ได้ในสถานะปัจจุบันของเวอร์ชัน",
		En: "Not allowed in the version's current status",
	},
	"versioning.locked": {
		Th: "มีเวอร์ชันที่รอตรวจหรืออนุมัติแล้ว จึงแก้ไขไม่ได้",
		En: "A version is under review or approved; it can't be edited",
	},
	"versioning.self_approval": {
		Th: "ผู้จัดทำ ผู้ส่งขออนุมัติ หรือผู้อนุมัติขั้นก่อนหน้า อนุมัติเองไม่ได้",
		En: "The author, requester or an earlier approver can't approve",
	},
	"versioning.invalid_request": {
		Th: "คำขอไม่ถูกต้อง",
		En: "Invalid request",
	},
	// ORG-19 audit log
	"audit.export_too_large": {
		Th: "รายการมากเกินกว่าจะส่งออกในครั้งเดียว กรุณากรองให้แคบลง",
		En: "Too many entries to export at once; narrow the filter",
	},
	// PLT-05 workflow
	"workflow.invalid_transition": {
		Th: "เปลี่ยนสถานะนี้ไม่ได้ในขั้นตอนปัจจุบัน",
		En: "Transition not allowed from the current state",
	},
	"workflow.invalid_definition": {
		Th: "นิยาม workflow ไม่ถูกต้อง",
		En: "Invalid workflow definition",
	},
	"workflow.invalid_request": {
		Th: "คำขอไม่ถูกต้อง",
		En: "Invalid workflow request",
	},
	// ORG-01 / ORG-04 structure
	"org.invalid": {
		Th: "ข้อมูลไม่ถูกต้อง",
		En: "Invalid organization data",
	},
	"org.cycle": {
		Th: "ย้ายหน่วยงานไปอยู่ใต้ตัวเองไม่ได้",
		En: "A unit can't be placed under itself",
	},
	"org.has_active_units": {
		Th: "ต้องปิดหน่วยงานย่อยที่ยังเปิดอยู่ก่อน",
		En: "Close the active units below it first",
	},
	"org.logo_not_usable": {
		Th: "โลโก้ต้องเป็นไฟล์ PNG หรือ JPEG ที่คุณอัปโหลดและผ่านการตรวจไวรัสแล้ว",
		En: "The logo must be your own clean PNG or JPEG upload",
	},
	// ORG-07 master data
	"org.master_read_only": {
		Th: "ข้อมูลตั้งต้นของแพลตฟอร์มแก้ไขหรือลบไม่ได้",
		En: "Platform default master data can't be changed",
	},
	"org.master_in_use": {
		Th: "รายการนี้ถูกใช้งานอยู่ จึงลบไม่ได้",
		En: "This entry is in use",
	},
	// ORG-20 calendars
	"org.invalid_calendar": {
		Th: "ข้อมูลปฏิทินไม่ถูกต้อง (ชื่อ เขตเวลา หรือวันทำการ)",
		En: "Invalid calendar (name, time zone or workdays)",
	},
	"org.duplicate_calendar_name": {
		Th: "มีปฏิทินชื่อนี้อยู่แล้ว",
		En: "A calendar with this name exists",
	},
	// CON consent
	"consent.invalid_decisions": {
		Th: "ข้อมูลการให้ความยินยอมไม่ถูกต้อง",
		En: "Invalid consent decisions",
	},
	"consent.publish_checks": {
		Th: "ยังไม่ผ่านรายการตรวจก่อนเผยแพร่",
		En: "Publish checks failed",
	},
	"consent.invalid_transition": {
		Th: "เปลี่ยนสถานะความยินยอมนี้ไม่ได้",
		En: "Invalid consent status change",
	},
	"consent.subject_conflict": {
		Th: "ตัวระบุที่ส่งมาเป็นของเจ้าของข้อมูลต่างคนกัน",
		En: "Identifiers belong to different data subjects",
	},
	"consent.in_use": {
		Th: "รายการนี้ถูกใช้งานอยู่",
		En: "This item is in use",
	},
	"consent.invalid_request": {
		Th: "ข้อมูลไม่ถูกต้อง",
		En: "Invalid consent request",
	},
	// PLT-16 document composer
	"docs.incomplete": {
		Th: "เอกสารยังไม่ครบ (merge field หรือข้อความสัญญาที่ยังไม่มีค่า)",
		En: "The document is incomplete (merge fields or clauses without a value)",
	},
	"docs.unknown_type": {
		Th: "ยังไม่มีโมดูลที่ใช้เอกสารประเภทนี้",
		En: "No module offers this document type",
	},
	"docs.invalid": {
		Th: "ข้อมูลเอกสารไม่ถูกต้อง",
		En: "Invalid document data",
	},
	"docs.invalid_state": {
		Th: "ทำรายการนี้ในสถานะปัจจุบันไม่ได้",
		En: "Not allowed in the current state",
	},
	"docs.code_taken": {
		Th: "รหัสนี้ถูกใช้แล้ว",
		En: "Code already used",
	},
	"docs.no_renderer": {
		Th: "ยังไม่ได้ตั้งค่าการสร้างไฟล์ PDF",
		En: "PDF rendering is not configured",
	},
	// BRE breach
	"breach.invalid": {
		Th: "ข้อมูลเหตุละเมิดไม่ถูกต้อง",
		En: "Invalid breach data",
	},
	"breach.self_approval": {
		Th: "ผู้จัดทำหนังสือแจ้งอนุมัติเองไม่ได้",
		En: "The maker of a notice can't approve it",
	},
	"breach.invalid_transition": {
		Th: "เปลี่ยนสถานะเหตุละเมิดนี้ไม่ได้",
		En: "Invalid breach status change",
	},
	"breach.closed": {
		Th: "เหตุนี้ปิดแล้ว แก้ไขไม่ได้",
		En: "The incident is closed",
	},
	"breach.no_assessment": {
		Th: "ต้องประเมินความเสี่ยงก่อน",
		En: "Assess the risk first",
	},
	"breach.pdpc_notice_missing": {
		Th: "ยังไม่ได้บันทึกการแจ้ง สคส.",
		En: "The PDPC notice has not been recorded",
	},
	"breach.subject_notice_missing": {
		Th: "ต้องแจ้งเจ้าของข้อมูลให้เสร็จก่อน",
		En: "The data subject notice has not finished sending",
	},
	"breach.bad_document": {
		Th: "ต้องเป็นแบบแจ้ง สคส. ที่เผยแพร่แล้ว",
		En: "Not a published PDPC notification form",
	},
	"breach.notice_not_required": {
		Th: "เหตุนี้ไม่ได้ตัดสินให้แจ้งเจ้าของข้อมูล",
		En: "Data subjects are not to be notified for this incident",
	},
	"breach.decision_too_weak": {
		Th: "การตัดสินต้องไม่น้อยกว่าที่ระดับความเสี่ยงกำหนด",
		En: "The decision is weaker than the assessed risk requires",
	},
	"breach.bad_form": {
		Th: "แบบประเมินต้องเป็นแบบประเมินเหตุละเมิดที่เผยแพร่แล้ว และมีระดับ none / low / high",
		En: "Not a published breach assessment form with none/low/high bands",
	},
	"breach.file_not_usable": {
		Th: "ใช้ไฟล์นี้ไม่ได้ (ไม่พบ ไม่ใช่ไฟล์ของคุณ ยังไม่ผ่านการตรวจไวรัส หรือถูกใช้แล้ว)",
		En: "File not found, not yours, not clean or already used",
	},
	// PLT-14 import
	"import.invalid_mapping": {
		Th: "การจับคู่คอลัมน์ไม่ถูกต้อง",
		En: "Invalid column mapping",
	},
	"import.file_not_usable": {
		Th: "ใช้ไฟล์นี้นำเข้าไม่ได้ (ไม่พบ ไม่ใช่ไฟล์ของคุณ หรือถูกใช้แล้ว)",
		En: "File not found, not yours, or already used",
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
