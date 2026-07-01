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
	"runs":             true, // durable tasks: ClaimDue is a global drain; tenant rides on the row, not the claim.
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

// ----------------------------------------------------------------------------
// Write-side tenant gate (per-function).
//
// The query gate above proves an INSERT into a tenant-owned table *names* the
// tenant_id column — it says nothing about whether the *value* stamped there is
// the right tenant. This gate closes that: a repository function whose INSERT
// stamps a tenant_id column must also call a write-side tenant guard —
// shared.RequireTenant (born-scoped queues: run) or shared.AssertTenantScope
// (caller-supplied tenant: user.Save) — so the stamped tenant is validated, not
// taken on faith.
//
// The trigger is INTRINSIC: any INSERT naming tenant_id in its column list,
// regardless of table — no table list to keep in sync, and a future table is
// covered the moment its INSERT names tenant_id.
//
// Honest boundary:
//   - For tenant-OWNED tables (users) it is airtight: the query gate already
//     forces every query to name tenant_id, so an INSERT that stamps it cannot
//     avoid this gate.
//   - For read-EXEMPT-but-stamping tables (runs) it rests on the NamedExec
//     idiom of naming every column. A positional INSERT, or one that omits
//     tenant_id to lean on the column DEFAULT, names no tenant_id and escapes both
//     gates (the row then silently defaults to the default tenant). Don't write
//     those.
//
// Escape hatch: a guard computed in a HELPER instead of inline reads as a
// violation here (the call isn't in this function) — answer it with an inline
// /* tenant-write-exempt: reason */ marker stating why the write is safe.

const writeExemptMarker = "tenant-write-exempt"

// insertColsRe captures the explicit column list of an INSERT (the first
// parenthesized group after the table). Column lists never nest parens, so
// [^)]* stops cleanly at the close before VALUES.
var insertColsRe = regexp.MustCompile(`(?i)\binsert\s+into\s+[a-zA-Z_][a-zA-Z0-9_]*\s*\(([^)]*)\)`)

// stampsTenantID reports whether s is an INSERT whose explicit column list names
// tenant_id — i.e. a write that places a row in a specific tenant and therefore
// must be guarded. Comments are stripped first so a tenant_id mentioned in prose
// doesn't count; the column list itself carries no comments.
func stampsTenantID(s string) bool {
	m := insertColsRe.FindStringSubmatch(stripSQLComments(s))
	if m == nil {
		return false
	}
	return strings.Contains(strings.ToLower(m[1]), "tenant_id")
}

// stringLit unquotes a STRING BasicLit node (handles both "…" and `…`).
func stringLit(n ast.Node) (string, bool) {
	lit, ok := n.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// isTenantGuardCall reports whether n calls a write-side tenant guard
// (…RequireTenant / …AssertTenantScope), matched on the selector name so it
// catches the shared.* qualifier without binding to the import alias.
func isTenantGuardCall(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "RequireTenant" || sel.Sel.Name == "AssertTenantScope"
}

// tenantWriteViolations returns the names of functions in one Go source file that
// INSERT a tenant_id-stamping row without an inline write-side guard and without an
// exempt marker. src is nil to read the file at name, or a string/[]byte to parse
// directly. It fails loudly if a tenant-stamping INSERT lives outside a top-level
// function (a package-level const or a closure), which this per-function scan would
// otherwise skip — so the gate can never be quietly evaded by hoisting the query.
func tenantWriteViolations(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	// Total tenant-stamping INSERTs at file scope, to cross-check the per-func scan
	// accounts for all of them.
	fileInserts := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := stringLit(n); ok && stampsTenantID(s) {
			fileInserts++
		}
		return true
	})

	var violations []string
	funcInserts := 0
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var inserts []string
		guarded := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if s, ok := stringLit(n); ok && stampsTenantID(s) {
				inserts = append(inserts, s)
			}
			if isTenantGuardCall(n) {
				guarded = true
			}
			return true
		})
		funcInserts += len(inserts)
		if len(inserts) == 0 {
			continue
		}
		exempt := false
		for _, ins := range inserts {
			if strings.Contains(ins, writeExemptMarker) {
				exempt = true
			}
		}
		if !guarded && !exempt {
			violations = append(violations, fmt.Sprintf(
				"%s INSERTs a tenant_id row without a RequireTenant/AssertTenantScope guard "+
					"(or a /* %s: reason */ marker)", fn.Name.Name, writeExemptMarker))
		}
	}

	if funcInserts < fileInserts {
		t.Fatalf("%s: a tenant_id-stamping INSERT lives outside a top-level function "+
			"(package-level const or a closure); move it into the repo method so the "+
			"write-side gate can see its guard", filepath.Base(name))
	}
	return violations
}

