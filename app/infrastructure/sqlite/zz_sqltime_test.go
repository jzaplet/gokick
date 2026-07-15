package sqlite_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Datetime comparison gate (F-043).
//
// DATETIME (TEXT) columns hold two text encodings — Go-written RFC3339
// ("…T10:00:00Z") and DB-clock ("… 10:00:00.123", NowExpr/LeaseExpr). The two
// do not compare as raw strings ('T' sorts above ' '), so every relational
// comparison or ORDER BY on a datetime column must go through julianday(col) —
// the one blessed idiom (see sqltime.go). This gate scans every string literal
// in the sqlite tree and fails on:
//   - a datetime column compared with < > <= >= as raw text, or via
//     datetime(col) (normalises, but is a second idiom — use julianday);
//   - a datetime column in an ORDER BY not wrapped in julianday().
//
// Datetime columns are recognised by naming convention: *_at and locked_until.
// Honest boundary: plain equality (=) is not checked — SET assignments use it
// pervasively and no repo compares datetime equality; MIN/MAX aggregation is
// likewise out of scope. Queries built by concatenation are checked fragment
// by fragment, so a comparison split across two Go literals would escape —
// keep a comparison inside one literal.

const dtColPattern = `(?:[a-zA-Z_][a-zA-Z0-9_]*\.)?(?:[a-zA-Z_][a-zA-Z0-9_]*_at|locked_until)`

var (
	// The blessed idiom — neutralised before the raw-column scan.
	dtJulianWrapRe = regexp.MustCompile(`(?i)\bjulianday\s*\(\s*` + dtColPattern + `\s*\)`)
	// datetime(col) normalises encodings but is a second idiom; unwrap it so the
	// column is judged (and flagged) by the raw-text rules below.
	dtDatetimeWrapRe = regexp.MustCompile(`(?i)\bdatetime\s*\(\s*(` + dtColPattern + `)\s*\)`)
	// A leading ':' is captured so named bind parameters can be skipped.
	dtColRe     = regexp.MustCompile(`:?\b` + dtColPattern + `\b`)
	dtOrderByRe = regexp.MustCompile(`(?i)\border\s+by\s+([\s\S]*?)(?:\blimit\b|$)`)
)

// datetimeComparisonViolations classifies one string literal. Comments are
// stripped first so prose mentioning a column can't trip the scan.
func datetimeComparisonViolations(s string) []string {
	sql := stripSQLComments(s)
	sql = dtJulianWrapRe.ReplaceAllString(sql, "julianday_ok")
	sql = dtDatetimeWrapRe.ReplaceAllString(sql, "$1")

	var v []string
	for _, m := range dtColRe.FindAllStringIndex(sql, -1) {
		tok := sql[m[0]:m[1]]
		if strings.HasPrefix(tok, ":") {
			continue // named bind parameter, not a column
		}
		before := strings.TrimRight(sql[:m[0]], " \t\n")
		after := strings.TrimLeft(sql[m[1]:], " \t\n")
		if strings.HasPrefix(after, "<") || strings.HasPrefix(after, ">") ||
			strings.HasSuffix(before, "<") || strings.HasSuffix(before, ">") ||
			strings.HasSuffix(before, "<=") || strings.HasSuffix(before, ">=") {
			v = append(v, fmt.Sprintf(
				"datetime column %q compared as raw text — wrap it in julianday() (sqltime.go)",
				tok,
			))
		}
	}
	for _, ob := range dtOrderByRe.FindAllStringSubmatch(sql, -1) {
		for _, tok := range dtColRe.FindAllString(ob[1], -1) {
			if strings.HasPrefix(tok, ":") {
				continue
			}
			v = append(v, fmt.Sprintf(
				"datetime column %q in ORDER BY without julianday() (sqltime.go)", tok))
		}
	}
	return v
}

// Every repo query compares/orders datetime columns via julianday only. A raw
// or datetime()-wrapped comparison fails here.
func TestSqlTimeConformance_ComparisonsUseJulianday(t *testing.T) {
	var violations []string
	err := filepath.WalkDir(sqliteDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, s := range sqlStringsInGoSource(t, path, nil) {
			for _, vio := range datetimeComparisonViolations(s) {
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
		t.Fatalf("datetime comparison violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The gate bites a raw-text comparison and the datetime() second idiom.
func TestSqlTimeConformance_FlagsRawAndDatetimeComparisons(t *testing.T) {
	if len(datetimeComparisonViolations(
		`DELETE FROM refresh_tokens WHERE expires_at < datetime('now')`)) == 0 {
		t.Fatal("a raw datetime comparison must be a violation; the gate does not bite")
	}
	if len(datetimeComparisonViolations(
		`DELETE FROM refresh_tokens WHERE datetime(expires_at) < datetime('now')`)) == 0 {
		t.Fatal("a datetime()-wrapped comparison must be a violation (julianday is the one idiom)")
	}
	if len(datetimeComparisonViolations(
		`SELECT * FROM runs WHERE julianday(locked_until) >= julianday('now')`)) != 0 {
		t.Fatal("a julianday-wrapped comparison must pass")
	}
}

// The gate bites a raw ORDER BY and accepts the julianday form.
func TestSqlTimeConformance_FlagsRawOrderBy(t *testing.T) {
	if len(datetimeComparisonViolations(`SELECT id FROM runs ORDER BY run_at LIMIT 1`)) == 0 {
		t.Fatal("a raw datetime ORDER BY must be a violation")
	}
	if v := datetimeComparisonViolations(
		`SELECT id FROM runs ORDER BY julianday(run_at) LIMIT 1`); len(v) != 0 {
		t.Fatalf("a julianday ORDER BY must pass, got %v", v)
	}
}

// Non-comparison uses stay untouched: NULL checks, SET assignments, named bind
// parameters, and ORDER BY on non-datetime columns.
func TestSqlTimeConformance_IgnoresNonComparisonUses(t *testing.T) {
	for _, q := range []string{
		`SELECT id FROM runs WHERE locked_until IS NULL AND completed_at IS NOT NULL`,
		`UPDATE runs SET locked_until = NULL, updated_at = strftime('%f','now') WHERE id = ?`,
		`INSERT INTO refresh_tokens (id, expires_at) VALUES (:id, :expires_at)`,
		`SELECT * FROM users ORDER BY nickname`,
	} {
		if v := datetimeComparisonViolations(q); len(v) != 0 {
			t.Fatalf("query %q must pass, got %v", q, v)
		}
	}
}
