package docs

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Text is a label in both UI languages.
type Text struct {
	Th string `json:"th"`
	En string `json:"en"`
}

// Field is a merge field the editor offers.
type Field struct {
	Key    string
	Source string // org | document
	Label  Text
}

// Fields is the merge field catalog. The organization's come from the document's legal entity (ORG-01); RoPA and
// vendor fields join when those modules exist (their records will be the document's entity_type / entity_id).
var Fields = []Field{
	{"org_name_th", "org", Text{"ชื่อองค์กร (ไทย)", "Organization name (Thai)"}},
	{"org_name_en", "org", Text{"ชื่อองค์กร (อังกฤษ)", "Organization name (English)"}},
	{"org_registration_no", "org", Text{"เลขทะเบียนนิติบุคคล", "Registration number"}},
	{"org_tax_id", "org", Text{"เลขประจำตัวผู้เสียภาษี", "Tax ID"}},
	{"org_address", "org", Text{"ที่อยู่องค์กร", "Organization address"}},
	{"org_email", "org", Text{"อีเมลติดต่อ", "Contact e-mail"}},
	{"org_phone", "org", Text{"โทรศัพท์ติดต่อ", "Contact phone"}},
	{"doc_title", "document", Text{"ชื่อเอกสาร", "Document title"}},
	{"doc_version", "document", Text{"เลขเวอร์ชัน", "Version number"}},
	{"doc_effective_date", "document", Text{"วันที่มีผล", "Effective date"}},
}

func knownField(key string) bool {
	for _, f := range Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

var (
	thaiMonths = []string{"มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน", "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}
)

// FormatDate writes a calendar date the way a document in that language states it: Thai documents use the
// Buddhist era ("1 มกราคม 2569"), English ones the Gregorian year ("1 January 2026"). The date is a calendar day
// (effective_from), so no time zone conversion applies.
func FormatDate(d time.Time, lang string) string {
	if lang == "th" {
		return fmt.Sprintf("%d %s %d", d.Day(), thaiMonths[d.Month()-1], d.Year()+543)
	}
	return d.Format("2 January 2006")
}

// fieldValues resolves every merge field a document can use, per language. Missing sources (no legal entity chosen,
// no effective date) leave their keys out, so a draft shows them as [key] and publishing refuses them.
func (s *Service) fieldValues(ctx context.Context, title string, legalEntityID *uuid.UUID, versionNo int32, effective *time.Time) (map[string]map[string]string, error) {
	base := map[string]string{"doc_title": title}
	if versionNo > 0 {
		base["doc_version"] = fmt.Sprint(versionNo)
	}
	if legalEntityID != nil && s.Org != nil {
		org, err := s.Org.MergeFields(ctx, *legalEntityID)
		if err != nil {
			return nil, err
		}
		for k, v := range org {
			if v != "" {
				base[k] = v
			}
		}
	}
	out := map[string]map[string]string{}
	for _, lang := range []string{"th", "en"} {
		m := make(map[string]string, len(base)+1)
		for k, v := range base {
			m[k] = v
		}
		if effective != nil {
			m["doc_effective_date"] = FormatDate(*effective, lang)
		}
		out[lang] = m
	}
	return out, nil
}
