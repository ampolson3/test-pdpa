package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
)

var noticeTypes = []string{"privacy_notice", "privacy_policy", "cookie_policy", "cctv", "layered_short", "employee"}
var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// Notice is a privacy notice / policy (PNG-01, ม.23): a thin, tenant-owned wrapper — legal entity, subject type,
// slug, ST-04 status — around the PLT-16 document (DocumentID) that actually holds its text. This module
// creates the notice and its document's first draft; editing the document, the ม.23 checklist (PNG-02), DPO
// approval (PNG-14) and publish/versioning (PNG-08) all happen through PLT-08/PLT-16's own endpoints on that
// document, not through this struct.
type Notice struct {
	ID                uuid.UUID
	LegalEntityID     uuid.UUID
	SubjectTypeID     *uuid.UUID
	NoticeType        string
	Title             string
	Slug              string
	DocumentID        uuid.UUID
	Status            string // draft | in_review | published | retired (ST-04)
	OwnerUserID       *uuid.UUID
	ReviewCycleMonths int
	ActivityIDs       []uuid.UUID
	// PublicKey is the platform.public_keys key for /public/v1/notices/{key} (PNG-06) — nil until the first
	// publish issues one; it then stays for the notice's lifetime, same as CON-09's collection_points.public_key.
	PublicKey *string
	// PendingIsMaterialChange / PendingChangesPurpose (PNG-07) are staged by SetPublishIntent right before the
	// next publish and consumed/reset by OnDocumentPublished — never meaningful outside that short window.
	PendingIsMaterialChange bool
	PendingChangesPurpose   bool
	RowVersion              int32
	UpdatedAt               time.Time
}

type NoticeCursor struct {
	UpdatedAt time.Time
	ID        uuid.UUID
}

type NoticeFilter struct {
	NoticeType string
	Status     string
	After      *NoticeCursor
	Limit      int
}

const noticePageSize = 50

// WizardInput is PNG-01's wizard: the user picks a legal entity, notice type and either the RoPA processing
// activities it covers (BP-04 t1, compose from real activity data) or a PNG-03 starter template group
// (canned DRAFT sample text for one of the 8 fixed data-subject groups) — the two content sources are
// mutually exclusive; picking neither still composes an all-placeholder draft, same as PNG-01 always did.
type WizardInput struct {
	LegalEntityID uuid.UUID
	SubjectTypeID *uuid.UUID
	NoticeType    string
	Title         string
	Slug          string
	ActivityIDs   []uuid.UUID
	TemplateGroup string
}

// ActivityHasNotice is RRA-04's own read (rule 9: risk reads notice only through this exported method,
// never notice.notice_activity_links directly) — whether any notice, published or not, already names this
// RoPA activity. A draft-only notice still counts: the gap this answers ("ไม่มีประกาศที่ครอบคลุม") is about a
// notice existing to be finished and published, not about publish status itself, which PNG-02/05's own
// gates already enforce before publish.
func (s *Service) ActivityHasNotice(ctx context.Context, activityID uuid.UUID) (bool, error) {
	return noticestore.New(pdb.MustTxFromContext(ctx)).ActivityHasNotice(ctx, activityID)
}

