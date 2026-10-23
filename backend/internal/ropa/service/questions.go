// ROPA-12: ask-the-DPO-or-advisor Q&A on each activity. Reuses PLT-07's own comment/mention layer
// exactly as its own module doc says ("ใช้ comment / mention กลาง + สถานะคำถาม") — a "question" is just
// a top-level comment on the activity, "answered" is its existing resolved flag; nothing new is stored.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
)

// ActivityCollabEntityType is the record type ropa.processing_activities is registered under with PLT-07
// (cmd/api's own collabSvc.Register) — a Q&A tab is the generic comment UI on this entity type.
const ActivityCollabEntityType = "ropa_activity"

// Question is one of the caller's own open (unanswered) questions, enriched with the activity it was
// asked on — the acceptance criterion's own "task list" entry.
type Question struct {
	CommentID    uuid.UUID
	ActivityID   uuid.UUID
	ActivityCode string
	ActivityName string
	AuthorID     uuid.UUID
	AuthorName   string
	Body         string
	CreatedAt    time.Time
}

// MyOpenQuestions lists every unresolved top-level comment on a ropa_activity record that @mentions the
// caller — their own Q&A task list. A record the caller can no longer see (deleted, or RLS-scoped out
// since the question was asked) is silently skipped rather than erroring the whole list.
func (s *Service) MyOpenQuestions(ctx context.Context) ([]Question, error) {
	g, _ := authz.FromContext(ctx)
	userID, err := uuid.Parse(g.UserID)
	if err != nil {
		return nil, ErrForbidden
	}
	comments, err := s.Collab.OpenQuestionsFor(ctx, ActivityCollabEntityType, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Question, 0, len(comments))
	for _, c := range comments {
		a, err := s.GetActivity(ctx, c.EntityID)
		if err != nil {
			continue
		}
		out = append(out, Question{
			CommentID: c.ID, ActivityID: a.ID, ActivityCode: a.Code, ActivityName: a.Name,
			AuthorID: c.AuthorID, AuthorName: c.AuthorName, Body: c.Body, CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}
