// Package connectorshttp holds PLT-20's own admin endpoints. Types in connectors.gen.go are generated
// from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml / make gen). The handler only
// translates between the wire format and internal/platform/connectors.
package connectorshttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/connectors"
)

type Strict struct {
	svc *connectors.Service
}

func NewStrict(svc *connectors.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListConnectorTypes(ctx context.Context, req PlatformListConnectorTypesRequestObject) (PlatformListConnectorTypesResponseObject, error) {
	types := h.svc.ListTypes()
	resp := PlatformListConnectorTypes200JSONResponse{Data: make([]ConnectorType, 0, len(types))}
	for _, t := range types {
		resp.Data = append(resp.Data, ConnectorType{Code: t.Code, Capabilities: toCapabilities(t.Capabilities)})
	}
	return resp, nil
}

func (h *Strict) PlatformListConnectors(ctx context.Context, req PlatformListConnectorsRequestObject) (PlatformListConnectorsResponseObject, error) {
	f := connectors.Filter{}
	if req.Params.ConnectorType != nil {
		f.ConnectorType = *req.Params.ConnectorType
	}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.Cursor = &c
	}
	list, next, err := h.svc.ListConnectors(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := PlatformListConnectors200JSONResponse{Data: make([]Connector, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toConnectorWire(c))
	}
	if next != nil {
		s := encodeCursor(*next)
		resp.NextCursor = &s
	}
	return resp, nil
}

func (h *Strict) PlatformCreateConnector(ctx context.Context, req PlatformCreateConnectorRequestObject) (PlatformCreateConnectorResponseObject, error) {
	c, err := h.svc.CreateConnector(ctx, toInput(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return PlatformCreateConnector201JSONResponse{Body: toConnectorWire(c), Headers: PlatformCreateConnector201ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func (h *Strict) PlatformGetConnector(ctx context.Context, req PlatformGetConnectorRequestObject) (PlatformGetConnectorResponseObject, error) {
	c, err := h.svc.GetConnector(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformGetConnector200JSONResponse{Body: toConnectorWire(c), Headers: PlatformGetConnector200ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func (h *Strict) PlatformUpdateConnector(ctx context.Context, req PlatformUpdateConnectorRequestObject) (PlatformUpdateConnectorResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c, err := h.svc.UpdateConnector(ctx, req.Id, toInput(*req.Body), rv)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformUpdateConnector200JSONResponse{Body: toConnectorWire(c), Headers: PlatformUpdateConnector200ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func (h *Strict) PlatformDeleteConnector(ctx context.Context, req PlatformDeleteConnectorRequestObject) (PlatformDeleteConnectorResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.DeleteConnector(ctx, req.Id, rv); err != nil {
		return nil, problem(err)
	}
	return PlatformDeleteConnector204Response{}, nil
}

func (h *Strict) PlatformTestConnector(ctx context.Context, req PlatformTestConnectorRequestObject) (PlatformTestConnectorResponseObject, error) {
	ok, msg, testedAt, c, err := h.svc.TestConnection(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	body := ConnectorTestResult{Ok: ok, TestedAt: testedAt.UTC()}
	if msg != "" {
		body.Message = &msg
	}
	return PlatformTestConnector200JSONResponse{Body: body, Headers: PlatformTestConnector200ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func toInput(in ConnectorInput) connectors.Input {
	out := connectors.Input{Name: in.Name, ConnectorType: in.ConnectorType, Secret: in.Secret}
	if in.AssetId != nil {
		id := uuid.UUID(*in.AssetId)
		out.AssetID = &id
	}
	if in.Config != nil {
		b, _ := json.Marshal(*in.Config)
		out.Config = json.RawMessage(b)
	}
	if in.Capabilities != nil {
		caps := make([]string, 0, len(*in.Capabilities))
		for _, c := range *in.Capabilities {
			caps = append(caps, string(c))
		}
		out.Capabilities = caps
	}
	return out
}

func toConnectorWire(c connectors.Connector) Connector {
	w := Connector{
		Id:            c.ID,
		ConnectorType: c.ConnectorType,
		Name:          c.Name,
		HasSecret:     c.HasSecret,
		Capabilities:  toCapabilities(c.Capabilities),
		Status:        ConnectorStatus(c.Status),
		RowVersion:    int(c.RowVersion),
		UpdatedAt:     c.UpdatedAt.UTC(),
	}
	if c.AssetID != nil {
		id := *c.AssetID
		w.AssetId = &id
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal(c.Config, &cfg)
	w.Config = cfg
	if c.LastTestedAt != nil {
		t := c.LastTestedAt.UTC()
		w.LastTestedAt = &t
	}
	return w
}

func toCapabilities(ss []string) []ConnectorCapability {
	out := make([]ConnectorCapability, 0, len(ss))
	for _, s := range ss {
		out = append(out, ConnectorCapability(s))
	}
	return out
}

func etag(v int32) *string {
	s := `"` + strconv.Itoa(int(v)) + `"`
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func encodeCursor(c connectors.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeCursor(s string) (connectors.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return connectors.Cursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return connectors.Cursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return connectors.Cursor{}, err
	}
	u, err := uuid.Parse(id)
	return connectors.Cursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, connectors.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, connectors.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, connectors.ErrInvalid):
		return httpx.UnprocessableEntity("platform.invalid_input", err.Error())
	case errors.Is(err, connectors.ErrUnsupported):
		return httpx.UnprocessableEntity("platform.connector_unsupported", err.Error())
	}
	return err
}
