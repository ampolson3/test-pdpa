package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
)

// DpoListDeadlines is DPO-05: near-deadline and overdue work from every module that already tracks a legal
// deadline, on one page.
func (h *Strict) DpoListDeadlines(ctx context.Context, req DpoListDeadlinesRequestObject) (DpoListDeadlinesResponseObject, error) {
	items, err := h.svc.Deadlines(ctx)
	if err != nil {
		return nil, err
	}
	resp := DpoListDeadlines200JSONResponse{Data: make([]DpoDeadlineItem, 0, len(items))}
	for _, it := range items {
		resp.Data = append(resp.Data, toDeadlineWire(it))
	}
	return resp, nil
}

func toDeadlineWire(it dposervice.DeadlineItem) DpoDeadlineItem {
	return DpoDeadlineItem{
		Source:         DpoDeadlineItemSource(it.Source),
		ReferenceId:    it.ReferenceID,
		ReferenceLabel: it.ReferenceLabel,
		DueAt:          it.DueAt,
		Status:         DpoDeadlineItemStatus(it.Status),
	}
}
