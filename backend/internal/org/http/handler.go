// Package orghttp holds the org module's admin endpoints (ORG-20 calendars and holidays so far). Types in
// org.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package orghttp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *orgservice.Service
}

func NewStrict(svc *orgservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) OrgListCalendars(ctx context.Context, _ OrgListCalendarsRequestObject) (OrgListCalendarsResponseObject, error) {
	list, err := h.svc.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	resp := OrgListCalendars200JSONResponse{Data: make([]BusinessCalendar, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toWire(c))
	}
	return resp, nil
}

func (h *Strict) OrgCreateCalendar(ctx context.Context, req OrgCreateCalendarRequestObject) (OrgCreateCalendarResponseObject, error) {
	c, err := h.svc.CreateCalendar(ctx, toInput(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return OrgCreateCalendar201JSONResponse{Body: toWire(c), Headers: OrgCreateCalendar201ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateCalendar(ctx context.Context, req OrgUpdateCalendarRequestObject) (OrgUpdateCalendarResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c, err := h.svc.UpdateCalendar(ctx, req.Id, v, toInput(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return OrgUpdateCalendar200JSONResponse{Body: toWire(c), Headers: OrgUpdateCalendar200ResponseHeaders{ETag: etag(c.RowVersion)}}, nil
}

func (h *Strict) OrgListHolidays(ctx context.Context, req OrgListHolidaysRequestObject) (OrgListHolidaysResponseObject, error) {
	year := 0
	if req.Params.Year != nil {
		year = *req.Params.Year
	}
	list, err := h.svc.ListHolidays(ctx, req.Id, year)
	if err != nil {
		return nil, problem(err)
	}
	resp := OrgListHolidays200JSONResponse{Data: make([]Holiday, 0, len(list))}
	for _, d := range list {
		resp.Data = append(resp.Data, Holiday{Date: openapi_types.Date{Time: d.Date}, Name: d.Name})
	}
	return resp, nil
}

func (h *Strict) OrgPutHoliday(ctx context.Context, req OrgPutHolidayRequestObject) (OrgPutHolidayResponseObject, error) {
	d, err := h.svc.PutHoliday(ctx, req.Id, req.Date.Time, req.Body.Name)
	if err != nil {
		return nil, problem(err)
	}
	return OrgPutHoliday200JSONResponse{Date: openapi_types.Date{Time: d.Date}, Name: d.Name}, nil
}

func (h *Strict) OrgDeleteHoliday(ctx context.Context, req OrgDeleteHolidayRequestObject) (OrgDeleteHolidayResponseObject, error) {
	if err := h.svc.DeleteHoliday(ctx, req.Id, req.Date.Time); err != nil {
		return nil, problem(err)
	}
	return OrgDeleteHoliday204Response{}, nil
}

func toInput(b BusinessCalendarInput) orgservice.CalendarInput {
	return orgservice.CalendarInput{Name: b.Name, Timezone: b.Timezone, Workdays: b.Workdays, MakeDefault: b.IsDefault != nil && *b.IsDefault}
}

func toWire(c orgservice.Calendar) BusinessCalendar {
	return BusinessCalendar{Id: c.ID, Name: c.Name, Timezone: c.Timezone, Workdays: c.Workdays, IsDefault: c.IsDefault,
		RowVersion: int(c.RowVersion), UpdatedAt: c.UpdatedAt.UTC()}
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, orgservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, orgservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, orgservice.ErrDuplicateName):
		return httpx.UnprocessableEntity("org.duplicate_calendar_name", err.Error())
	case errors.Is(err, orgservice.ErrInvalid):
		return httpx.UnprocessableEntity("org.invalid_calendar", err.Error())
	}
	return err
}
