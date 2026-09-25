package app_test

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// No-SQLite gate — the static third of the "switch the adapter and no SQLite is
// left anywhere" guarantee. The other two: `go test -tags nosqlite` does not even
// compile SQLite in, and testfx refuses an adapter the run did not select.
//
// Rules, over every Go file in app/ and cmd/:
//
//  1. A file that imports SQLite (the adapter, or the ncruces driver) must be
//     excluded by the nosqlite build tag — so a -tags nosqlite build cannot link
//     it. Outside the adapter these are exactly the two openers
//     (infrastructure/persistence/sqlite.go, internal/testfx/sqlite.go).
//  2. Every file under infrastructure/sqlite/ is excluded by the nosqlite tag.
//  3. Every package under infrastructure/sqlite/ that has tests declares
//     TestMain as testfx.MainFor(m, database.DriverSQLite), so a run targeting
//     another adapter skips them instead of quietly testing SQLite.
//  4. No string literal in a test, or in the test fixtures under app/internal/,
//     carries SQLite dialect unless its file is SQLite-tagged:
//     julianday()/strftime()/datetime(), PRAGMA, sqlite_master, INSERT OR
//     REPLACE/IGNORE, or a *.db file path. SQL a test needs lives in a testfx
//     helper, portable or per backend. (Production code is covered by rules 1-2
//     and the adapter boundary; config legitimately names the SQLite file path.)

var sqliteImportRe = regexp.MustCompile(
	`^(gokick/app/infrastructure/sqlite(/.*)?|github\.com/ncruces/.*)$`)

var sqliteDialectRe = regexp.MustCompile(`(?i)\b(?:julianday|strftime|datetime)\s*\(` +
	`|\bpragma\s+[a-z_]` +
	`|\bsqlite_(?:master|schema|sequence)\b` +
	`|\binsert\s+or\s+(?:replace|ignore)\b` +
	`|\.db$`)

const sqliteAdapterDir = "app/infrastructure/sqlite"

// excludedByNoSQLite reports whether src's //go:build line keeps the file out
// of a -tags nosqlite build. Every other tag is taken as set, so only the
// nosqlite term can exclude it — `loadtest && !nosqlite` counts, a bare
// `loadtest` does not.
func excludedByNoSQLite(src []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return false
		}
		return !expr.Eval(func(string) bool { return true })
	}
	return false
}

// sqliteDialectLiterals returns the string literals in f that carry SQLite
// dialect.
func sqliteDialectLiterals(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil && sqliteDialectRe.MatchString(s) {
			out = append(out, s)
		}
		return true
	})
	return out
}

// goFileFacts is what the gate needs to know about one source file.
type goFileFacts struct {
	rel      string // slash path from the module root
	excluded bool   // kept out of -tags nosqlite builds
	imports  []string
	dialect  []string // string literals carrying SQLite dialect
	testMain string   // source of a TestMain declaration, if any
}

func readGoFile(t *testing.T, root, path string) goFileFacts {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	rel, _ := filepath.Rel(root, path)
	facts := goFileFacts{rel: filepath.ToSlash(rel), excluded: excludedByNoSQLite(src)}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		facts.imports = append(facts.imports, p)
	}
	facts.dialect = sqliteDialectLiterals(f)
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "TestMain" && fn.Recv == nil {
			facts.testMain = string(
				src[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset],
			)
		}
	}
	return facts
}

func walkGoFiles(t *testing.T) []goFileFacts {
	t.Helper()
	var files []goFileFacts
	eachGoFile(t, func(path string) { files = append(files, readGoFile(t, repoRoot(), path)) })
	return files
}

func noSQLiteViolations(files []goFileFacts) []string {
	var v []string
	testsByPkg := map[string]bool{} // adapter package dir → has tests
	mainForPkg := map[string]bool{} // adapter package dir → TestMain via MainFor(sqlite)
	for _, f := range files {
		for _, imp := range f.imports {
			if sqliteImportRe.MatchString(imp) && !f.excluded {
				v = append(v, fmt.Sprintf("%s imports %s without //go:build !nosqlite", f.rel, imp))
			}
		}
		if strings.HasPrefix(f.rel, sqliteAdapterDir+"/") {
			v = append(v, adapterFileViolations(f, testsByPkg, mainForPkg)...)
			continue
		}
		testCode := strings.HasSuffix(f.rel, "_test.go") ||
			strings.HasPrefix(f.rel, "app/internal/")
		if !testCode || f.excluded || strings.HasSuffix(f.rel, "zz_nosqlite_test.go") {
			continue
		}
		for _, s := range f.dialect {
			v = append(
				v,
				fmt.Sprintf("%s carries SQLite dialect outside the adapter: %q", f.rel, s),
			)
		}
	}
	for dir := range testsByPkg {
		if !mainForPkg[dir] {
			v = append(v, fmt.Sprintf(
				"%s has tests but no TestMain calling testfx.MainFor(m, database.DriverSQLite)",
				dir,
			))
		}
	}
	sort.Strings(v)
	return v
}

