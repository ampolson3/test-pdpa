package service

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	orgstore "pdpa-platform/internal/org/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/importer"
)

// HolidayImportType is the PLT-14 import type for holidays (ORG-20): one row per holiday, into the calendar
// named in the row or, when that column is empty or unmapped, the tenant's default calendar.
const HolidayImportType = "org.holiday"

// HolidayImport describes the holiday import. Messages in the error report are codes:
// invalid_date, too_long, unknown_calendar, no_default_calendar (and the framework's required).
func (s *Service) HolidayImport() importer.Type {
	return importer.Type{
		Permission: "org.settings.update",
		Columns: []importer.Column{
			{Key: "date", Required: true, Label: map[string]string{"th": "วันที่", "en": "Date"}, Aliases: []string{"holiday_date", "วันหยุด"}},
			{Key: "name", Required: true, Label: map[string]string{"th": "ชื่อวันหยุด", "en": "Holiday name"}, Aliases: []string{"holiday", "ชื่อ"}},
			{Key: "calendar", Label: map[string]string{"th": "ปฏิทิน", "en": "Calendar"}},
		},
		Validate: func(ctx context.Context, _ int, r importer.Row) []importer.FieldError {
			var errs []importer.FieldError
			if _, err := importer.ParseDate(r["date"]); err != nil {
				errs = append(errs, importer.FieldError{Column: "date", Message: "invalid_date"})
			}
			if utf8.RuneCountInString(r["name"]) > 200 {
				errs = append(errs, importer.FieldError{Column: "name", Message: "too_long"})
			}
			if _, err := s.importCalendar(ctx, r["calendar"]); err != nil {
				msg := "unknown_calendar"
				if r["calendar"] == "" {
					msg = "no_default_calendar"
				}
				if !errors.Is(err, ErrNotFound) {
					msg = "lookup_failed"
				}
				errs = append(errs, importer.FieldError{Column: "calendar", Message: msg})
			}
			return errs
		},
		Apply: func(ctx context.Context, _ int, r importer.Row) error {
			d, err := importer.ParseDate(r["date"])
			if err != nil {
				return err
			}
			id, err := s.importCalendar(ctx, r["calendar"])
			if err != nil {
				return err
			}
			_, _, err = s.putHoliday(ctx, id, d, r["name"])
			return err
		},
	}
}

// importCalendar resolves a row's calendar: by name (case-insensitive), or the default when name is empty.
func (s *Service) importCalendar(ctx context.Context, name string) (uuid.UUID, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var id uuid.UUID
	var err error
	if name == "" {
		var r orgstore.GetDefaultCalendarRow
		r, err = q.GetDefaultCalendar(ctx)
		id = r.ID
	} else {
		var r orgstore.GetCalendarByNameRow
		r, err = q.GetCalendarByName(ctx, name)
		id = r.ID
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}
