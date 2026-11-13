package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// AnnexEntityType is the PLT-09 entity type a flow-diagram export attaches under.
const AnnexEntityType = "agreement_annex"

// AnnexTypeFlowDiagram is the only annex type this feature writes — the module doc's own other three
// (data_list, security_measures, request_form) have nothing generating their own content yet (data_list and
// security_measures are already satisfied live by DPA-04's own ProcessingSchedule read; request_form has no
// owning feature at all), so there is nothing for this pass to attach there without inventing content no
// screen has asked for.
const AnnexTypeFlowDiagram = "flow_diagram"

// Dataflow is what agreement reads from the dataflow module (rule 9) — dataflow/service already imports
// agreement/service directly (DFG-04/DSA-12's own missing-agreement warnings call agreement.ListAgreements),
// so agreement can't import it back; internal/wiring.AgreementDataflow adapts the real
// *dataflowservice.Service to this interface, string types only (no dataflowservice.ExportFormat/Paper import).
type Dataflow interface {
	ExportDiagram(ctx context.Context, legalEntityID *uuid.UUID, format, paper string) ([]byte, string, error)
}

// Annex is one of agreement.annexes' own rows.
type Annex struct {
	ID          uuid.UUID
	AgreementID uuid.UUID
	AnnexType   string
	FileID      *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	RowVersion  int32
}

func toAnnex(r agreementstore.ListAnnexesRow) Annex {
	return Annex{ID: r.ID, AgreementID: r.AgreementID, AnnexType: r.AnnexType, FileID: uuidPtr(r.FileID),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
}

// ListAnnexes is the read side — whatever's currently attached, by type.
func (s *Service) ListAnnexes(ctx context.Context, agreementID uuid.UUID) ([]Annex, error) {
	if _, err := s.GetAgreement(ctx, agreementID); err != nil {
		return nil, err
	}
	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListAnnexes(ctx, agreementID)
	if err != nil {
		return nil, err
	}
	out := make([]Annex, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAnnex(r))
	}
	return out, nil
}

// AttachFlowDiagramAnnex is DSA-07's own acceptance criterion literally ("ภาคผนวกแนบภาพแผนผังล่าสุดได้"):
// renders the live data-flow diagram (DFG-01/08, scoped to this agreement's own legal entity — read off the
// document DPA-02's own wizard composed it from) and attaches it as the agreement's flow_diagram annex.
// "Latest" means there is only ever one current image per agreement, not an accumulating history, so a
// repeat call replaces whatever was attached before (delete-then-insert in the same transaction, since
// agreement.annexes carries no unique constraint on (agreement_id, annex_type) to upsert against).
func (s *Service) AttachFlowDiagramAnnex(ctx context.Context, agreementID uuid.UUID) (Annex, error) {
	if s.Dataflow == nil {
		return Annex{}, fmt.Errorf("%w: no data-flow renderer configured", ErrInvalid)
	}
	ag, err := s.GetAgreement(ctx, agreementID)
	if err != nil {
		return Annex{}, err
	}
	doc, err := s.Docs.Get(ctx, ag.DocumentID)
	if err != nil {
		return Annex{}, err
	}
	png, _, err := s.Dataflow.ExportDiagram(ctx, doc.LegalEntityID, "png", "a4")
	if err != nil {
		return Annex{}, fmt.Errorf("agreement: render flow diagram: %w", err)
	}
	f, err := s.Files.SaveGenerated(ctx, "flow-diagram.png", bytes.NewReader(png), AnnexEntityType, agreementID)
	if err != nil {
		return Annex{}, fmt.Errorf("agreement: save flow diagram: %w", err)
	}
	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	if err := q.DeleteAnnexesByType(ctx, agreementstore.DeleteAnnexesByTypeParams{AgreementID: agreementID, AnnexType: AnnexTypeFlowDiagram}); err != nil {
		return Annex{}, err
	}
	row, err := q.InsertAnnex(ctx, agreementstore.InsertAnnexParams{AgreementID: agreementID, AnnexType: AnnexTypeFlowDiagram, FileID: pgUUID(&f.ID)})
	if err != nil {
		return Annex{}, err
	}
	annex := toAnnex(agreementstore.ListAnnexesRow(row))
	return annex, s.audit(ctx, "agreement.annex.attach_flow_diagram", agreementID, nil, map[string]any{"file_id": f.ID})
}
