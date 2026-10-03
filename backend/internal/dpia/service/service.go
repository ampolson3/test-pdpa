// Package service is the dpia module (DPIA-01/02): the DPIA screening — whether a RoPA processing activity
// needs a full impact assessment (ม.37(1), TDPG 4.0-P) — on top of a PLT-06 form, judged against the
// tenant's own configurable thresholds (assess.screening_rules). It works in the transaction of the context
// (CLAUDE.md rule 1) and reads other modules only through their exported services (rule 9). Full DPIA
// authoring (describing the processing, scoring risk, DPO opinion, approval — DPIA-04 onward) is a sibling
// feature layered on the same assess.assessments row, not built here.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
)

// Entity types for versioning/audit (none registered with PLT-08 yet — screening is a one-shot write, no
// draft/approval cycle of its own).
const (
	AssessmentEntityType    = "dpia_assessment"
	ScreeningRuleEntityType = "dpia_screening_rule"
)

var (
	ErrNotFound        = errors.New("dpia: not found")
	ErrInvalid         = errors.New("dpia: invalid")
	ErrVersionMismatch = errors.New("dpia: version mismatch")
)

// FieldError is one problem with the request (CLAUDE.md's usual FieldError shape).
type FieldError struct{ Field, Code string }

// ValidationError lists every problem with a screening submission (missing/invalid answers).
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return fmt.Sprintf("dpia: invalid: %v", e.Fields) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Ropa is what dpia reads from the RoPA module (rule 9): the activity a screening is about must be visible
// under the caller's own RLS before dpia ever references its id. DPIA-04's own read-only description
// (ActivityDescription) needs the same activity's child rows, already exported by ropaservice for its own
// completeness checks — no new ropa-side code, just a wider interface here.
type Ropa interface {
	GetActivity(ctx context.Context, id uuid.UUID) (ropaservice.Activity, error)
	ListActivityPurposes(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityPurpose, error)
	ListActivityData(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityData, error)
	ListActivityRecipients(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityRecipient, error)
	ListActivityTransfers(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityTransfer, error)
	ListRetentionRules(ctx context.Context, activityID uuid.UUID) ([]ropaservice.RetentionRule, error)
}

// Org is what dpia reads from the org module (rule 9): master-data names (lawful basis, data category,
// subject type, country) and external-party names for DPIA-04's own description.
type Org interface {
	GetMaster(ctx context.Context, kind string, id uuid.UUID) (orgservice.MasterItem, error)
	ListMaster(ctx context.Context, kind string) ([]orgservice.MasterItem, error)
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
}

// Service runs the dpia module for the tenant of the transaction in ctx.
type Service struct {
	Forms *forms.Service
	Ropa  Ropa
	Org   Org
	Audit *audit.Service
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		// No authz.Grants on a public route would land here too (see CON-11), but every dpia endpoint is
		// admin-only — this fallback only guards against a missing/odd principal, not a real public path.
		var t string
		if terr := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); terr != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("dpia: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if u, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Audit.Write(ctx, e)
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
