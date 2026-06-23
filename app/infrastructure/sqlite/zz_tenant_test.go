package sqlite_test

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
	"strconv"
	"strings"
	"testing"
)

// Tenant conformance gate (per-query).
//
// Every SQL query in a SQLite repository is checked:
//   - a query touching a TENANT-OWNED table must scope by tenant_id, OR carry an
//     inline /* tenant-scope-exempt: reason */ marker (auth/identity queries that
//     legitimately run before/without a tenant);
//   - a query touching an EXEMPT table (control-plane / global) is fine;
//   - a query touching an UNCLASSIFIED table fails — so a new product table can't
//     ship without a conscious tenant decision ("born scoped").
//
// users is tenant-owned: admin reads/writes scope by tenant_id; the login /
// identity queries carry the exempt marker. Resolution is data-driven (the JWT
// carries the tenant); this gate enforces that the queries actually use it.

const exemptMarker = "tenant-scope-exempt"

// tenantOwnedTables — every query must scope by tenant_id or carry the marker.
var tenantOwnedTables = map[string]bool{
	"users": true,
}

// exemptTables — control-plane / global, never tenant-scoped.
var exemptTables = map[string]bool{
	"refresh_tokens":   true, // keyed by token hash (a secret); refresh runs without an access token.
	"audit_log":        true, // control-plane, raw pool, records pre-auth events.
	"jobs":             true, // ClaimDue is a global drain; the tenant rides on the row, not the claim.
	"tenants":          true, // the tenant registry itself.
	"sqlite_master":    true, // SQLite internals.
	"sqlite_sequence":  true,
	"goose_db_version": true,
}

var (
	sqlVerbRe    = regexp.MustCompile(`(?i)\b(?:select|insert|update|delete)\b`)
	sqlCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/|--[^\n]*`)
	tableRe      = regexp.MustCompile(`(?i)\b(?:from|into|update|join)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
)

func stripSQLComments(s string) string { return sqlCommentRe.ReplaceAllString(s, " ") }

// tablesInSQL returns the table names a SQL string references (lowercased).
// Comments are stripped first so neither prose nor the exempt marker can be
// mistaken for a table.
func tablesInSQL(s string) []string {
	if !sqlVerbRe.MatchString(s) {
		return nil
	}
	stripped := stripSQLComments(s)
	var out []string
	for _, m := range tableRe.FindAllStringSubmatch(stripped, -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	return out
}

// violationsInSQL classifies a single query string. tenant_id is checked on the
// comment-stripped SQL (so a comment mentioning it doesn't count as scoping);
// the exempt marker is checked on the raw string (it lives in a comment).
func violationsInSQL(s string) []string {
	tables := tablesInSQL(s)
	if len(tables) == 0 {
		return nil
	}
	scopedOrExempt := strings.Contains(stripSQLComments(s), "tenant_id") ||
		strings.Contains(s, exemptMarker)

	var v []string
	for _, tbl := range tables {
		switch {
		case exemptTables[tbl]:
		case tenantOwnedTables[tbl]:
			if !scopedOrExempt {
				v = append(v, fmt.Sprintf(
					"unscoped query on tenant-owned table %q (add a tenant_id filter, "+
						"or a /* %s: reason */ marker if it is a legitimate identity/auth query)",
					tbl, exemptMarker))
			}
		default:
			v = append(v, fmt.Sprintf(
				"unclassified table %q (declare it in tenantOwnedTables or exemptTables)", tbl))
		}
	}
	return v
}

// sqlStringsInGoSource returns every string literal in Go source (so SQL in //
// comments is ignored — only real query strings). src is nil to read the file at
// name, or a string/[]byte to parse directly.
func sqlStringsInGoSource(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var lits []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if val, err := strconv.Unquote(lit.Value); err == nil {
			lits = append(lits, val)
		}
		return true
	})
	return lits
}

// sqliteDir is this package's directory (app/infrastructure/sqlite), resolved
// from the compiled-in source path so the scan is independent of the test's CWD.
func sqliteDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

// Every repo query scopes-or-exempts tenant-owned tables and touches no
// unclassified table. An unscoped admin read or a new product table fails here.
func TestTenantConformance_RepoQueriesScopedOrExempt(t *testing.T) {
	var violations []string
	err := filepath.WalkDir(sqliteDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, s := range sqlStringsInGoSource(t, path, nil) {
			for _, vio := range violationsInSQL(s) {
				violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), vio))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repos: %v", err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("tenant conformance violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The gate bites an unclassified table.
func TestTenantConformance_FlagsUnclassifiedTable(t *testing.T) {
	if len(violationsInSQL("SELECT * FROM widgets WHERE id = ?")) == 0 {
		t.Fatal("an unclassified table must be a violation; the gate does not bite")
	}
}

// The gate bites an unscoped tenant-owned query and accepts a scoped or
// exempt-marked one.
func TestTenantConformance_TenantOwnedMustScopeOrMark(t *testing.T) {
	if len(violationsInSQL("SELECT * FROM users WHERE nickname = ?")) == 0 {
		t.Fatal("an unscoped tenant-owned query must be a violation")
	}
	if v := violationsInSQL("SELECT * FROM users WHERE tenant_id = ?"); len(v) != 0 {
		t.Fatalf("a tenant_id-scoped query must pass, got %v", v)
	}
	if v := violationsInSQL(
		"SELECT * FROM users WHERE id = ? /* tenant-scope-exempt: identity */"); len(v) != 0 {
		t.Fatalf("an exempt-marked query must pass, got %v", v)
	}
}

// The file scan actually extracts SQL from Go source — else the repo walk could
// pass vacuously by finding nothing.
func TestTenantConformance_FileScanExtractsSQL(t *testing.T) {
	const fake = "package fake\n\nconst q = `SELECT * FROM widgets WHERE tenant_id = ?`\n"
	found := false
	for _, s := range sqlStringsInGoSource(t, "fake.go", fake) {
		if strings.Contains(s, "widgets") {
			found = true
		}
	}
	if !found {
		t.Fatal("file scan must extract the SQL literal from Go source")
	}
}
