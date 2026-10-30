package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	breachservice "pdpa-platform/internal/breach/service"
	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/authz"
)

// DeadlineItem is one near-deadline or overdue item surfaced on DPO-05's notification center, from a module
// that already tracks a legal deadline.
type DeadlineItem struct {
	Source         string // dsar_request | breach_incident
	ReferenceID    uuid.UUID
	ReferenceLabel string
	DueAt          time.Time
	Status         string // at_risk | overdue
}

// openDSARStatuses are ST-02's non-terminal states (docs/states/state-machines.yaml#ST-02) — a request in
// any of these still has a live SLA clock worth watching.
var openDSARStatuses = []string{"received", "verifying", "in_review", "in_progress", "awaiting_info"}

// breachAtRiskWindow mirrors breach's own 66-hour escalation checkpoint (role EXEC is alerted from 66h of
// the 72-hour PDPC clock, BRE-07) — this dashboard flags an incident at_risk from that same point rather
// than inventing a second threshold for the same deadline.
const breachAtRiskWindow = 6 * time.Hour

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Deadlines is DPO-05's acceptance criterion: near-deadline and overdue work from every module that already
// tracks a legal deadline, on one page. DPIA review cycles, contract expiry and notice-review reminders
// aren't built yet (no owning feature exists — DPIA-13/PNG-15/the DPA-DSA modules), so they're left out
// until they exist, the same "no consumer yet" deferral this codebase uses elsewhere (see PLT-13's own
// README note). Each source is skipped, not errored, when the caller lacks that module's own read
// permission — dpo.report.read on this endpoint is not a blanket grant to every module's detail.
func (s *Service) Deadlines(ctx context.Context) ([]DeadlineItem, error) {
	now := s.now()
	var out []DeadlineItem

	if g, _ := authz.FromContext(ctx); s.Dsar != nil && g.Has("dsar.request.read") {
		for _, status := range openDSARStatuses {
			reqs, _, err := s.Dsar.ListRequests(ctx, dsarservice.RequestFilter{Status: status, Limit: 50})
			if err != nil {
				return nil, err
			}
			for _, r := range reqs {
				sla := dsarservice.SLAStatus(now, r.DueAt)
				if sla == "on_track" {
					continue
				}
				out = append(out, DeadlineItem{Source: "dsar_request", ReferenceID: r.ID, ReferenceLabel: r.RequestNo, DueAt: r.DueAt, Status: sla})
			}
		}
	}

	if s.Breach != nil {
		incidents, err := s.breachOpenIncidents(ctx)
		if err != nil {
			return nil, err
		}
		for _, in := range incidents {
			status := breachDeadlineStatus(now, in.DueAt)
			if status == "" {
				continue
			}
			out = append(out, DeadlineItem{Source: "breach_incident", ReferenceID: in.ID, ReferenceLabel: in.No, DueAt: in.DueAt, Status: status})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].DueAt.Before(out[j].DueAt) })
	return out, nil
}

// breachOpenIncidents drains every page of the tenant's open incidents, or nil (not an error) when the
// caller doesn't hold breach.incident.read.
func (s *Service) breachOpenIncidents(ctx context.Context) ([]breachservice.Incident, error) {
	var out []breachservice.Incident
	cursor := ""
	for {
		page, next, err := s.Breach.List(ctx, breachservice.Filter{OpenOnly: true, Limit: 100, Cursor: cursor})
		if errors.Is(err, breachservice.ErrForbidden) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if next == "" {
			return out, nil
		}
		cursor = next
	}
}

func breachDeadlineStatus(now, dueAt time.Time) string {
	switch {
	case now.After(dueAt):
		return "overdue"
	case !dueAt.Add(-breachAtRiskWindow).After(now):
		return "at_risk"
	default:
		return ""
	}
}
