package app_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Parameter-count gate.
//
// A function with more than maxParams parameters carries its coupling in the
// call site — callers must marshal a long positional list, and two adjacent
// same-typed params invite silent swaps. Cohesive groups belong in a small
// struct (see worker.hbHandles / worker.runOutcome, userwrite.Deps/Plane/
// CreateSpec, seeder.seedSpec — the refactors that brought the tree under this
// gate).
//
// Exemption: DI composition signatures — constructors (New*), Wire providers
// (provide*) and middleware-chain builders (*Chain). Compile-time DI without a
// container enumerates dependencies by design; folding them into a struct
// would only hide the arity without reducing coupling, and Wire needs plain
// constructors. Everything else must fit maxParams.
//
// Scope: app/ + cmd/, production code only (tests excluded, wire_gen.go
// skipped). Receivers don't count; a grouped param list (a, b string) counts
// per name.

const maxParams = 5

var exemptFuncRe = regexp.MustCompile(`^(New|provide)|Chain$`)

// paramCount counts declared parameters (grouped names count individually;
// an unnamed param counts once).
func paramCount(fn *ast.FuncDecl) int {
	n := 0
	for _, p := range fn.Type.Params.List {
		if len(p.Names) == 0 {
			n++
		} else {
			n += len(p.Names)
		}
	}
	return n
}

// paramViolations returns the offending function names in one Go source file.
// src is nil to read the file at name, or a string/[]byte to parse directly.
func paramViolations(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var v []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if exemptFuncRe.MatchString(fn.Name.Name) {
			return true
		}
		if c := paramCount(fn); c > maxParams {
			v = append(v, fmt.Sprintf(
				"%s has %d params (max %d) — group the cohesive ones into a struct "+
					"(or name it New*/provide*/*Chain if it is genuinely DI composition)",
				fn.Name.Name, c, maxParams))
		}
		return true
	})
	return v
}

// repoRoot resolves the module root (parent of app/) from this file's compiled-in
// path so the scan is independent of the test's CWD.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

// Every production function in app/ + cmd/ fits the parameter budget or is a
// named DI composition shape.
func TestParamsConformance_ProductionFunctionsFitBudget(t *testing.T) {
	var violations []string
	for _, root := range []string{filepath.Join(repoRoot(), "app"), filepath.Join(repoRoot(), "cmd")} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "wire_gen.go") {
				return nil
			}
			for _, vio := range paramViolations(t, path, nil) {
				violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), vio))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("parameter-count violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The gate bites an over-budget function and honors the DI exemptions.
func TestParamsConformance_GateBitesAndExempts(t *testing.T) {
	const over = "package fake\nfunc doWork(a, b, c, d, e, f string) {}\n"
	if len(paramViolations(t, "fake.go", over)) == 0 {
		t.Fatal("a 6-param function must be a violation; the gate does not bite")
	}
	const grouped = "package fake\nfunc doWork(a, b string, c, d int, e bool) {}\n"
	if v := paramViolations(t, "fake.go", grouped); len(v) != 0 {
		t.Fatalf("a 5-param function must pass, got %v", v)
	}
	for _, src := range []string{
		"package fake\nfunc NewThing(a, b, c, d, e, f string) *int { return nil }\n",
		"package fake\nfunc provideThing(a, b, c, d, e, f string) *int { return nil }\n",
		"package fake\nfunc CommandChain(a, b, c, d, e, f string) {}\n",
	} {
		if v := paramViolations(t, "fake.go", src); len(v) != 0 {
			t.Fatalf("a DI composition shape must be exempt, got %v", v)
		}
	}
}
