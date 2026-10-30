package versioning

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
)

// Change is one difference between two snapshots: the JSON path of a value and its old and new
// values (nil when the path did not exist on that side).
type Change struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

// Diff compares two JSON documents field by field. Objects are compared key by key (in key order);
// arrays element by element by index, with the extra elements of the longer one added or removed;
// anything else is compared as a whole.
func Diff(before, after json.RawMessage) ([]Change, error) {
	var a, b any
	if len(before) > 0 {
		if err := json.Unmarshal(before, &a); err != nil {
			return nil, err
		}
	}
	if len(after) > 0 {
		if err := json.Unmarshal(after, &b); err != nil {
			return nil, err
		}
	}
	var out []Change
	diff("", a, b, &out)
	return out, nil
}

func diff(path string, a, b any, out *[]Change) {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			diff(join(path, k), am[k], bm[k], out)
		}
		return
	}
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if aok && bok {
		n := max(len(as), len(bs))
		for i := 0; i < n; i++ {
			var x, y any
			if i < len(as) {
				x = as[i]
			}
			if i < len(bs) {
				y = bs[i]
			}
			diff(path+"["+strconv.Itoa(i)+"]", x, y, out)
		}
		return
	}
	if !reflect.DeepEqual(a, b) {
		p := path
		if p == "" {
			p = "$"
		}
		*out = append(*out, Change{Path: p, Before: a, After: b})
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
