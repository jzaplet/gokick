package docpaths

import (
	"path/filepath"
	"strings"
	"testing"
)

// fixtureScan keeps the fixtures out of a real .claude/skills/ directory —
// otherwise every tool that finds skills by globbing that path (Claude Code
// included) would load the fixture's fake skill as one of this repo's.
var fixtureScan = scan{docRoots: []string{"skills", "docs"}, skillsDir: "skills"}

// details flattens a scan into "file:line: detail" strings for assertion.
func details(res *result) []string {
	out := make([]string, 0, len(res.violations))
	for _, v := range res.violations {
		out = append(out, v.detail)
	}
	return out
}

// hasDetail reports whether any violation's text contains want.
func hasDetail(res *result, want string) bool {
	for _, d := range details(res) {
		if strings.Contains(d, want) {
			return true
		}
	}
	return false
}

// The clean fixture pins the OTHER half of the gate's worth: everything it
// must NOT flag. A checker that fires on prose shorthand, a URL, a TS alias or
// a placeholder gets switched off within a week, so each of those shapes is a
// regression test, not decoration.
func TestRun_CleanFixtureHasNoViolations(t *testing.T) {
	res, err := run(filepath.Join("testdata", "clean"), fixtureScan)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.violations) != 0 {
		t.Fatalf("clean fixture must report nothing, got: %v", details(res))
	}
	// The zero-sites guard only protects if the extractor really saw paths.
	if res.candidates < 5 {
		t.Fatalf("expected the extractor to find the fixture's citations, got %d", res.candidates)
	}
}

// The gate must bite on every dead-reference class it claims to cover — this
// is the test that proves it is not a no-op.
func TestRun_DeadFixtureFlagsEveryClass(t *testing.T) {
	res, err := run(filepath.Join("testdata", "dead"), fixtureScan)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, want := range []string{
		"docs/gone.md does not exist",                 // dead path
		"/gk-missing has no",                          // dead skill cross-reference
		"anchors line 9999",                           // line anchor past EOF
		"docs/{short,vanished}.md does not exist",     // dead brace member
		"tools/oldtool/**: the tools/oldtool subtree", // dead subtree pattern
		"\"docs/never-mentioned.md\" exempts nothing", // stale escape marker
		"\"docs/reasonless.md\" has no reason",        // reasonless escape marker
	} {
		if !hasDetail(res, want) {
			t.Errorf("gate did not bite on %q; got: %v", want, details(res))
		}
	}
}

// An escape marker must actually silence its path — otherwise the only way to
// keep a fictional example is to switch the gate off.
func TestRun_IgnoreMarkerSilencesItsPath(t *testing.T) {
	res, err := run(filepath.Join("testdata", "clean"), fixtureScan)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hasDetail(res, "imaginary-section") {
		t.Fatalf("a marked fictional path must not be reported: %v", details(res))
	}
}

func TestIsPathCandidate(t *testing.T) {
	for _, s := range []string{"app/domain/user/user.go", "docs/framework/architecture.md", ".claude/skills/gk/SKILL.md"} {
		if !isPathCandidate(s) {
			t.Errorf("%q must be checked as a repo path", s)
		}
	}
	for _, s := range []string{
		"FindAll/FindByIDAcrossTenants", // prose shorthand holding a slash
		"/api/v1/admin/users",           // a URL path
		"@/app-ui/Auth",                 // a TS import alias
		"app/domain/<ctx>/",             // a placeholder
		"migrations/…_create_runs.sql",  // an elided name
		"https://example.com/x",         // a link
		"GET /api/v1/platform",          // prose with a space
		"admin:users:read",              // a permission string
	} {
		if isPathCandidate(s) {
			t.Errorf("%q must NOT be treated as a repo path", s)
		}
	}
}

func TestExpandBraces(t *testing.T) {
	got := expandBraces("app/auth/{login,logout}.go")
	want := []string{"app/auth/login.go", "app/auth/logout.go"}
	if len(got) != len(want) {
		t.Fatalf("expandBraces = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expandBraces = %v, want %v", got, want)
		}
	}
	if plain := expandBraces("app/auth/login.go"); len(plain) != 1 ||
		plain[0] != "app/auth/login.go" {
		t.Fatalf("a braceless path must pass through unchanged, got %v", plain)
	}
}
