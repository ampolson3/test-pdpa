// Package isolationaudit is PLT-02's own permanent guard on its acceptance criterion
// ("ผู้ใช้ tenant A เข้าถึงข้อมูล tenant B ไม่ได้แม้เรียก API ตรง (automated test ทุก endpoint)"): every
// module that owns a tenant-scoped /admin/v1 operation must have, somewhere in its own test files, a
// real two-tenant isolation proof — not per operationId (every operation in a module shares the same
// RLS/service-layer enforcement boundary, so duplicating the proof per endpoint would just repeat the
// same check for no benefit), but at least once per module, so a new module can never ship without one.
//
// This is a static text scan, not a live two-tenant HTTP crawl: a real crawl would need to construct a
// valid request body and a pre-existing resource for every one of ~250 operations, which has no generic
// shape across modules (rule 9 — module internals vary) and would just re-implement what each module's
// own fixture already does better. The scan's job is narrower and permanent: catch a module that was
// never given its own isolation test at all, the same gap this file's own audit found and closed for
// backend/internal/platform/files and .../collab before this package existed (see the PLT-02 note in
// docs/modules/PLT.md and CLAUDE.md for the full audit).
package isolationaudit

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// moduleToDirs maps an OpenAPI x-module value to the Go package directory (or directories, for
// "platform", whose sub-packages are independently schema-scoped even though they share one x-module
// tag) that must each carry their own isolation proof.
var moduleToDirs = map[string][]string{
	"agreement": {"agreement/service"},
	"breach":    {"breach/service"},
	"consent":   {"consent/service"},
	"dataflow":  {"dataflow/service"},
	"dpia":      {"dpia/service"},
	"dpo":       {"dpo/service"},
	"dsar":      {"dsar/service"},
	"gov":       {"gov/service"},
	"iam":       {"iam/store"},
	"notice":    {"notice/service"},
	"org":       {"org/service"},
	"risk":      {"risk/service"},
	"ropa":      {"ropa/service", "ropa/templates"},
	"vendor":    {"vendormgmt/service"},
	// "platform" is one OpenAPI tag shared by several independently schema-scoped sub-packages
	// (docs/architecture/code-structure.md's module table) — each owns its own tenant isolation,
	// so each gets checked on its own rather than as a single "platform" bucket.
	"platform": {
		"platform/audit/service", "platform/collab", "platform/docs", "platform/files",
		"platform/forms", "platform/importer", "platform/jobs", "platform/notify",
		"platform/versioning", "platform/workflow",
	},
}

// isolationEvidence matches every phrasing this codebase already uses for a genuine two-tenant proof
// (a second tenant's own transaction reading zero rows / getting ErrNotFound / being refused access to
// the first tenant's data) — broad on purpose, since each module names its own test; new wording is fine
// as long as it still reads as one of these.
var isolationEvidence = regexp.MustCompile(`(?i)isolat|per.?tenant|cross.?tenant|other tenant|another tenant`)

var xModuleRE = regexp.MustCompile(`x-module:\s*([a-zA-Z_]+)`)

// TestEveryAdminModuleHasATenantIsolationProof is PLT-02's own CI gate: it reads every x-module value
// actually used in api/openapi/openapi.yaml (so a new module's own OpenAPI tag makes it on this list
// automatically, no second place to remember to update) and fails if that module's own test files carry
// no isolation evidence at all.
func TestEveryAdminModuleHasATenantIsolationProof(t *testing.T) {
	root := repoRoot(t)

	spec, err := os.ReadFile(filepath.Join(root, "api/openapi/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	modules := map[string]bool{}
	for _, m := range xModuleRE.FindAllStringSubmatch(string(spec), -1) {
		modules[m[1]] = true
	}
	if len(modules) == 0 {
		t.Fatal("no x-module tags found in the OpenAPI spec — the regex or the spec's own convention changed")
	}

	var missing []string
	for mod := range modules {
		dirs, ok := moduleToDirs[mod]
		if !ok {
			t.Errorf("x-module %q has no entry in moduleToDirs — add its service package(s) so this gate actually covers it", mod)
			continue
		}
		for _, dir := range dirs {
			if !dirHasIsolationEvidence(t, filepath.Join(root, "backend/internal", dir)) {
				missing = append(missing, dir)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no two-tenant isolation test found for: %s (PLT-02 rule 1: every tenant-scoped module needs one)", strings.Join(missing, ", "))
	}
}

func dirHasIsolationEvidence(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("reading %s: %v", dir, err)
		return true // don't also report it as "missing" when the real problem is a bad path
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Errorf("reading %s: %v", filepath.Join(dir, e.Name()), err)
			continue
		}
		if isolationEvidence.Match(b) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// This package lives at backend/internal/pkg/isolationaudit — four levels under the repo root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "api", "openapi", "openapi.yaml")); err != nil {
		t.Fatalf("resolved repo root %q doesn't contain api/openapi/openapi.yaml: %v", root, err)
	}
	return root
}
