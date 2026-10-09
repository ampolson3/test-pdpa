// DPA-03 (ข้อกำหนดที่ต้องมี): attach clauses from PLT-16's clause library (DPA-01) to an agreement, and check
// the result against agreement.mandatory_rules before the wrapping document can be submitted for approval
// (the acceptance criterion — a contract missing a mandatory clause cannot be sent for approval).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/versioning"
)

// Clause is one platform.clause_library entry attached to an agreement.
type Clause struct {
	ID              uuid.UUID
	AgreementID     uuid.UUID
	ClauseID        uuid.UUID
	ClauseCode      string
	ClauseTitle     string
	LegalRef        string
	ClauseVersionNo int32
	Position        int16
	IsMandatory     bool
	RowVersion      int32
	CreatedAt       time.Time
}

// AddClauseInput attaches a clause. Position of 0 means "append after every clause already attached".
type AddClauseInput struct {
	ClauseID uuid.UUID
	Position int16
}

// AddClause attaches a published clause from the library (DPA-01) to an agreement — only while the
// agreement is still a draft: once it's submitted, CheckSubmittable has already checked the attached set,
// and editing it afterward would make that check meaningless (PLT-08 itself locks the document's own
// content the same way once a version leaves draft; agreement.clauses needs its own lock since it's a
// sibling table PLT-08 doesn't know about).
func (s *Service) AddClause(ctx context.Context, agreementID uuid.UUID, in AddClauseInput) (Clause, error) {
	a, err := s.GetAgreement(ctx, agreementID)
	if err != nil {
		return Clause{}, err
	}
	if a.Status != "draft" {
		return Clause{}, fmt.Errorf("%w: agreement is not a draft", ErrInvalid)
	}
	clause, _, err := s.Docs.GetClause(ctx, in.ClauseID)
	if err != nil {
		if errors.Is(err, docs.ErrNotFound) {
			return Clause{}, fmt.Errorf("%w: clause_id", ErrInvalid)
		}
		return Clause{}, err
	}
	if clause.Status != "published" {
		return Clause{}, fmt.Errorf("%w: clause_id must be a published clause", ErrInvalid)
	}

	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	have, err := q.ListAgreementClauseCodes(ctx, agreementID)
	if err != nil {
		return Clause{}, err
	}
	for _, code := range have {
		if code == clause.Code {
			return Clause{}, fmt.Errorf("%w: clause %s is already attached", ErrInvalid, clause.Code)
		}
	}
	position := in.Position
	if position == 0 {
		n, err := q.CountAgreementClauses(ctx, agreementID)
		if err != nil {
			return Clause{}, err
		}
		position = int16(n) + 1
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Clause{}, err
	}
	versionNo := clause.VersionNo
	row, err := q.InsertAgreementClause(ctx, agreementstore.InsertAgreementClauseParams{
		ID: id, AgreementID: agreementID, ClauseID: pgUUID(&in.ClauseID), ClauseVersionNo: &versionNo,
		Position: position, IsMandatory: clause.IsMandatory,
	})
	if err != nil {
		return Clause{}, err
	}
	if err := s.audit(ctx, "agreement.clause.add", agreementID, nil, map[string]any{"clause_code": clause.Code}); err != nil {
		return Clause{}, err
	}
	return toClause(row, clause.Code, title(clause), clause.LegalRef), nil
}

