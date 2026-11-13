package agreementhttp

import (
	"context"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// AgreementListAnnexes is DSA-07's read side — whatever's currently attached to this agreement.
func (h *Strict) AgreementListAnnexes(ctx context.Context, req AgreementListAnnexesRequestObject) (AgreementListAnnexesResponseObject, error) {
	as, err := h.svc.ListAnnexes(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	out := make([]AgreementAnnex, 0, len(as))
	for _, a := range as {
		out = append(out, toAnnexWire(a))
	}
	return AgreementListAnnexes200JSONResponse{Data: out}, nil
}

// AgreementAttachFlowDiagramAnnex is DSA-07's own acceptance criterion: render the latest data-flow
// diagram and attach it, replacing whatever flow-diagram annex was there before.
func (h *Strict) AgreementAttachFlowDiagramAnnex(ctx context.Context, req AgreementAttachFlowDiagramAnnexRequestObject) (AgreementAttachFlowDiagramAnnexResponseObject, error) {
	a, err := h.svc.AttachFlowDiagramAnnex(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementAttachFlowDiagramAnnex201JSONResponse(toAnnexWire(a)), nil
}

func toAnnexWire(a agreementservice.Annex) AgreementAnnex {
	return AgreementAnnex{Id: a.ID, AgreementId: a.AgreementID, AnnexType: AgreementAnnexAnnexType(a.AnnexType),
		FileId: a.FileID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, RowVersion: int(a.RowVersion)}
}
