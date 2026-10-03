package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
)

var purposeTitleTh = map[string]string{"result": "ผลการพิจารณา", "rejection": "ผลการพิจารณา (ปฏิเสธ)", "request_info": "ขอข้อมูลเพิ่มเติม"}

// generateResponseLetter is DSAR-13's own logic: a response letter, TH/EN, generated automatically from the
// request's type and the outcome/status that triggered it (result | rejection | request_info) — the module
// doc's literal acceptance criterion. The letter is a PLT-16 document draft (doc_type "dsar_letter", already
// registered with a DPO approval step in internal/wiring.Docs): DPO reviews and edits it before it is ever
// sent ("เลือก template + แก้ก่อนส่ง" — choosing when to actually publish/send it is a later step this
// feature doesn't own).
//
// Bodies are seeded per *purpose* only (migration 00043), not per (type × purpose): {{request_type_name}} is
// substituted with this request's own type name at generation time, so the one result/rejection/request_info
// body correctly reads as being about "การเข้าถึงข้อมูล" for an access request and "การลบข้อมูล" for an
// erasure request without 27 near-duplicate template bodies to keep in sync.
func (s *Service) generateResponseLetter(ctx context.Context, req Request, rt RequestType, purpose string) (uuid.UUID, error) {
	tpl, err := templateContent(ctx, purpose)
	if err != nil {
		return uuid.Nil, err
	}
	if len(tpl) == 0 {
		return uuid.Nil, fmt.Errorf("dsar: no published %s response template", purpose)
	}

	nameEnc, err := s.decryptRequesterName(ctx, req)
	if err != nil {
		return uuid.Nil, err
	}

	varsTh := map[string]string{"requester_name": nameEnc, "request_no": req.RequestNo, "request_type_name": rt.NameTh, "rejection_reason": deref(req.RejectionReasonCode)}
	varsEn := map[string]string{"requester_name": nameEnc, "request_no": req.RequestNo, "request_type_name": rt.NameEn, "rejection_reason": deref(req.RejectionReasonCode)}

	content := render.Content{}
	if n, ok := tpl["th"]; ok {
		content["th"] = substitute(n, varsTh)
	}
	if n, ok := tpl["en"]; ok {
		content["en"] = substitute(n, varsEn)
	}

	title := fmt.Sprintf("หนังสือตอบกลับคำขอ %s — %s", req.RequestNo, purposeTitleTh[purpose])
	doc, err := s.Docs.Create(ctx, docsservice.CreateInput{DocType: "dsar_letter", Title: title, LegalEntityID: &req.LegalEntityID})
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := s.Docs.SaveDraft(ctx, doc.ID, doc.RowVersion, docsservice.Draft{Title: title, LegalEntityID: &req.LegalEntityID, Content: content}); err != nil {
		return uuid.Nil, err
	}
	if err := s.audit(ctx, "dsar.request.letter_generated", req.ID, nil, map[string]any{"purpose": purpose, "document_id": doc.ID}); err != nil {
		return uuid.Nil, err
	}
	return doc.ID, nil
}

func (s *Service) decryptRequesterName(ctx context.Context, req Request) (string, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetRequestRequesterName(ctx, req.ID)
	if err != nil {
		return "", err
	}
	plain, err := s.Keyring.Decrypt(ctx, cryptoClass, requesterNameContext, row)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func templateContent(ctx context.Context, purpose string) (render.Content, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetResponseTemplateContent(ctx, purpose)
	if err != nil {
		return nil, err
	}
	out := make(render.Content, len(rows))
	for _, r := range rows {
		var node render.Node
		if err := json.Unmarshal(r.Content, &node); err != nil {
			return nil, err
		}
		out[r.Language] = node
	}
	return out, nil
}

// substitute replaces {{key}} placeholders in every text node's literal text with vars[key] — plain string
// substitution, not PLT-16's mergeField mechanism (which only resolves org/DPO fields live at every render;
// these values are specific to this one immutable letter instance, so they're baked in as plain text once,
// at generation time).
func substitute(n render.Node, vars map[string]string) render.Node {
	if n.Type == "text" && n.Text != "" {
		text := n.Text
		for k, v := range vars {
			text = strings.ReplaceAll(text, "{{"+k+"}}", v)
		}
		n.Text = text
	}
	if len(n.Content) > 0 {
		content := make([]render.Node, len(n.Content))
		for i, c := range n.Content {
			content[i] = substitute(c, vars)
		}
		n.Content = content
	}
	return n
}
