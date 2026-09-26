package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/docs/render"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// section is one ม.23 topic, in both languages.
type section struct {
	headingTh, headingEn string
	bodyTh, bodyEn       []string // one paragraph per string; a placeholder if nothing could be derived
}

var rightsBoilerplate = section{
	headingTh: "สิทธิของเจ้าของข้อมูลส่วนบุคคล",
	headingEn: "Your rights as a data subject",
	bodyTh: []string{
		"ท่านมีสิทธิขอเข้าถึง ขอสำเนา ขอแก้ไข ขอให้โอนย้าย ขอให้ระงับการใช้ ขอให้ลบหรือทำลาย และคัดค้านการประมวลผลข้อมูลส่วนบุคคลของท่าน รวมถึงสิทธิขอถอนความยินยอม ตามที่กฎหมายคุ้มครองข้อมูลส่วนบุคคลกำหนด",
	},
	bodyEn: []string{
		"You have the right to access, obtain a copy of, correct, transfer, restrict the processing of, erase or destroy, and object to the processing of your personal data, and to withdraw consent, as provided by the Personal Data Protection Act.",
	},
}

var consequenceSection = section{
	headingTh: "ผลกระทบหากไม่ให้ข้อมูลส่วนบุคคล",
	headingEn: "Consequences of not providing your personal data",
	bodyTh:    []string{"[โปรดระบุผลกระทบหากท่านไม่ให้ข้อมูลที่จำเป็นตามกฎหมายหรือสัญญา]"},
	bodyEn:    []string{"[Please state the consequence of not providing data required by law or contract]"},
}

