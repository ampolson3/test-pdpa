// Package notifyhttp holds the PLT-04 endpoints: template management with preview, the delivery log,
// and the signed-in user's in-app inbox. Types in notify.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml). The SSE stream is stream.go.
package notifyhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/notify"
)

type Strict struct {
	svc *notify.Service
}

func NewStrict(svc *notify.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListNotificationTemplates(ctx context.Context, _ PlatformListNotificationTemplatesRequestObject) (PlatformListNotificationTemplatesResponseObject, error) {
	list, err := h.svc.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	out := PlatformListNotificationTemplates200JSONResponse{Data: make([]NotificationTemplate, 0, len(list))}
	for _, t := range list {
		out.Data = append(out.Data, toTemplate(t))
	}
	return out, nil
}

func (h *Strict) PlatformCreateNotificationTemplate(ctx context.Context, req PlatformCreateNotificationTemplateRequestObject) (PlatformCreateNotificationTemplateResponseObject, error) {
	b := req.Body
	t, err := h.svc.CreateTemplate(ctx, notify.TemplateInput{
		Code: b.Code, Channel: string(b.Channel), Language: string(b.Language), Subject: deref(b.Subject), Body: b.Body, Variables: derefSlice(b.Variables),
	})
	if err != nil {
		return nil, problem(err)
	}
	return PlatformCreateNotificationTemplate201JSONResponse{
		Body: toTemplate(t), Headers: PlatformCreateNotificationTemplate201ResponseHeaders{ETag: ptr(etag(t.RowVersion))},
	}, nil
}

func (h *Strict) PlatformPreviewNotificationTemplate(_ context.Context, req PlatformPreviewNotificationTemplateRequestObject) (PlatformPreviewNotificationTemplateResponseObject, error) {
	b := req.Body
	vars := map[string]any{}
	if b.Values != nil {
		for k, v := range *b.Values {
			vars[k] = v
		}
	}
	r, err := notify.Preview(notify.TemplateInput{Subject: deref(b.Subject), Body: b.Body, Variables: derefSlice(b.Variables)}, vars)
	if err != nil {
		return nil, problem(err)
	}
	out := PlatformPreviewNotificationTemplate200JSONResponse{Body: r.Body}
	if r.Subject != "" {
		out.Subject = &r.Subject
	}
	return out, nil
}

func (h *Strict) PlatformGetNotificationTemplate(ctx context.Context, req PlatformGetNotificationTemplateRequestObject) (PlatformGetNotificationTemplateResponseObject, error) {
	t, err := h.svc.GetTemplate(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformGetNotificationTemplate200JSONResponse{
		Body: toTemplate(t), Headers: PlatformGetNotificationTemplate200ResponseHeaders{ETag: ptr(etag(t.RowVersion))},
	}, nil
}

func (h *Strict) PlatformUpdateNotificationTemplate(ctx context.Context, req PlatformUpdateNotificationTemplateRequestObject) (PlatformUpdateNotificationTemplateResponseObject, error) {
	version, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := req.Body
	t, err := h.svc.UpdateTemplate(ctx, req.Id, version, notify.TemplateInput{Subject: deref(b.Subject), Body: b.Body, Variables: derefSlice(b.Variables)})
	if err != nil {
		return nil, problem(err)
	}
	return PlatformUpdateNotificationTemplate200JSONResponse{
		Body: toTemplate(t), Headers: PlatformUpdateNotificationTemplate200ResponseHeaders{ETag: ptr(etag(t.RowVersion))},
	}, nil
}

func (h *Strict) PlatformDeleteNotificationTemplate(ctx context.Context, req PlatformDeleteNotificationTemplateRequestObject) (PlatformDeleteNotificationTemplateResponseObject, error) {
	version, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.DeleteTemplate(ctx, req.Id, version); err != nil {
		return nil, problem(err)
	}
	return PlatformDeleteNotificationTemplate204Response{}, nil
}

func (h *Strict) PlatformListNotifications(ctx context.Context, req PlatformListNotificationsRequestObject) (PlatformListNotificationsResponseObject, error) {
	f := notify.ListFilter{}
	if p := req.Params; p.Status != nil {
		f.Status = string(*p.Status)
	}
	if p := req.Params; p.Channel != nil {
		f.Channel = string(*p.Channel)
	}
	if req.Params.Cursor != nil {
		f.Cursor = *req.Params.Cursor
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	list, next, err := h.svc.List(ctx, f)
	if errors.Is(err, notify.ErrInvalidCursor) {
		return nil, httpx.RequestInvalid("cursor is not from a previous page")
	}
	if err != nil {
		return nil, err
	}
	out := PlatformListNotifications200JSONResponse{Data: make([]NotificationDelivery, 0, len(list))}
	for _, d := range list {
		out.Data = append(out.Data, toDelivery(d))
	}
	if next != "" {
		out.NextCursor = &next
	}
	return out, nil
}

func (h *Strict) PlatformGetNotification(ctx context.Context, req PlatformGetNotificationRequestObject) (PlatformGetNotificationResponseObject, error) {
	d, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformGetNotification200JSONResponse(toDelivery(d)), nil
}

func (h *Strict) PlatformGetInbox(ctx context.Context, req PlatformGetInboxRequestObject) (PlatformGetInboxResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := 20
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	items, unread, err := h.svc.Inbox(ctx, user, limit)
	if err != nil {
		return nil, err
	}
	out := PlatformGetInbox200JSONResponse{Unread: int(unread)}
	out.Items = make([]struct {
		Body      string    `json:"body"`
		CreatedAt Timestamp `json:"created_at"`
		Id        Uuid      `json:"id"`
		Read      bool      `json:"read"`
		Title     string    `json:"title"`
	}, 0, len(items))
	for _, it := range items {
		out.Items = append(out.Items, struct {
			Body      string    `json:"body"`
			CreatedAt Timestamp `json:"created_at"`
			Id        Uuid      `json:"id"`
			Read      bool      `json:"read"`
			Title     string    `json:"title"`
		}{Body: it.Body, CreatedAt: it.CreatedAt.UTC(), Id: it.ID, Read: it.Read, Title: it.Title})
	}
	return out, nil
}

func (h *Strict) PlatformMarkInboxRead(ctx context.Context, req PlatformMarkInboxReadRequestObject) (PlatformMarkInboxReadResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.MarkRead(ctx, user, req.Id); err != nil {
		return nil, problem(err)
	}
	return PlatformMarkInboxRead204Response{}, nil
}

func currentUser(ctx context.Context) (uuid.UUID, error) {
	g, _ := authz.FromContext(ctx)
	id, err := uuid.Parse(g.UserID)
	if err != nil {
		return uuid.Nil, httpx.AuthnRequired()
	}
	return id, nil
}

func toTemplate(t notify.Template) NotificationTemplate {
	out := NotificationTemplate{
		Id: t.ID, Global: t.Global, Code: t.Code, Channel: NotificationChannel(t.Channel), Language: t.Language,
		Body: t.Body, Variables: t.Variables, RowVersion: int(t.RowVersion), UpdatedAt: t.UpdatedAt.UTC(),
	}
	if out.Variables == nil {
		out.Variables = []string{}
	}
	if t.Subject != "" {
		out.Subject = &t.Subject
	}
	return out
}

func toDelivery(d notify.Delivery) NotificationDelivery {
	out := NotificationDelivery{
		Id: d.ID, Channel: NotificationChannel(d.Channel), Status: NotificationStatus(d.Status), Attempts: d.Attempts,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(), RecipientUserId: d.RecipientUserID, EntityId: d.EntityID,
	}
	str := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	out.TemplateCode, out.RecipientMasked, out.ProviderMessageId = str(d.TemplateCode), str(d.RecipientMasked), str(d.ProviderMessageID)
	out.Error, out.EntityType = str(d.Error), str(d.EntityType)
	if d.SentAt != nil {
		t := d.SentAt.UTC()
		out.SentAt = &t
	}
	return out
}

// ETag is the row_version in quotes (api/openapi/README.md); If-Match may be quoted or weak-prefixed.
func etag(v int32) string { return fmt.Sprintf("%q", strconv.Itoa(int(v))) }

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefSlice(s *[]string) []string {
	if s == nil {
		return nil
	}
	return *s
}

func problem(err error) error {
	switch {
	case errors.Is(err, notify.ErrNotFound), errors.Is(err, notify.ErrTemplateNotFound):
		return httpx.NotFound()
	case errors.Is(err, notify.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, notify.ErrDuplicateTemplate):
		return httpx.Problem{Status: http.StatusConflict, Code: "notify.template_exists", Title: "Template already exists"}
	case errors.Is(err, notify.ErrTemplateInvalid):
		return httpx.UnprocessableEntity("notify.template_invalid", err.Error())
	case errors.Is(err, notify.ErrInvalidRequest):
		return httpx.UnprocessableEntity("notify.invalid", err.Error())
	}
	return err
}

func ptr(s string) *string { return &s }
