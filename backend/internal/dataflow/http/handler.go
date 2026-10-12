// Package dataflowhttp holds the dataflow module's admin endpoint (DFG-01/DFG-03: the RoPA-generated data
// flow diagram). Types in dataflow.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see
// oapi-codegen.yaml).
package dataflowhttp

import (
	"context"

	dataflowservice "pdpa-platform/internal/dataflow/service"
)

type Strict struct {
	svc *dataflowservice.Service
}

func NewStrict(svc *dataflowservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) DataflowGetGraph(ctx context.Context, req DataflowGetGraphRequestObject) (DataflowGetGraphResponseObject, error) {
	g, err := h.svc.Graph(ctx, req.Params.LegalEntityId)
	if err != nil {
		return nil, err
	}
	return DataflowGetGraph200JSONResponse(toGraphWire(g)), nil
}

func toGraphWire(g dataflowservice.Graph) DataflowGraph {
	w := DataflowGraph{ActivityCount: g.ActivityCount, GeneratedAt: g.GeneratedAt}
	for _, n := range g.Nodes {
		dn := DataflowNode{Id: n.ID, Type: DataflowNodeType(n.Type), Label: n.Label}
		if n.ParentID != "" {
			dn.ParentId = &n.ParentID
		}
		if n.ActivityCode != "" {
			dn.ActivityCode = &n.ActivityCode
		}
		if n.ActivityStatus != "" {
			dn.ActivityStatus = &n.ActivityStatus
		}
		if n.Role != "" {
			dn.Role = &n.Role
		}
		if len(n.Purposes) > 0 {
			dn.Purposes = &n.Purposes
		}
		if len(n.DataCategories) > 0 {
			dn.DataCategories = &n.DataCategories
		}
		if n.RecipientRole != "" {
			dn.RecipientRole = &n.RecipientRole
		}
		if n.DisclosureBasis != "" {
			dn.DisclosureBasis = &n.DisclosureBasis
		}
		if n.CountryCode != "" {
			dn.CountryCode = &n.CountryCode
		}
		if len(n.DisposalMethods) > 0 {
			dn.DisposalMethods = &n.DisposalMethods
		}
		w.Nodes = append(w.Nodes, dn)
	}
	for _, e := range g.Edges {
		de := DataflowEdge{Id: e.ID, Source: e.Source, Target: e.Target, Type: DataflowEdgeType(e.Type), Warning: e.Warning}
		if e.Label != "" {
			de.Label = &e.Label
		}
		w.Edges = append(w.Edges, de)
	}
	return w
}
