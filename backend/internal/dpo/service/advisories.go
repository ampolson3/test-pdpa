// DPO-07: dpo.advisories/dpo.kb_articles were already fully specified in the baseline migrations
// (00016_dpo.sql) — a department's own question to the DPO, tracked until answered, with every answer
// promoted into the same searchable knowledge base (dpo.kb_articles) this feature's own acceptance
// criterion names ("คำถามที่ตอบแล้วค้นได้ในคลังคำตอบ"). No queries existed for either table until now.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpostore "pdpa-platform/internal/dpo/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// Advisory is one department's own question to the DPO (ม.42(1)).
type Advisory struct {
	ID              uuid.UUID
	AdvisoryNo      string
	OrgUnitID       *uuid.UUID
	RequesterUserID uuid.UUID
	RequesterName   string
	Subject         string
	Question        string
	Status          string // open | answered | closed
	Answer          *string
	AnsweredBy      *uuid.UUID
	AnsweredAt      *time.Time
	KbArticleID     *uuid.UUID
	RowVersion      int32
	CreatedAt       time.Time
}

// KbArticle is one published knowledge-base article — either a DPO-authored FAQ/policy note, or the
// automatic promotion of an answered Advisory (source "faq", category "advisory").
type KbArticle struct {
	ID          uuid.UUID
	Category    string
	Title       string
	Body        map[string]any
	Language    string
	Tags        []string
	Source      string
	PublishedAt *time.Time
	RowVersion  int32
}

func toAdvisory(r dpostore.DpoAdvisory) Advisory {
	a := Advisory{ID: r.ID, AdvisoryNo: r.AdvisoryNo, OrgUnitID: uuidPtr(r.OrgUnitID), RequesterUserID: r.RequesterUserID,
		Subject: r.Subject, Question: r.Question, Status: r.Status, Answer: r.Answer, AnsweredBy: uuidPtr(r.AnsweredBy),
		KbArticleID: uuidPtr(r.KbArticleID), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.AnsweredAt.Valid {
		t := r.AnsweredAt.Time
		a.AnsweredAt = &t
	}
	return a
}

func toKbArticle(r dpostore.DpoKbArticle) (KbArticle, error) {
	a := KbArticle{ID: r.ID, Category: r.Category, Title: r.Title, Language: r.Language, Tags: r.Tags, Source: r.Source, RowVersion: r.RowVersion}
	if len(r.Body) > 0 {
		if err := json.Unmarshal(r.Body, &a.Body); err != nil {
			return KbArticle{}, err
		}
	}
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time
		a.PublishedAt = &t
	}
	return a, nil
}

