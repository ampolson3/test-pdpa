package service

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	govstore "pdpa-platform/internal/gov/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// RetentionSchedule is one asset's recurring retention/disposal clock, driven by a ropa.retention_rules row.
type RetentionSchedule struct {
	ID              uuid.UUID
	RetentionRuleID uuid.UUID
	AssetID         uuid.UUID
	NextDueAt       time.Time
	Status          string // active | paused | retired
	RowVersion      int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DisposalJob is one run of actually disposing of an asset's data once its schedule is due.
type DisposalJob struct {
	ID             uuid.UUID
	ScheduleID     uuid.UUID
	DueAt          time.Time
	Method         string // delete | destroy | anonymize | return
	LegalHoldID    *uuid.UUID
	AssigneeUserID *uuid.UUID
	ApprovedBy     *uuid.UUID
	Status         string // pending | approved | in_progress | done | on_hold
	RecordCount    *int32
	CompletedAt    *time.Time
	EvidenceFileID *uuid.UUID
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

var scheduleStatuses = []string{"active", "paused", "retired"}

// SaveSchedule creates a retention schedule, checking both FKs are visible under the caller's own
// RLS first (rule 1) — the same pattern ROPA-02/DPO-01 already use for a cross-schema FK.
func (s *Service) SaveSchedule(ctx context.Context, sch RetentionSchedule) (RetentionSchedule, error) {
	if !has(ctx, PermCreate) {
		return RetentionSchedule{}, ErrForbidden
	}
	if _, err := s.Ropa.GetRetentionRule(ctx, sch.RetentionRuleID); err != nil {
		return RetentionSchedule{}, fmt.Errorf("%w: retention_rule_id", ErrInvalid)
	}
	if _, err := s.Ropa.GetAsset(ctx, sch.AssetID); err != nil {
		return RetentionSchedule{}, fmt.Errorf("%w: asset_id", ErrInvalid)
	}
	if sch.NextDueAt.IsZero() {
		return RetentionSchedule{}, fmt.Errorf("%w: next_due_at", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return RetentionSchedule{}, err
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertRetentionSchedule(ctx, govstore.InsertRetentionScheduleParams{
		ID: id, RetentionRuleID: sch.RetentionRuleID, AssetID: sch.AssetID, NextDueAt: pgDate(sch.NextDueAt),
	})
	if err != nil {
		return RetentionSchedule{}, err
	}
	out := toSchedule(row)
	if err := s.audit(ctx, "gov.retention_schedule.create", ScheduleType, out.ID, nil, scheduleAudit(out)); err != nil {
		return RetentionSchedule{}, err
	}
	return out, nil
}

// SetScheduleStatus pauses/resumes/retires a schedule — a paused or retired schedule is never
// picked up by the daily due-check.
func (s *Service) SetScheduleStatus(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (RetentionSchedule, error) {
	if !has(ctx, PermUpdate) {
		return RetentionSchedule{}, ErrForbidden
	}
	if !slices.Contains(scheduleStatuses, status) {
		return RetentionSchedule{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateRetentionScheduleStatus(ctx, govstore.UpdateRetentionScheduleStatusParams{ID: id, RowVersion: rowVersion, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RetentionSchedule{}, ErrVersionMismatch
		}
		return RetentionSchedule{}, err
	}
	out := toSchedule(govstore.InsertRetentionScheduleRow(row))
	if err := s.audit(ctx, "gov.retention_schedule.status", ScheduleType, out.ID, nil, scheduleAudit(out)); err != nil {
		return RetentionSchedule{}, err
	}
	return out, nil
}

func (s *Service) GetSchedule(ctx context.Context, id uuid.UUID) (RetentionSchedule, error) {
	if !has(ctx, PermRead) {
		return RetentionSchedule{}, ErrForbidden
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).GetRetentionSchedule(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RetentionSchedule{}, ErrNotFound
		}
		return RetentionSchedule{}, err
	}
	return toSchedule(govstore.InsertRetentionScheduleRow(row)), nil
}

type ScheduleFilter struct {
	AssetID  *uuid.UUID
	Status   *string
	CursorAt *time.Time
	CursorID *uuid.UUID
	Limit    int32
}

func (s *Service) ListSchedules(ctx context.Context, f ScheduleFilter) ([]RetentionSchedule, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	lim := f.Limit
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListRetentionSchedules(ctx, govstore.ListRetentionSchedulesParams{
		AssetID: pgUUIDPtr(f.AssetID), Status: f.Status, CursorAt: cursorTime(f.CursorAt), CursorID: pgUUIDPtr(f.CursorID), Lim: lim,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RetentionSchedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSchedule(govstore.InsertRetentionScheduleRow(r)))
	}
	return out, nil
}

// --- disposal jobs ---

type JobFilter struct {
	Status   *string
	CursorAt *time.Time
	CursorID *uuid.UUID
	Limit    int32
}

func (s *Service) ListJobs(ctx context.Context, f JobFilter) ([]DisposalJob, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	lim := f.Limit
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListDisposalJobs(ctx, govstore.ListDisposalJobsParams{
		Status: f.Status, CursorAt: cursorTime(f.CursorAt), CursorID: pgUUIDPtr(f.CursorID), Lim: lim,
	})
	if err != nil {
		return nil, err
	}
	out := make([]DisposalJob, 0, len(rows))
	for _, r := range rows {
		out = append(out, toJob(govstore.InsertDisposalJobRow(r)))
	}
	return out, nil
}

func (s *Service) GetJob(ctx context.Context, id uuid.UUID) (DisposalJob, error) {
	if !has(ctx, PermRead) {
		return DisposalJob{}, ErrForbidden
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).GetDisposalJob(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisposalJob{}, ErrNotFound
		}
		return DisposalJob{}, err
	}
	return toJob(govstore.InsertDisposalJobRow(row)), nil
}

// ApproveJob is BP-11's confirm+approve step, merged into the one approved_by column the schema
// actually has (a scope trim from BP-11's own three-human-step flow, documented in docs/modules/DPX.md).
// A legal hold still active on the job's asset blocks approval outright.
func (s *Service) ApproveJob(ctx context.Context, id uuid.UUID, rowVersion int32, assigneeUserID *uuid.UUID) (DisposalJob, error) {
	if !has(ctx, PermApprove) {
		return DisposalJob{}, ErrForbidden
	}
	cur, err := s.GetJob(ctx, id)
	if err != nil {
		return DisposalJob{}, err
	}
	if blocked, err := s.blockedByHold(ctx, cur); err != nil {
		return DisposalJob{}, err
	} else if blocked {
		return DisposalJob{}, ErrOnLegalHold
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.ApproveDisposalJob(ctx, govstore.ApproveDisposalJobParams{ID: id, RowVersion: rowVersion, AssigneeUserID: pgUUIDPtr(assigneeUserID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisposalJob{}, ErrInvalidTransition
		}
		return DisposalJob{}, err
	}
	out := toJob(govstore.InsertDisposalJobRow(row))
	if err := s.audit(ctx, "gov.disposal_job.approve", JobType, out.ID, nil, jobAudit(out)); err != nil {
		return DisposalJob{}, err
	}
	return out, nil
}

func (s *Service) StartJob(ctx context.Context, id uuid.UUID, rowVersion int32) (DisposalJob, error) {
	if !has(ctx, PermExecute) {
		return DisposalJob{}, ErrForbidden
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.StartDisposalJob(ctx, govstore.StartDisposalJobParams{ID: id, RowVersion: rowVersion})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisposalJob{}, ErrInvalidTransition
		}
		return DisposalJob{}, err
	}
	out := toJob(govstore.InsertDisposalJobRow(row))
	if err := s.audit(ctx, "gov.disposal_job.start", JobType, out.ID, nil, jobAudit(out)); err != nil {
		return DisposalJob{}, err
	}
	return out, nil
}

// CompleteJob is the acceptance criterion's other half: a job cannot close without evidence. The
// file must be the caller's own clean, still-unattached upload (the same rule PLT-09 evidence
// attachments use everywhere else in this codebase).
func (s *Service) CompleteJob(ctx context.Context, id uuid.UUID, rowVersion int32, evidenceFileID uuid.UUID, recordCount *int32) (DisposalJob, error) {
	if !has(ctx, PermExecute) {
		return DisposalJob{}, ErrForbidden
	}
	if evidenceFileID == uuid.Nil {
		return DisposalJob{}, ErrEvidenceRequired
	}
	f, err := s.Files.Get(ctx, evidenceFileID)
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
		return DisposalJob{}, ErrFileNotUsable
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.CompleteDisposalJob(ctx, govstore.CompleteDisposalJobParams{ID: id, RowVersion: rowVersion, EvidenceFileID: evidenceFileID, RecordCount: recordCount})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisposalJob{}, ErrInvalidTransition
		}
		return DisposalJob{}, err
	}
	if err := s.Files.AttachSystem(ctx, evidenceFileID, JobType, id); err != nil {
		return DisposalJob{}, err
	}
	out := toJob(govstore.InsertDisposalJobRow(row))
	if err := s.audit(ctx, "gov.disposal_job.complete", JobType, out.ID, nil, jobAudit(out)); err != nil {
		return DisposalJob{}, err
	}
	return out, nil
}

// ReleaseHold clears an on_hold job back to pending once its legal hold is lifted — re-approval
// (ApproveJob) re-checks the hold is actually gone before letting the job move forward again.
func (s *Service) ReleaseHold(ctx context.Context, id uuid.UUID, rowVersion int32) (DisposalJob, error) {
	if !has(ctx, PermApprove) {
		return DisposalJob{}, ErrForbidden
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.ReleaseDisposalJobHold(ctx, govstore.ReleaseDisposalJobHoldParams{ID: id, RowVersion: rowVersion})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DisposalJob{}, ErrInvalidTransition
		}
		return DisposalJob{}, err
	}
	out := toJob(govstore.InsertDisposalJobRow(row))
	if err := s.audit(ctx, "gov.disposal_job.release_hold", JobType, out.ID, nil, jobAudit(out)); err != nil {
		return DisposalJob{}, err
	}
	return out, nil
}

// blockedByHold asks dsar (rule 9) whether the job's own schedule's asset is under an active
// legal hold right now, and — if so — puts the job itself on_hold so the state is visible without
// waiting for the next daily sweep.
func (s *Service) blockedByHold(ctx context.Context, job DisposalJob) (bool, error) {
	if s.Dsar == nil {
		return false, nil
	}
	sch, err := s.GetSchedule(ctx, job.ScheduleID)
	if err != nil {
		return false, err
	}
	hold, err := s.Dsar.ActiveHoldForAsset(ctx, sch.AssetID)
	if err != nil {
		return false, err
	}
	if hold == nil {
		return false, nil
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.HoldDisposalJob(ctx, govstore.HoldDisposalJobParams{ID: job.ID, RowVersion: job.RowVersion, LegalHoldID: pgUUIDPtr(&hold.ID)}); err != nil {
		return false, err
	}
	return true, nil
}

// RunRetentionCheck is the daily per-tenant sweep's own work (jobs.go's gov.retention_check):
// every active schedule past its next_due_at with no disposal job still open gets a fresh one —
// unless the schedule's own asset is under an active legal hold, in which case the job is created
// already on_hold rather than pending (so it never shows up for approval until the hold lifts).
func (s *Service) RunRetentionCheck(ctx context.Context) (created int, err error) {
	q := govstore.New(pdb.MustTxFromContext(ctx))
	due, err := q.ListDueSchedulesWithoutOpenJob(ctx, pgDate(s.now()))
	if err != nil {
		return 0, err
	}
	for _, r := range due {
		sch := toSchedule(govstore.InsertRetentionScheduleRow(r))
		rule, err := s.Ropa.GetRetentionRule(ctx, sch.RetentionRuleID)
		if err != nil {
			continue // the rule or asset disappeared/became invisible since the schedule was made; skip, don't fail the whole sweep
		}
		status := "pending"
		var holdID *uuid.UUID
		if s.Dsar != nil {
			if hold, err := s.Dsar.ActiveHoldForAsset(ctx, sch.AssetID); err == nil && hold != nil {
				status, holdID = "on_hold", &hold.ID
			}
		}
		id, err := uuid.NewV7()
		if err != nil {
			return created, err
		}
		row, err := q.InsertDisposalJob(ctx, govstore.InsertDisposalJobParams{
			ID: id, ScheduleID: sch.ID, DueAt: pgDate(sch.NextDueAt), Method: rule.DisposalMethod, LegalHoldID: pgUUIDPtr(holdID), Status: status,
		})
		if err != nil {
			return created, err
		}
		out := toJob(govstore.InsertDisposalJobRow(row))
		if err := s.audit(ctx, "gov.disposal_job.create", JobType, out.ID, nil, jobAudit(out)); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func toSchedule(r govstore.InsertRetentionScheduleRow) RetentionSchedule {
	return RetentionSchedule{
		ID: r.ID, RetentionRuleID: r.RetentionRuleID, AssetID: r.AssetID, NextDueAt: fromPgDate(r.NextDueAt),
		Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func toJob(r govstore.InsertDisposalJobRow) DisposalJob {
	j := DisposalJob{
		ID: r.ID, ScheduleID: r.ScheduleID, DueAt: fromPgDate(r.DueAt), Method: r.Method, LegalHoldID: fromPgUUID(r.LegalHoldID),
		AssigneeUserID: fromPgUUID(r.AssigneeUserID), ApprovedBy: fromPgUUID(r.ApprovedBy), Status: r.Status, RecordCount: r.RecordCount,
		EvidenceFileID: fromPgUUID(r.EvidenceFileID), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		j.CompletedAt = &t
	}
	return j
}

func scheduleAudit(s RetentionSchedule) map[string]any {
	return map[string]any{"asset_id": s.AssetID, "retention_rule_id": s.RetentionRuleID, "next_due_at": s.NextDueAt, "status": s.Status}
}

func jobAudit(j DisposalJob) map[string]any {
	return map[string]any{"schedule_id": j.ScheduleID, "status": j.Status, "method": j.Method}
}
