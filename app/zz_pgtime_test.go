package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Postgres clock gate (N9 of the adapter plan) — the twin of the SQLite adapter's
// datetime gate.
//
// now(), CURRENT_TIMESTAMP, transaction_timestamp() and LOCALTIMESTAMP are the
// START of the transaction. Inside a longer transaction every timestamp and every
// lease computed from them would stand still at the first statement — a lease
// renewed late in a transaction could already be expired when written. The
// adapter's clock is statement_timestamp() (postgres.NowExpr / NowPlus), the start
// of the statement, the same clock SQLite's 'now' is. This gate scans the SQL in
// the adapter's source and in the Postgres migrations (a column DEFAULT is a
// stamp too) and fails on the transaction clock. SQL comments are stripped first,
// so a comment explaining the rule does not trip it.

var pgTxClockRe = regexp.MustCompile(
	`(?i)\bnow\s*\(|\bcurrent_timestamp\b|\btransaction_timestamp\s*\(|\blocaltimestamp\b`)

func pgTxClockViolations(sql string) []string {
	return pgTxClockRe.FindAllString(stripSQLComments(sql), -1)
}

func TestPostgresClock_StatementTimestampOnly(t *testing.T) {
	var violations []string
	literals := 0
	err := filepath.WalkDir(
		adapterDir("postgres"),
		func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			for _, s := range sqlStringsInGoSource(t, path, nil) {
				literals++
				for _, v := range pgTxClockViolations(s) {
					violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), v))
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("walk the adapter: %v", err)
	}
	migrationsDir := filepath.Join(repoRoot(), "migrations", "postgres")
	migrationFiles, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, path := range migrationFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, v := range pgTxClockViolations(string(b)) {
			violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), v))
		}
	}
	if literals == 0 || len(migrationFiles) == 0 {
		t.Fatalf("scanned %d literals and %d migrations — the scan went blind",
			literals, len(migrationFiles))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("the transaction clock in Postgres SQL — use statement_timestamp() "+
			"(postgres.NowExpr / NowPlus):\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestPostgresClock_GateBites(t *testing.T) {
	for _, bad := range []string{
		"UPDATE runs SET updated_at = now()",
		"UPDATE runs SET updated_at = NOW ()",
		"DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP",
		"SELECT transaction_timestamp()",
		"created_at timestamp NOT NULL DEFAULT LOCALTIMESTAMP",
	} {
		if len(pgTxClockViolations(bad)) == 0 {
			t.Errorf("not flagged: %s", bad)
		}
	}
	for _, good := range []string{
		"UPDATE runs SET updated_at = statement_timestamp()",
		"UPDATE runs SET known_at = $1 -- never now()",
		"SELECT 1 /* now() is the transaction start */",
	} {
		if v := pgTxClockViolations(good); len(v) != 0 {
			t.Errorf("flagged %v in: %s", v, good)
		}
	}
}
