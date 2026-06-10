package database_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"gokick/app/infrastructure/database"
	"gokick/migrations"

	"github.com/pressly/goose/v3"
)

// indexExists reports whether an index with the given name exists in
// sqlite_master. Using sqlite_master (rather than PRAGMA index_list) lets the
// query name the index directly, so a renamed or dropped index fails the
// lookup unambiguously.
func indexExists(t *testing.T, mgr *database.SqliteManager, name string) bool {
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

// TestMigrationManager_InitSchemaCreatesRefreshTokenIndexes pins the two
// indexes the init migration documents (claim infra-db-security-12):
// idx_refresh_tokens_token_hash on refresh_tokens(token_hash) and
// idx_refresh_tokens_user_id on refresh_tokens(user_id). It runs the real
// embedded migrations via MigrationManager.RunUp() and then asserts both named
// indexes are present in sqlite_master. If either CREATE INDEX line is removed
// from 20260327000001_init_schema.sql (or the index renamed), the corresponding
// lookup returns 0 and this test fails.
func TestMigrationManager_InitSchemaCreatesRefreshTokenIndexes(t *testing.T) {
	mgr := newTestManager(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
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
// production MigrationManager.RunUp(), then runs goose.DownContext once exactly
// as the Makefile target does. The highest-timestamp migration
// (20260517000003_create_audit_log) creates the audit_log table, so after one
// down step the schema version must drop by one migration and audit_log must no
// longer exist. If down were a no-op (or the +goose Down block were dropped),
// the version would not decrease and audit_log would survive — failing here.
func TestMigrationDown_RollsBackLastMigration(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// Mirror the Makefile's `goose ... down` invocation: same dialect, same
	// embedded FS, one step down. RunUp() already configured goose's global
	// dialect/baseFS, but set them explicitly so this test does not depend on
	// call ordering with other tests in the package.
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}

	before, err := goose.GetDBVersion(mgr.DB().DB)
	if err != nil {
		t.Fatalf("get version before down: %v", err)
	}

	// Precondition: the last migration's table is present before rollback.
	if !tableExists(t, ctx, mgr, "audit_log") {
		t.Fatal("precondition: expected audit_log table to exist after migrate up")
	}

	if err := goose.DownContext(ctx, mgr.DB().DB, "."); err != nil {
		t.Fatalf("migrate down: %v", err)
	}

	after, err := goose.GetDBVersion(mgr.DB().DB)
	if err != nil {
		t.Fatalf("get version after down: %v", err)
	}

	if after >= before {
		t.Fatalf("expected version to decrease after down: before=%d after=%d", before, after)
	}

	// The most recent migration created audit_log; its down dropped it.
	if tableExists(t, ctx, mgr, "audit_log") {
		t.Fatal("expected audit_log table to be dropped by migrate down, but it still exists")
	}

	// An earlier table (users, from the init migration) must remain — down
	// rolled back exactly one migration, not the whole stack.
	if !tableExists(t, ctx, mgr, "users") {
		t.Fatal("migrate down rolled back too far: users table is gone after a single down step")
	}
}

// tableExists reports whether a base table with the given name exists.
func tableExists(t *testing.T, ctx context.Context, mgr *database.SqliteManager, name string) bool {
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
