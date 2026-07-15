package errfields

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The fixture pair pins the whole pipeline: which Go literals are collected
// (qualified + local + multi-line; "" and exempted skipped), which FE keys are
// parsed, and both diff directions.
func TestCollectAndDiff(t *testing.T) {
	goFields, err := collectGoFields(filepath.Join("testdata", "go-src"))
	if err != nil {
		t.Fatalf("collectGoFields: %v", err)
	}
	var keys []string
	for _, r := range goFields {
		keys = append(keys, r.key)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "email,nickname,role"; got != want {
		t.Fatalf(
			"collected Go fields %q, want %q (\"\" routes to general, id is exempt)",
			got,
			want,
		)
	}

	feKeys, err := collectFeKeys(filepath.Join("testdata", "fe"))
	if err != nil {
		t.Fatalf("collectFeKeys: %v", err)
	}
	if len(feKeys) != 5 {
		t.Fatalf("collected %d FE keys, want 5 (incl. general + phantom)", len(feKeys))
	}

	violations := diff(goFields, feKeys)
	if len(violations) != 1 || !strings.Contains(violations[0], `"phantom"`) {
		t.Fatalf("want exactly the phantom-key violation, got %v", violations)
	}
}

// An *Errors file that breaks the `key?: string;` convention must fail the
// parse — a silently skipped line would hide keys from the parity check.
func TestFeParse_RejectsNonConventionLines(t *testing.T) {
	_, err := collectFeKeys(filepath.Join("testdata", "fe-bad"))
	if err == nil || !strings.Contains(err.Error(), "unexpected line") {
		t.Fatalf("want an unexpected-line error, got %v", err)
	}
}

// Both directions must report position-tagged violations.
func TestDiff_BothDirections(t *testing.T) {
	goFields := []fieldRef{{key: "orphaned", pos: "x.go:1"}}
	feKeys := map[string][]string{"general": {"a.ts:2"}, "phantom": {"a.ts:3"}}
	violations := diff(goFields, feKeys)
	if len(violations) != 2 {
		t.Fatalf("want 2 violations (no home + phantom), got %v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "no home") || !strings.Contains(joined, "phantom") {
		t.Fatalf("missing a direction in %v", violations)
	}
}
