package tsgen

import (
	"flag"
	"go/parser"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Golden tests: testdata/src holds fixture DTOs exercising every mapped shape;
// testdata/golden holds the exact expected TS output. Any change to the
// generator's emission — intended or not — shows up as a byte diff here.
// Regenerate the goldens after an INTENDED change with:
//
//	go test ./tsgen -run TestGolden -update
//
// and review the golden diff like any other code change.
var update = flag.Bool("update", false, "rewrite the golden files from current output")

func generate(t *testing.T, dir string) (map[string]string, error) {
	t.Helper()
	dtos, err := collect(dir)
	if err != nil {
		return nil, err
	}
	byGo, err := indexDTOs(dtos)
	if err != nil {
		return nil, err
	}
	// optionalReady=false mirrors the real repo: guards.ts does not export
	// `optional` until the first omitempty DTO ships it.
	return render(dtos, byGo, false)
}

func TestGolden_FixturesRenderExactly(t *testing.T) {
	files, err := generate(t, filepath.Join("testdata", "src"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var got []string
	for path := range files {
		got = append(got, filepath.Base(path))
	}
	sort.Strings(got)

	if *update {
		emitted := map[string]bool{}
		for path, content := range files {
			out := filepath.Join("testdata", "golden", filepath.Base(path))
			emitted[out] = true
			if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
				t.Fatalf("update golden %s: %v", out, err)
			}
		}
		// Prune goldens no fixture produces anymore — without this, deleting
		// or renaming a fixture DTO makes -update unable to converge (the
		// file-set comparison below would fail forever on the stale golden).
		stale, err := filepath.Glob(filepath.Join("testdata", "golden", "*.ts"))
		if err != nil {
			t.Fatalf("glob goldens: %v", err)
		}
		for _, g := range stale {
			if !emitted[g] {
				if err := os.Remove(g); err != nil {
					t.Fatalf("prune stale golden %s: %v", g, err)
				}
			}
		}
	}

	goldens, err := filepath.Glob(filepath.Join("testdata", "golden", "*.ts"))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("no golden files (run with -update once): %v", err)
	}
	var want []string
	for _, g := range goldens {
		want = append(want, filepath.Base(g))
	}
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("emitted file set %v != golden set %v", got, want)
	}

	for path, content := range files {
		golden := filepath.Join("testdata", "golden", filepath.Base(path))
		wantBytes, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		if content != string(wantBytes) {
			t.Errorf("%s differs from golden:\n--- got ---\n%s\n--- want ---\n%s",
				path, content, string(wantBytes))
		}
	}
}

// The generator must REFUSE bad input loudly rather than emit something wrong.
func TestErrors_BadInputFailsLoudly(t *testing.T) {
	cases := []struct {
		dir  string
		want string
	}{
		{"bad-directive", "malformed directive"},
		{"bad-type", "unmapped Go type"},
		{"conflict", "different fields"},
		{"nested-noguard", "marked noguard"},
		// The `optional` helper is deliberately absent from guards.ts until
		// the first omitempty DTO — emitting a call to it would produce
		// non-compiling generated code, so the generator must refuse.
		{"omitempty", "does not export the `optional` helper"},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			_, err := generate(t, filepath.Join("testdata", c.dir))
			if err == nil {
				t.Fatalf("expected an error containing %q, got success", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// Once guards.ts ships the helper (probe true), the same omitempty fixture
// must render optional(...) instead of erroring — the refusal self-disarms.
func TestOmitempty_RendersOnceHelperExists(t *testing.T) {
	dtos, err := collect(filepath.Join("testdata", "omitempty"))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	byGo, err := indexDTOs(dtos)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	files, err := render(dtos, byGo, true)
	if err != nil {
		t.Fatalf("render with optionalReady: %v", err)
	}
	var all []string
	for _, content := range files {
		all = append(all, content)
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "optional(isString)") {
		t.Fatalf("expected optional(isString) in output:\n%s", joined)
	}
	if !strings.Contains(joined, "optional") || !strings.Contains(joined, "guards';") {
		t.Fatalf("expected the optional helper import in output:\n%s", joined)
	}
}

// Guard composition cannot express nested arrays / double pointers — mapType
// must reject them instead of emitting a guard that lies.
func TestMapType_RejectsNonComposableShapes(t *testing.T) {
	for _, src := range []string{"[][]string", "**string", "[]*string"} {
		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatalf("parse %s: %v", src, err)
		}
		if _, _, _, err := mapType(expr); err == nil {
			t.Errorf("mapType(%s) must error (not guard-composable)", src)
		}
	}
}

// orphans() is what both the prune (generate) and the drift report (check)
// stand on: a generated file (DO-NOT-EDIT header) the current directive set no
// longer produces must be found; hand-written files must never be touched.
func TestOrphans_FindsStaleGeneratedFilesOnly(t *testing.T) {
	root := t.TempDir()
	typesDir := filepath.Join(root, "assets", "x", "types")
	if err := os.MkdirAll(typesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(typesDir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return filepath.ToSlash(mustRel(root, p))
	}
	live := write("Live.ts", genHeader+"\nexport type Live = { a: string };\n")
	stale := write("Stale.ts", genHeader+"\nexport type Stale = { b: string };\n")
	write("HandWritten.ts", "export type HandWritten = { c: string };\n")

	got := orphans(root, map[string]string{live: "current"})
	if len(got) != 1 || got[0] != stale {
		t.Fatalf("orphans = %v, want exactly [%s] (hand-written files untouched)", got, stale)
	}
}
