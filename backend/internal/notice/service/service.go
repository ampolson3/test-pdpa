// Package service is the notice module's business logic (PNG-01: the wizard-based privacy-notice generator).
// A notice is a thin wrapper — legal entity, subject type, slug, status (ST-04) — around a PLT-16 document
// (notices.document_id, NOT NULL): the wizard's only job is to compose that document's first draft from data
// already recorded elsewhere (ORG-07 master data, ROPA-03/06/07/08 processing activities) so a non-legal user
// gets a complete ม.23 draft without re-typing anything the platform already knows. Editing the composed text,
// the ม.23 checklist gate (PNG-02), DPO approval (PNG-14) and publish/versioning (PNG-08) are PLT-08/PLT-16's
// own machinery plus sibling PNG features layered on top — not rebuilt here.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

var (
	ErrNotFound        = errors.New("notice: not found")
	ErrInvalid         = errors.New("notice: invalid input")
	ErrVersionMismatch = errors.New("notice: version mismatch")
)

// Org is what notice reads from the organization module (rule 9): FK visibility (rule 1) for legal_entity_id and
// subject_type_id, master-data names (lawful bases, data categories, countries) to compose the draft's text, and
// the legal entity's merge fields for its contact section.
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
	GetMaster(ctx context.Context, kind string, id uuid.UUID) (orgservice.MasterItem, error)
	ListMaster(ctx context.Context, kind string) ([]orgservice.MasterItem, error)
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
	MergeFields(ctx context.Context, id uuid.UUID) (map[string]string, error)
}

// Ropa is what notice reads from the RoPA module (rule 9): a linked processing activity's purposes, lawful
// bases, data categories, retention and recipients/transfers — BP-04 step t2, "ดึง...จาก RoPA".
type Ropa interface {
	GetActivity(ctx context.Context, id uuid.UUID) (ropaservice.Activity, error)
	ListActivityPurposes(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityPurpose, error)
	ListActivityData(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityData, error)
	ListRetentionRules(ctx context.Context, activityID uuid.UUID) ([]ropaservice.RetentionRule, error)
	ListActivityRecipients(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityRecipient, error)
	ListActivityTransfers(ctx context.Context, activityID uuid.UUID) ([]ropaservice.ActivityTransfer, error)
}

// Docs is what notice reads/writes in the document composer (PLT-16, rule 9): every notice's content lives in
// its own platform.documents row (doc_type "notice"), already registered with a DPO approval step in
// internal/wiring.Docs — notice never touches platform.documents directly.
type Docs interface {
	Create(ctx context.Context, in docsservice.CreateInput) (docsservice.Document, error)
	SaveDraft(ctx context.Context, id uuid.UUID, rowVersion int32, d docsservice.Draft) (docsservice.Document, error)
}

type Service struct {
	Audit *audit.Service
	Org   Org
	Ropa  Ropa
	Docs  Docs
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("notice: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", EntityType: "notice", EntityID: &id, Action: action, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
