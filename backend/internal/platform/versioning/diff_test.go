package versioning

import (
	"encoding/json"
	"testing"
)

func TestDiff(t *testing.T) {
	before := json.RawMessage(`{"title":"ประกาศ","purposes":["a","b"],"contact":{"email":"x@example.com","phone":"1"},"same":1}`)
	after := json.RawMessage(`{"title":"ประกาศ v2","purposes":["a","c","d"],"contact":{"email":"x@example.com"},"same":1,"new":true}`)
	got, err := Diff(before, after)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]any{
		"contact.phone": {"1", nil},
		"new":           {nil, true},
		"purposes[1]":   {"b", "c"},
		"purposes[2]":   {nil, "d"},
		"title":         {"ประกาศ", "ประกาศ v2"},
	}
	if len(got) != len(want) {
		t.Fatalf("changes: %+v", got)
	}
	for _, c := range got {
		w, ok := want[c.Path]
		if !ok || c.Before != w[0] || c.After != w[1] {
			t.Errorf("%s: %v → %v, want %v", c.Path, c.Before, c.After, w)
		}
	}
	if got[0].Path != "contact.phone" {
		t.Errorf("not in key order: %s first", got[0].Path)
	}
	if none, _ := Diff(before, before); len(none) != 0 {
		t.Errorf("identical: %+v", none)
	}
	if first, _ := Diff(nil, json.RawMessage(`{"a":1}`)); len(first) != 1 || first[0].Path != "$" {
		t.Errorf("first version: %+v", first)
	}
	if _, err := Diff(json.RawMessage(`{`), nil); err == nil {
		t.Error("bad JSON accepted")
	}
}
