package i18n

// messages are fixed strings the backend itself puts in front of people outside the admin app — e.g. the lines
// of a consent receipt e-mail. Keys are stable; Message falls back to Thai, then to the key.
var messages = map[string]map[Lang]string{
	// CON-15 receipt e-mail: one line per decision
	"consent.decision.CONSENTED":           {Th: "ยินยอม", En: "Consented"},
	"consent.decision.NOT_CONSENTED":       {Th: "ไม่ยินยอม", En: "Not consented"},
	"consent.decision.WITHDRAWN":           {Th: "ถอนความยินยอม", En: "Withdrawn"},
	"consent.decision.EXTENDED":            {Th: "ยืนยันความยินยอม", En: "Consent confirmed"},
	"consent.decision.CHANGED_PREFERENCES": {Th: "เปลี่ยนตัวเลือก", En: "Preferences changed"},
}

// Message returns the text of key in lang.
func Message(key string, lang Lang) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	if s, ok := m[lang]; ok && s != "" {
		return s
	}
	if s, ok := m[Th]; ok {
		return s
	}
	return key
}
