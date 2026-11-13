package service

import (
	"strings"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
)

// ORG-03 (ม.37(5)): the notice's contact section names the legal entity's local representative only when
// one is on record, in both languages.
func TestContactSection_RepresentativeShownWhenSet(t *testing.T) {
	without := contactSection(orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด"})
	for _, body := range [][]string{without.bodyTh, without.bodyEn} {
		for _, p := range body {
			if strings.Contains(p, "ผู้แทน") || strings.Contains(p, "representative") {
				t.Errorf("no representative on record, but body mentions one: %q", p)
			}
		}
	}

	with := contactSection(orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด",
		Representative: &orgservice.Representative{NameTh: "นายผู้แทน ใจดี", NameEn: "Mr. Representative"}})
	joinedTh, joinedEn := strings.Join(with.bodyTh, " "), strings.Join(with.bodyEn, " ")
	if !strings.Contains(joinedTh, mergeFieldPlaceholder("representative_name_th")) {
		t.Errorf("th body missing representative merge field: %q", joinedTh)
	}
	if !strings.Contains(joinedEn, mergeFieldPlaceholder("representative_name_en")) {
		t.Errorf("en body missing representative merge field: %q", joinedEn)
	}
	if len(with.bodyTh) != len(without.bodyTh)+1 || len(with.bodyEn) != len(without.bodyEn)+1 {
		t.Errorf("expected exactly one extra paragraph per language when set")
	}
}
