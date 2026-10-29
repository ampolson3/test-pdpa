// DPO-08 (compliance calendar, ม.42(2)): aggregates every module's own review/due-date cycle already
// built by an earlier pass (ROPA-15's review cycle, PNG-15's notice review cycle, VEN-10's vendor
// reassessment, DPX-01's training due dates) plus DPIA's own already-seeded assess.assessments.
// next_review_at column (unused by any sweep yet, read live here) — the same cross-module aggregation
// shape DPO-05's own Deadlines already established, generalized from "near-deadline" to "scheduled on a
// date range," and exported as an iCal feed per the module description's own "export iCal."
package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
)

// CalendarItem is one scheduled compliance activity, from whichever module owns it.
type CalendarItem struct {
	Source string // ropa_review | notice_review | dpia_review | training_due | vendor_reassessment
	Title  string
	DueAt  time.Time
	RefID  uuid.UUID
}

// Notice is what DPO-08 reads from the notice module (rule 9): notice already imports dpo/service
// (PNG-07's own OpenConsentTask), so this interface's own return type is dpo's own CalendarItem —
// notice's matching method returns that type directly, no reverse import or adapter needed.
type Notice interface {
	ListUpcomingReviews(ctx context.Context, from, to time.Time) ([]CalendarItem, error)
}

const calendarWindow = 90 * 24 * time.Hour

// ComplianceCalendar is the acceptance criterion itself ("ปฏิทินแสดงกิจกรรมจากทุกโมดูลตรงกับกำหนดการจริง"):
// every module's own review/due-date cycle, read live (never a stale copy) within [from, to] — each source
// is skipped, not errored, when the caller lacks that module's own read permission, the same discipline
// DPO-05's own Deadlines already established; dpo.report.read on this endpoint is not a blanket grant.
func (s *Service) ComplianceCalendar(ctx context.Context, from, to time.Time) ([]CalendarItem, error) {
	var out []CalendarItem
	g, _ := authz.FromContext(ctx)

	if s.Ropa != nil && g.Has("ropa.activity.read") {
		rows, err := s.Ropa.ListUpcomingReviews(ctx, from, to)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, CalendarItem{Source: "ropa_review", Title: r.Title, DueAt: r.DueAt, RefID: r.ActivityID})
		}
	}

	if s.Notice != nil && g.Has("notice.document.read") {
		items, err := s.Notice.ListUpcomingReviews(ctx, from, to)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}

	if s.Dpia != nil && g.Has("assessment.dpia.read") {
		rows, err := s.Dpia.ListUpcomingReviews(ctx, from, to)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, CalendarItem{Source: "dpia_review", Title: r.Title, DueAt: r.DueAt, RefID: r.AssessmentID})
		}
	}

	if s.Gov != nil && g.Has("dpx.training.read") {
		rows, err := s.Gov.ListUpcomingTrainingDueDates(ctx, from, to)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, CalendarItem{Source: "training_due", Title: r.Title, DueAt: r.DueAt, RefID: r.AssignmentID})
		}
	}

	if s.Vendor != nil && g.Has("vendor.vendor.read") {
		rows, err := s.Vendor.ListUpcomingReassessments(ctx, from, to)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, CalendarItem{Source: "vendor_reassessment", Title: r.Title, DueAt: r.DueAt, RefID: r.VendorID})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].DueAt.Before(out[j].DueAt) })
	return out, nil
}

// ComplianceCalendarICS renders the same items as a plain iCalendar (RFC 5545) feed — the module
// description's own "export iCal," hand-written since the format is simple enough not to need a
// dependency, the same "no Docker/no library, hand-roll the simple format" move PLT-16's own DOCX
// composer already made.
func ComplianceCalendarICS(items []CalendarItem) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//PDPA Platform//Compliance Calendar//TH\r\nCALSCALE:GREGORIAN\r\n")
	for _, it := range items {
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:%s-%s@pdpa-platform\r\n", it.Source, it.RefID)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", time.Now().UTC().Format("20060102T150405Z"))
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", it.DueAt.Format("20060102"))
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icsEscape(it.Title))
		fmt.Fprintf(&b, "CATEGORIES:%s\r\n", it.Source)
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