func (s *Service) ListNotices(ctx context.Context, f NoticeFilter) ([]Notice, *NoticeCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > noticePageSize {
		limit = noticePageSize
	}
	p := noticestore.ListNoticesParams{Lim: int32(limit + 1)}
	if f.NoticeType != "" {
		p.NoticeType = &f.NoticeType
	}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.UpdatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListNotices(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Notice, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &NoticeCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}, nil
		}
		out = append(out, toNotice(noticestore.GetNoticeRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetNotice(ctx context.Context, id uuid.UUID) (Notice, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetNotice(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, err
	}
	n := toNotice(r)
	links, err := q.ListNoticeActivityLinks(ctx, id)
	if err != nil {
		return Notice{}, err
	}
	n.ActivityIDs = links
	return n, nil
}

// CreateWizard runs the wizard end to end (BP-04 t1-t2, the [*] -> draft transition of ST-04): validates the
// FKs are visible under RLS (rule 1), composes the draft content from the linked RoPA activities and org data,
// creates the underlying PLT-16 document with it, and links the activities — all in the request's own
// transaction (rule 1). Submitting for DPO review (ST-04 draft -> in_review) is PNG-14's job.
func (s *Service) CreateWizard(ctx context.Context, in WizardInput) (Notice, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len([]rune(in.Title)) > 300 {
		return Notice{}, fmt.Errorf("%w: title", ErrInvalid)
	}
	in.Slug = strings.TrimSpace(in.Slug)
	if !slugRE.MatchString(in.Slug) || len(in.Slug) > 120 {
		return Notice{}, fmt.Errorf("%w: slug", ErrInvalid)
	}
	if !contains(noticeTypes, in.NoticeType) {
		return Notice{}, fmt.Errorf("%w: notice_type", ErrInvalid)
	}
	legalEntity, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID)
	if err != nil {
		return Notice{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	if in.SubjectTypeID != nil {
		if _, err := s.Org.GetMaster(ctx, "data_subject_types", *in.SubjectTypeID); err != nil {
			return Notice{}, fmt.Errorf("%w: subject_type_id", ErrInvalid)
		}
	}

	if in.TemplateGroup != "" && len(in.ActivityIDs) > 0 {
		return Notice{}, fmt.Errorf("%w: template_group and activity_ids are mutually exclusive", ErrInvalid)
	}

	var content render.Content
	if in.TemplateGroup != "" {
		content, err = s.templateContent(ctx, in.TemplateGroup)
	} else {
		content, err = s.compose(ctx, legalEntity, in.ActivityIDs)
	}
	if err != nil {
		return Notice{}, err
	}

	doc, err := s.Docs.Create(ctx, docsservice.CreateInput{DocType: "notice", Title: in.Title, LegalEntityID: &in.LegalEntityID})
	if err != nil {
		return Notice{}, err
	}
	if _, err := s.Docs.SaveDraft(ctx, doc.ID, doc.RowVersion, docsservice.Draft{Title: in.Title, LegalEntityID: &in.LegalEntityID, Content: content}); err != nil {
		return Notice{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Notice{}, err
	}
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	var row noticestore.InsertNoticeRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = noticestore.New(pdb.MustTxFromContext(ctx)).InsertNotice(ctx, noticestore.InsertNoticeParams{
			ID: id, LegalEntityID: in.LegalEntityID, SubjectTypeID: pgUUID(in.SubjectTypeID), NoticeType: in.NoticeType,
			Title: in.Title, Slug: in.Slug, DocumentID: doc.ID,
		})
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Notice{}, fmt.Errorf("%w: slug", ErrInvalid)
	}
	if err != nil {
		return Notice{}, err
	}
	for _, aid := range in.ActivityIDs {
		if err := q.InsertNoticeActivityLink(ctx, noticestore.InsertNoticeActivityLinkParams{NoticeID: id, ActivityID: aid}); err != nil {
			return Notice{}, err
		}
	}

	out := toNotice(noticestore.GetNoticeRow(row))
	out.ActivityIDs = in.ActivityIDs
	if err := s.audit(ctx, "notice.notice.create", out.ID, nil, map[string]any{"title": out.Title, "notice_type": out.NoticeType, "document_id": out.DocumentID}); err != nil {
		return Notice{}, err
	}
	return out, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func toNotice(r noticestore.GetNoticeRow) Notice {
	return Notice{
		ID: r.ID, LegalEntityID: r.LegalEntityID, SubjectTypeID: uuidPtr(r.SubjectTypeID), NoticeType: r.NoticeType,
		Title: r.Title, Slug: r.Slug, DocumentID: r.DocumentID, Status: r.Status, OwnerUserID: uuidPtr(r.OwnerUserID),
		ReviewCycleMonths: int(r.ReviewCycleMonths), PublicKey: r.PublicKey,
		PendingIsMaterialChange: r.PendingIsMaterialChange, PendingChangesPurpose: r.PendingChangesPurpose,
		RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time,
	}
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
