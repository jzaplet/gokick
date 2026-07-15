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
	return render(dtos, byGo)
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
		for path, content := range files {
			out := filepath.Join("testdata", "golden", filepath.Base(path))
			if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
				t.Fatalf("update golden %s: %v", out, err)
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
