package sqlite_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Krok 3 — tenant conformance gate.
//
// Every table a SQLite repository touches must be CLASSIFIED: either
// tenant-owned (its queries must scope by tenant_id — that half is enforced
// once such a table exists, Krok 4) or exempt (control-plane / identity-root /
// global). A table in NEITHER list fails this test, so the first product table
// cannot ship without a conscious tenant decision.
//
// Today there are no tenant-owned tables (users is identity-root: login/refresh
// look it up before any tenant is known), so everything is exempt. This gate is
// the guardrail that bites the moment that changes.

// tenantOwnedTables — queries MUST include tenant_id. Empty until the first real
// tenant-owned table lands (Krok 4 / product).
var tenantOwnedTables = map[string]bool{}

// exemptTables — legitimately unscoped. Mirrors docs/framework/gokick-roadmap.md
// ("Legitimně nescopované").
var exemptTables = map[string]bool{
	"users":            true, // identity-root; login/refresh look it up pre-tenant. Krok 4 scopes admin queries.
	"refresh_tokens":   true, // keyed by token hash (a secret); refresh runs without an access token.
	"audit_log":        true, // control-plane, raw pool, records pre-auth events.
	"jobs":             true, // ClaimDue is a global drain; the tenant rides on the row, not the claim query.
	"tenants":          true, // the tenant registry itself (control-plane).
	"sqlite_master":    true, // SQLite internals.
	"sqlite_sequence":  true,
	"goose_db_version": true,
}

func isClassified(table string) bool {
	return tenantOwnedTables[table] || exemptTables[table]
}

var (
	sqlVerbRe    = regexp.MustCompile(`(?i)\b(?:select|insert|update|delete)\b`)
	sqlCommentRe = regexp.MustCompile(`--[^\n]*`)
	tableRe      = regexp.MustCompile(`(?i)\b(?:from|into|update|join)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
)

// tablesInSQL returns the table names referenced in a SQL string (lowercased).
// Strings without a SQL verb are ignored (prose/log lines don't match), and SQL
// line comments are stripped first so "-- pre-update column" can't masquerade
// as `UPDATE column`.
func tablesInSQL(s string) []string {
	if !sqlVerbRe.MatchString(s) {
		return nil
	}
	s = sqlCommentRe.ReplaceAllString(s, " ")
	var out []string
	for _, m := range tableRe.FindAllStringSubmatch(s, -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	return out
}

// tablesInGoSource extracts table names from every string literal in Go source
// (so SQL mentioned in // comments is ignored — only real query strings). src is
// nil to read the file at name, or a string/[]byte to parse directly.
func tablesInGoSource(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var tables []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		tables = append(tables, tablesInSQL(val)...)
		return true
	})
	return tables
}

func tablesInFile(t *testing.T, path string) []string {
	t.Helper()
	return tablesInGoSource(t, path, nil)
}

// sqliteDir is this package's directory (app/infrastructure/sqlite), resolved
// from the compiled-in source path so the scan is independent of the test's CWD.
func sqliteDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

// Every table touched by a repository must be classified. A new product repo
// that touches an unclassified table fails here until it's declared tenant-owned
// or exempt — the "born scoped" guarantee.
func TestTenantConformance_AllRepoTablesClassified(t *testing.T) {
	seen := map[string]bool{}
	err := filepath.WalkDir(sqliteDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, tbl := range tablesInFile(t, path) {
			seen[tbl] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repos: %v", err)
	}

	var unclassified []string
	for tbl := range seen {
		if !isClassified(tbl) {
			unclassified = append(unclassified, tbl)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("repo SQL touches unclassified table(s) %v — classify each in zz_tenant_test.go: "+
			"tenantOwnedTables (queries must include tenant_id) or exemptTables (control-plane/identity-root)",
			unclassified)
	}
}

// The gate must actually bite: a table in neither list is a violation. Proves
// TestTenantConformance_AllRepoTablesClassified would fail on a new unclassified
// table rather than passing vacuously.
func TestTenantConformance_FlagsUnclassifiedTable(t *testing.T) {
	tables := tablesInSQL("SELECT * FROM widgets WHERE id = ?")
	if len(tables) == 0 {
		t.Fatal("scanner failed to extract a table name from a SELECT")
	}
	for _, tbl := range tables {
		if isClassified(tbl) {
			t.Fatalf("fixture table %q must be unclassified to prove the gate bites", tbl)
		}
	}
}

// The file scan must actually extract tables from Go source — otherwise a broken
// AST walk would make TestTenantConformance_AllRepoTablesClassified pass
// vacuously (no tables found → nothing unclassified → green for the wrong reason).
func TestTenantConformance_FileScanExtractsTables(t *testing.T) {
	const fake = "package fake\n\nconst q = `SELECT * FROM widgets WHERE tenant_id = ?`\n"
	tables := tablesInGoSource(t, "fake.go", fake)
	found := false
	for _, tbl := range tables {
		if tbl == "widgets" {
			found = true
		}
	}
	if !found {
		t.Fatalf("file scan must extract 'widgets' from Go source, got %v", tables)
	}
}
