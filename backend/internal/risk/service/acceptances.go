// RRA-08 (ยอมรับความเสี่ยงพร้อมผู้อนุมัติ, «extend» RRA-06): risk.acceptances was already fully specified in
// the baseline migrations, unused until now. Deliberately scoped to the one acceptance criterion
// ("การยอมรับหมดอายุและเตือนทบทวนตามวันที่ตั้ง") — a single reminder checkpoint per acceptance's own
// expires_at, the same one-time-per-record pattern DPA-10/VEN-06 already established, not a recurring
// sweep (an acceptance's own review date doesn't move on its own the way a recurring re-assessment cycle
// does). RecordAcceptance is gated by the already-seeded dpo.risk.approve permission (DPO + EXEC, matching
// this feature's own actor line exactly) — the caller recording the acceptance already *is* the approver,
// so there is no separate request/decide round through platform.approvals (PLT-08's own engine, built for
// versioned documents, not a one-time decision) — leaving risk.acceptances.approval_id NULL.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	riskstore "pdpa-platform/internal/risk/store"
)

// Acceptance is one risk.acceptances row.
type Acceptance struct {
	ID         uuid.UUID
	RiskID     uuid.UUID
	Reason     string
	ApprovedBy *uuid.UUID
	ApprovedAt time.Time
	ExpiresAt  time.Time
	RowVersion int32
	CreatedAt  time.Time
}

// RecordAcceptance is this feature's own write path: the caller (DPO or EXEC, both holding dpo.risk.approve
// — checked here, not just at the endpoint, since this is a permanent decision like ORG-06/DPIA-03/VEN-14's
// own approve-gated actions) records that they accept the risk's residual as-is, with a reason and a review
// date. The risk itself flips to status "accepted" in the same call; a later RecordAcceptance (a fresh
// decision before the previous one expired) simply supersedes it with a newer row and keeps the risk
// accepted. A risk already closed is refused — there is nothing left to accept.
func (s *Service) RecordAcceptance(ctx context.Context, riskID uuid.UUID, reason string, expiresAt time.Time) (Acceptance, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("dpo.risk.approve") {
		return Acceptance{}, fmt.Errorf("%w: dpo.risk.approve required", ErrForbidden)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Acceptance{}, fmt.Errorf("%w: reason", ErrInvalid)
	}
	risk, err := s.GetRisk(ctx, riskID)
	if err != nil {
		return Acceptance{}, err
	}
	if risk.Status == "closed" {
		return Acceptance{}, fmt.Errorf("%w: risk is closed", ErrInvalidTransition)
	}
	today := s.now().UTC().Truncate(24 * time.Hour)
	if !expiresAt.After(today) {
		return Acceptance{}, fmt.Errorf("%w: expires_at must be in the future", ErrInvalid)
	}

	var actor *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		actor = &u
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Acceptance{}, err
	}
	now := s.now()
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).InsertAcceptance(ctx, riskstore.InsertAcceptanceParams{
		ID: id, RiskID: riskID, Reason: reason, ApprovedBy: pgUUID(actor),
		ApprovedAt: pgtype.Timestamptz{Time: now, Valid: true}, ExpiresAt: pgtype.Date{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Acceptance{}, err
	}
	if risk.Status != "accepted" {
		if _, err := riskstore.New(pdb.MustTxFromContext(ctx)).SetRiskStatus(ctx, riskstore.SetRiskStatusParams{
			ID: riskID, Status: "accepted", RowVersion: risk.RowVersion,
		}); err != nil {
			return Acceptance{}, err
		}
	}
	out := toAcceptance(row)
	if err := s.scheduleAcceptanceReviewReminder(ctx, out.ID, out.ExpiresAt); err != nil {
		return Acceptance{}, err
	}
	if err := s.audit(ctx, "risk.acceptance.record", "risk", riskID, nil,
		map[string]any{"acceptance_id": out.ID, "expires_at": out.ExpiresAt, "reason": out.Reason}); err != nil {
		return Acceptance{}, err
	}
	return out, nil
}

// ListAcceptances returns every acceptance ever recorded for a risk, newest first.
func (s *Service) ListAcceptances(ctx context.Context, riskID uuid.UUID) ([]Acceptance, error) {
	if _, err := s.GetRisk(ctx, riskID); err != nil {
		return nil, err
	}
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListAcceptancesForRisk(ctx, riskID)
	if err != nil {
		return nil, err
	}
	out := make([]Acceptance, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAcceptance(r))
	}
	return out, nil
}

