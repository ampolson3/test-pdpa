// Package service is the org module's business logic. So far: business calendars and holidays (ORG-20),
// which other modules read through Calendars to count SLAs on the tenant's working days.
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	orgstore "pdpa-platform/internal/org/store"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/bizcal"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
)

// CalendarEntityType is the audit entity type of a business calendar (holiday changes are audited on it).
const CalendarEntityType = "business_calendar"

var (
	ErrNotFound        = errors.New("org: not found")
	ErrInvalid         = errors.New("org: invalid input")
	ErrDuplicateName   = errors.New("org: a calendar with this name exists")
	ErrVersionMismatch = errors.New("org: version mismatch")
)

// Calendars is what other modules use (CLAUDE.md rule 9): the tenant's business calendar, ready for
// bizcal arithmetic. It reads under the RLS of the transaction in ctx.
type Calendars interface {
	// BusinessCalendar returns calendar id, or the tenant's default when id is nil. A tenant without any
	// calendar gets bizcal.Default() (Monday–Friday, Asia/Bangkok, no holidays) and a nil id.
	BusinessCalendar(ctx context.Context, id *uuid.UUID) (bizcal.Calendar, *uuid.UUID, error)
}

var _ Calendars = (*Service)(nil)

// Service owns the org schema. Files is needed only to attach legal-entity logos (ORG-01).
type Service struct {
	Audit *audit.Service
	Files FileStore
}

// Calendar is a business calendar as the admin sees it.
type Calendar struct {
	ID         uuid.UUID
	Name       string
	Timezone   string
	Workdays   []int // ISO weekdays, 1 = Monday … 7 = Sunday
	IsDefault  bool
	RowVersion int32
	UpdatedAt  time.Time
}

// CalendarInput is what an admin edits. MakeDefault moves the tenant's default to this calendar.
type CalendarInput struct {
	Name        string
	Timezone    string
	Workdays    []int
	MakeDefault bool
}

// Holiday is one day off in a calendar.
type Holiday struct {
	Date time.Time // midnight UTC of the date
	Name string
}

func (in CalendarInput) normalize() (CalendarInput, []int16, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		return in, nil, fmt.Errorf("%w: name", ErrInvalid)
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" || in.Timezone == "Local" {
		return in, nil, fmt.Errorf("%w: timezone", ErrInvalid)
	}
	days := slices.Clone(in.Workdays)
	slices.Sort(days)
	days = slices.Compact(days)
	if _, err := bizcal.New(time.UTC, days, nil); err != nil {
		return in, nil, fmt.Errorf("%w: workdays", ErrInvalid)
	}
	out := make([]int16, len(days))
	for i, d := range days {
		out[i] = int16(d)
	}
	in.Workdays = days
	return in, out, nil
}

func (s *Service) ListCalendars(ctx context.Context) ([]Calendar, error) {
	rows, err := orgstore.New(pdb.MustTxFromContext(ctx)).ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Calendar, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCalendar(orgstore.GetCalendarRow(r)))
	}
	return out, nil
}

// CreateCalendar adds a calendar; the tenant's first one becomes its default.
func (s *Service) CreateCalendar(ctx context.Context, in CalendarInput) (Calendar, error) {
	in, days, err := in.normalize()
	if err != nil {
		return Calendar{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Calendar{}, err
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var row orgstore.GetCalendarRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err := q.InsertCalendar(ctx, orgstore.InsertCalendarParams{ID: id, Name: in.Name, Timezone: in.Timezone, Workdays: days})
		row = orgstore.GetCalendarRow(r)
		return err
	})
	if isUnique(err) {
		return Calendar{}, ErrDuplicateName
	}
	if err != nil {
		return Calendar{}, err
	}
	if in.MakeDefault && !row.IsDefault {
		if err := s.makeDefault(ctx, row.ID); err != nil {
			return Calendar{}, err
		}
		row.IsDefault = true
	} else if row.IsDefault {
		if err := q.UpsertDefaultCalendarSetting(ctx, pgtype.UUID{Bytes: row.ID, Valid: true}); err != nil {
			return Calendar{}, err
		}
	}
	c := toCalendar(row)
	return c, s.audit(ctx, "org.calendar.create", c.ID, nil, calendarAudit(c))
}

// UpdateCalendar changes a calendar if version still matches (If-Match).
func (s *Service) UpdateCalendar(ctx context.Context, id uuid.UUID, version int32, in CalendarInput) (Calendar, error) {
	in, days, err := in.normalize()
	if err != nil {
		return Calendar{}, err
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	before, err := q.GetCalendar(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Calendar{}, ErrNotFound
	}
	if err != nil {
		return Calendar{}, err
	}
	if before.RowVersion != version {
		return Calendar{}, ErrVersionMismatch
	}
	var row orgstore.GetCalendarRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err := q.UpdateCalendar(ctx, orgstore.UpdateCalendarParams{ID: id, RowVersion: version, Name: in.Name, Timezone: in.Timezone, Workdays: days})
		row = orgstore.GetCalendarRow(r)
		return err
	})
	switch {
	case isUnique(err):
		return Calendar{}, ErrDuplicateName
	case errors.Is(err, pgx.ErrNoRows):
		return Calendar{}, ErrVersionMismatch
	case err != nil:
		return Calendar{}, err
	}
	if in.MakeDefault && !row.IsDefault {
		if err := s.makeDefault(ctx, id); err != nil {
			return Calendar{}, err
		}
		row.IsDefault = true
	}
	c := toCalendar(row)
	return c, s.audit(ctx, "org.calendar.update", id, calendarAudit(toCalendar(before)), calendarAudit(c))
}

