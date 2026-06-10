package domain_test

// This file lives in app/domain/ (package domain_test, shared with
// zz_audit_test.go) and pins overview-41 / architecture.md § Vrstvy:
//
//	"HTTP handlers must not import sqlite, security, or event packages."
//
// Why a hand-rolled parser walk rather than leaning on go-arch-lint: the
// presentation layer is *allowed* to depend on infrastructure in the matrix
// (presentation -> infrastructure is a legal edge), so arch-lint does NOT block
// a handler from reaching directly into infrastructure/sqlite or
// infrastructure/security. And the only application "event" package lives at
// application/user/event, which is covered by the application/** glob the
// handler may already use — so arch-lint does not catch a handler importing it
// either (closing that hole would require adding a dedicated `event` component,
// exactly as the ledger's "jak uzavřít" notes). This test is therefore strictly
// stronger than the linter: it fails the build the moment a handler source file
// reaches past the bus/domain-interface boundary into a concrete repository, a
// crypto/JWT implementation, or an event handler package.
//
// The walk reads handler files by path with go/parser — it does not import the
// handler package, so this test honours the "domain may import only stdlib +
// uuid + domain" rule that the sibling zz_audit_test.go enforces.
//
// Reuses the moduleRoot const and the runtime.Caller location trick from
// zz_audit_test.go (same package); every helper here has a fresh name to avoid
// redefining those already in scope (domainDir, collectDomainImports, isStdlib).

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Concrete-implementation roots that a handler must never import directly. These
// are the packages whose presence in a handler import set means the presentation
// layer has bypassed the bus / domain-interface contract.
const (
	infraSqliteRoot   = "gokick/app/infrastructure/sqlite"
	infraSecurityRoot = "gokick/app/infrastructure/security"
	applicationPrefix = "gokick/app/application/"
)

// handlerDir resolves the absolute path to app/presentation/http/handler from
// this test file's location (app/domain/), independent of the working
// directory. It is deliberately strict: a wrong path would make the walk find
// zero files and the guard pass vacuously, so we re-anchor on app/domain first
// (same check zz_audit_test.go's domainDir does) and then hop to the handler
// dir. The caller t.Fatals if the resolved dir is missing.
func handlerDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate test source file")
	}
	dir := filepath.Dir(thisFile)
	if filepath.Base(dir) != "domain" || filepath.Base(filepath.Dir(dir)) != "app" {
		t.Fatalf("expected this test to live in .../app/domain, got %q", dir)
	}
	appDir := filepath.Dir(dir) // .../app
	hdir := filepath.Join(appDir, "presentation", "http", "handler")
	info, err := os.Stat(hdir)
	if err != nil || !info.IsDir() {
		t.Fatalf("handler dir not found at %q (err=%v); the walk would be vacuous", hdir, err)
	}
	return hdir
}

// isEventPackage reports whether an import path names an application event
// package (application/<ctx>/event or a subpackage thereof). Matching on the
// path segment "/event" — not the substring "event" — is what keeps
// gokick/app/domain/shared (which defines DomainEvent / EventCollector) from
// false-matching: it does not end in "/event" nor contain "/event/".
func isEventPackage(importPath string) bool {
	if !strings.HasPrefix(importPath, applicationPrefix) {
		return false
	}
	return strings.HasSuffix(importPath, "/event") || strings.Contains(importPath, "/event/")
}

// forbiddenHandlerImport encodes the overview-41 rule: a handler import is
// forbidden iff it targets the sqlite repository root, the security
// (crypto/JWT) root, or an application event package. Everything else
// (application command/query packages, the bus, domain packages, the response
// and request presentation packages, stdlib) is permitted.
func forbiddenHandlerImport(importPath string) bool {
	switch {
	case importPath == infraSqliteRoot || strings.HasPrefix(importPath, infraSqliteRoot+"/"):
		return true
	case importPath == infraSecurityRoot || strings.HasPrefix(importPath, infraSecurityRoot+"/"):
		return true
	case isEventPackage(importPath):
		return true
	default:
		return false
	}
}

