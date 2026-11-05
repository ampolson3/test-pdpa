// Package breachpublichttp holds the /public/v1 incident-report-form endpoints (BRE-01): an external person
// reporting a breach with no login — the tenant comes from the public key (publickeys.Middleware), the same
// mechanism consent's own /public/v1 collection-point form, notice's own /public/v1 notice pages and dsar's
// own /public/v1 dsar-forms already use. Types in public.gen.go are generated from api/openapi/openapi.yaml
// by oapi-codegen (see oapi-codegen.yaml).
package breachpublichttp

import (
	"context"
	"errors"

	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

type Strict struct {
	svc *breach.Service
}

func NewStrict(svc *breach.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// reportForm is the key of this request, which must stand for a breach report form.
func reportForm(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityBreachForm {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func problem(err error) error {
	var ve *breach.ValidationError
	switch {
	case errors.As(err, &ve):
		p := httpx.UnprocessableEntity("breach.invalid", "Invalid request")
		for _, f := range ve.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Code: f.Code})
		}
		return p
	case errors.Is(err, breach.ErrNotFound):
		return httpx.NotFound()
	}
	return err
}

func (h *Strict) BreachGetPublicReportForm(ctx context.Context, _ BreachGetPublicReportFormRequestObject) (BreachGetPublicReportFormResponseObject, error) {
	k, err := reportForm(ctx)
	if err != nil {
		return nil, err
	}
	f, types, err := h.svc.PublicReportForm(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	out := BreachPublicReportForm{Id: f.ID, BreachTypes: make([]BreachPublicReportFormBreachTypes, 0, len(types))}
	for _, t := range types {
		out.BreachTypes = append(out.BreachTypes, BreachPublicReportFormBreachTypes(t))
	}
	return BreachGetPublicReportForm200JSONResponse(out), nil
}

func (h *Strict) BreachSubmitPublicReport(ctx context.Context, req BreachSubmitPublicReportRequestObject) (BreachSubmitPublicReportResponseObject, error) {
	k, err := reportForm(ctx)
	if err != nil {
		return nil, err
	}
	b := *req.Body
	in := breach.PublicReport{Title: b.Title, Description: b.Description, OccurredAt: b.OccurredAt}
	for _, t := range b.BreachTypes {
		in.BreachTypes = append(in.BreachTypes, string(t))
	}
	if b.ReporterContact != nil {
		in.ReporterContact = *b.ReporterContact
	}
	inc, err := h.svc.CreatePublicReport(ctx, k.EntityID, in)
	if err != nil {
		return nil, problem(err)
	}
	return BreachSubmitPublicReport201JSONResponse{IncidentNo: inc.No, Status: inc.Status}, nil
}