// AcceptanceReviewArgs is risk.acceptance_review: the single checkpoint of one acceptance's own expires_at.
// ExpiresAt pins the schedule so a stale tick (the acceptance was superseded by a fresh RecordAcceptance
// call, or the risk/acceptance no longer exists) is harmless.
type AcceptanceReviewArgs struct {
	jobs.TenantArgs
	AcceptanceID string `json:"acceptance_id"`
	ExpiresAt    string `json:"expires_at"`
}

func (AcceptanceReviewArgs) Kind() string { return "risk.acceptance_review" }

// scheduleAcceptanceReviewReminder enqueues the single checkpoint exactly at expiresAt — unlike VEN-06's own
// certificate reminder (which fires a notice period *ahead* of expiry), the acceptance criterion reads the
// set review date itself as the moment both things happen: the acceptance expires and the reminder fires,
// so there is only the one checkpoint, not an earlier warning. A date already in the past (shouldn't happen
// given RecordAcceptance's own future-date validation, but defensive the same way every other scheduler in
// this codebase is) still fires once, immediately.
func (s *Service) scheduleAcceptanceReviewReminder(ctx context.Context, id uuid.UUID, expiresAt time.Time) error {
	if s.River == nil {
		return nil
	}
	at := expiresAt
	if now := s.now(); at.Before(now) {
		at = now
	}
	tenant, err := tenantIDFromTx(ctx)
	if err != nil {
		return err
	}
	args := AcceptanceReviewArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, AcceptanceID: id.String(),
		ExpiresAt: expiresAt.Format("2006-01-02")}
	_, err = jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// FireAcceptanceReviewReminder is the acceptance criterion itself: while the acceptance's own expires_at
// still matches what the job was scheduled for (it wasn't superseded by a fresh acceptance or deleted
// since), and the risk it backs is still "accepted" (an owner may already have moved it on by hand), the
// risk reverts to "open" — the expired decision no longer holds — and every DPO is notified that this
// risk's acceptance has expired and needs review.
func (s *Service) FireAcceptanceReviewReminder(ctx context.Context, id uuid.UUID, expiresAt time.Time) error {
	q := riskstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetAcceptance(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	acc := toAcceptance(row)
	if !acc.ExpiresAt.Equal(expiresAt) {
		return nil // stale schedule — superseded by a fresh acceptance with a later date
	}
	latest, err := q.GetLatestAcceptanceForRisk(ctx, acc.RiskID)
	if err != nil {
		return err
	}
	if latest.ID != acc.ID {
		return nil // a newer acceptance already supersedes this one
	}
	risk, err := s.GetRisk(ctx, acc.RiskID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if risk.Status == "accepted" {
		if _, err := q.SetRiskStatus(ctx, riskstore.SetRiskStatusParams{ID: risk.ID, Status: "open", RowVersion: risk.RowVersion}); err != nil {
			return err
		}
	}
	if err := s.audit(ctx, "risk.acceptance.expire", "risk", acc.RiskID, nil,
		map[string]any{"acceptance_id": acc.ID, "expires_at": acc.ExpiresAt}); err != nil {
		return err
	}
	if s.Notify == nil {
		return nil
	}
	recipients, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	vars := map[string]any{"risk_title": risk.Title, "expires_at": acc.ExpiresAt.Format("2 Jan 2006")}
	for _, uid := range recipients {
		uid := uid
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "risk.acceptance_review", Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: "risk", EntityID: &acc.RiskID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

// AcceptanceReviewWorker works risk.acceptance_review.
type AcceptanceReviewWorker struct {
	river.WorkerDefaults[AcceptanceReviewArgs]
	Service *Service
}

func (w *AcceptanceReviewWorker) Work(ctx context.Context, job *river.Job[AcceptanceReviewArgs]) error {
	id, err1 := uuid.Parse(job.Args.AcceptanceID)
	at, err2 := time.Parse("2006-01-02", job.Args.ExpiresAt)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireAcceptanceReviewReminder(ctx, id, at)
}

func toAcceptance(r riskstore.RiskAcceptance) Acceptance {
	a := Acceptance{ID: r.ID, RiskID: r.RiskID, Reason: r.Reason, RowVersion: r.RowVersion}
	if r.ApprovedBy.Valid {
		u := uuid.UUID(r.ApprovedBy.Bytes)
		a.ApprovedBy = &u
	}
	if r.ApprovedAt.Valid {
		a.ApprovedAt = r.ApprovedAt.Time
	}
	if r.ExpiresAt.Valid {
		a.ExpiresAt = r.ExpiresAt.Time
	}
	if r.CreatedAt.Valid {
		a.CreatedAt = r.CreatedAt.Time
	}
	return a
}

func tenantIDFromTx(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}
