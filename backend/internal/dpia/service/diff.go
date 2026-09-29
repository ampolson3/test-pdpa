package service

import (
	"context"
	"reflect"
	"sort"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/forms"
)

// AssessmentDiffChange is one screening question whose answer differs between an assessment round and the
// round it supersedes (Assessment.PreviousID).
type AssessmentDiffChange struct {
	Question string
	Before   any // nil when the question wasn't answered in the previous round
	After    any // nil when the question isn't answered in this round
}

// AssessmentDiff is DPIA-14's acceptance criterion: what changed since the previous round.
type AssessmentDiff struct {
	AssessmentID uuid.UUID
	PreviousID   *uuid.UUID
	Changes      []AssessmentDiffChange
}

// CompareToPrevious diffs a screening round's factor answers against the round it supersedes
// (Assessment.PreviousID — DPIA-01's own round chain, built for re-screening). Scoped to screening factors
// only: necessity (DPIA-05) has no round concept to diff against, just a redo within one assessment id.
// Empty Changes for a first round (PreviousID nil) or an unchanged re-screen.
func (s *Service) CompareToPrevious(ctx context.Context, assessmentID uuid.UUID) (AssessmentDiff, error) {
	current, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return AssessmentDiff{}, err
	}
	diff := AssessmentDiff{AssessmentID: current.ID, PreviousID: current.PreviousID}
	if current.PreviousID == nil {
		return diff, nil
	}
	previous, err := s.GetAssessment(ctx, *current.PreviousID)
	if err != nil {
		return AssessmentDiff{}, err
	}
	before := answersByQuestion(previous.Factors)
	after := answersByQuestion(current.Factors)

	questions := make(map[string]struct{}, len(before)+len(after))
	for q := range before {
		questions[q] = struct{}{}
	}
	for q := range after {
		questions[q] = struct{}{}
	}
	sorted := make([]string, 0, len(questions))
	for q := range questions {
		sorted = append(sorted, q)
	}
	sort.Strings(sorted)

	for _, q := range sorted {
		b, hasBefore := before[q]
		a, hasAfter := after[q]
		if hasBefore && hasAfter && equalAnswers(b, a) {
			continue
		}
		change := AssessmentDiffChange{Question: q}
		if hasBefore {
			change.Before = b
		}
		if hasAfter {
			change.After = a
		}
		diff.Changes = append(diff.Changes, change)
	}
	return diff, nil
}

func answersByQuestion(factors []forms.Contribution) map[string]any {
	out := make(map[string]any, len(factors))
	for _, f := range factors {
		out[f.Question] = f.Answer
	}
	return out
}

func equalAnswers(a, b any) bool {
	return reflect.DeepEqual(a, b)
}
