package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/publickeys"
)

// NoticeVersion is one published snapshot of a notice (PNG-06, ม.23): the effective date, which languages it
// carries, the ม.23 checklist it passed at that moment, and a link to the immutable PLT-16 document version
// that actually holds the frozen text.
type NoticeVersion struct {
	ID                uuid.UUID
	NoticeID          uuid.UUID
	VersionNo         int32
	DocumentVersionID uuid.UUID
	Languages         []string
	EffectiveFrom     time.Time
	IsMaterialChange  bool
	ChangesPurpose    bool
	Checklist         []ChecklistItem
	PublicURL         string
	PublishedAt       time.Time
}

// OnDocumentPublished is PNG-06's own acceptance criterion, wired into docs.Service as the "notice" document
// type's SetOnPublished hook (cmd/api/main.go, right after docsSvc.SetValidate("notice", ...) — same
// sequencing rule): every time a notice's underlying PLT-16 document is published, record one
// notice.notice_versions row (effective date, languages, the ม.23 checklist this exact text passed —
// PNG-02's own Checklist(), snapshotted rather than recomputed later so a past round's result never drifts
// if the checklist logic itself changes), flip the notice to published with this as its current version, and
// — on the very first publish only — issue the platform.public_keys key the public page resolves (PNG-06's
// "หน้า public แสดงเวอร์ชันปัจจุบัน"); later publishes reuse the same key, so a bookmarked public URL keeps
// working across every future update. "วันมีผล" (effective date) is whatever the draft's own EffectiveFrom
// said (the same field PLT-16's publish endpoint already asks every document type for); a notice published
// without one defaults to the publish moment itself, so the column is never left to a legally meaningless
// NULL. `is_material_change`/`changes_purpose` (PNG-07's re-consent trigger) have no input wired yet — no
// screen asks for them in this pass — so they stay at the column's own default (false) until PNG-07 exists
// to set them for real.
func (s *Service) OnDocumentPublished(ctx context.Context, documentID uuid.UUID, u docsservice.PublishedUpdate) error {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	nr, err := q.GetNoticeByDocumentID(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	n := toNotice(noticestore.GetNoticeRow(nr))

	key := n.PublicKey
	if key == nil {
		k, err := publickeys.Issue(ctx, publickeys.EntityNotice, n.ID, nil)
		if err != nil {
			return err
		}
		key = &k
	}
	publicURL := "/n/" + *key

	eff := s.now()
	if u.EffectiveFrom != nil {
		eff = *u.EffectiveFrom
	}
	checklistJSON, err := json.Marshal(Checklist(u.Content))
	if err != nil {
		return err
	}

	vid, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if _, err := q.InsertNoticeVersion(ctx, noticestore.InsertNoticeVersionParams{
		ID: vid, NoticeID: n.ID, VersionNo: u.VersionNo, DocumentVersionID: u.DocumentVersionID,
		Languages: u.Languages, EffectiveFrom: pgtype.Date{Time: eff, Valid: true},
		ChecklistResult: checklistJSON, PublicUrl: &publicURL,
	}); err != nil {
		return err
	}

	var pkParam *string
	if n.PublicKey == nil {
		pkParam = key // first publish: store the key just issued; a later one leaves the existing one alone
	}
	if _, err := q.SetNoticePublished(ctx, noticestore.SetNoticePublishedParams{
		ID: n.ID, CurrentVersionID: pgtype.UUID{Bytes: vid, Valid: true}, PublicKey: pkParam,
	}); err != nil {
		return err
	}
	return s.audit(ctx, "notice.notice.publish", n.ID, nil,
		map[string]any{"version_no": u.VersionNo, "document_version_id": u.DocumentVersionID, "effective_from": eff.Format(time.DateOnly)})
}

// ListNoticeVersions is the "ดูประวัติย้อนหลัง" half of the acceptance criterion: every published version of
// a notice, newest first.
func (s *Service) ListNoticeVersions(ctx context.Context, noticeID uuid.UUID) ([]NoticeVersion, error) {
	if _, err := s.GetNotice(ctx, noticeID); err != nil {
		return nil, err
	}
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListNoticeVersions(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	out := make([]NoticeVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, toNoticeVersion(noticestore.GetNoticeVersionRow(r)))
	}
	return out, nil
}

// --- Public page (PNG-06): no permission check — the resolved public key already named exactly this notice. ---

// PublicNotice resolves the notice a public key stands for; callers must have already checked
// publickeys.Key.EntityType == publickeys.EntityNotice.
func (s *Service) PublicNotice(ctx context.Context, noticeID uuid.UUID) (Notice, error) {
	r, err := noticestore.New(pdb.MustTxFromContext(ctx)).GetNoticeByPublicKeyEntity(ctx, noticeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, err
	}
	n := toNotice(noticestore.GetNoticeRow(r))
	if n.Status != "published" {
		return Notice{}, ErrNotFound
	}
	return n, nil
}

// PublicVersions is PublicNotice's history list.
func (s *Service) PublicVersions(ctx context.Context, noticeID uuid.UUID) ([]NoticeVersion, error) {
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListNoticeVersions(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	out := make([]NoticeVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, toNoticeVersion(noticestore.GetNoticeVersionRow(r)))
	}
	return out, nil
}

// PublicVersionHTML renders one of the notice's published versions (versionNo nil: the current one) in lang.
func (s *Service) PublicVersionHTML(ctx context.Context, n Notice, versionNo *int32, lang string) ([]byte, error) {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	var v noticestore.GetNoticeVersionRow
	if versionNo == nil {
		rows, err := q.ListNoticeVersions(ctx, n.ID)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, ErrNotFound
		}
		v = noticestore.GetNoticeVersionRow(rows[0]) // newest first
	} else {
		r, err := q.GetNoticeVersion(ctx, noticestore.GetNoticeVersionParams{NoticeID: n.ID, VersionNo: *versionNo})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		v = noticestore.GetNoticeVersionRow(r)
	}
	return s.Docs.PublicVersionHTML(ctx, n.DocumentID, v.DocumentVersionID, lang)
}

func toNoticeVersion(r noticestore.GetNoticeVersionRow) NoticeVersion {
	var checklist []ChecklistItem
	_ = json.Unmarshal(r.ChecklistResult, &checklist)
	return NoticeVersion{
		ID: r.ID, NoticeID: r.NoticeID, VersionNo: r.VersionNo, DocumentVersionID: r.DocumentVersionID,
		Languages: r.Languages, EffectiveFrom: r.EffectiveFrom.Time, IsMaterialChange: r.IsMaterialChange,
		ChangesPurpose: r.ChangesPurpose, Checklist: checklist, PublicURL: deref(r.PublicUrl), PublishedAt: r.PublishedAt.Time,
	}
}
