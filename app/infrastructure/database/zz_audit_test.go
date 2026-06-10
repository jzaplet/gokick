package database_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"

	"github.com/google/uuid"
)

// newManagerWithMode opens a SqliteManager with an explicit journal mode.
// Returns the manager and the construction error so whitelist-rejection
// cases can assert on the error directly.
func newManagerWithMode(t *testing.T, mode string) (*database.SqliteManager, error) {
	t.Helper()
	cfg := &config.Config{
		DBPath:        filepath.Join(t.TempDir(), "test.db"),
		DBJournalMode: mode,
	}
	mgr, err := database.NewSqliteManager(cfg)
	if mgr != nil {
		t.Cleanup(func() { _ = mgr.Close() })
	}
	return mgr, err
}

// TestSqliteManager_BusyTimeoutPragmaIsApplied pins the busy_timeout(5000)
// DSN pragma (claims infra-db-security-01, infra-db-security-03): a pooled
// connection must report PRAGMA busy_timeout == 5000. If the DSN dropped the
// busy_timeout pragma it would revert to SQLite's default (0) and this fails.
func TestSqliteManager_BusyTimeoutPragmaIsApplied(t *testing.T) {
	mgr := newTestManager(t)

	var busyTimeout int
	if err := mgr.DB().Get(&busyTimeout, `PRAGMA busy_timeout`); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout: got %d, want 5000", busyTimeout)
	}
}

// TestSqliteManager_ForeignKeysEnabledPerConnection pins the
// foreign_keys(on) DSN pragma and its per-connection guarantee (claims
// infra-db-security-01, infra-db-security-04, roadmap-87). It checks the
// pragma on two distinct, simultaneously-held pool connections, then proves
// FK enforcement actually bites on a fresh connection via a hand-rolled
// parent/child schema (no migration coupling). Removing foreign_keys(on)
// from the DSN defaults the pragma OFF on every connection, so the violating
// insert would silently succeed and this test would fail.
func TestSqliteManager_ForeignKeysEnabledPerConnection(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	// Hold one connection open so the second Conn() is forced to be a
	// physically distinct pooled connection — proves the pragma is on per
	// connection, not just on whichever connection ran the first PRAGMA.
	c1, err := mgr.DB().Connx(ctx)
	if err != nil {
		t.Fatalf("acquire conn 1: %v", err)
	}
	defer func() { _ = c1.Close() }()

	c2, err := mgr.DB().Connx(ctx)
	if err != nil {
		t.Fatalf("acquire conn 2: %v", err)
	}
	defer func() { _ = c2.Close() }()

	var fk1, fk2 int
	if err := c1.GetContext(ctx, &fk1, `PRAGMA foreign_keys`); err != nil {
		t.Fatalf("conn 1 read foreign_keys: %v", err)
	}
	if err := c2.GetContext(ctx, &fk2, `PRAGMA foreign_keys`); err != nil {
		t.Fatalf("conn 2 read foreign_keys: %v", err)
	}
	if fk1 != 1 || fk2 != 1 {
		t.Fatalf("foreign_keys per connection: got conn1=%d conn2=%d, want both 1", fk1, fk2)
	}

	// Behavioral proof on a fresh connection: a child row referencing a
	// missing parent must be rejected with a FOREIGN KEY constraint error.
	if _, err := mgr.DB().Exec(`CREATE TABLE parent (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := mgr.DB().Exec(
		`CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id))`,
	); err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err = mgr.DB().
		Exec(`INSERT INTO child (id, parent_id) VALUES (?, ?)`, "c1", "does-not-exist")
	if err == nil {
		t.Fatal("expected FOREIGN KEY constraint violation, got nil error (foreign_keys is OFF)")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
		t.Fatalf("expected FOREIGN KEY constraint error, got: %v", err)
	}
}

// TestSqliteManager_RefreshTokensCascadeOnUserDelete pins the migration's
// ON DELETE CASCADE on refresh_tokens.user_id (claim infra-db-security-13).
// It runs the real migrations, inserts a user + a refresh token referencing
// it, deletes the user, and asserts the refresh_tokens row was cascade
// deleted. Without ON DELETE CASCADE (or with FKs off) the orphan row would
// survive and the count would be 1.
func TestSqliteManager_RefreshTokensCascadeOnUserDelete(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID := uuid.New().String()
	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO users (id, nickname, password_hash, role) VALUES (?, ?, ?, ?)`,
		userID, "cascadeuser", "hash", "user",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := mgr.DB().ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, datetime('now', '+1 hour'))`,
		uuid.New().String(), userID, "tokenhash",
	); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}

	var before int
	if err := mgr.DB().GetContext(ctx, &before, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("count before: %v", err)
	}
	if before != 1 {
		t.Fatalf("precondition: expected 1 refresh token before delete, got %d", before)
	}

	if _, err := mgr.DB().ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var after int
	if err := mgr.DB().GetContext(ctx, &after, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != 0 {
		t.Fatalf("expected refresh_tokens cascade-deleted, got %d remaining", after)
	}
}

// TestSqliteManager_RejectsUnknownJournalMode pins the APP_DB_JOURNAL_MODE
// whitelist (claims infra-db-security-05, roadmap-83). An out-of-whitelist
// value (a PRAGMA-injection attempt) must make NewSqliteManager return an
// error naming the allowed modes; it must not silently open the pool.
func TestSqliteManager_RejectsUnknownJournalMode(t *testing.T) {
	mgr, err := newManagerWithMode(t, "WAL; PRAGMA foreign_keys=off")
	if err == nil {
		t.Fatal("expected error for non-whitelisted journal mode, got nil")
	}
	if mgr != nil {
		t.Fatal("expected nil manager when journal mode is rejected")
	}
	if !strings.Contains(err.Error(), "WAL|DELETE|MEMORY") {
		t.Fatalf("error should name the allowed modes WAL|DELETE|MEMORY, got: %v", err)
	}
}

// TestSqliteManager_AcceptsWhitelistedJournalModes pins the accepted half of
// the whitelist (claims infra-db-security-05, roadmap-83): each of WAL,
// DELETE, MEMORY opens successfully. For WAL — the only mode the code claims
// persists on the pool — the readback is also asserted (SQLite reports it
// lowercase). DELETE/MEMORY only assert successful open, since their PRAGMA
// runs once and a readback may land on a different pooled connection.
func TestSqliteManager_AcceptsWhitelistedJournalModes(t *testing.T) {
	for _, mode := range []string{"WAL", "DELETE", "MEMORY"} {
		t.Run(mode, func(t *testing.T) {
			mgr, err := newManagerWithMode(t, mode)
			if err != nil {
				t.Fatalf("NewSqliteManager(%q) returned error: %v", mode, err)
			}
			if mgr == nil {
				t.Fatalf("NewSqliteManager(%q) returned nil manager", mode)
			}

			if mode == "WAL" {
				mgr.DB().SetMaxOpenConns(1)
				var got string
				if err := mgr.DB().Get(&got, `PRAGMA journal_mode`); err != nil {
					t.Fatalf("read journal_mode: %v", err)
				}
				if !strings.EqualFold(got, mode) {
					t.Fatalf("journal_mode: got %q, want %q (case-insensitive)", got, mode)
				}
			}
		})
	}
}
