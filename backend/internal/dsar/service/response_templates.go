package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs/render"
)

// ResponseTemplateOverride is one tenant-authored response letter, under a code the DPO picked themselves
// (never one of the global purpose codes result/rejection/request_info — DSAR-20's own `response_code_*`
// columns on a request type point here by that code to opt into it, otherwise the shared default applies).
type ResponseTemplateOverride struct {
	ID       uuid.UUID
	Code     string
	Name     string
	Language string
	Content  render.Node
}

// SaveResponseTemplateOverride is DSAR-20's own "template ตอบกลับ" write: upserts one language of one
// tenant-owned code — a second save of the same (code, language) replaces the content in place, since this
// is internal reference content a DPO edits freely, not a versioned legal document (rule 8's DRAFT/approval
// gate applies to notices/letters sent to a data subject or regulator; this is only ever the starting text a
// generated letter — itself still a draft PLT-16 document needing DPO approval before it's ever sent — is
// built from).
func (s *Service) SaveResponseTemplateOverride(ctx context.Context, code, language string, content render.Node) (ResponseTemplateOverride, error) {
	code = strings.TrimSpace(code)
	if code == "" || code == "result" || code == "rejection" || code == "request_info" {
		return ResponseTemplateOverride{}, fmt.Errorf("%w: code", ErrInvalid)
	}
	if language != "th" && language != "en" {
		return ResponseTemplateOverride{}, fmt.Errorf("%w: language", ErrInvalid)
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return ResponseTemplateOverride{}, err
	}
	if err := (render.Content{"th": content}).Validate(); err != nil {
		return ResponseTemplateOverride{}, fmt.Errorf("%w: content", ErrInvalid)
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).UpsertResponseTemplateOverride(ctx, dsarstore.UpsertResponseTemplateOverrideParams{
		Code: code, Name: code, Language: language, Content: raw,
	})
	if err != nil {
		return ResponseTemplateOverride{}, err
	}
	return toResponseTemplateOverride(row), s.audit(ctx, "dsar.response_template.save", row.ID, nil, map[string]any{"code": code, "language": language})
}

// ListResponseTemplateOverrides lists this tenant's own custom codes (never the global defaults — those
// are read-only via the existing shared-purpose lookup every letter already uses).
func (s *Service) ListResponseTemplateOverrides(ctx context.Context) ([]ResponseTemplateOverride, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListResponseTemplateOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ResponseTemplateOverride, 0, len(rows))
	for _, r := range rows {
		out = append(out, toResponseTemplateOverride(dsarstore.UpsertResponseTemplateOverrideRow(r)))
	}
	return out, nil
}

func toResponseTemplateOverride(r dsarstore.UpsertResponseTemplateOverrideRow) ResponseTemplateOverride {
	out := ResponseTemplateOverride{ID: r.ID, Code: r.Code, Name: r.Name, Language: r.Language}
	_ = json.Unmarshal(r.Content, &out.Content)
	return out
}
