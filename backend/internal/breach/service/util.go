package service

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func optText(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

// bangkok formats a moment for alert texts (rule 11: displayed in Asia/Bangkok).
func bangkok(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	return t.In(loc).Format("2006-01-02 15:04") + " (ICT)"
}
