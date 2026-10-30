package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func (h *Strict) DsarListAgents(ctx context.Context, req DsarListAgentsRequestObject) (DsarListAgentsResponseObject, error) {
	agents, err := h.svc.ListAgents(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListAgents200JSONResponse{Data: make([]DsarAgent, 0, len(agents))}
	for _, a := range agents {
		resp.Data = append(resp.Data, toAgentWire(a))
	}
	return resp, nil
}

func (h *Strict) DsarRecordAgent(ctx context.Context, req DsarRecordAgentRequestObject) (DsarRecordAgentResponseObject, error) {
	b := *req.Body
	a, err := h.svc.RecordAgent(ctx, req.Id, dsarservice.RecordAgentInput{
		AgentName: b.AgentName, AuthorityType: string(b.AuthorityType), AuthorityFileID: b.AuthorityFileId,
	})
	if err != nil {
		return nil, problem(err)
	}
	return DsarRecordAgent201JSONResponse(toAgentWire(a)), nil
}

func (h *Strict) DsarVerifyAgent(ctx context.Context, req DsarVerifyAgentRequestObject) (DsarVerifyAgentResponseObject, error) {
	a, err := h.svc.VerifyAgent(ctx, req.Id, req.AgentId)
	if err != nil {
		return nil, problem(err)
	}
	return DsarVerifyAgent200JSONResponse(toAgentWire(a)), nil
}

func toAgentWire(a dsarservice.Agent) DsarAgent {
	w := DsarAgent{Id: a.ID, RequestId: a.RequestID, AuthorityType: DsarAuthorityType(a.AuthorityType),
		AuthorityFileId: a.AuthorityFileID, CreatedAt: a.CreatedAt.UTC(), RowVersion: int(a.RowVersion)}
	if a.VerifiedAt != nil {
		t := a.VerifiedAt.UTC()
		w.VerifiedAt = &t
	}
	return w
}
