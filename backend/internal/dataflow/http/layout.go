// DFG-05: handlers for the drag-and-drop layout overlay (internal/dataflow/service/layout.go).
package dataflowhttp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DataflowGetLayout(ctx context.Context, req DataflowGetLayoutRequestObject) (DataflowGetLayoutResponseObject, error) {
	l, err := h.svc.GetLayout(ctx, req.Params.LegalEntityId, toView(req.Params.View))
	if err != nil {
		return nil, err
	}
	return DataflowGetLayout200JSONResponse{Body: toLayoutWire(l), Headers: DataflowGetLayout200ResponseHeaders{ETag: dfEtag(l.RowVersion)}}, nil
}

func (h *Strict) DataflowSaveLayout(ctx context.Context, req DataflowSaveLayoutRequestObject) (DataflowSaveLayoutResponseObject, error) {
	v, err := dfParseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	positions := map[string]dataflowservice.NodePosition{}
	notes := map[string]string{}
	if req.Body != nil {
		for _, p := range req.Body.Positions {
			positions[p.NodeId] = dataflowservice.NodePosition{X: float64(p.X), Y: float64(p.Y)}
		}
		for _, n := range req.Body.Notes {
			notes[n.NodeId] = n.Text
		}
	}
	l, err := h.svc.SaveLayout(ctx, req.Params.LegalEntityId, toView(req.Params.View), positions, notes, v)
	if err != nil {
		return nil, layoutProblem(err)
	}
	return DataflowSaveLayout200JSONResponse{Body: toLayoutWire(l), Headers: DataflowSaveLayout200ResponseHeaders{ETag: dfEtag(l.RowVersion)}}, nil
}

func (h *Strict) DataflowResetLayout(ctx context.Context, req DataflowResetLayoutRequestObject) (DataflowResetLayoutResponseObject, error) {
	if err := h.svc.ResetLayout(ctx, req.Params.LegalEntityId, toView(req.Params.View)); err != nil {
		return nil, err
	}
	return DataflowResetLayout204Response{}, nil
}

func layoutProblem(err error) error {
	if errors.Is(err, dataflowservice.ErrVersionMismatch) {
		return httpx.VersionMismatch()
	}
	return err
}

func toLayoutWire(l dataflowservice.Layout) DataflowLayout {
	w := DataflowLayout{RowVersion: int(l.RowVersion), Positions: []DataflowNodePosition{}, Notes: []DataflowNodeNote{}}
	for id, p := range l.Positions {
		w.Positions = append(w.Positions, DataflowNodePosition{NodeId: id, X: float32(p.X), Y: float32(p.Y)})
	}
	for id, text := range l.Notes {
		w.Notes = append(w.Notes, DataflowNodeNote{NodeId: id, Text: text})
	}
	if l.RowVersion > 0 {
		t := l.UpdatedAt.Time.UTC()
		w.UpdatedAt = &t
	}
	return w
}

func dfEtag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func dfParseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}