// makeDefault moves the default flag (and org_settings.default_calendar_id, kept in step) to id.
func (s *Service) makeDefault(ctx context.Context, id uuid.UUID) error {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	if err := q.ClearDefaultCalendar(ctx, id); err != nil {
		return err
	}
	if err := q.MarkDefaultCalendar(ctx, id); err != nil {
		return err
	}
	return q.UpsertDefaultCalendarSetting(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// ListHolidays returns a calendar's holidays, of one Gregorian year when year > 0.
func (s *Service) ListHolidays(ctx context.Context, calendarID uuid.UUID, year int) ([]Holiday, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	if _, err := s.calendar(ctx, calendarID); err != nil {
		return nil, err
	}
	p := orgstore.ListHolidaysParams{CalendarID: calendarID}
	if year > 0 {
		p.FromDate = pgtype.Date{Time: time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
		p.ToDate = pgtype.Date{Time: time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	rows, err := q.ListHolidays(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]Holiday, 0, len(rows))
	for _, r := range rows {
		out = append(out, Holiday{Date: r.HolidayDate.Time, Name: r.Name})
	}
	return out, nil
}

// PutHoliday adds the holiday on date, or renames it.
func (s *Service) PutHoliday(ctx context.Context, calendarID uuid.UUID, date time.Time, name string) (Holiday, error) {
	h, before, err := s.putHoliday(ctx, calendarID, date, name)
	if err != nil {
		return Holiday{}, err
	}
	var b any
	if before != "" {
		b = map[string]any{"date": dateString(date), "name": before}
	}
	return h, s.audit(ctx, "org.holiday.put", calendarID, b, map[string]any{"date": dateString(date), "name": h.Name})
}

// putHoliday is PutHoliday without the audit row (the import audits itself as one change). It returns
// the previous name, if the date was a holiday already.
func (s *Service) putHoliday(ctx context.Context, calendarID uuid.UUID, date time.Time, name string) (Holiday, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 200 {
		return Holiday{}, "", fmt.Errorf("%w: name", ErrInvalid)
	}
	if _, err := s.calendar(ctx, calendarID); err != nil { // FKs bypass RLS: check visibility first
		return Holiday{}, "", err
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	d := pgtype.Date{Time: time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
	prev := ""
	if old, err := q.ListHolidays(ctx, orgstore.ListHolidaysParams{CalendarID: calendarID, FromDate: d, ToDate: pgtype.Date{Time: d.Time.AddDate(0, 0, 1), Valid: true}}); err != nil {
		return Holiday{}, "", err
	} else if len(old) == 1 {
		prev = old[0].Name
	}
	r, err := q.UpsertHoliday(ctx, orgstore.UpsertHolidayParams{CalendarID: calendarID, HolidayDate: d, Name: name})
	if err != nil {
		return Holiday{}, "", err
	}
	return Holiday{Date: r.HolidayDate.Time, Name: r.Name}, prev, nil
}

func (s *Service) DeleteHoliday(ctx context.Context, calendarID uuid.UUID, date time.Time) error {
	if _, err := s.calendar(ctx, calendarID); err != nil {
		return err
	}
	d := pgtype.Date{Time: time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
	n, err := orgstore.New(pdb.MustTxFromContext(ctx)).DeleteHoliday(ctx, orgstore.DeleteHolidayParams{CalendarID: calendarID, HolidayDate: d})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "org.holiday.delete", calendarID, map[string]any{"date": dateString(date)}, nil)
}

// BusinessCalendar implements Calendars.
func (s *Service) BusinessCalendar(ctx context.Context, id *uuid.UUID) (bizcal.Calendar, *uuid.UUID, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var row orgstore.GetCalendarRow
	var err error
	if id != nil {
		row, err = q.GetCalendar(ctx, *id)
	} else {
		var r orgstore.GetDefaultCalendarRow
		r, err = q.GetDefaultCalendar(ctx)
		row = orgstore.GetCalendarRow(r)
		if errors.Is(err, pgx.ErrNoRows) {
			return bizcal.Default(), nil, nil
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return bizcal.Calendar{}, nil, ErrNotFound
	}
	if err != nil {
		return bizcal.Calendar{}, nil, err
	}
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		return bizcal.Calendar{}, nil, fmt.Errorf("org: calendar %s: %w", row.ID, err)
	}
	hs, err := q.ListHolidays(ctx, orgstore.ListHolidaysParams{CalendarID: row.ID})
	if err != nil {
		return bizcal.Calendar{}, nil, err
	}
	dates := make([]time.Time, len(hs))
	for i, h := range hs {
		dates[i] = h.HolidayDate.Time
	}
	c, err := bizcal.New(loc, toInts(row.Workdays), dates)
	if err != nil {
		return bizcal.Calendar{}, nil, err
	}
	return c, &row.ID, nil
}

func (s *Service) calendar(ctx context.Context, id uuid.UUID) (orgstore.GetCalendarRow, error) {
	row, err := orgstore.New(pdb.MustTxFromContext(ctx)).GetCalendar(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil { // a worker job: the tenant is the transaction's
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("org: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityTypeOf(action), EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}

func calendarAudit(c Calendar) map[string]any {
	return map[string]any{"name": c.Name, "timezone": c.Timezone, "workdays": c.Workdays, "is_default": c.IsDefault}
}

func toCalendar(r orgstore.GetCalendarRow) Calendar {
	return Calendar{ID: r.ID, Name: r.Name, Timezone: r.Timezone, Workdays: toInts(r.Workdays), IsDefault: r.IsDefault, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
}

func toInts(v []int16) []int {
	out := make([]int, len(v))
	for i, d := range v {
		out[i] = int(d)
	}
	return out
}

func dateString(t time.Time) string { return t.Format("2006-01-02") }

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
