package domain_test

// This file lives in app/domain/, which has no production package of its own —
// it is the parent of the bounded-context packages (shared, user, token, job).
// The test below parses every non-test .go file under this directory and pins
// the documented architecture promise (overview-39 / layers.md § Dependency
// matrix, .go-arch-lint.yml): the domain layer may depend ONLY on the standard
// library, the single permitted vendor dependency github.com/google/uuid, and
// other domain packages (domain/shared is a commonComponent). It must never
// import application, infrastructure, or presentation — i.e. no gokick/...
// package outside gokick/app/domain/, and no third-party module besides uuid.
//
// This is the same invariant go-arch-lint enforces, expressed as a real Go test
// that fails the build the moment a cross-layer or stray-vendor import sneaks
// into a domain source file.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	moduleRoot      = "gokick"
	domainPrefix    = "gokick/app/domain/"
	allowedVendor   = "github.com/google/uuid"
	domainDirSuffix = "app/domain"
)

// domainDir resolves the absolute path to app/domain/ from this test file's
// location, so the walk is independent of the working directory.
func domainDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate test source file")
	}
	dir := filepath.Dir(thisFile)
	if filepath.Base(dir) != "domain" || filepath.Base(filepath.Dir(dir)) != "app" {
		t.Fatalf("expected this test to live in .../app/domain, got %q", dir)
	}
	return dir
}

// isStdlib reports whether an import path is part of the standard library.
// Standard-library paths have no dot in their first path segment (e.g.
// "context", "database/sql") AND are not under this module's root path —
// "gokick/..." has no dot either, so the module root must be excluded
// explicitly or internal imports would be misclassified as stdlib.
func isStdlib(importPath string) bool {
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	if first == moduleRoot {
		return false
	}
	return !strings.Contains(first, ".")
}

// importAllowed encodes the documented dependency rule for the domain layer.
func importAllowed(importPath string) bool {
	switch {
	case isStdlib(importPath):
		return true
	case importPath == allowedVendor:
		return true
	case importPath == domainPrefix[:len(domainPrefix)-1]: // gokick/app/domain itself (no subpkg)
		return true
	case strings.HasPrefix(importPath, domainPrefix): // other domain packages
		return true
	default:
		return false
	}
}

// collectDomainImports parses every non-test .go file under app/domain/ and
// returns, per file, the list of import paths it declares.
func collectDomainImports(t *testing.T) map[string][]string {
	t.Helper()
	root := domainDir(t)
	fset := token.NewFileSet()
	result := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("failed to parse %s: %v", path, perr)
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		imps := make([]string, 0, len(f.Imports))
		for _, spec := range f.Imports {
			// spec.Path.Value is the quoted import path, e.g. `"context"`.
			imps = append(imps, strings.Trim(spec.Path.Value, `"`))
		}
		result[rel] = imps
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return result
}

// TestDomainImportsAreStdlibUuidOrDomainOnly is the core guard: it fails if any
// domain source file imports application/infrastructure/presentation (any
// gokick/... path outside gokick/app/domain/) or any third-party module other
// than github.com/google/uuid. Pins overview-39.
func TestDomainImportsAreStdlibUuidOrDomainOnly(t *testing.T) {
	byFile := collectDomainImports(t)
	if len(byFile) == 0 {
		t.Fatal("no domain .go files were parsed; the walk found nothing")
	}

	for file, imps := range byFile {
		for _, imp := range imps {
			if !importAllowed(imp) {
				t.Errorf(
					"domain file %s imports forbidden package %q: domain may depend only on stdlib, %q, or other gokick/app/domain/ packages",
					file,
					imp,
					allowedVendor,
				)
			}
		}
	}
}

// TestDomainNeverImportsOuterLayers is a sharper, explicit restatement of the
// same promise aimed at the highest-risk regression: a domain file reaching
// "up" into application, infrastructure, or presentation. It asserts that no
// domain import is a gokick/... path that is NOT under gokick/app/domain/.
func TestDomainNeverImportsOuterLayers(t *testing.T) {
	byFile := collectDomainImports(t)

	outerLayers := map[string]string{
		"app/application/":    "application",
		"app/infrastructure/": "infrastructure",
		"app/presentation/":   "presentation",
	}

	for file, imps := range byFile {
		for _, imp := range imps {
			if !strings.HasPrefix(imp, moduleRoot+"/") {
				continue // not an internal gokick import
			}
			if strings.HasPrefix(imp, domainPrefix) {
				continue // allowed: another domain package
			}
			// Any remaining internal gokick import is forbidden. Name the
			// offending layer when we recognize it, for a clearer failure.
			layer := "non-domain internal"
			for seg, name := range outerLayers {
				if strings.Contains(imp, seg) {
					layer = name + " layer"
					break
				}
			}
			t.Errorf(
				"domain file %s imports %s package %q (forbidden by the layer dependency rule)",
				file,
				layer,
				imp,
			)
		}
	}
}

// TestDomainImportsIncludeUuid is a sanity anchor proving the parser actually
// sees real imports (not an empty/short-circuited walk): the documented single
// permitted vendor dependency, github.com/google/uuid, must appear somewhere in
// the domain (it backs uuid.New() in entity factories). If this regresses to
// zero, the "allow uuid" arm of the rule above would silently never be
// exercised, so this keeps the guard honest.
func TestDomainImportsIncludeUuid(t *testing.T) {
	byFile := collectDomainImports(t)
	for _, imps := range byFile {
		for _, imp := range imps {
			if imp == allowedVendor {
				return
			}
		}
	}
	t.Fatalf(
		"expected at least one domain file to import %q; none did (parser may not be seeing real imports)",
		allowedVendor,
	)
}

// TestIsStdlibClassification documents and locks the stdlib-vs-module
// classification the whole guard rests on. If this heuristic broke, the import
// rule above could wrongly accept an outer-layer import; this nails the exact
// boundary cases (internal gokick paths and vendor paths must NOT be stdlib;
// real stdlib paths must be).
func TestIsStdlibClassification(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"context", true},
		{"database/sql", true},
		{"strings", true},
		{"github.com/google/uuid", false},
		{"gokick/app/domain/shared", false},
		{"gokick/app/application/bus", false},
	}
	for _, tc := range cases {
		if got := isStdlib(tc.path); got != tc.want {
			t.Errorf("isStdlib(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