// collectHandlerImports parses every non-test .go file directly under the
// handler dir (non-recursive: the handler package is flat) and returns, per
// file, its declared import paths.
func collectHandlerImports(t *testing.T) map[string][]string {
	t.Helper()
	root := handlerDir(t)
	fset := token.NewFileSet()
	result := make(map[string][]string)

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading handler dir %s: %v", root, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(root, name)
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("failed to parse %s: %v", path, perr)
		}
		imps := make([]string, 0, len(f.Imports))
		for _, spec := range f.Imports {
			imps = append(imps, strings.Trim(spec.Path.Value, `"`))
		}
		result[name] = imps
	}
	return result
}

// TestHandlersDoNotImportSqliteSecurityOrEvent is the core guard for
// overview-41: no handler source file may import infrastructure/sqlite,
// infrastructure/security, or an application/**/event/** package.
//
// It includes two anti-vacuity controls so a broken walk cannot pass green
// having checked nothing: (1) at least one handler .go file must be parsed, and
// (2) a known-good import the handlers genuinely declare
// (gokick/app/presentation/http/response, plus gokick/app/application/bus) must
// be observed — proving the parser saw real code, not an empty set.
func TestHandlersDoNotImportSqliteSecurityOrEvent(t *testing.T) {
	byFile := collectHandlerImports(t)
	if len(byFile) == 0 {
		t.Fatal("no handler .go files were parsed; the walk found nothing (vacuous)")
	}

	// Positive controls: confirm the parser actually read handler imports.
	sawResponse := false
	sawBus := false
	for _, imps := range byFile {
		for _, imp := range imps {
			switch imp {
			case "gokick/app/presentation/http/response":
				sawResponse = true
			case "gokick/app/application/bus":
				sawBus = true
			}
		}
	}
	if !sawResponse || !sawBus {
		t.Fatalf(
			"positive control failed (sawResponse=%v sawBus=%v): parser did not see the imports handlers are known to declare — the walk is not reading real code",
			sawResponse,
			sawBus,
		)
	}

	// The actual invariant.
	for file, imps := range byFile {
		for _, imp := range imps {
			if forbiddenHandlerImport(imp) {
				t.Errorf(
					"handler file %s imports forbidden package %q: HTTP handlers must not reach into sqlite, security, or application event packages — go through the bus / domain interfaces instead",
					file,
					imp,
				)
			}
		}
	}
}

// TestForbiddenHandlerImportClassification locks the matcher the guard rests on.
// Because the handler files are production code this test must not edit, this
// table is what makes the guard verifiably mutation-breakable: it proves the
// matcher returns true for representative sqlite/security/event paths and false
// for the application/bus/domain/response paths handlers legitimately use. If
// the matcher silently stopped catching one of these, this fails even though no
// handler file changed. Mirrors zz_audit_test.go's TestIsStdlibClassification.
func TestForbiddenHandlerImportClassification(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Forbidden: concrete infrastructure + event packages.
		{"gokick/app/infrastructure/sqlite", true},
		{"gokick/app/infrastructure/sqlite/user", true},
		{"gokick/app/infrastructure/sqlite/audit", true},
		{"gokick/app/infrastructure/security", true},
		{"gokick/app/application/user/event", true},
		// Allowed: application command/query, the bus, domain, presentation helpers, stdlib.
		{"gokick/app/application/bus", false},
		{"gokick/app/application/user/command", false},
		{"gokick/app/application/user/query", false},
		{"gokick/app/application/dashboard/query", false},
		{
			"gokick/app/domain/shared",
			false,
		}, // holds DomainEvent/EventCollector — must NOT match the event rule
		{"gokick/app/domain/user", false},
		{"gokick/app/presentation/http/response", false},
		{"gokick/app/presentation/http/request", false},
		{"context", false},
		{"net/http", false},
	}
	for _, tc := range cases {
		if got := forbiddenHandlerImport(tc.path); got != tc.want {
			t.Errorf("forbiddenHandlerImport(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