// ListClauses returns every clause attached to an agreement, in position order, with the library's own
// code/title/legal_ref joined in for display.
func (s *Service) ListClauses(ctx context.Context, agreementID uuid.UUID) ([]Clause, error) {
	if _, err := s.GetAgreement(ctx, agreementID); err != nil {
		return nil, err
	}
	rows, err := agreementstore.New(pdb.MustTxFromContext(ctx)).ListAgreementClauses(ctx, agreementID)
	if err != nil {
		return nil, err
	}
	out := make([]Clause, 0, len(rows))
	for _, r := range rows {
		var versionNo int32
		if r.ClauseVersionNo != nil {
			versionNo = *r.ClauseVersionNo
		}
		var legalRef string
		if r.ClauseLegalRef != nil {
			legalRef = *r.ClauseLegalRef
		}
		var code, clauseTitle string
		if r.ClauseCode != nil {
			code = *r.ClauseCode
		}
		if r.ClauseTitle != nil {
			clauseTitle = *r.ClauseTitle
		}
		out = append(out, Clause{
			ID: r.ID, AgreementID: r.AgreementID, ClauseID: pgUUIDVal(r.ClauseID), ClauseCode: code, ClauseTitle: clauseTitle,
			LegalRef: legalRef, ClauseVersionNo: versionNo, Position: r.Position, IsMandatory: r.IsMandatory,
			RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// RemoveClause detaches a clause from an agreement — only while the agreement is still a draft.
func (s *Service) RemoveClause(ctx context.Context, agreementID, clauseRowID uuid.UUID) error {
	a, err := s.GetAgreement(ctx, agreementID)
	if err != nil {
		return err
	}
	if a.Status != "draft" {
		return fmt.Errorf("%w: agreement is not a draft", ErrInvalid)
	}
	n, err := agreementstore.New(pdb.MustTxFromContext(ctx)).DeleteAgreementClause(ctx, agreementstore.DeleteAgreementClauseParams{
		ID: clauseRowID, AgreementID: agreementID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "agreement.clause.remove", agreementID, nil, map[string]any{"clause_row_id": clauseRowID})
}

// MissingClause is one agreement.mandatory_rules entry this agreement still needs — DPA-03's own "แผงตรวจ
// clause ที่ขาด" (missing-clause panel).
type MissingClause struct {
	Code     string
	LegalRef string
}

// ruleCondition is agreement.mandatory_rules.condition's only shape used so far: a rule that applies only
// when the agreement actually involves a cross-border transfer (ม.28-29) — every other seeded rule has no
// condition and is always required. A richer condition language can be added here if a future rule needs
// one; there's nothing to design ahead of that need.
type ruleCondition struct {
	RequiresTransfer bool `json:"requires_transfer"`
}

// MissingMandatoryClauses is the read side of DPA-03's own panel: which agreement.mandatory_rules codes for
// this agreement's type are not yet attached, live (never cached, same as every other completeness check in
// this codebase).
func (s *Service) MissingMandatoryClauses(ctx context.Context, agreementID uuid.UUID) ([]MissingClause, error) {
	a, err := s.GetAgreement(ctx, agreementID)
	if err != nil {
		return nil, err
	}
	return s.missingMandatoryClauses(ctx, a)
}

func (s *Service) missingMandatoryClauses(ctx context.Context, a Agreement) ([]MissingClause, error) {
	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	rules, err := q.ListMandatoryRules(ctx, a.AgreementType)
	if err != nil {
		return nil, err
	}
	have, err := q.ListAgreementClauseCodes(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	haveSet := make(map[string]bool, len(have))
	for _, c := range have {
		haveSet[c] = true
	}
	var transferChecked, hasTransfer bool
	var out []MissingClause
	for _, r := range rules {
		if haveSet[r.ClauseCode] {
			continue
		}
		if len(r.Condition) > 0 {
			var cond ruleCondition
			if err := json.Unmarshal(r.Condition, &cond); err != nil {
				return nil, err
			}
			if cond.RequiresTransfer {
				if !transferChecked {
					hasTransfer, err = s.agreementHasTransfer(ctx, a)
					if err != nil {
						return nil, err
					}
					transferChecked = true
				}
				if !hasTransfer {
					continue // this rule doesn't apply to an agreement with no cross-border transfer on record
				}
			}
		}
		out = append(out, MissingClause{Code: r.ClauseCode, LegalRef: r.LegalRef})
	}
	return out, nil
}

// agreementHasTransfer reads ropa (rule 9, through the already-exported Ropa interface) rather than
// querying ropa.activity_transfers directly — unlike platform.clause_library's own global reference rows,
// activity_transfers is another module's real tenant data.
func (s *Service) agreementHasTransfer(ctx context.Context, a Agreement) (bool, error) {
	for _, aid := range a.ActivityIDs {
		transfers, err := s.Ropa.ListActivityTransfers(ctx, aid)
		if err != nil {
			return false, err
		}
		if len(transfers) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ErrMissingMandatoryClauses blocks a draft agreement's document from being submitted for approval while a
// ม.40 mandatory clause (or, for the cross-border rule, one that actually applies to this agreement) hasn't
// been attached yet — DPA-03's acceptance criterion. Unwrap reports it as an invalid request through PLT-08's
// generic submit endpoint (422), following notice's own ErrChecklistIncomplete pattern for the same kind of
// gate (PNG-02).
type ErrMissingMandatoryClauses struct{ Missing []MissingClause }

func (e *ErrMissingMandatoryClauses) Error() string {
	codes := make([]string, len(e.Missing))
	for i, m := range e.Missing {
		codes[i] = m.Code
	}
	return fmt.Sprintf("agreement: missing mandatory clauses: %s", strings.Join(codes, ", "))
}
func (e *ErrMissingMandatoryClauses) Unwrap() error { return versioning.ErrInvalidRequest }

// CheckSubmittable is docs.Service's per-type pre-submit gate for "dpa" documents (wired with
// SetSubmitValidate in cmd/api/main.go, once both docs.Service and this service exist) — PLT-08's generic
// Submit endpoint calls it, through docs.Service, for every document type that registers one, the same
// sequencing SetValidate already established for publish-time gates.
func (s *Service) CheckSubmittable(ctx context.Context, documentID uuid.UUID) error {
	row, err := agreementstore.New(pdb.MustTxFromContext(ctx)).GetAgreementByDocumentID(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // not an agreement-backed document (shouldn't happen for "dpa", but never block on a surprise)
	}
	if err != nil {
		return err
	}
	a, err := s.GetAgreement(ctx, row.ID)
	if err != nil {
		return err
	}
	missing, err := s.missingMandatoryClauses(ctx, a)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return &ErrMissingMandatoryClauses{Missing: missing}
	}
	return nil
}

func title(c docs.Clause) string {
	if b, ok := c.Body["th"]; ok {
		return b.Title
	}
	return ""
}

func toClause(r agreementstore.AgreementClause, code, clauseTitle, legalRef string) Clause {
	var versionNo int32
	if r.ClauseVersionNo != nil {
		versionNo = *r.ClauseVersionNo
	}
	return Clause{
		ID: r.ID, AgreementID: r.AgreementID, ClauseID: pgUUIDVal(r.ClauseID), ClauseCode: code, ClauseTitle: clauseTitle,
		LegalRef: legalRef, ClauseVersionNo: versionNo, Position: r.Position, IsMandatory: r.IsMandatory,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}
