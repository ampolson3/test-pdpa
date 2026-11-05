package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	consentstore "pdpa-platform/internal/consent/store"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/importer"
)

// ImportType is CON-22's own PLT-14 import type: one row per (data subject × purpose) decision carried over
// from a legacy consent system, with its own original date. It writes through the exact same Record/
// writeDecisions core every live submission uses (hash-chained receipt, append-only transaction, ST-01
// status projection, consent.* events) — the acceptance criterion's own "imported completely and reconciles"
// is PLT-14's own all-or-nothing Apply plus its error report, not a bespoke reconciliation table: nothing
// in this schema models "old system vs. platform" reconciliation (that is CON-21's own `reconcile_items`/
// `reconcile_runs`, a different BP-02 concern — downstream-webhook drift, not a one-time backfill).
const ImportType = "consent.legacy"

var legacyDecisions = map[string]string{"consented": TxConsented, "not_consented": TxNotConsented, "withdrawn": TxWithdrawn}

// LegacyImport is the import type's own definition.
func (s *Service) LegacyImport() importer.Type {
	return importer.Type{
		Permission: "consent.bulk.execute",
		Columns: []importer.Column{
			{Key: "legal_entity_registration_no", Label: map[string]string{"th": "เลขทะเบียนนิติบุคคล", "en": "Legal entity registration no."}, Required: true},
			{Key: "identifier_type", Label: map[string]string{"th": "ประเภทตัวระบุ", "en": "Identifier type"}, Required: true, Aliases: []string{"identifier type"}},
			{Key: "identifier_value", Label: map[string]string{"th": "ตัวระบุ", "en": "Identifier"}, Required: true, Aliases: []string{"identifier"}},
			{Key: "purpose_code", Label: map[string]string{"th": "รหัสวัตถุประสงค์", "en": "Purpose code"}, Required: true, Aliases: []string{"purpose"}},
			{Key: "decision", Label: map[string]string{"th": "การตัดสินใจ", "en": "Decision"}, Required: true},
			{Key: "occurred_at", Label: map[string]string{"th": "วันที่ตัดสินใจเดิม", "en": "Original decision date"}, Required: true, Aliases: []string{"date"}},
		},
		Validate: s.validateLegacyRow,
		Apply:    s.applyLegacyRow,
	}
}

func (s *Service) validateLegacyRow(ctx context.Context, _ int, row importer.Row) []importer.FieldError {
	var errs []importer.FieldError
	if _, err := s.legacyEntity(ctx, row["legal_entity_registration_no"]); err != nil {
		errs = append(errs, importer.FieldError{Column: "legal_entity_registration_no", Message: "unknown_legal_entity"})
	}
	kind := crypto.IdentifierKind(strings.ToLower(strings.TrimSpace(row["identifier_type"])))
	if _, err := crypto.Normalize(kind, row["identifier_value"]); err != nil {
		errs = append(errs, importer.FieldError{Column: "identifier_value", Message: "invalid_identifier"})
	}
	if _, err := s.legacyPurpose(ctx, row["purpose_code"]); err != nil {
		errs = append(errs, importer.FieldError{Column: "purpose_code", Message: "unknown_purpose"})
	}
	if _, ok := legacyDecisions[strings.ToLower(strings.TrimSpace(row["decision"]))]; !ok {
		errs = append(errs, importer.FieldError{Column: "decision", Message: "invalid_decision"})
	}
	if _, err := importer.ParseDate(row["occurred_at"]); err != nil {
		errs = append(errs, importer.FieldError{Column: "occurred_at", Message: "invalid_date"})
	}
	return errs
}