// Every repo function that stamps a tenant_id on INSERT guards it. A new
// tenant-owned write that forgets RequireTenant/AssertTenantScope fails here.
func TestTenantConformance_TenantWritesGuarded(t *testing.T) {
	var violations []string
	err := filepath.WalkDir(sqliteDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, v := range tenantWriteViolations(t, path, nil) {
			violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), v))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repos: %v", err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("unguarded tenant writes:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The write gate bites an INSERT that stamps tenant_id with no guard.
func TestTenantConformance_WriteGate_FlagsUnguardedInsert(t *testing.T) {
	const src = "package fake\n" +
		"func (r *Repo) Save() error {\n" +
		"\tconst q = `INSERT INTO widgets (id, tenant_id) VALUES (:id, :tenant_id)`\n" +
		"\treturn r.Exec(q)\n}\n"
	if v := tenantWriteViolations(t, "fake.go", src); len(v) == 0 {
		t.Fatal("an INSERT stamping tenant_id without a guard must be flagged")
	}
}

// An inline RequireTenant/AssertTenantScope call satisfies the gate.
func TestTenantConformance_WriteGate_AcceptsGuardedInsert(t *testing.T) {
	const src = "package fake\n" +
		"func (r *Repo) Save() error {\n" +
		"\tif _, err := shared.RequireTenant(r.t, r.m); err != nil {\n\t\treturn err\n\t}\n" +
		"\tconst q = `INSERT INTO widgets (id, tenant_id) VALUES (:id, :tenant_id)`\n" +
		"\treturn r.Exec(q)\n}\n"
	if v := tenantWriteViolations(t, "fake.go", src); len(v) != 0 {
		t.Fatalf("a guarded INSERT must pass, got %v", v)
	}
}

// The exempt marker is the documented escape hatch (e.g. a guard in a helper).
func TestTenantConformance_WriteGate_AcceptsExemptMarker(t *testing.T) {
	const src = "package fake\n" +
		"func (r *Repo) Save() error {\n" +
		"\tconst q = `INSERT INTO widgets (id, tenant_id) VALUES (:id, :tenant_id) " +
		"/* tenant-write-exempt: guarded in helper */`\n" +
		"\treturn r.Exec(q)\n}\n"
	if v := tenantWriteViolations(t, "fake.go", src); len(v) != 0 {
		t.Fatalf("an exempt-marked INSERT must pass, got %v", v)
	}
}

// An INSERT that does not stamp tenant_id needs no guard (e.g. refresh_tokens).
func TestTenantConformance_WriteGate_IgnoresInsertWithoutTenantID(t *testing.T) {
	const src = "package fake\n" +
		"func (r *Repo) Save() error {\n" +
		"\tconst q = `INSERT INTO refresh_tokens (id, token_hash) VALUES (:id, :token_hash)`\n" +
		"\treturn r.Exec(q)\n}\n"
	if v := tenantWriteViolations(t, "fake.go", src); len(v) != 0 {
		t.Fatalf("an INSERT not stamping tenant_id needs no guard, got %v", v)
	}
}