// adapterFileViolations checks one file inside the SQLite adapter (rule 2) and
// records its package's tests and TestMain for rule 3.
func adapterFileViolations(f goFileFacts, testsByPkg, mainForPkg map[string]bool) []string {
	var v []string
	if !f.excluded {
		v = append(
			v,
			fmt.Sprintf("%s is in the SQLite adapter but lacks //go:build !nosqlite", f.rel),
		)
	}
	if !strings.HasSuffix(f.rel, "_test.go") {
		return v
	}
	dir := filepath.Dir(f.rel)
	testsByPkg[dir] = true
	if strings.Contains(f.testMain, "testfx.MainFor(m, database.DriverSQLite)") {
		mainForPkg[dir] = true
	}
	return v
}

func TestNoSQLite_OutsideTheAdapter(t *testing.T) {
	files := walkGoFiles(t) // eachGoFile, repoRoot: zz_params_test.go
	adapterFiles, openers := 0, 0
	for _, f := range files {
		if strings.HasPrefix(f.rel, sqliteAdapterDir+"/") {
			adapterFiles++
		} else if f.excluded {
			openers++
		}
	}
	// Anti-vacuity: the walk must have found the adapter and the two openers.
	if adapterFiles == 0 || openers < 2 {
		t.Fatalf("walk found %d adapter files and %d nosqlite-excluded files outside it — "+
			"the scan is looking in the wrong place", adapterFiles, openers)
	}
	if v := noSQLiteViolations(files); len(v) > 0 {
		t.Fatalf("SQLite outside the nosqlite boundary:\n  %s", strings.Join(v, "\n  "))
	}
}

// The rules bite: each synthetic file breaks exactly one of them.
func TestNoSQLite_RulesBite(t *testing.T) {
	tagged := func(rel string) goFileFacts { return goFileFacts{rel: rel, excluded: true} }
	adapterTest := tagged(sqliteAdapterDir + "/x_test.go")
	adapterTest.testMain = "func TestMain(m *testing.M) { testfx.MainFor(m, database.DriverSQLite) }"
	cases := []struct {
		name string
		file goFileFacts
	}{
		{"untagged sqlite import", goFileFacts{
			rel: "app/x/x.go", imports: []string{"gokick/app/infrastructure/sqlite"}}},
		{"untagged ncruces import", goFileFacts{
			rel: "app/x/x.go", imports: []string{"github.com/ncruces/go-sqlite3"}}},
		{"untagged adapter file", goFileFacts{rel: sqliteAdapterDir + "/user/x.go"}},
		{"adapter tests without MainFor", tagged(sqliteAdapterDir + "/run/x_test.go")},
		{"dialect in a test", goFileFacts{
			rel: "app/x/x_test.go", dialect: []string{"UPDATE runs SET x = strftime('%s')"}}},
	}
	for _, tc := range cases {
		if v := noSQLiteViolations([]goFileFacts{tc.file, adapterTest}); len(v) == 0 {
			t.Errorf("%s: the gate did not bite", tc.name)
		}
	}
	if v := noSQLiteViolations([]goFileFacts{adapterTest,
		tagged("app/internal/testfx/sqlite.go")}); len(v) != 0 {
		t.Errorf("tagged files must pass, got %v", v)
	}
}

// The dialect matcher: SQLite syntax matches, the portable subset does not.
func TestNoSQLite_DialectMatcher(t *testing.T) {
	for _, s := range []string{
		"SELECT julianday('now')", "strftime('%Y', x)", "WHERE created_at < datetime('now')",
		"PRAGMA busy_timeout", "SELECT name FROM sqlite_master", "INSERT OR REPLACE INTO t",
		"fixture.db",
	} {
		if !sqliteDialectRe.MatchString(s) {
			t.Errorf("must match SQLite dialect: %q", s)
		}
	}
	for _, s := range []string{
		"SELECT COUNT(*) FROM runs WHERE kind = ?", "UPDATE users SET active = ? WHERE id = ?",
		"created_at", "the database", "app.db.example",
	} {
		if sqliteDialectRe.MatchString(s) {
			t.Errorf("portable SQL / prose must not match: %q", s)
		}
	}
	for src, want := range map[string]bool{
		"//go:build !nosqlite\n\npackage x":             true,
		"//go:build loadtest && !nosqlite\n\npackage x": true,
		"//go:build loadtest\n\npackage x":              false,
		"package x // nosqlite":                         false,
	} {
		if got := excludedByNoSQLite([]byte(src)); got != want {
			t.Errorf("excludedByNoSQLite(%q) = %v, want %v", src, got, want)
		}
	}
}
