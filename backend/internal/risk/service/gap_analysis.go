package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	riskstore "pdpa-platform/internal/risk/store"
)

// Notice is what risk reads from the notice module (rule 9) for RRA-04's own "ไม่มีประกาศที่ครอบคลุม" gap —
// a local interface risk/service owns, satisfied directly by *noticeservice.Service (no import cycle:
// notice/service never imports risk/service, so no adapter struct is needed either).
type Notice interface {
	ActivityHasNotice(ctx context.Context, activityID uuid.UUID) (bool, error)
}

// GapRule is one row of risk.gap_rules (RRA-04): a named legal-gap check, global or tenant-owned, the
// same ORG-07 master-data visibility pattern risk.controls already uses. The actual check per code is
// gapPresent's own Go switch below — expression stays an unused placeholder until a real rule
// interpreter is ever needed.
type GapRule struct {
	ID       uuid.UUID
	Code     string
	Name     string
	Severity string // low | medium | high
	LegalRef string
}

// GapFinding is one risk.gap_findings row: one rule's result against one activity, open until the gap
// stops applying (AnalyzeActivity resolves it automatically on the next run) or RRA-07 waives it.
type GapFinding struct {
	ID         uuid.UUID
	RuleID     uuid.UUID
	RuleCode   string
	ActivityID uuid.UUID
	Status     string // open | resolved | waived
	DetectedAt time.Time
	ResolvedAt *time.Time
	TaskID     *uuid.UUID
}

func toGapFinding(id, ruleID uuid.UUID, ruleCode string, activityID uuid.UUID, status string, detectedAt, resolvedAt pgtype.Timestamptz, taskID pgtype.UUID) GapFinding {
	f := GapFinding{ID: id, RuleID: ruleID, RuleCode: ruleCode, ActivityID: activityID, Status: status}
	if detectedAt.Valid {
		f.DetectedAt = detectedAt.Time
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		f.ResolvedAt = &t
	}
	if taskID.Valid {
		tid := uuid.UUID(taskID.Bytes)
		f.TaskID = &tid
	}
	return f
}

// ListGapRules returns every active rule visible to the caller (RRA-04's own catalog, for reference and
// for the UI to show which legal article each gap type cites).
func (s *Service) ListGapRules(ctx context.Context) ([]GapRule, error) {
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListActiveGapRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GapRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, GapRule{ID: r.ID, Code: r.Code, Name: r.Name, Severity: r.Severity, LegalRef: r.LegalRef})
	}
	return out, nil
}

// gapPresent is RRA-04's own rule engine: one Go case per rule code. "เพิ่ม rule ได้" (rules are
// extensible) means adding a migration row here plus a new case — no generic expression language exists
// to interpret, since nothing in this feature's acceptance criterion asks for one.
func gapPresent(code string, missing map[string]bool, hasNotice bool) bool {
	switch code {
	case "no_lawful_basis":
		return missing["purpose"]
	case "no_retention":
		return missing["retention"]
	case "no_notice_coverage":
		return !hasNotice
	case "transfer_no_basis":
		return missing["transfer_basis"]
	case "sensitive_no_consent":
		return missing["sensitive_consent"]
	default:
		return false // an unknown/custom rule code with no Go case yet is a no-op, not an error
	}
}

// AnalyzeActivity is RRA-04's own acceptance criterion: every active rule is checked against the
// activity's current RoPA data, each gap that is present gets (or keeps) an open risk.gap_findings row,
// and any rule that no longer applies has its existing open finding resolved automatically — so re-running
// this (on demand now, by a future sweep later) always reflects the activity's live state, the same "live,
// never cached" discipline RRA-01's own Score already follows.
func (s *Service) AnalyzeActivity(ctx context.Context, activityID uuid.UUID) ([]GapFinding, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	missingList, err := s.Ropa.MissingItems(ctx, activityID)
	if err != nil {
		return nil, err
	}
	missing := make(map[string]bool, len(missingList))
	for _, m := range missingList {
		missing[m] = true
	}
	hasNotice := true
	if s.Notice != nil {
		hasNotice, err = s.Notice.ActivityHasNotice(ctx, activityID)
		if err != nil {
			return nil, err
		}
	}

	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return nil, err
	}
	q := riskstore.New(pdb.MustTxFromContext(ctx))
	out := make([]GapFinding, 0, len(rules))
	for _, r := range rules {
		present := gapPresent(r.Code, missing, hasNotice)
		existing, err := q.GetOpenGapFinding(ctx, riskstore.GetOpenGapFindingParams{RuleID: r.ID, ActivityID: activityID})
		notFound := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !notFound {
			return nil, err
		}
		switch {
		case present && notFound:
			row, err := q.InsertGapFinding(ctx, riskstore.InsertGapFindingParams{RuleID: r.ID, ActivityID: activityID})
			if err != nil {
				return nil, err
			}
			out = append(out, toGapFinding(row.ID, row.RuleID, r.Code, row.ActivityID, row.Status, row.DetectedAt, row.ResolvedAt, row.TaskID))
		case present:
			out = append(out, toGapFinding(existing.ID, existing.RuleID, r.Code, existing.ActivityID, existing.Status, existing.DetectedAt, existing.ResolvedAt, existing.TaskID))
		case !present && !notFound:
			if _, err := q.ResolveGapFinding(ctx, existing.ID); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// AnalyzeAllActivities sweeps every activity visible to the caller's tenant — the batch counterpart to
// AnalyzeActivity, for an on-demand "re-check everything" action (and the natural body of a future
// scheduled sweep once one is needed).
func (s *Service) AnalyzeAllActivities(ctx context.Context) (int, error) {
	ids, err := s.Ropa.ListActivityIDs(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := s.AnalyzeActivity(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// ListGapFindingsForActivity is the read-only half for one activity's own page — no recomputation,
// just what AnalyzeActivity last found (open, resolved and waived alike, newest first).
func (s *Service) ListGapFindingsForActivity(ctx context.Context, activityID uuid.UUID) ([]GapFinding, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListGapFindingsByActivity(ctx, activityID)
	if err != nil {
		return nil, err
	}
	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return nil, err
	}
	codeByID := make(map[uuid.UUID]string, len(rules))
	for _, r := range rules {
		codeByID[r.ID] = r.Code
	}
	out := make([]GapFinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, toGapFinding(r.ID, r.RuleID, codeByID[r.RuleID], r.ActivityID, r.Status, r.DetectedAt, r.ResolvedAt, r.TaskID))
	}
	return out, nil
}

// GetGapFinding returns one finding by id, RLS-scoped to the caller's tenant like every other read here.
func (s *Service) GetGapFinding(ctx context.Context, id uuid.UUID) (GapFinding, error) {
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).GetGapFinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return GapFinding{}, ErrNotFound
	}
	if err != nil {
		return GapFinding{}, err
	}
	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return GapFinding{}, err
	}
	code := ""
	for _, r := range rules {
		if r.ID == row.RuleID {
			code = r.Code
			break
		}
	}
	return toGapFinding(row.ID, row.RuleID, code, row.ActivityID, row.Status, row.DetectedAt, row.ResolvedAt, row.TaskID), nil
}

