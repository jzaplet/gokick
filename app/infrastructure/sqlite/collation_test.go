//go:build !nosqlite

package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"

	"github.com/google/uuid"
)

// goldenSortDir holds the Czech sort/search contract shared by every adapter:
// corpus.txt (unsorted input), expected.txt (its ascending order) and like.txt
// (case-insensitive substring pattern → match count). The expectations were
// produced by Postgres with an ICU cs-CZ collation — the order the Postgres
// adapter uses — so SQLite reproducing them exactly IS the guarantee that both
// backends sort and search identically. The day a CLDR update on either side
// (golang.org/x/text here, ICU there) changes an order, this fails first.
const goldenSortDir = "../database/testdata/sort_cs"

func readGoldenLines(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenSortDir, name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

// loadSortCorpus creates a scratch table holding every corpus line.
func loadSortCorpus(t *testing.T, mgr *sqlite.Manager) {
	t.Helper()
	ctx := context.Background()
	if _, err := mgr.DB().ExecContext(ctx, `CREATE TABLE sort_corpus (s TEXT NOT NULL)`); err != nil {
		t.Fatalf("create corpus table: %v", err)
	}
	for _, s := range readGoldenLines(t, "corpus.txt") {
		if _, err := mgr.DB().ExecContext(ctx, `INSERT INTO sort_corpus (s) VALUES (?)`, s); err != nil {
			t.Fatalf("insert %q: %v", s, err)
		}
	}
}

func TestSortCollation_MatchesGoldenCzechOrder(t *testing.T) {
	mgr := newTestManager(t)
	loadSortCorpus(t, mgr)
	ctx := context.Background()
	want := readGoldenLines(t, "expected.txt")

	var asc []string
	if err := mgr.DB().SelectContext(ctx, &asc,
		`SELECT s FROM sort_corpus ORDER BY s`+database.CollateSort+`, s`); err != nil {
		t.Fatalf("select asc: %v", err)
	}
	assertSameOrder(t, "ASC", asc, want)

	// DESC is the exact reverse: no two corpus lines are equal under the collation,
	// so the binary tie-break never decides and the order simply flips.
	var desc []string
	if err := mgr.DB().SelectContext(ctx, &desc,
		`SELECT s FROM sort_corpus ORDER BY s`+database.CollateSort+` DESC, s`); err != nil {
		t.Fatalf("select desc: %v", err)
	}
	reversed := slices.Clone(want)
	slices.Reverse(reversed)
	assertSameOrder(t, "DESC", desc, reversed)
}

func assertSameOrder(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d rows, want %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf(
				"%s: first difference at position %d: got %q, want %q",
				label,
				i,
				got[i],
				want[i],
			)
		}
	}
}

// The search half of the contract: case-insensitive substring matching is
// Unicode-aware (Č finds č, Ř finds ř) and accent-sensitive (cerny does not find
// Černý), with exactly the counts Postgres ILIKE gives on the same corpus.
func TestUnicodeLike_MatchesGoldenCounts(t *testing.T) {
	mgr := newTestManager(t)
	loadSortCorpus(t, mgr)
	ctx := context.Background()

	for _, line := range readGoldenLines(t, "like.txt") {
		pattern, countStr, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("malformed like.txt line %q", line)
		}
		want, err := strconv.Atoi(countStr)
		if err != nil {
			t.Fatalf("like.txt count %q: %v", countStr, err)
		}
		var got int
		if err := mgr.DB().GetContext(ctx, &got,
			`SELECT COUNT(*) FROM sort_corpus WHERE s LIKE ?`+database.LikeEscape,
			database.LikeContains(pattern)); err != nil {
			t.Fatalf("count %q: %v", pattern, err)
		}
		if got != want {
			t.Errorf("LIKE %q: got %d matches, want %d", pattern, got, want)
		}
	}
}

// What a user types into a search box is searched for literally: % and _ are
// data, not wildcards, and the escape character itself is escaped too.
func TestLikeContains_MatchesWildcardCharactersLiterally(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.DB().ExecContext(ctx, `CREATE TABLE names (s TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, s := range []string{"a_b", "axb", "50%", "500", `c\d`, "cxd"} {
		if _, err := mgr.DB().ExecContext(ctx, `INSERT INTO names (s) VALUES (?)`, s); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	for search, want := range map[string][]string{
		"a_b": {"a_b"},
		"50%": {"50%"},
		`c\d`: {`c\d`},
		"A_B": {"a_b"}, // still case-insensitive
	} {
		var got []string
		if err := mgr.DB().SelectContext(ctx, &got,
			`SELECT s FROM names WHERE s LIKE ?`+database.LikeEscape+` ORDER BY s`,
			database.LikeContains(search)); err != nil {
			t.Fatalf("search %q: %v", search, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("search %q: got %v, want %v", search, got, want)
		}
	}
}

// uuidv7() exists in SQL — the name Postgres 18 ships natively — so a hand-written
// seed can mint ids the same way on both backends.
func TestUUIDv7SQLFunction_ReturnsDistinctVersion7IDs(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	var a, b string
	if err := mgr.DB().QueryRowContext(ctx, `SELECT uuidv7(), uuidv7()`).Scan(&a, &b); err != nil {
		t.Fatalf("select uuidv7(): %v", err)
	}
	for _, s := range []string{a, b} {
		id, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("uuidv7() returned %q: %v", s, err)
		}
		if id.Version() != 7 {
			t.Fatalf("uuidv7() returned version %d, want 7", id.Version())
		}
	}
	if a == b {
		t.Fatalf("two uuidv7() calls returned the same id %q", a)
	}
}

// The collation and functions are registered by the driver's per-connection init
// callback, so EVERY pooled connection has them — not only the first one opened.
// Hold several connections at once (forcing the pool to open distinct ones) and
// use the collation and uuidv7() on each.
func TestConnFuncs_RegisteredOnEveryPooledConnection(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	const held = 4
	conns := make([]*sql.Conn, 0, held)
	t.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
	})
	for i := range held {
		c, err := mgr.DB().Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		conns = append(conns, c)
		var first, id string
		if err := c.QueryRowContext(ctx,
			`SELECT s, uuidv7() FROM (SELECT 'čaj' AS s UNION ALL SELECT 'cibule')
			  ORDER BY s`+database.CollateSort+` LIMIT 1`).Scan(&first, &id); err != nil {
			t.Fatalf("conn %d: collation/uuidv7 unavailable: %v", i, err)
		}
		if first != "cibule" {
			t.Fatalf("conn %d: Czech order wants cibule before čaj, got %q first", i, first)
		}
	}
}
