// DPX-04 (audit remediation tracking): both columns this feature needs — gov.audit_findings.task_id and
// evidence_file_id — existed since the baseline migration but nothing wrote to either until now. The
// acceptance criterion itself ("ข้อตรวจพบปิดได้เมื่อแนบหลักฐาน") is a gate on SetAuditFindingStatus, not a
// new endpoint: a finding may not enter "done" or "accepted" without evidence already on record, mirroring
// DPX-05's own CompleteJob ("a job cannot close without evidence") almost exactly.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	govstore "pdpa-platform/internal/gov/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// GetAuditFinding reads one finding by id, RLS-scoped to the caller's tenant.
func (s *Service) GetAuditFinding(ctx context.Context, id uuid.UUID) (AuditFinding, error) {
	if !has(ctx, AuditPermRead) {
		return AuditFinding{}, ErrForbidden
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).GetAuditFinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditFinding{}, ErrNotFound
	}
	if err != nil {
		return AuditFinding{}, err
	}
	return toFinding(auditFindingRow(row)), nil
}

// AttachFindingEvidence is DPX-04's own "แนบหลักฐาน": the file must be the caller's own clean,
// still-unattached PLT-09 upload — the same rule PLT-09 evidence attachments use everywhere else in this
// codebase (DPX-05's own CompleteJob, DSAR-04's agent document, VEN-06's certificate, VEN-14's destruction
// certificate).
func (s *Service) AttachFindingEvidence(ctx context.Context, id uuid.UUID, rowVersion int32, evidenceFileID uuid.UUID) (AuditFinding, error) {
	if !has(ctx, AuditPermUpdate) {
		return AuditFinding{}, ErrForbidden
	}
	if evidenceFileID == uuid.Nil {
		return AuditFinding{}, fmt.Errorf("%w: evidence_file_id", ErrInvalid)
	}
	f, err := s.Files.Get(ctx, evidenceFileID)
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
		return AuditFinding{}, ErrFileNotUsable
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).SetAuditFindingEvidence(ctx, govstore.SetAuditFindingEvidenceParams{
		ID: id, RowVersion: rowVersion, EvidenceFileID: pgUUIDPtr(&evidenceFileID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditFinding{}, ErrVersionMismatch
		}
		return AuditFinding{}, err
	}
	if err := s.Files.AttachSystem(ctx, evidenceFileID, AuditFindingType, id); err != nil {
		return AuditFinding{}, err
	}
	out := toFinding(auditFindingRow(row))
	return out, s.audit(ctx, "gov.audit_finding.evidence", AuditFindingType, out.ID, nil, map[string]any{"evidence_file_id": evidenceFileID})
}

// AssignFindingTask is DPX-04's own "มอบหมายงานตามข้อตรวจพบ": opens one dpo.tasks remediation job for this
// finding (through the dpo module's own exported service, rule 9 — gov never writes to dpo.tasks directly)
// and links it back via task_id. Idempotent, the same DPIA-08/RRA-08/RTG-10 dedup discipline: a finding
// that already has a task is returned unchanged rather than opening a second one.
func (s *Service) AssignFindingTask(ctx context.Context, id uuid.UUID, rowVersion int32, in AssignFindingTaskInput) (AuditFinding, error) {
	if !has(ctx, AuditPermUpdate) {
		return AuditFinding{}, ErrForbidden
	}
	current, err := s.GetAuditFinding(ctx, id)
	if err != nil {
		return AuditFinding{}, err
	}
	if current.TaskID != nil {
		return current, nil
	}
	if s.Dpo == nil {
		return AuditFinding{}, fmt.Errorf("%w: assigning a task is not configured", ErrInvalid)
	}
	priority := in.Priority
	if priority == "" {
		priority = "medium"
	}
	if !validPriority(priority) {
		return AuditFinding{}, fmt.Errorf("%w: priority", ErrInvalid)
	}
	description := ""
	if current.Recommendation != nil {
		description = *current.Recommendation
	}
	taskID, err := s.Dpo.OpenAuditFindingTask(ctx, id, current.Title, description, in.AssigneeUserID, in.DueAt, priority)
	if err != nil {
		return AuditFinding{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).SetAuditFindingTask(ctx, govstore.SetAuditFindingTaskParams{
		ID: id, RowVersion: rowVersion, TaskID: pgUUIDPtr(&taskID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditFinding{}, ErrVersionMismatch
		}
		return AuditFinding{}, err
	}
	out := toFinding(auditFindingRow(row))
	return out, s.audit(ctx, "gov.audit_finding.assign_task", AuditFindingType, out.ID, nil, map[string]any{"task_id": taskID})
}

// AssignFindingTaskInput is what a caller provides when mobilizing work on a finding.
type AssignFindingTaskInput struct {
	AssigneeUserID *uuid.UUID
	DueAt          *time.Time
	Priority       string
}

func validPriority(p string) bool {
	switch p {
	case "low", "medium", "high", "urgent":
		return true
	default:
		return false
	}
}
