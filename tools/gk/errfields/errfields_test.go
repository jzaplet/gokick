package errfields

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The clean fixtures pin the happy collector paths: qualified + local +
// multi-line literals, raw-string and escaped Field values decoded via
// strconv.Unquote, "" routed to general, and exemptions holding over both
// single- and multi-line literals.
func TestCollect_CleanFixtures(t *testing.T) {
	refs, violations, err := collectGoFields(filepath.Join("testdata", "go-src"))
	if err != nil {
		t.Fatalf("collectGoFields: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("clean fixtures produced violations: %v", violations)
	}
	var keys []string
	for _, r := range refs {
		keys = append(keys, r.key)
	}
	sort.Strings(keys)
	want := "email,escaped,nickname,rawfield,role"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("collected %q, want %q (\"\"→general; id + avatar exempt)", got, want)
	}
}

// Every collector violation class must fire exactly once on the bad fixtures:
// positional literal, dynamic Field, trailing marker, unused marker,
// reasonless marker — and the trailing marker must NOT exempt its line.
func TestCollect_ViolationFixtures(t *testing.T) {
	refs, violations, err := collectGoFields(filepath.Join("testdata", "go-bad"))
	if err != nil {
		t.Fatalf("collectGoFields: %v", err)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"positional ValidationError literal",
		"not a string literal",
		"must stand alone",
		"unused //gkerrf:exempt",
		"without a reason",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing violation %q in:\n%s", want, joined)
		}
	}
	if len(violations) != 5 {
		t.Errorf("want exactly 5 violations, got %d:\n%s", len(violations), joined)
	}
	var keys []string
	for _, r := range refs {
		keys = append(keys, r.key)
	}
	if got := strings.Join(keys, ","); got != "trail" {
		t.Errorf("collected %q, want just \"trail\" (a trailing marker exempts nothing)", got)
	}
}

// The name-matching tripwires: a second ValidationError type outside
// domain/shared and a NewValidationError constructor are violations.
func TestCollect_Tripwires(t *testing.T) {
	_, violations, err := collectGoFields(filepath.Join("testdata", "tripwire"))
	if err != nil {
		t.Fatalf("collectGoFields: %v", err)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"a second ValidationError type",
		"NewValidationError constructor",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing tripwire %q in:\n%s", want, joined)
		}
	}
	if len(violations) != 2 {
		t.Errorf("want exactly 2 tripwire violations, got %d:\n%s", len(violations), joined)
	}
}

// FE keys parse from *Errors.ts files and the diff flags exactly the phantom.
func TestFeKeysAndDiff(t *testing.T) {
	feKeys, err := collectFeKeys(filepath.Join("testdata", "fe"))
	if err != nil {
		t.Fatalf("collectFeKeys: %v", err)
	}
	if len(feKeys) != 7 {
		t.Fatalf("collected %d FE keys, want 7 (incl. general + phantom)", len(feKeys))
	}
	refs, _, err := collectGoFields(filepath.Join("testdata", "go-src"))
	if err != nil {
		t.Fatalf("collectGoFields: %v", err)
	}
	violations := diff(refs, feKeys)
	if len(violations) != 1 || !strings.Contains(violations[0], `"phantom"`) {
		t.Fatalf("want exactly the phantom-key violation, got %v", violations)
	}
}

// An *Errors type declared outside a *Errors.ts file dodges collectFeKeys, so
// it must be a violation; a type-only IMPORT naming an *Errors type must not.
func TestStrayErrorsTypes(t *testing.T) {
	violations, err := strayErrorsTypes(filepath.Join("testdata", "fe"))
	if err != nil {
		t.Fatalf("strayErrorsTypes: %v", err)
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "stray.ts") {
		t.Fatalf("want exactly the stray.ts violation, got %v", violations)
	}
}

// An *Errors file that breaks the `key?: ApiMessage;` convention must fail the
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
	feKeys := map[string]string{"general": "a.ts:2", "phantom": "a.ts:3"}
	violations := diff(goFields, feKeys)
	if len(violations) != 2 {
		t.Fatalf("want 2 violations (no home + phantom), got %v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "no home") || !strings.Contains(joined, "phantom") {
		t.Fatalf("missing a direction in %v", violations)
	}
}
