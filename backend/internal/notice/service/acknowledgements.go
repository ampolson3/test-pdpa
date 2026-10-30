package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// AckChannels is every channel an acknowledgement can be recorded through — the module doc's own
// "ผ่าน API / SDK / หน้า public" literally: a staff-recorded one (API), a website's embedded SDK, a
// plain web page view, or an e-mailed link.
var AckChannels = []string{"web", "sdk", "email", "api"}

// Acknowledgement is PNG-09's own evidence row: who (when known) acknowledged which published notice
// version, through which channel, when — append-only, never edited (notice.acknowledgements has no
// update/delete query and is REVOKE UPDATE/DELETE/TRUNCATE'd in deploy/db/10-grants.sql, rule 4's own
// evidence pattern).
type Acknowledgement struct {
	ID              uuid.UUID
	NoticeVersionID uuid.UUID
	UserID          *uuid.UUID
	UserName        string
	Channel         string
	OccurredAt      pgtype.Timestamptz
	IP              *netip.Addr
}

func toAcknowledgement(id, versionID uuid.UUID, userID pgtype.UUID, channel string, occurredAt pgtype.Timestamptz, ip *netip.Addr) Acknowledgement {
	a := Acknowledgement{ID: id, NoticeVersionID: versionID, Channel: channel, OccurredAt: occurredAt, IP: ip}
	if userID.Valid {
		u := uuid.UUID(userID.Bytes)
		a.UserID = &u
	}
	return a
}

func validAckChannel(c string) bool {
	for _, v := range AckChannels {
		if c == v {
			return true
		}
	}
	return false
}

// resolveVersionID looks up the notice_versions row id for noticeID + versionNo (nil: the current/newest
// published version) — the same resolution PublicVersionHTML/PublicDownloadURL already do.
func (s *Service) resolveVersionID(ctx context.Context, noticeID uuid.UUID, versionNo *int32) (uuid.UUID, int32, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	if versionNo == nil {
		rows, err := q.ListNoticeVersions(ctx, noticeID)
		if err != nil {
			return uuid.Nil, 0, err
		}
		if len(rows) == 0 {
			return uuid.Nil, 0, ErrNotFound
		}
		return rows[0].ID, rows[0].VersionNo, nil // newest first
	}
	r, err := q.GetNoticeVersion(ctx, noticestore.GetNoticeVersionParams{NoticeID: noticeID, VersionNo: *versionNo})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, 0, err
	}
	return r.ID, r.VersionNo, nil
}

func (s *Service) insertAcknowledgement(ctx context.Context, versionID uuid.UUID, userID *uuid.UUID, channel string, ip *netip.Addr) (Acknowledgement, error) {
	if !validAckChannel(channel) {
		return Acknowledgement{}, fmt.Errorf("%w: channel", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Acknowledgement{}, err
	}
	var userParam pgtype.UUID
	if userID != nil {
		userParam = pgtype.UUID{Bytes: *userID, Valid: true}
	}
	row, err := noticestore.New(pdb.MustTxFromContext(ctx)).InsertAcknowledgement(ctx, noticestore.InsertAcknowledgementParams{
		ID: id, NoticeVersionID: versionID, UserID: userParam, Channel: channel, Ip: ip,
	})
	if err != nil {
		return Acknowledgement{}, err
	}
	a := toAcknowledgement(row.ID, row.NoticeVersionID, row.UserID, row.Channel, row.OccurredAt, row.Ip)
	if err := s.audit(ctx, "notice.acknowledgement.create", versionID, nil, map[string]any{"channel": channel}); err != nil {
		return Acknowledgement{}, err
	}
	return a, nil
}

// RecordPublicAcknowledgement is the public, permission-free half of PNG-09 — the portal page (or an
// embedded SDK calling the same endpoint) tells the API a visitor acknowledged the notice. No identity is
// ever required or accepted here (rule 3: this module has no machinery to resolve a raw identifier to a
// real consent.data_subjects row without crossing a module boundary it shouldn't) — only the channel, time
// and IP are recorded, same as every other anonymous public evidence write in this codebase.
func (s *Service) RecordPublicAcknowledgement(ctx context.Context, n Notice, versionNo *int32, channel string, ip *netip.Addr) (Acknowledgement, int32, error) {
	versionID, resolvedVersionNo, err := s.resolveVersionID(ctx, n.ID, versionNo)
	if err != nil {
		return Acknowledgement{}, 0, err
	}
	a, err := s.insertAcknowledgement(ctx, versionID, nil, channel, ip)
	if err != nil {
		return Acknowledgement{}, 0, err
	}
	return a, resolvedVersionNo, nil
}

// RecordAcknowledgement is the authenticated half — staff (or a future API-client integration, ORG-16)
// recording that a named user (e.g. an employee who read the internal notice) acknowledged a specific
// version. userID must be a real, active user of the tenant (rule 1 FK-visibility, via iamservice.Names).
func (s *Service) RecordAcknowledgement(ctx context.Context, noticeID uuid.UUID, versionNo int32, userID uuid.UUID, channel string) (Acknowledgement, error) {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return Acknowledgement{}, err
	}
	versionID, _, err := s.resolveVersionID(ctx, noticeID, &versionNo)
	if err != nil {
		return Acknowledgement{}, err
	}
	names, err := iamservice.Names(ctx, []uuid.UUID{userID})
	if err != nil {
		return Acknowledgement{}, err
	}
	if _, ok := names[userID]; !ok {
		return Acknowledgement{}, fmt.Errorf("%w: user_id", ErrInvalid)
	}
	return s.insertAcknowledgement(ctx, versionID, &userID, channel, nil)
}

// ListAcknowledgements is PNG-09's own acceptance criterion read-side: "ตรวจได้ว่าบุคคลรับทราบประกาศ
// เวอร์ชันใด" — every acknowledgement of one notice version, newest first, with each named user's display
// name resolved (an anonymous public ack has none). Cursor pagination on occurred_at, matching this
// codebase's other evidence-log reads.
func (s *Service) ListAcknowledgements(ctx context.Context, noticeID uuid.UUID, versionNo int32, before *pgtype.Timestamptz, limit int32) ([]Acknowledgement, int64, error) {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return nil, 0, err
	}
	versionID, _, err := s.resolveVersionID(ctx, noticeID, &versionNo)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	var beforeParam pgtype.Timestamptz
	if before != nil {
		beforeParam = *before
	}
	rows, err := q.ListAcknowledgementsByVersion(ctx, noticestore.ListAcknowledgementsByVersionParams{
		NoticeVersionID: versionID, Column2: beforeParam, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountAcknowledgementsByVersion(ctx, versionID)
	if err != nil {
		return nil, 0, err
	}
	var userIDs []uuid.UUID
	out := make([]Acknowledgement, 0, len(rows))
	for _, r := range rows {
		a := toAcknowledgement(r.ID, r.NoticeVersionID, r.UserID, r.Channel, r.OccurredAt, r.Ip)
		if a.UserID != nil {
			userIDs = append(userIDs, *a.UserID)
		}
		out = append(out, a)
	}
	if len(userIDs) > 0 {
		names, err := iamservice.AllNames(ctx, userIDs)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			if out[i].UserID != nil {
				out[i].UserName = names[*out[i].UserID]
			}
		}
	}
	return out, total, nil
}
