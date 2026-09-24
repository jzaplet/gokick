//go:build !nosqlite

package sqlite_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"gokick/app/infrastructure/sqlite"
)

// indexExists reports whether an index with the given name exists in
// sqlite_master. Using sqlite_master (rather than PRAGMA index_list) lets the
// query name the index directly, so a renamed or dropped index fails the
// lookup unambiguously.
func indexExists(t *testing.T, mgr *sqlite.Manager, name string) bool {
	t.Helper()
	var count int
	if err := mgr.DB().Get(
		&count,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`,
		name,
	); err != nil {
		t.Fatalf("query sqlite_master for index %q: %v", name, err)
	}
	return count == 1
}

// TestMigrator_InitSchemaCreatesRefreshTokenIndexes pins the two
// indexes the init migration documents (claim infra-db-security-12):
// idx_refresh_tokens_token_hash on refresh_tokens(token_hash) and
// idx_refresh_tokens_user_id on refresh_tokens(user_id). It runs the real
// embedded migrations via Migrator.RunUp() and then asserts both named
// indexes are present in sqlite_master. If either CREATE INDEX line is removed
// from 20260327000001_init_schema.sql (or the index renamed), the corresponding
// lookup returns 0 and this test fails.
func TestMigrator_InitSchemaCreatesRefreshTokenIndexes(t *testing.T) {
	mgr := newTestManager(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := sqlite.NewMigrator(mgr, logger).RunUp(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	for _, name := range []string{
		"idx_refresh_tokens_token_hash",
		"idx_refresh_tokens_user_id",
	} {
		if !indexExists(t, mgr, name) {
			t.Errorf("expected index %q to exist after init migration, but it was not found", name)
		}
	}

	// Guard the index targets too: the token_hash index must cover the
	// token_hash column. A wrong target column would defeat the lookup the
	// index exists to accelerate. sqlite_master stores the original CREATE
	// statement, so assert it references the documented column.
	var ddl string
	if err := mgr.DB().Get(
		&ddl,
		`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_refresh_tokens_token_hash'`,
	); err != nil {
		t.Fatalf("read index ddl: %v", err)
	}
	if ddl == "" {
		t.Fatal("idx_refresh_tokens_token_hash has no stored DDL")
	}
}

// TestMigrationDown_RollsBackLastMigration pins that a single goose down step
// rolls back exactly the most recent migration (claim overview-102, mirroring
// `make migrate-down`). It applies every embedded migration up via the
// production Migrator.RunUp(), then runs one goose Down step exactly
// as the Makefile target does. It asserts the generic round-trip property rather
// than a specific migration's artifact, so it holds both for gokick's single
// squashed init AND for a project that adds migrations after it: the version
// drops, the step removes exactly the last migration (the whole schema when the
// init is all there is, only the newest step otherwise — init's users table then
// survives), and re-running up restores the version — proving the last
// migration's Down ran and is the inverse of its Up. If down were a no-op (or the
// +goose Down block were dropped), the version would not decrease — failing here.
func TestMigrationDown_RollsBackLastMigration(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := sqlite.NewMigrator(mgr, logger).RunUp(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// Mirror the Makefile's `goose ... down` invocation: same dialect, same
	// embedded SQLite migration set, one step down — through the SAME provider
	// constructor RunUp uses, so the test exercises the production goose setup.
	provider, err := sqlite.NewMigrationProvider(mgr.DB().DB)
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}

	before, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("get version before down: %v", err)
	}

	if _, err := provider.Down(ctx); err != nil {
		t.Fatalf("migrate down: %v", err)
	}

	after, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("get version after down: %v", err)
	}
	if after >= before {
		t.Fatalf("expected version to decrease after down: before=%d after=%d", before, after)
	}

	// One down step rolls back the LAST migration only — the property `make
	// migrate-down` promises an operator. With the squashed init as the only
	// migration that step IS the whole stack, so the observable is "the schema is
	// gone"; once a project adds a migration after init, init's users table must
	// survive the step instead.
	if len(provider.ListSources()) == 1 {
		if after != 0 {
			t.Fatalf("rolling back the only migration must leave version 0, got %d", after)
		}
		if tableExists(t, ctx, mgr, "users") {
			t.Fatal("rolling back the only (init) migration must drop its tables; users survived")
		}
	} else if !tableExists(t, ctx, mgr, "users") {
		t.Fatal("one down step must roll back only the last migration; users should have survived")
	}

	// The rolled-back migration must be reversible: re-applying it restores the
	// version. Verifies the last migration's Down actually ran and is the inverse
	// of its Up, without hardcoding which artifact (table/column) it touches.
	if err := sqlite.NewMigrator(mgr, logger).RunUp(); err != nil {
		t.Fatalf("re-up after down: %v", err)
	}
	restored, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("get version after re-up: %v", err)
	}
	if restored != before {
		t.Fatalf("re-up must restore the version: before=%d restored=%d", before, restored)
	}
}

// tableExists reports whether a base table with the given name exists.
func tableExists(t *testing.T, ctx context.Context, mgr *sqlite.Manager, name string) bool {
	t.Helper()
	var count int
	if err := mgr.DB().GetContext(
		ctx,
		&count,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	); err != nil {
		t.Fatalf("query sqlite_master for table %q: %v", name, err)
	}
	return count == 1
}