func (s *Service) applyLegacyRow(ctx context.Context, _ int, row importer.Row) error {
	entity, err := s.legacyEntity(ctx, row["legal_entity_registration_no"])
	if err != nil {
		return err
	}
	purpose, err := s.legacyPurpose(ctx, row["purpose_code"])
	if err != nil {
		return err
	}
	decision, ok := legacyDecisions[strings.ToLower(strings.TrimSpace(row["decision"]))]
	if !ok {
		return invalid("decision")
	}
	when, err := importer.ParseDate(row["occurred_at"])
	if err != nil {
		return err
	}
	cpID, err := s.legacyCollectionPoint(ctx, entity.ID)
	if err != nil {
		return err
	}
	cp := CollectionPoint{ID: cpID, Channel: "import", Status: "active", Purposes: []CPPurpose{purpose}}
	id := Identifier{Type: string(crypto.IdentifierKind(strings.ToLower(strings.TrimSpace(row["identifier_type"])))), Value: row["identifier_value"]}

	// A withdrawn legacy record never had a "" -> WITHDRAWN edge of its own (ST-01 only declares
	// ACTIVE -> WITHDRAWN): the source system showing "withdrawn" implies consent was given at some point
	// before it was taken back, so that's replayed as two receipts at the same original date — writeDecisions'
	// own chain-ordering already bumps the second a microsecond past the first, keeping both on record.
	steps := []string{decision}
	if decision == TxWithdrawn {
		steps = []string{TxConsented, TxWithdrawn}
	}
	for _, step := range steps {
		sub := Submission{CollectionPointID: cpID, Identifiers: []Identifier{id}, Source: "import", Language: "th", OccurredAt: &when,
			Decisions: []Decision{{PurposeCode: purpose.Code, PurposeVersionNo: *purpose.CurrentVersionNo, Decision: step}}}
		decisions, err := s.checkDecisions(ctx, cp, sub)
		if err != nil {
			return err
		}
		if _, err := s.writeDecisions(ctx, cp, sub, decisions); err != nil {
			return err
		}
	}
	return nil
}

// legacyEntity resolves a row's legal entity by its registration number (ORG-01's own real, mod-11-checked
// business key) rather than a free-text fuzzy name match — every legal entity already carries one.
func (s *Service) legacyEntity(ctx context.Context, no string) (orgservice.LegalEntity, error) {
	no = strings.TrimSpace(no)
	all, err := s.Org.ListLegalEntities(ctx)
	if err != nil {
		return orgservice.LegalEntity{}, err
	}
	for _, e := range all {
		if e.RegistrationNo == no {
			return e, nil
		}
	}
	return orgservice.LegalEntity{}, fmt.Errorf("%w: legal_entity_registration_no", ErrInvalidRequest)
}

// legacyPurpose resolves a row's purpose by its code against the tenant's own currently-published purposes —
// only a published purpose has a current version to decide against.
func (s *Service) legacyPurpose(ctx context.Context, code string) (CPPurpose, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	purposes, err := s.ListPurposes(ctx)
	if err != nil {
		return CPPurpose{}, err
	}
	for _, p := range purposes {
		if p.Code != code || p.Status != "active" || p.CurrentVersion == nil {
			continue
		}
		no := p.CurrentVersion.No
		return CPPurpose{PurposeID: p.ID, Code: p.Code, Status: p.Status, IsSensitive: p.IsSensitive,
			CurrentVersionID: &p.CurrentVersion.ID, CurrentVersionNo: &no, ConsentText: p.Live.ConsentText}, nil
	}
	return CPPurpose{}, fmt.Errorf("%w: purpose_code", ErrInvalidRequest)
}

// legacyCollectionPoint gets or creates the one "legacy import" collection point a legal entity's imported
// rows attribute their receipt to (channel = "import", CON-01's own `channels` var deliberately excludes it —
// "import ... points are made by their own features", i.e. this one) — never reached from /public/v1, so it
// skips CON-09's checklist/public-key publish step entirely (ActivateCollectionPoint).
func (s *Service) legacyCollectionPoint(ctx context.Context, legalEntityID uuid.UUID) (uuid.UUID, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	code := "legacy-import-" + legalEntityID.String()
	if id, err := q.GetCollectionPointByCode(ctx, code); err == nil {
		return id, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	if err := q.InsertCollectionPoint(ctx, consentstore.InsertCollectionPointParams{
		ID: id, Code: code, Name: "Legacy import", Channel: "import", LegalEntityID: legalEntityID,
		AllowedOrigins: []string{}, VerificationMethod: "none",
	}); err != nil {
		return uuid.Nil, err
	}
	if err := q.ActivateCollectionPoint(ctx, id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
