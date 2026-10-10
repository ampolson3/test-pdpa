package service

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func optText(s string) *string {
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

func optInt16(v *int) *int16 {
	if v == nil {
		return nil
	}
	x := int16(*v)
	return &x
}

func optInt32(v *int) *int32 {
	if v == nil {
		return nil
	}
	x := int32(*v)
	return &x
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func pgUUIDv(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}
