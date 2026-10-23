// Package audithttp holds the ORG-19 audit-log endpoints. Types in audit.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package audithttp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/httpx"
	audit "pdpa-platform/internal/platform/audit/service"
)

type Strict struct {
	svc *audit.Service
}

func NewStrict(svc *audit.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformSearchAuditLog(ctx context.Context, req PlatformSearchAuditLogRequestObject) (PlatformSearchAuditLogResponseObject, error) {
	p := req.Params
	f := filter(p.ActorId, p.EntityType, p.EntityId, p.ActionPrefix, p.From, p.To, (*string)(p.Kind))
	cursor, limit := "", 50
	if p.Cursor != nil {
		cursor = *p.Cursor
	}
	if p.Limit != nil {
		limit = *p.Limit
	}
	entries, next, err := h.svc.Search(ctx, f, cursor, limit)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformSearchAuditLog200JSONResponse{Data: make([]AuditEntry, 0, len(entries))}
	for _, e := range entries {
		resp.Data = append(resp.Data, wire(e))
	}
	if next != "" {
		resp.NextCursor = &next
	}
	return resp, nil
}

func (h *Strict) PlatformExportAuditLog(ctx context.Context, req PlatformExportAuditLogRequestObject) (PlatformExportAuditLogResponseObject, error) {
	p := req.Params
	buf, _, err := h.svc.Export(ctx, filter(p.ActorId, p.EntityType, p.EntityId, p.ActionPrefix, p.From, p.To, (*string)(p.Kind)))
	if err != nil {
		return nil, problem(err)
	}
	name := fmt.Sprintf(`attachment; filename="audit-log-%s.csv"`, time.Now().UTC().Format("20060102-150405"))
	return PlatformExportAuditLog200TextcsvResponse{Body: buf, ContentLength: int64(buf.Len()), Headers: PlatformExportAuditLog200ResponseHeaders{ContentDisposition: &name}}, nil
}

func (h *Strict) PlatformVerifyAuditLog(ctx context.Context, _ PlatformVerifyAuditLogRequestObject) (PlatformVerifyAuditLogResponseObject, error) {
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return nil, httpx.AuthzDenied()
	}
	res, err := h.svc.Verify(ctx, tenant)
	if err != nil {
		return nil, err
	}
	out := PlatformVerifyAuditLog200JSONResponse{Ok: res.OK, Checked: res.Checked}
	if !res.OK {
		id := res.BrokenAt
		reason := PlatformVerifyAuditLog200JSONResponseBodyReason(res.Reason)
		out.BrokenAtId, out.Reason = &id, &reason
	}
	return out, nil
}

func filter(actor *uuid.UUID, entityType *string, entityID *uuid.UUID, prefix *string, from, to *time.Time, kind *string) audit.Filter {
	f := audit.Filter{ActorID: actor, EntityID: entityID, From: from, To: to}
	if entityType != nil {
		f.EntityType = *entityType
	}
	if prefix != nil {
		f.ActionPrefix = *prefix
	}
	if kind != nil {
		f.Kind = *kind
	}
	return f
}

func wire(e audit.LogEntry) AuditEntry {
	w := AuditEntry{Id: e.ID, OccurredAt: e.OccurredAt.UTC(), ActorType: AuditEntryActorType(e.ActorType), ActorId: e.ActorID, Action: e.Action,
		EntityId: e.EntityID, Before: e.Before, After: e.After}
	if e.ActorName != "" {
		w.ActorName = &e.ActorName
	}
	if e.EntityType != "" {
		w.EntityType = &e.EntityType
	}
	if e.EntityName != "" {
		w.EntityName = &e.EntityName
	}
	if e.IP != nil {
		ip := e.IP.String()
		w.Ip = &ip
	}
	if e.UserAgent != "" {
		w.UserAgent = &e.UserAgent
	}
	return w
}

func problem(err error) error {
	switch {
	case errors.Is(err, audit.ErrBadCursor), errors.Is(err, audit.ErrBadFilter):
		return httpx.RequestInvalid(err.Error())
	case errors.Is(err, audit.ErrExportTooLarge):
		return httpx.UnprocessableEntity("audit.export_too_large", err.Error())
	}
	return err
}
