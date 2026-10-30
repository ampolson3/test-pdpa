package wiring

import (
	"context"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	dataflowservice "pdpa-platform/internal/dataflow/service"
)

// agreementDataflow adapts *dataflow/service.Service to agreementservice.Dataflow — dataflow/service
// already imports agreement/service (DFG-04/DSA-12's own missing-agreement warnings call
// agreement.ListAgreements), so agreement/service can't import it back; this is the glue, built where
// both concrete packages are already importable, converting the plain strings DSA-07's own call uses
// into dataflowservice's own typed ExportFormat/Paper.
type agreementDataflow struct{ svc *dataflowservice.Service }

func (a agreementDataflow) ExportDiagram(ctx context.Context, legalEntityID *uuid.UUID, format, paper string) ([]byte, string, error) {
	return a.svc.ExportDiagram(ctx, legalEntityID, dataflowservice.ExportFormat(format), dataflowservice.Paper(paper))
}

// AgreementDataflow builds the Dataflow adapter DSA-07's AttachFlowDiagramAnnex needs.
func AgreementDataflow(svc *dataflowservice.Service) agreementservice.Dataflow {
	return agreementDataflow{svc: svc}
}
