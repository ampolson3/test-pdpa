package dpohttp

import (
	"context"
	"fmt"
	"strings"
	"time"

	dposervice "pdpa-platform/internal/dpo/service"
)

// calendarWindow resolves the [from, to] range for DPO-08's own compliance calendar — a 90-day forward
// window by default, matching dposervice.calendarWindow.
func calendarWindow(from, to *time.Time) (time.Time, time.Time) {
	f, t := time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 90)
	if from != nil {
		f = *from
	}
	if to != nil {
		t = *to
	}
	return f, t
}

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

// DpoGetComplianceCalendar is DPO-08's own acceptance criterion: every module's own scheduled review/
// due-date cycle, read live within [from, to].
func (h *Strict) DpoGetComplianceCalendar(ctx context.Context, req DpoGetComplianceCalendarRequestObject) (DpoGetComplianceCalendarResponseObject, error) {
	var from, to *time.Time
	if req.Params.From != nil {
		t := req.Params.From.Time
		from = &t
	}
	if req.Params.To != nil {
		t := req.Params.To.Time
		to = &t
	}
	f, t := calendarWindow(from, to)
	items, err := h.svc.ComplianceCalendar(ctx, f, t)
	if err != nil {
		return nil, err
	}
	resp := DpoGetComplianceCalendar200JSONResponse{Data: make([]DpoCalendarItem, 0, len(items))}
	for _, it := range items {
		resp.Data = append(resp.Data, toCalendarItemWire(it))
	}
	return resp, nil
}

// DpoGetComplianceCalendarIcs is DPO-08's own "export iCal": the identical items as a plain iCalendar feed.
func (h *Strict) DpoGetComplianceCalendarIcs(ctx context.Context, req DpoGetComplianceCalendarIcsRequestObject) (DpoGetComplianceCalendarIcsResponseObject, error) {
	var from, to *time.Time
	if req.Params.From != nil {
		t := req.Params.From.Time
		from = &t
	}
	if req.Params.To != nil {
		t := req.Params.To.Time
		to = &t
	}
	f, t := calendarWindow(from, to)
	items, err := h.svc.ComplianceCalendar(ctx, f, t)
	if err != nil {
		return nil, err
	}
	ics := dposervice.ComplianceCalendarICS(items)
	name := fmt.Sprintf(`attachment; filename="compliance-calendar-%s.ics"`, time.Now().UTC().Format("20060102-150405"))
	return DpoGetComplianceCalendarIcs200TextcalendarResponse{Body: strings.NewReader(ics), ContentLength: int64(len(ics)),
		Headers: DpoGetComplianceCalendarIcs200ResponseHeaders{ContentDisposition: &name}}, nil
}

func toCalendarItemWire(it dposervice.CalendarItem) DpoCalendarItem {
	return DpoCalendarItem{Source: DpoCalendarItemSource(it.Source), Title: it.Title, DueAt: it.DueAt, RefId: it.RefID}
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