// compose builds the wizard's first draft (BP-04 t1-t2): one ม.23 section per topic, filled in from every
// linked RoPA processing activity's purposes/lawful basis, data categories, retention, recipients and transfers,
// plus the legal entity's own contact details as merge fields — anything that can't be derived becomes a
// bracketed placeholder for the user to fill in during t3 (content editing stays the document composer's job).
func (s *Service) compose(ctx context.Context, legalEntity orgservice.LegalEntity, activityIDs []uuid.UUID) (render.Content, error) {
	bases, err := masterByCode(ctx, s.Org, "lawful_bases")
	if err != nil {
		return nil, err
	}
	countries, err := masterByCode(ctx, s.Org, "countries")
	if err != nil {
		return nil, err
	}

	var purposeSec, dataSec, retentionSec, recipientSec section
	purposeSec.headingTh, purposeSec.headingEn = "วัตถุประสงค์และฐานทางกฎหมายในการเก็บรวบรวม ใช้ หรือเปิดเผยข้อมูล", "Purposes and lawful basis for collecting, using or disclosing your data"
	dataSec.headingTh, dataSec.headingEn = "ข้อมูลส่วนบุคคลที่เก็บรวบรวมและระยะเวลาการเก็บรักษา", "Personal data collected and how long it is kept"
	retentionSec.headingTh, retentionSec.headingEn = "ระยะเวลาการเก็บรักษาข้อมูล", "Data retention period"
	recipientSec.headingTh, recipientSec.headingEn = "ผู้รับข้อมูลและการโอนข้อมูลไปต่างประเทศ", "Recipients of your data and international transfers"

	for _, id := range activityIDs {
		a, err := s.Ropa.GetActivity(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("%w: activity_ids", ErrInvalid)
		}
		purposes, err := s.Ropa.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range purposes {
			basis := bases[p.LawfulBasisCode]
			purposeSec.bodyTh = append(purposeSec.bodyTh, fmt.Sprintf("%s (ฐานทางกฎหมาย: %s)", p.PurposeText, basisName(basis, p.LawfulBasisCode, "th")))
			purposeSec.bodyEn = append(purposeSec.bodyEn, fmt.Sprintf("%s (lawful basis: %s)", p.PurposeText, basisName(basis, p.LawfulBasisCode, "en")))
		}

		data, err := s.Ropa.ListActivityData(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, d := range data {
			cat, err := s.Org.GetMaster(ctx, "data_categories", d.DataCategoryID)
			if err != nil {
				return nil, fmt.Errorf("%w: activity_ids", ErrInvalid)
			}
			dataSec.bodyTh = append(dataSec.bodyTh, cat.NameTh)
			dataSec.bodyEn = append(dataSec.bodyEn, orDefault(cat.NameEn, cat.NameTh))
		}

		rules, err := s.Ropa.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range rules {
			retentionSec.bodyTh = append(retentionSec.bodyTh, retentionText(r, "th"))
			retentionSec.bodyEn = append(retentionSec.bodyEn, retentionText(r, "en"))
		}

		recipients, err := s.Ropa.ListActivityRecipients(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, rc := range recipients {
			party, err := s.Org.GetExternalParty(ctx, rc.PartyID)
			if err != nil {
				return nil, fmt.Errorf("%w: activity_ids", ErrInvalid)
			}
			recipientSec.bodyTh = append(recipientSec.bodyTh, party.NameTh)
			recipientSec.bodyEn = append(recipientSec.bodyEn, orDefault(party.NameEn, party.NameTh))
		}

		transfers, err := s.Ropa.ListActivityTransfers(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range transfers {
			c := countries[t.CountryCode]
			recipientSec.bodyTh = append(recipientSec.bodyTh, fmt.Sprintf("โอนข้อมูลไปยังประเทศ %s (ฐาน: %s)", orDefault(c.NameTh, t.CountryCode), t.TransferBasis))
			recipientSec.bodyEn = append(recipientSec.bodyEn, fmt.Sprintf("Transferred to %s (basis: %s)", orDefault(c.NameEn, t.CountryCode), t.TransferBasis))
		}
	}

	if len(purposeSec.bodyTh) == 0 {
		purposeSec.bodyTh, purposeSec.bodyEn = []string{"[โปรดระบุวัตถุประสงค์และฐานทางกฎหมาย]"}, []string{"[Please state the purpose and lawful basis]"}
	}
	if len(dataSec.bodyTh) == 0 {
		dataSec.bodyTh, dataSec.bodyEn = []string{"[โปรดระบุประเภทข้อมูลที่เก็บรวบรวม]"}, []string{"[Please state the categories of data collected]"}
	}
	if len(retentionSec.bodyTh) == 0 {
		retentionSec.bodyTh, retentionSec.bodyEn = []string{"[โปรดระบุระยะเวลาการเก็บรักษาข้อมูล]"}, []string{"[Please state the retention period]"}
	}
	if len(recipientSec.bodyTh) == 0 {
		recipientSec.bodyTh, recipientSec.bodyEn = []string{"ไม่มีการเปิดเผยข้อมูลแก่บุคคลภายนอก เว้นแต่กฎหมายกำหนด"}, []string{"Your data is not disclosed to third parties except as required by law."}
	}

	sections := []section{purposeSec, consequenceSection, dataSec, retentionSec, recipientSec, contactSection(legalEntity), rightsBoilerplate}
	return render.Content{"th": docNode(sections, "th"), "en": docNode(sections, "en")}, nil
}

func contactSection(e orgservice.LegalEntity) section {
	return section{
		headingTh: "ข้อมูลติดต่อผู้ควบคุมข้อมูลส่วนบุคคลและเจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล (DPO)",
		headingEn: "Contact details of the data controller and Data Protection Officer (DPO)",
		bodyTh:    []string{mergeFieldPlaceholder("org_name_th") + " อีเมล " + mergeFieldPlaceholder("org_email") + " โทร " + mergeFieldPlaceholder("org_phone") + " ที่อยู่ " + mergeFieldPlaceholder("org_address")},
		bodyEn:    []string{mergeFieldPlaceholder("org_name_en") + " e-mail " + mergeFieldPlaceholder("org_email") + " phone " + mergeFieldPlaceholder("org_phone") + " address " + mergeFieldPlaceholder("org_address")},
	}
}

// mergeFieldPlaceholder is inlined as plain text and later split out by docNode into a real mergeField node —
// keeping section bodies as plain []string keeps the composer above simple; only docNode deals with render.Node.
func mergeFieldPlaceholder(key string) string { return "{{" + key + "}}" }

func docNode(sections []section, lang string) render.Node {
	var content []render.Node
	for _, sec := range sections {
		heading, body := sec.headingTh, sec.bodyTh
		if lang == "en" {
			heading, body = sec.headingEn, sec.bodyEn
		}
		content = append(content, render.Node{Type: "heading", Attrs: map[string]any{"level": float64(2)}, Content: []render.Node{{Type: "text", Text: heading}}})
		for _, p := range body {
			content = append(content, render.Node{Type: "paragraph", Content: inlineWithFields(p)})
		}
	}
	return render.Node{Type: "doc", Content: content}
}

// inlineWithFields splits a paragraph's plain text on {{merge_field_key}} placeholders into text and mergeField
// nodes.
func inlineWithFields(text string) []render.Node {
	var out []render.Node
	for len(text) > 0 {
		start := strings.Index(text, "{{")
		if start < 0 {
			out = append(out, render.Node{Type: "text", Text: text})
			return out
		}
		if start > 0 {
			out = append(out, render.Node{Type: "text", Text: text[:start]})
		}
		rest := text[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			out = append(out, render.Node{Type: "text", Text: text[start:]})
			return out
		}
		out = append(out, render.Node{Type: "mergeField", Attrs: map[string]any{"key": rest[:end]}})
		text = rest[end+2:]
	}
	if len(out) == 0 {
		return []render.Node{{Type: "text", Text: ""}}
	}
	return out
}

func masterByCode(ctx context.Context, org Org, kind string) (map[string]orgservice.MasterItem, error) {
	items, err := org.ListMaster(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make(map[string]orgservice.MasterItem, len(items))
	for _, it := range items {
		out[it.Code] = it
	}
	return out, nil
}

func basisName(it orgservice.MasterItem, code, lang string) string {
	if it.Code == "" {
		return code
	}
	if lang == "en" {
		return orDefault(it.NameEn, it.NameTh)
	}
	return it.NameTh
}

func retentionText(r ropaservice.RetentionRule, lang string) string {
	if r.RetentionMonths == nil {
		if lang == "en" {
			return fmt.Sprintf("Until %s, then %s", r.TriggerEvent, r.DisposalMethod)
		}
		return fmt.Sprintf("จนกว่าจะถึงเหตุการณ์: %s จากนั้นจะ%s", r.TriggerEvent, r.DisposalMethod)
	}
	if lang == "en" {
		return fmt.Sprintf("%d months, then %s", *r.RetentionMonths, r.DisposalMethod)
	}
	return fmt.Sprintf("%d เดือน จากนั้นจะ%s", *r.RetentionMonths, r.DisposalMethod)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