var validTaskPriorities = map[string]bool{"low": true, "medium": true, "high": true, "urgent": true}

// RemediateFinding is RRA-07's own "เลือกช่องว่าง → สร้างงาน": the finding must still be open and, if
// given, the assignee must be a real active user of this tenant; the opened dpo.tasks job is linked back
// onto the finding's own (until now unused) task_id column. The other half of the acceptance criterion —
// closing that task re-checks the rule and clears the finding once it passes — lives on the dpo side
// (dpo/service's own task-status transition calls back into AnalyzeActivity), not here.
func (s *Service) RemediateFinding(ctx context.Context, findingID uuid.UUID, assigneeUserID *uuid.UUID, dueAt *time.Time, priority string) (GapFinding, error) {
	finding, err := s.GetGapFinding(ctx, findingID)
	if err != nil {
		return GapFinding{}, err
	}
	if finding.Status != "open" {
		return GapFinding{}, fmt.Errorf("%w: finding is not open", ErrInvalid)
	}
	if priority == "" {
		priority = "medium"
	}
	if !validTaskPriorities[priority] {
		return GapFinding{}, fmt.Errorf("%w: priority", ErrInvalid)
	}
	if assigneeUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*assigneeUserID})
		if err != nil {
			return GapFinding{}, err
		}
		if _, ok := names[*assigneeUserID]; !ok {
			return GapFinding{}, fmt.Errorf("%w: assignee_user_id", ErrInvalid)
		}
	}
	if s.Dpo == nil {
		return GapFinding{}, errors.New("risk: no task service configured")
	}
	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return GapFinding{}, err
	}
	ruleName := finding.RuleCode
	for _, r := range rules {
		if r.ID == finding.RuleID {
			ruleName = r.Name
			break
		}
	}
	taskID, err := s.Dpo.OpenGapRemediationTask(ctx, findingID, "แก้ไขช่องว่าง: "+ruleName,
		fmt.Sprintf("แก้ไขช่องว่างทางกฎหมายที่ตรวจพบ: %s", ruleName), assigneeUserID, dueAt, priority)
	if err != nil {
		return GapFinding{}, err
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).SetGapFindingTask(ctx,
		riskstore.SetGapFindingTaskParams{ID: findingID, TaskID: pgUUID(&taskID)})
	if err != nil {
		return GapFinding{}, err
	}
	if err := s.audit(ctx, "risk.gap_finding.remediate", "gap_finding", findingID, nil,
		map[string]any{"task_id": taskID}); err != nil {
		return GapFinding{}, err
	}
	return toGapFinding(row.ID, row.RuleID, finding.RuleCode, row.ActivityID, row.Status, row.DetectedAt, row.ResolvedAt, row.TaskID), nil
}

// ListOpenGapFindings is the tenant-wide gap register (RRA-04's own "รายการช่องว่าง" list) — every open
// finding across every activity, newest first.
func (s *Service) ListOpenGapFindings(ctx context.Context) ([]GapFinding, error) {
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListOpenGapFindings(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return nil, err
	}
	codeByID := make(map[uuid.UUID]string, len(rules))
	for _, r := range rules {
		codeByID[r.ID] = r.Code
	}
	out := make([]GapFinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, toGapFinding(r.ID, r.RuleID, codeByID[r.RuleID], r.ActivityID, r.Status, r.DetectedAt, r.ResolvedAt, r.TaskID))
	}
	return out, nil
}
