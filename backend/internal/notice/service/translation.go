package service

import (
	"context"
	"reflect"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
)

// ErrTranslationStale blocks a bilingual notice's document from publishing when the Thai content changed since
// the version this one supersedes but the English content did not (PNG-05's acceptance criterion: a
// translation that wasn't updated to match the latest version). Unwrap follows ErrChecklistIncomplete's own
// pattern for reporting through PLT-08's publish endpoint (422).
type ErrTranslationStale struct{}

func (e *ErrTranslationStale) Error() string {
	return "notice: the English translation was not updated to match the latest Thai content"
}
func (e *ErrTranslationStale) Unwrap() error { return versioning.ErrInvalidRequest }

// StaleTranslation is PNG-05's check, as a pure function: true only when both versions carry English content,
// the Thai content changed between them, and the English content did not — i.e. someone edited the Thai
// section without touching its translation. Adding English for the first time, or dropping it, is never
// flagged: those aren't "an update the translation missed".
func StaleTranslation(current, previous render.Content) bool {
	prevEn, hadEn := previous["en"]
	curEn, hasEn := current["en"]
	if !hadEn || !hasEn {
		return false
	}
	thChanged := !reflect.DeepEqual(current["th"], previous["th"])
	enChanged := !reflect.DeepEqual(curEn, prevEn)
	return thChanged && !enChanged
}

// GetTranslationStatus is the read-side check for the UI (PNG-05): whether the notice's *current draft* would
// be blocked by staleTranslation if submitted and published right now, compared against the document's
// currently published content. false with no error when the document has never been published (nothing to
// compare against yet) or has no open draft.
func (s *Service) GetTranslationStatus(ctx context.Context, noticeID uuid.UUID) (bool, error) {
	n, err := s.GetNotice(ctx, noticeID)
	if err != nil {
		return false, err
	}
	doc, err := s.Docs.Get(ctx, n.DocumentID)
	if err != nil {
		return false, err
	}
	if doc.Draft == nil {
		return false, nil
	}
	published, ok, err := s.Docs.PublishedContent(ctx, n.DocumentID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return StaleTranslation(doc.Draft.Content, published), nil
}
