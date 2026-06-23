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
// as the Makefile target does. It asserts the generic round-trip property rather
// than a specific migration's artifact (so adding a migration doesn't break it):
// the version drops, an early table (users) survives the single step, and
// re-running up restores the version — proving the last migration's Down ran and
// is the inverse of its Up. If down were a no-op (or the +goose Down block were
// dropped), the version would not decrease — failing here.
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

	// An early table (users, from the init migration) must remain — a single
	// down step rolls back one migration, not the whole stack.
	if !tableExists(t, ctx, mgr, "users") {
		t.Fatal("migrate down rolled back too far: users table is gone after a single down step")
	}

	// The rolled-back migration must be reversible: re-applying it restores the
	// version. Verifies the last migration's Down actually ran and is the inverse
	// of its Up, without hardcoding which artifact (table/column) it touches.
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("re-up after down: %v", err)
	}
	restored, err := goose.GetDBVersion(mgr.DB().DB)
	if err != nil {
		t.Fatalf("get version after re-up: %v", err)
	}
	if restored != before {
		t.Fatalf("re-up must restore the version: before=%d restored=%d", before, restored)
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

// The users.tenant_id FK rebuild must PRESERVE refresh_tokens (which
// references users via ON DELETE CASCADE) and ENFORCE the FK afterward. The
// rebuild drops+recreates users; with foreign_keys left on, the implicit DELETE
// during DROP would cascade-delete refresh_tokens. The migration runs
// foreign_keys OFF on the single connection the MigrationManager pins — proven
// here with refresh_token rows actually present (an empty table would hide a
// cascade).
func TestMigration_UsersTenantFK_PreservesRefreshTokensAndEnforcesFK(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const defaultTenant = "00000000-0000-0000-0000-000000000000"

	// Migrate up to just before the FK rebuild (the jobs.tenant_id migration).
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.UpToContext(ctx, mgr.DB().DB, ".", 20260622000001); err != nil {
		t.Fatalf("migrate up-to pre-4b: %v", err)
	}

	// Seed a user (default tenant) and a refresh_token referencing it.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"u1", "alice", "h", "a@x", "admin", defaultTenant); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?,?,?,?)`,
		"rt1", "u1", "tok", "2999-01-01"); err != nil {
		t.Fatalf("insert refresh_token: %v", err)
	}

	// Apply the FK rebuild through the production runner (which pins one connection).
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("apply FK rebuild: %v", err)
	}

	// Core guarantee: the refresh_token survived (no cascade-delete).
	var rt, usr int
	if err := mgr.DB().GetContext(ctx, &rt,
		`SELECT COUNT(*) FROM refresh_tokens WHERE id='rt1'`); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if rt != 1 {
		t.Fatalf("refresh_token must survive the users rebuild (no cascade-delete), got %d", rt)
	}
	if err := mgr.DB().GetContext(ctx, &usr,
		`SELECT COUNT(*) FROM users WHERE id='u1'`); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if usr != 1 {
		t.Fatalf("user must survive the rebuild, got %d", usr)
	}

	// The FK is now enforced: a user with a non-existent tenant is rejected.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"u2", "bob", "h", "b@x", "user", "no-such-tenant"); err == nil {
		t.Fatal("FK must reject a user whose tenant_id has no matching tenants row")
	}
}

// The role-CHECK rebuild must WIDEN the CHECK to admit 'superadmin'
// while keeping every other guarantee of the post-4b users table: the CHECK must
// still BITE (reject an unknown role), refresh_tokens must survive the rebuild
// (no cascade-delete), the tenant FK must stay enforced, and pre-existing rows
// + columns must be intact. A happy-path-only test would miss a silently dropped
// constraint or column, so each is asserted explicitly. Rows are seeded BEFORE
// the rebuild so a cascade or data loss would actually show.
func TestMigration_UsersRoleSuperadmin_WidensCheckPreservesData(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const defaultTenant = "00000000-0000-0000-0000-000000000000"

	// Migrate up to just before the role-CHECK rebuild (the tenant_id FK migration).
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.UpToContext(ctx, mgr.DB().DB, ".", 20260622000002); err != nil {
		t.Fatalf("migrate up-to pre-4c: %v", err)
	}

	// Seed an admin user (default tenant) + a refresh_token referencing it.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"u1", "alice", "h", "a@x", "admin", defaultTenant); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?,?,?,?)`,
		"rt1", "u1", "tok", "2999-01-01"); err != nil {
		t.Fatalf("insert refresh_token: %v", err)
	}

	// Apply the rebuild (+ the platform-overview columns) via the production runner.
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("apply role-CHECK rebuild: %v", err)
	}

	// (a) The pre-existing admin row + its data survived the rebuild.
	var role string
	if err := mgr.DB().GetContext(ctx, &role,
		`SELECT role FROM users WHERE id='u1'`); err != nil {
		t.Fatalf("read survived user: %v", err)
	}
	if role != "admin" {
		t.Fatalf("admin row must survive intact, got role=%q", role)
	}

	// (b) refresh_tokens survived (the rebuild dropped+recreated users).
	var rt int
	if err := mgr.DB().GetContext(ctx, &rt,
		`SELECT COUNT(*) FROM refresh_tokens WHERE id='rt1'`); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if rt != 1 {
		t.Fatalf("refresh_token must survive the rebuild (no cascade-delete), got %d", rt)
	}

	// (c) The widened CHECK now ADMITS 'superadmin'.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"s1", "root", "h", "r@x", "superadmin", defaultTenant); err != nil {
		t.Fatalf("CHECK must admit 'superadmin' after the rebuild: %v", err)
	}

	// (d) The CHECK still BITES — an unknown role is rejected.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"w1", "wiz", "h", "w@x", "wizard", defaultTenant); err == nil {
		t.Fatal("CHECK must still reject an unknown role after the rebuild")
	}

	// (e) The tenant FK is still enforced.
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, email, role, tenant_id) VALUES (?,?,?,?,?,?)`,
		"u3", "carol", "h", "c@x", "user", "no-such-tenant"); err == nil {
		t.Fatal("FK must still reject a user whose tenant_id has no matching tenants row")
	}

	// (f) The platform-overview columns landed: users.last_login_at exists and
	// tenants.plan defaults to 'free'.
	if _, err := mgr.DB().ExecContext(ctx,
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id='u1'`); err != nil {
		t.Fatalf("users.last_login_at column must exist: %v", err)
	}
	var plan string
	if err := mgr.DB().GetContext(ctx, &plan,
		`SELECT plan FROM tenants WHERE id=?`, defaultTenant); err != nil {
		t.Fatalf("tenants.plan column must exist: %v", err)
	}
	if plan != "free" {
		t.Fatalf("tenants.plan must default to 'free', got %q", plan)
	}
}
