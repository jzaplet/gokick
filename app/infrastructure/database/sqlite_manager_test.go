package database_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
)

// F-047: the connection pool must be BOUNDED (a write burst can otherwise inflate
// WASM SQLite connections unbounded → OOM). An explicit APP_DB_MAX_CONNS wins;
// unset auto-scales from CPU count, clamped to [4, 32].
func TestSqliteManager_PoolCap(t *testing.T) {
	explicit := &config.Config{DBPath: filepath.Join(t.TempDir(), "cap.db"), DBMaxConns: 7}
	mgr, err := database.NewSqliteManager(explicit)
	if err != nil {
		t.Fatalf("open explicit: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	if got := mgr.DB().Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("explicit cap: got %d want 7", got)
	}

	auto := &config.Config{DBPath: filepath.Join(t.TempDir(), "auto.db")} // DBMaxConns 0 → auto
	mgrAuto, err := database.NewSqliteManager(auto)
	if err != nil {
		t.Fatalf("open auto: %v", err)
	}
	t.Cleanup(func() { _ = mgrAuto.Close() })
	want := 2 * runtime.NumCPU()
	if want < 4 {
		want = 4
	}
	if want > 32 {
		want = 32
	}
	if got := mgrAuto.DB().Stats().MaxOpenConnections; got != want {
		t.Fatalf("auto cap: got %d want %d (clamp 2×NumCPU to [4,32])", got, want)
	}
}

// F-047 regression: MigrationManager.RunUp pins the pool to one connection for the
// migration run and must hand the configured cap back afterwards — BOTH limits.
// database/sql has no "restore": SetMaxOpenConns(0) means unlimited, and narrowing
// the open limit also lowers the idle limit. RunUp runs on every CLI start, so a
// naive restore left every process with an unbounded pool that pooled one idle
// connection — the exact OOM exposure the cap exists to prevent.
func TestMigrationManager_RunUp_KeepsPoolCap(t *testing.T) {
	const poolCap = 7
	mgr, err := database.NewSqliteManager(&config.Config{
		DBPath:     filepath.Join(t.TempDir(), "migrate_cap.db"),
		DBMaxConns: poolCap,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.NewMigrationManager(mgr, logger).RunUp(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if got := mgr.DB().Stats().MaxOpenConnections; got != poolCap {
		t.Fatalf("open cap after RunUp: got %d want %d (0 = unlimited)", got, poolCap)
	}

	// Stats does not expose the idle limit, so observe it: hold several
	// connections at once, release them, and every one must stay pooled. An idle
	// limit left at 1 would close all but one of them on release.
	const held = 5
	ctx := context.Background()
	conns := make([]*sql.Conn, 0, held)
	for range held {
		c, err := mgr.DB().Conn(ctx)
		if err != nil {
			t.Fatalf("acquire conn: %v", err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		_ = c.Close()
	}
	if got := mgr.DB().Stats().Idle; got != held {
		t.Fatalf("idle conns after release: got %d want %d (idle limit must be the cap, not 1)",
			got, held)
	}
}

// The no-transaction zone guard: BeginTx must fail closed when ctx is marked by
// shared.ContextForbidTx (a durable run handler ctx), so an accidental transaction
// in a long-running run surfaces immediately instead of freezing the DB by holding
// the global SQLite write lock for the run's lifetime.
func TestSqliteManager_BeginTx_FailsClosedInNoTxZone(t *testing.T) {
	mgr := newTestManager(t)

	// A normal ctx → BeginTx succeeds (and we roll it back).
	if ctx, err := mgr.BeginTx(context.Background()); err != nil {
		t.Fatalf("BeginTx on a normal ctx must succeed, got %v", err)
	} else if rbErr := mgr.Rollback(ctx); rbErr != nil {
		t.Fatalf("rollback: %v", rbErr)
	}

	// A no-transaction zone → BeginTx fails closed, no tx opened.
	if _, err := mgr.BeginTx(shared.ContextForbidTx(context.Background())); err == nil {
		t.Fatal("BeginTx in a no-transaction zone must fail closed")
	} else if !strings.Contains(err.Error(), "no-transaction zone") {
		t.Fatalf("error must name the no-transaction zone, got %v", err)
	}
}

// TestSqliteManager_ConcurrentTxWritesDoNotReturnBusy reproduces the
// "sqlite3: database is locked" regression caused by BEGIN DEFERRED +
// no busy_timeout. The handler pattern read → CPU-hold → write inside
// one transaction must survive a concurrent committed write on a
// sibling connection. Without _txlock=immediate this fails almost
// every run with SQLITE_BUSY_SNAPSHOT during the UPDATE; with
// IMMEDIATE the writers serialize at BEGIN and all goroutines win.
func TestSqliteManager_ConcurrentTxWritesDoNotReturnBusy(t *testing.T) {
	mgr := newTestManager(t)

	if _, err := mgr.DB().Exec(`CREATE TABLE counters (id INTEGER PRIMARY KEY, val INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := mgr.DB().Exec(`INSERT INTO counters (id, val) VALUES (1, 0)`); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	const (
		goroutines = 4
		iterations = 25
		// Simulates work held inside the tx (think bcrypt). Long enough
		// that a sibling writer is virtually guaranteed to commit during
		// the window — that's what made BUSY_SNAPSHOT flake into a
		// near-certainty in serve mode.
		holdInsideTx = 5 * time.Millisecond
	)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := bumpCounterInTx(mgr, holdInsideTx); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			if strings.Contains(err.Error(), "database is locked") {
				t.Fatalf("regression: %v", err)
			}
			t.Fatalf("unexpected error: %v", err)
		}
	}

	var got int
	if err := mgr.DB().Get(&got, `SELECT val FROM counters WHERE id = 1`); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if want := goroutines * iterations; got != want {
		t.Fatalf("counter mismatch (lost updates): got %d, want %d", got, want)
	}
}

// bumpCounterInTx mirrors the read-then-CPU-then-write shape that
// CreateUser uses: SELECT the current value, hold the tx open for a
// CPU-ish moment, then UPDATE based on what was read. With IMMEDIATE
// locking each call serializes cleanly; with DEFERRED it races the
// snapshot.
func bumpCounterInTx(mgr *database.SqliteManager, hold time.Duration) error {
	ctx, err := mgr.BeginTx(context.Background())
	if err != nil {
		return err
	}

	tx := database.TxFromContext(ctx)
	var val int
	if err := tx.Get(&val, `SELECT val FROM counters WHERE id = 1`); err != nil {
		_ = mgr.Rollback(ctx)
		return err
	}

	time.Sleep(hold)

	if _, err := tx.Exec(`UPDATE counters SET val = ? WHERE id = 1`, val+1); err != nil {
		_ = mgr.Rollback(ctx)
		return err
	}

	return mgr.Commit(ctx)
}

func newTestManager(t *testing.T) *database.SqliteManager {
	t.Helper()
	cfg := &config.Config{
		DBPath: filepath.Join(t.TempDir(), "test.db"),
	}
	mgr, err := database.NewSqliteManager(cfg)
	if err != nil {
		t.Fatalf("open manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}
