package authz_test

import (
	"reflect"
	"sort"
	"testing"

	"pdpa-platform/internal/pkg/authz"
)

func str(s string) *string { return &s }

// TestLegalEntityScope is ORG-02's own pure-function test for the acceptance criterion's underlying rule.
func TestLegalEntityScope(t *testing.T) {
	cases := []struct {
		name   string
		scopes []authz.DataScope
		ok     bool
		ids    []string
	}{
		{name: "no scopes at all (worker/system call)", scopes: nil, ok: false},
		{name: "a tenant-scoped assignment means unrestricted, even alongside a legal_entity one",
			scopes: []authz.DataScope{{ScopeType: "legal_entity", LegalEntityID: str("a")}, {ScopeType: "tenant"}}, ok: false},
		{name: "one legal_entity scope restricts to it",
			scopes: []authz.DataScope{{ScopeType: "legal_entity", LegalEntityID: str("a")}}, ok: true, ids: []string{"a"}},
		{name: "several legal_entity scopes union, deduplicated",
			scopes: []authz.DataScope{{ScopeType: "legal_entity", LegalEntityID: str("a")}, {ScopeType: "legal_entity", LegalEntityID: str("b")},
				{ScopeType: "legal_entity", LegalEntityID: str("a")}}, ok: true, ids: []string{"a", "b"}},
		{name: "org_unit/self scopes alone don't narrow legal-entity visibility",
			scopes: []authz.DataScope{{ScopeType: "org_unit"}, {ScopeType: "self"}}, ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ids, ok := authz.Grants{Scopes: c.scopes}.LegalEntityScope()
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			sort.Strings(ids)
			sort.Strings(c.ids)
			if !reflect.DeepEqual(ids, c.ids) && !(len(ids) == 0 && len(c.ids) == 0) {
				t.Errorf("ids = %v, want %v", ids, c.ids)
			}
		})
	}
}
