package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	cookiestore "pdpa-platform/internal/cookie/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// ScriptRuleEntityType is this module's own audit entity type for cookie.script_rules.
const ScriptRuleEntityType = "cookie_script_rule"

var scriptActions = []string{"block", "allow"}

// ScriptRule declares which category a script belongs to — the acceptance criterion's own mechanism:
// toggling the category off/on takes effect on every script whose pattern matches, immediately, without a
// page reload (the SDK applies these on every preference change — see packages/cookie-sdk).
type ScriptRule struct {
	ID         uuid.UUID
	DomainID   uuid.UUID
	Pattern    string
	CategoryID uuid.UUID
	Action     string // block | allow
	RowVersion int32
}

// CreateScriptRuleInput is what the DPO gives to declare a script's own category.
type CreateScriptRuleInput struct {
	Pattern    string
	CategoryID uuid.UUID
	Action     string
}

// CreateScriptRule adds a script rule for a domain — the category must already be visible under the
// caller's own RLS (rule 1, via GetCategory).
func (s *Service) CreateScriptRule(ctx context.Context, domainID uuid.UUID, in CreateScriptRuleInput) (ScriptRule, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.create") {
		return ScriptRule{}, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return ScriptRule{}, err
	}
	pattern := strings.TrimSpace(in.Pattern)
	if pattern == "" {
		return ScriptRule{}, fmt.Errorf("%w: pattern", ErrInvalid)
	}
	if in.Action == "" {
		in.Action = "block"
	}
	if !contains(scriptActions, in.Action) {
		return ScriptRule{}, fmt.Errorf("%w: action", ErrInvalid)
	}
	if _, err := s.GetCategory(ctx, in.CategoryID); err != nil {
		return ScriptRule{}, fmt.Errorf("%w: category_id", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ScriptRule{}, err
	}
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).InsertScriptRule(ctx, cookiestore.InsertScriptRuleParams{
		ID: id, DomainID: domainID, Pattern: pattern, CategoryID: in.CategoryID, Action: in.Action,
	})
	if err != nil {
		return ScriptRule{}, err
	}
	rule := ScriptRule{ID: row.ID, DomainID: row.DomainID, Pattern: row.Pattern, CategoryID: row.CategoryID, Action: row.Action, RowVersion: row.RowVersion}
	if err := s.audit(ctx, "cookie.script_rule.create", ScriptRuleEntityType, id, nil, map[string]any{"domain_id": domainID, "pattern": pattern}); err != nil {
		return ScriptRule{}, err
	}
	return rule, nil
}

// ListScriptRules returns every script rule for a domain, by pattern.
func (s *Service) ListScriptRules(ctx context.Context, domainID uuid.UUID) ([]ScriptRule, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.read") {
		return nil, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return nil, err
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListScriptRulesByDomain(ctx, domainID)
	if err != nil {
		return nil, err
	}
	out := make([]ScriptRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, ScriptRule{ID: r.ID, DomainID: r.DomainID, Pattern: r.Pattern, CategoryID: r.CategoryID, Action: r.Action, RowVersion: r.RowVersion})
	}
	return out, nil
}

// DeleteScriptRule removes a script rule.
func (s *Service) DeleteScriptRule(ctx context.Context, id uuid.UUID) error {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.delete") {
		return ErrForbidden
	}
	n, err := cookiestore.New(pdb.MustTxFromContext(ctx)).DeleteScriptRule(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "cookie.script_rule.delete", ScriptRuleEntityType, id, nil, nil)
}

// PublicScriptRule is the SDK-facing shape: a category by its own code, not id, matching how the public
// banner read already names categories.
type PublicScriptRule struct {
	Pattern      string
	CategoryCode string
	Action       string
}

// PublicScriptRules returns every script rule for a domain, keyed by category code — no permission check,
// the public key itself already proves this call is scoped to the right tenant (the same shape
// PublicBannerConfig/PublicCategories already use).
func (s *Service) PublicScriptRules(ctx context.Context, domainID uuid.UUID) ([]PublicScriptRule, error) {
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListScriptRulesWithCategoryCode(ctx, domainID)
	if err != nil {
		return nil, err
	}
	out := make([]PublicScriptRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, PublicScriptRule{Pattern: r.Pattern, CategoryCode: r.CategoryCode, Action: r.Action})
	}
	return out, nil
}