func nextAdvisoryNo(ctx context.Context, q *dpostore.Queries) (string, error) {
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockAdvisoryNumbering(ctx, year); err != nil {
		return "", err
	}
	prefix := "ADV-" + year + "-"
	n, err := q.CountAdvisoriesInYear(ctx, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%04d", prefix, n+1), nil
}

// SubmitAdvisory is BP's own "หน่วยงานส่งคำถาม": any employee may ask (dpo.advisory.create, EMP/OWNER/IT/
// MKT in the seeded RBAC), optionally against their own department (checked visible under RLS, rule 1,
// when given).
func (s *Service) SubmitAdvisory(ctx context.Context, orgUnitID *uuid.UUID, subject, question string) (Advisory, error) {
	subject, question = trimmed(subject), trimmed(question)
	if subject == "" || question == "" {
		return Advisory{}, fmt.Errorf("%w: subject and question are required", ErrInvalid)
	}
	if orgUnitID != nil {
		if _, err := s.Org.GetOrgUnit(ctx, *orgUnitID); err != nil {
			return Advisory{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
		}
	}
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return Advisory{}, fmt.Errorf("%w: no caller in context", ErrInvalid)
	}
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	no, err := nextAdvisoryNo(ctx, q)
	if err != nil {
		return Advisory{}, err
	}
	row, err := q.InsertAdvisory(ctx, dpostore.InsertAdvisoryParams{
		AdvisoryNo: no, OrgUnitID: pgUUID(orgUnitID), RequesterUserID: me, Subject: subject, Question: question, Actor: pgUUID(&me),
	})
	if err != nil {
		return Advisory{}, err
	}
	out := toAdvisory(row)
	return out, s.audit(ctx, "dpo.advisory.submit", out.ID, nil, map[string]any{"advisory_no": out.AdvisoryNo})
}

// GetAdvisory reads one advisory by id — RLS already scopes it to this tenant.
func (s *Service) GetAdvisory(ctx context.Context, id uuid.UUID) (Advisory, error) {
	r, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetAdvisory(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Advisory{}, ErrNotFound
	}
	if err != nil {
		return Advisory{}, err
	}
	out := toAdvisory(r)
	if names, err := iamservice.Names(ctx, []uuid.UUID{out.RequesterUserID}); err == nil {
		out.RequesterName = names[out.RequesterUserID]
	}
	return out, nil
}

// ListAdvisories filters by department and/or status.
func (s *Service) ListAdvisories(ctx context.Context, orgUnitID *uuid.UUID, status string) ([]Advisory, error) {
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListAdvisories(ctx, dpostore.ListAdvisoriesParams{
		OrgUnitID: pgUUID(orgUnitID), Status: optStatusPtr(status),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Advisory, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a := toAdvisory(r)
		out = append(out, a)
		ids = append(ids, a.RequesterUserID)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].RequesterName = names[out[i].RequesterUserID]
	}
	return out, nil
}

func optStatusPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// AnswerAdvisory is the acceptance criterion's own write half ("ตอบ → เก็บเป็นคลังคำตอบ"): a still-open
// advisory (dpo.advisory.update, DPO/LEGAL/PRIVACY in the seeded RBAC — the same roles that also hold
// dpo.kb.create) is answered and, in the same call, promoted into one new, already-published
// dpo.kb_articles row (category "advisory", source "faq") — every answered question is searchable in
// the knowledge base from the moment it's answered, with nothing left for a separate "publish" step.
func (s *Service) AnswerAdvisory(ctx context.Context, id uuid.UUID, answer string, version int32) (Advisory, error) {
	answer = trimmed(answer)
	if answer == "" {
		return Advisory{}, fmt.Errorf("%w: answer is required", ErrInvalid)
	}
	cur, err := s.GetAdvisory(ctx, id)
	if err != nil {
		return Advisory{}, err
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	row, err := q.AnswerAdvisory(ctx, dpostore.AnswerAdvisoryParams{ID: id, Answer: answer, AnsweredBy: pgUUID(me), RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Advisory{}, ErrVersionMismatch
	}
	if err != nil {
		return Advisory{}, err
	}
	body, err := json.Marshal(map[string]any{"question": cur.Question, "answer": answer})
	if err != nil {
		return Advisory{}, err
	}
	art, err := q.InsertKbArticle(ctx, dpostore.InsertKbArticleParams{
		Category: "advisory", Title: cur.Subject, Body: body, Language: "th", Tags: []string{}, Source: "faq", Actor: pgUUID(me),
	})
	if err != nil {
		return Advisory{}, err
	}
	if err := q.LinkAdvisoryKbArticle(ctx, dpostore.LinkAdvisoryKbArticleParams{ID: id, KbArticleID: pgUUID(&art.ID)}); err != nil {
		return Advisory{}, err
	}
	out := toAdvisory(row)
	out.KbArticleID = &art.ID
	return out, s.audit(ctx, "dpo.advisory.answer", out.ID, map[string]any{"status": "open"}, map[string]any{"status": "answered", "kb_article_id": art.ID})
}

// SearchKbArticles is the acceptance criterion's own read half ("ค้นได้ในคลังคำตอบ") — every published
// article, optionally narrowed by category or a plain substring match on title/body.
func (s *Service) SearchKbArticles(ctx context.Context, category, q string) ([]KbArticle, error) {
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).SearchKbArticles(ctx, dpostore.SearchKbArticlesParams{
		Category: optStrPtr(category), Q: optStrPtr(q),
	})
	if err != nil {
		return nil, err
	}
	out := make([]KbArticle, 0, len(rows))
	for _, r := range rows {
		a, err := toKbArticle(r)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func optStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func trimmed(s string) string {
	return strings.TrimSpace(s)
}
