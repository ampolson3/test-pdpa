// Package service is the gov module's first feature: DPX-05, retention & disposal management
// (ม.33/ม.37(3)). A gov.retention_schedules row names a ropa.retention_rules rule and the
// ropa.assets system it governs; a daily sweep (jobs.go) finds schedules past their next_due_at
// with no disposal job still open and opens a fresh gov.disposal_jobs row — the acceptance
// criterion's "ข้อมูลครบกำหนดสร้างงานทำลายอัตโนมัติ" (data reaching its due date automatically
// creates a disposal job). A job can only reach "done" once it carries evidence
// ("...และปิดได้เมื่อแนบหลักฐาน").
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// Entity types (audit, files).
const (
	ScheduleType = "gov_retention_schedule"
	JobType      = "gov_disposal_job"
)

// Permission codes (docs/security/permissions.yaml, already seeded).
const (
	PermRead    = "dpx.retention.read"
	PermCreate  = "dpx.retention.create"
	PermUpdate  = "dpx.retention.update"
	PermDelete  = "dpx.retention.delete"
	PermApprove = "dpx.retention.approve"
	PermExecute = "dpx.retention.execute"
)

var (
	ErrNotFound          = errors.New("gov: not found")
	ErrInvalid           = errors.New("gov: invalid input")
	ErrForbidden         = errors.New("gov: forbidden")
	ErrVersionMismatch   = errors.New("gov: version mismatch")
	ErrInvalidTransition = errors.New("gov: invalid transition")
	ErrOnLegalHold       = errors.New("gov: asset is under an active legal hold")
	ErrEvidenceRequired  = errors.New("gov: evidence file is required to close a disposal job")
	ErrFileNotUsable     = errors.New("gov: file not usable")
)

var disposalMethods = []string{"delete", "destroy", "anonymize", "return"}

// Ropa is what gov reads from the RoPA module (rule 9): FK visibility (rule 1) for a schedule's
// retention_rule_id/asset_id.
type Ropa interface {
	GetAsset(ctx context.Context, id uuid.UUID) (ropaservice.Asset, error)
	GetRetentionRule(ctx context.Context, id uuid.UUID) (ropaservice.RetentionRule, error)
}

// Dsar is what gov reads from the DSAR module (rule 9): whether an asset is under an active legal
// hold (dsar.legal_holds), which must suspend that asset's disposal jobs. No import cycle: dsar
// does not import gov, so (unlike the ropa<->dpia situation DPX-06 had to route around with a
// wiring adapter) gov can reference dsarservice's own exported return type directly here, the same
// way breach's own Org interface references orgservice.LegalEntity directly.
type Dsar interface {
	ActiveHoldForAsset(ctx context.Context, assetID uuid.UUID) (*dsarservice.LegalHold, error)
}

// Files is what gov needs from file storage (PLT-09) for a disposal job's evidence.
type Files interface {
	Get(ctx context.Context, id uuid.UUID) (files.File, error)
	AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error
}

// Service runs the gov module for the tenant of the transaction in ctx.
type Service struct {
	Audit *audit.Service
	Ropa  Ropa
	Dsar  Dsar
	Files Files
	River *river.Client[pgx.Tx]
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

func currentUser(ctx context.Context) *uuid.UUID {
	g, _ := authz.FromContext(ctx)
	if u, err := uuid.Parse(g.UserID); err == nil {
		return &u
	}
	return nil
}

func has(ctx context.Context, perm string) bool {
	g, _ := authz.FromContext(ctx)
	return g.Has(perm)
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if u := currentUser(ctx); u != nil {
		e.ActorType, e.ActorID = "user", u
	}
	return s.Audit.Write(ctx, e)
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func fromPgDate(d pgtype.Date) time.Time { return d.Time }

func pgUUIDPtr(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func cursorTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func fromPgUUID(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
