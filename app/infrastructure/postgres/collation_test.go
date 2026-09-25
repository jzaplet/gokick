package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/jmoiron/sqlx"
)

// The Postgres half of the Czech sort/search contract the SQLite adapter checks
// in its own collation_test.go, against the same golden files: the app_sort
// collation of the migrations and postgres.ILikeContains must reproduce them
// exactly. The expectations came from ICU cs-CZ; a Postgres whose ICU sorts
// differently — a newer CLDR in the image — fails here before a grid changes order.
const goldenSortDir = "../database/testdata/sort_cs"

func readGoldenLines(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenSortDir, name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

// scratchDB opens a migrated database as the superuser (it creates scratch
// tables) and loads a table of the given rows.
func scratchDB(t *testing.T, rows []string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open(postgres.DriverName, pgfx.New(t).AdminURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE scratch (s text NOT NULL)`); err != nil {
		t.Fatalf("create scratch table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO scratch (s) SELECT unnest($1::text[])`, rows); err != nil {
		t.Fatalf("load scratch table: %v", err)
	}
	return db
}

func TestSortCollation_MatchesGoldenCzechOrder(t *testing.T) {
	db := scratchDB(t, readGoldenLines(t, "corpus.txt"))
	want := readGoldenLines(t, "expected.txt")
	ctx := context.Background()

	var asc []string
	if err := db.SelectContext(ctx, &asc,
		`SELECT s FROM scratch ORDER BY s`+database.CollateSort+`, s`); err != nil {
		t.Fatalf("select asc: %v", err)
	}
	if !slices.Equal(asc, want) {
		t.Fatalf(
			"ASC order differs from the golden Czech order (%d vs %d rows)",
			len(asc),
			len(want),
		)
	}
	var desc []string
	if err := db.SelectContext(ctx, &desc,
		`SELECT s FROM scratch ORDER BY s`+database.CollateSort+` DESC, s`); err != nil {
		t.Fatalf("select desc: %v", err)
	}
	reversed := slices.Clone(want)
	slices.Reverse(reversed)
	if !slices.Equal(desc, reversed) {
		t.Fatal("DESC order is not the golden order reversed")
	}
}

// Case-insensitive, accent-sensitive substring search gives exactly the golden
// counts — the same the SQLite adapter's Unicode LIKE gives.
func TestILikeContains_MatchesGoldenCounts(t *testing.T) {
	db := scratchDB(t, readGoldenLines(t, "corpus.txt"))
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
		var a postgres.Args
		var got int
		if err := db.GetContext(ctx, &got,
			`SELECT COUNT(*) FROM scratch WHERE `+postgres.ILikeContains("s", &a, pattern), a...); err != nil {
			t.Fatalf("count %q: %v", pattern, err)
		}
		if got != want {
			t.Errorf("ILIKE %q: got %d matches, want %d", pattern, got, want)
		}
	}
}

// What a user types is searched for literally: % and _ are data, and the escape
// character itself is escaped too.
func TestILikeContains_MatchesWildcardCharactersLiterally(t *testing.T) {
	db := scratchDB(t, []string{"a_b", "axb", "50%", "500", `c\d`, "cxd"})
	ctx := context.Background()
	for search, want := range map[string][]string{
		"a_b": {"a_b"},
		"50%": {"50%"},
		`c\d`: {`c\d`},
		"A_B": {"a_b"}, // still case-insensitive
	} {
		var a postgres.Args
		var got []string
		if err := db.SelectContext(ctx, &got,
			`SELECT s FROM scratch WHERE `+postgres.ILikeContains("s", &a, search)+` ORDER BY s`,
			a...); err != nil {
			t.Fatalf("search %q: %v", search, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("search %q: got %v, want %v", search, got, want)
		}
	}
}
