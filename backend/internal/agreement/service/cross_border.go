// DPA-05 (ข้อสัญญาการโอนต่างประเทศ): DPA-03/DSA-05 already built the generic clause-completeness gate —
// agreement.mandatory_rules' own "requires_transfer" condition (migrations 00057/00061) already refuses to
// submit a cross-border agreement that's missing its transfer clause — but that gate only checks what's
// already attached; nothing ever attached it automatically. This file is the other half of the same
// description ("clause มาตรฐานตามประกาศ ม.29 พ.ศ. 2566 แทรกเมื่อมีการโอน"): the standard transfer clause is
// inserted the moment a cross-border transfer is on record, so LEGAL finds it already there rather than
// having to remember to search the clause library for it.
package service

import (
	"context"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/docs"
)

// crossBorderClauseCode is the same per-type code migrations 00057 ("dpa")/00061 ("dsa") seed as the
// conditional "requires_transfer" mandatory_rules row.
var crossBorderClauseCode = map[string]string{
	"dpa": "dpa.cross_border_transfer",
	"dsa": "dsa.cross_border_transfer",
}

// autoAttachCrossBorderClause is DPA-05's own acceptance criterion directly: once this agreement's own
// linked activities show it involves a cross-border transfer (ROPA-08), attach the matching standard clause
// right away. A harmless no-op for an agreement type with no such clause code, one with no transfer on
// record, or a tenant whose clause library doesn't have that code published yet (nothing to attach
// automatically — DPA-03's own gate still gives LEGAL the itemized "missing" list either way).
func (s *Service) autoAttachCrossBorderClause(ctx context.Context, a Agreement) error {
	code, ok := crossBorderClauseCode[a.AgreementType]
	if !ok {
		return nil
	}
	hasTransfer, err := s.agreementHasTransfer(ctx, a)
	if err != nil {
		return err
	}
	if !hasTransfer {
		return nil
	}
	clauses, err := s.Docs.ListClauses(ctx, docs.ClauseFilter{AppliesTo: a.AgreementType, PublishedOnly: true})
	if err != nil {
		return err
	}
	var clauseID uuid.UUID
	for _, c := range clauses {
		if c.Code == code {
			clauseID = c.ID
			break
		}
	}
	if clauseID == uuid.Nil {
		return nil
	}
	if _, err := s.AddClause(ctx, a.ID, AddClauseInput{ClauseID: clauseID}); err != nil {
		return err
	}
	return nil
}
